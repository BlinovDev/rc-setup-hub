package photos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"
)

// Crop is an integer square in normalized full-image pixels, before thumbnail resizing.
type Crop struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	Size int `json:"size"`
}

func CenterCrop(width, height int) Crop {
	size := min(width, height)
	return Crop{X: (width - size) / 2, Y: (height - size) / 2, Size: size}
}

// Validate uses subtraction to reject overflow without adding untrusted coordinates.
func (c Crop) Validate(width, height int) error {
	if c.Size < 1 || c.X < 0 || c.Y < 0 || c.Size > width || c.Size > height || c.X > width-c.Size || c.Y > height-c.Size {
		return errors.New("crop must be a positive in-bounds integer square in full-image pixels")
	}
	return nil
}

var ErrBusy = errors.New("photo processor busy; retry later")

// CropThumbnail publishes a new immutable thumbnail directory. It never opens full media for writing.
// The caller owns DB publication and cleanup; old thumbnails stay intact on any failure.
func (s *Store) CropThumbnail(ctx context.Context, sourceKey string, crop Crop) (Image, error) {
	if !safeKey.MatchString(sourceKey) {
		return Image{}, errors.New("invalid photo key")
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Image{}, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	source, err := s.Open(sourceKey, false)
	if err != nil {
		return Image{}, err
	}
	cfg, _, err := image.DecodeConfig(source)
	_ = source.Close()
	if err != nil {
		return Image{}, err
	}
	if err := crop.Validate(cfg.Width, cfg.Height); err != nil {
		return Image{}, err
	}
	dir, err := os.MkdirTemp(s.root, ".upload-")
	if err != nil {
		return Image{}, err
	}
	defer os.RemoveAll(dir)
	result, err := s.writeThumbnail(ctx, dir, filepath.Join(s.root, sourceKey, "full.jpg"), crop)
	if err != nil {
		return Image{}, err
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
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
	if err = os.Rename(dir, filepath.Join(s.root, result.Key)); err != nil {
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

func (s *Store) writeThumbnail(ctx context.Context, dir, source string, crop Crop) (Image, error) {
	geometry := fmt.Sprintf("%dx%d+%d+%d", crop.Size, crop.Size, crop.X, crop.Y)
	output := filepath.Join(dir, "thumbnail.jpg")
	_, err := s.run(ctx, dir, s.convert, "-limit", "memory", "256MiB", "-limit", "map", "256MiB", "-limit", "disk", "512MiB", "JPEG:"+source, "-crop", geometry, "+repage", "-resize", "640x640>", "-strip", "-quality", "85", "JPEG:"+output)
	if err != nil {
		return Image{}, err
	}
	f, err := os.OpenFile(output, os.O_RDWR, 0600)
	if err != nil {
		return Image{}, err
	}
	info, statErr := f.Stat()
	if statErr != nil || info.Size() > processedFileLimit {
		_ = f.Close()
		return Image{}, errors.New("cropped thumbnail exceeds publication limit")
	}
	cfg, _, decodeErr := image.DecodeConfig(f)
	syncErr := f.Sync()
	closeErr := f.Close()
	if decodeErr != nil || syncErr != nil || closeErr != nil || cfg.Width != cfg.Height || cfg.Width != min(crop.Size, 640) {
		return Image{}, errors.New("cropped thumbnail publication failed")
	}
	return Image{ThumbnailWidth: cfg.Width, ThumbnailHeight: cfg.Height, Crop: crop}, nil
}
