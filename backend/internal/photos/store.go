// Package photos processes untrusted images in bounded external decoder processes.
package photos

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed testdata/phone-format.heic
var decoderFixture []byte

const MaxBytes = 25 * 1024 * 1024
const MaxPixels = 50_000_000

var ErrImage = errors.New("unsupported, corrupt or oversized image; use JPEG, PNG, WebP or HEIC/HEIF (25 MiB, 50 megapixels maximum)")
var safeKey = regexp.MustCompile(`^[0-9a-f]{32}$`)

type Image struct {
	Crop                                           Crop
	Key                                            string
	Width, Height, ThumbnailWidth, ThumbnailHeight int
}
type Store struct {
	root, convert, identify, prlimit string
	slots                            chan struct{}
}

// New checks private persistent storage and required Linux decoder executables.
func New(root string) (*Store, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == "/" {
		return nil, errors.New("MEDIA_DIR must be an absolute dedicated directory")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid media directory")
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, err
	}
	s := &Store{root: root, slots: make(chan struct{}, 2)}
	for name, target := range map[string]*string{"convert": &s.convert, "identify": &s.identify, "prlimit": &s.prlimit} {
		*target, err = exec.LookPath(name)
		if err != nil {
			return nil, fmt.Errorf("photo decoder prerequisite %s missing", name)
		}
	}
	formats, err := exec.Command(s.identify, "-list", "format").Output()
	if err != nil {
		return nil, err
	}
	for _, format := range []string{"HEIC", "JPEG", "PNG", "WEBP"} {
		if !strings.Contains(string(formats), format) {
			return nil, fmt.Errorf("photo decoder %s missing", format)
		}
	}
	f, err := os.CreateTemp(root, ".write-check-")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	// Merely advertising HEIC is insufficient without its HEVC decoder plugin.
	probe, err := s.Process(context.Background(), decoderFixture)
	if err != nil {
		return nil, fmt.Errorf("HEIC conversion self-test failed: %w", err)
	}
	if err := s.Remove(probe.Key); err != nil {
		return nil, err
	}
	return s, nil
}

// Format inspects file signatures, never extension or client Content-Type.
func Format(raw []byte) (string, error) {
	switch {
	case len(raw) >= 3 && bytes.Equal(raw[:3], []byte{0xff, 0xd8, 0xff}):
		return "JPEG", nil
	case len(raw) >= 8 && bytes.Equal(raw[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return "PNG", nil
	case len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		return "WEBP", nil
	case len(raw) >= 16 && string(raw[4:8]) == "ftyp":
		// Whitelist HEVC/HEIF still-image brands; AVIF is deliberately unsupported.
		boxLength := int(binary.BigEndian.Uint32(raw[:4]))
		if boxLength < 16 || boxLength > 4096 || boxLength > len(raw) {
			return "", ErrImage
		}
		heif := false
		for i := 8; i+4 <= boxLength; i += 4 {
			if i == 12 {
				continue
			} // minor version is not a compatibility brand
			switch string(raw[i : i+4]) {
			case "avif", "avis":
				return "", ErrImage
			case "heic", "heix", "hevc", "hevx", "mif1":
				heif = true
			}
		}
		if heif {
			return "HEIC", nil
		}
	}
	return "", ErrImage
}
func (s *Store) run(ctx context.Context, dir, binary string, args ...string) ([]byte, error) {
	// Address space, CPU, output file size, decoder cache and wall time are bounded.
	cmd := exec.CommandContext(ctx, s.prlimit, append([]string{"--as=1073741824", "--cpu=20", "--fsize=67108864", "--", binary}, args...)...)
	cmd.Env = append(os.Environ(), "OMP_NUM_THREADS=1", "MAGICK_THREAD_LIMIT=1", "MAGICK_TEMPORARY_PATH="+dir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return out.Bytes(), err
}
func (s *Store) Process(ctx context.Context, raw []byte) (Image, error) {
	if len(raw) == 0 || len(raw) > MaxBytes {
		return Image{}, ErrImage
	}
	format, err := Format(raw)
	if err != nil {
		return Image{}, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Image{}, errors.New("photo processor busy; retry later")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp(s.root, ".upload-")
	if err != nil {
		return Image{}, err
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		return Image{}, err
	}
	input := format + ":" + source + "[0]"
	bounds, err := s.run(ctx, dir, s.identify, "-limit", "memory", "256MiB", "-limit", "map", "256MiB", "-limit", "disk", "512MiB", "-ping", "-format", "%w %h", input)
	if err != nil {
		return Image{}, ErrImage
	}
	dims := strings.Fields(string(bounds))
	if len(dims) != 2 {
		return Image{}, ErrImage
	}
	w, e1 := strconv.ParseInt(dims[0], 10, 64)
	h, e2 := strconv.ParseInt(dims[1], 10, 64)
	if e1 != nil || e2 != nil || w < 1 || h < 1 || w > MaxPixels || h > MaxPixels || w*h > MaxPixels {
		return Image{}, ErrImage
	}
	for _, output := range []struct{ name, size string }{{"full.jpg", "2400x2400>"}} {
		_, err = s.run(ctx, dir, s.convert, "-limit", "memory", "256MiB", "-limit", "map", "256MiB", "-limit", "disk", "512MiB", input, "-auto-orient", "-resize", output.size, "-background", "white", "-alpha", "remove", "-colorspace", "sRGB", "-strip", "-quality", "85", "JPEG:"+filepath.Join(dir, output.name))
		if err != nil {
			return Image{}, ErrImage
		}
	}
	fullFile, err := os.Open(filepath.Join(dir, "full.jpg"))
	if err != nil {
		return Image{}, err
	}
	fullConfig, _, err := image.DecodeConfig(fullFile)
	_ = fullFile.Close()
	if err != nil {
		return Image{}, err
	}
	result, err := s.writeThumbnail(ctx, dir, filepath.Join(dir, "full.jpg"), CenterCrop(fullConfig.Width, fullConfig.Height))
	if err != nil {
		return Image{}, err
	}
	for i, name := range []string{"full.jpg", "thumbnail.jpg"} {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR, 0600)
		if err != nil {
			return Image{}, err
		}
		cfg, _, decodeErr := image.DecodeConfig(f)
		syncErr := f.Sync()
		closeErr := f.Close()
		if decodeErr != nil || syncErr != nil || closeErr != nil {
			return Image{}, errors.New("processed photo write failed")
		}
		if i == 0 {
			result.Width = cfg.Width
			result.Height = cfg.Height
		} else {
			result.ThumbnailWidth = cfg.Width
			result.ThumbnailHeight = cfg.Height
		}
	}
	if err := os.Remove(source); err != nil {
		return Image{}, err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return Image{}, err
	}
	result.Key = hex.EncodeToString(random)
	directory, err := os.Open(dir)
	if err != nil {
		return Image{}, err
	}
	err = directory.Sync()
	_ = directory.Close()
	if err != nil {
		return Image{}, err
	}
	if err := os.Rename(dir, filepath.Join(s.root, result.Key)); err != nil {
		return Image{}, err
	}
	root, err := os.Open(s.root)
	if err != nil {
		return Image{}, err
	}
	err = root.Sync()
	_ = root.Close()
	if err != nil {
		_ = s.Remove(result.Key)
		return Image{}, err
	}
	return result, nil
}
func (s *Store) Open(key string, thumbnail bool) (*os.File, error) {
	if !safeKey.MatchString(key) {
		return nil, errors.New("invalid photo key")
	}
	name := "full.jpg"
	if thumbnail {
		name = "thumbnail.jpg"
	}
	return os.Open(filepath.Join(s.root, key, name))
}
func (s *Store) Remove(key string) error {
	if !safeKey.MatchString(key) {
		return errors.New("invalid photo key")
	}
	return os.RemoveAll(filepath.Join(s.root, key))
}

// Sweep only removes abandoned temp/publication directories older than one hour.
// known must be loaded from the database successfully; never sweep on a DB failure.
func (s *Store) Sweep(known map[string]bool) error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		key := entry.Name()
		if known[key] || (!safeKey.MatchString(key) && !strings.HasPrefix(key, ".upload-")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if time.Since(info.ModTime()) < time.Hour {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.root, key)); err != nil {
			return err
		}
	}
	return nil
}
