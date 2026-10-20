package photos

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{200, 50, 20, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestFormatsAndProcessing(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pngBytes := testPNG(t)
	img, _, _ := image.Decode(bytes.NewReader(pngBytes))
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	heic, err := os.ReadFile("testdata/phone-format.heic")
	if err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(t.TempDir(), "input.png")
	if err := os.WriteFile(temp, pngBytes, 0600); err != nil {
		t.Fatal(err)
	}
	webp := filepath.Join(t.TempDir(), "input.webp")
	if out, err := exec.Command(s.convert, temp, webp).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	webpBytes, _ := os.ReadFile(webp)
	for _, tc := range []struct {
		name string
		raw  []byte
	}{{"PNG", pngBytes}, {"JPEG", jpg.Bytes()}, {"HEIC", heic}, {"WEBP", webpBytes}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format(tc.raw)
			if err != nil || got != tc.name {
				t.Fatal(got, err)
			}
			photo, err := s.Process(context.Background(), tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if photo.Width > 2400 || photo.ThumbnailWidth > 640 || photo.ThumbnailWidth != photo.ThumbnailHeight || photo.Crop != CenterCrop(photo.Width, photo.Height) {
				t.Fatal("bounds", photo)
			}
			for _, thumb := range []bool{false, true} {
				f, err := s.Open(photo.Key, thumb)
				if err != nil {
					t.Fatal(err)
				}
				decoded, format, err := image.Decode(f)
				_ = f.Close()
				if err != nil || format != "jpeg" || decoded.Bounds().Dx() == 0 {
					t.Fatal(format, err)
				}
			}
			entries, err := os.ReadDir(filepath.Join(s.root, photo.Key))
			if err != nil || len(entries) != 2 {
				t.Fatal("source retained", entries, err)
			}
			if err := s.Remove(photo.Key); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCorruptUnsupportedAndLimits(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw := testPNG(t)
	huge := append([]byte(nil), raw...)
	binary.BigEndian.PutUint32(huge[16:20], 10000)
	binary.BigEndian.PutUint32(huge[20:24], 10000)
	for _, input := range [][]byte{nil, []byte("<svg/>"), []byte("GIF89a"), []byte("#!/bin/sh"), append([]byte{0xff, 0xd8, 0xff}, []byte("broken")...), huge, make([]byte, MaxBytes+1)} {
		if _, err := s.Process(context.Background(), input); err == nil {
			t.Fatal("invalid image accepted")
		}
		entries, _ := os.ReadDir(s.root)
		if len(entries) != 0 {
			t.Fatal("failed upload leaked files", entries)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Process(ctx, raw); err == nil {
		t.Fatal("canceled work accepted")
	}
	if _, err := s.Open("../secret", false); err == nil {
		t.Fatal("unsafe key accepted")
	}
}
func TestOrientationMetadataResizeAndSweep(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Real EXIF TIFF orientation=6 in a JPEG APP1 segment; auto-orient rotates 80x40 to 40x80.
	img, _, _ := image.Decode(bytes.NewReader(testPNG(t)))
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	data := b.Bytes()
	oriented := append([]byte{}, data[:2]...)
	oriented = append(oriented, 0xff, 0xe1, byte((len(exif)+2)>>8), byte(len(exif)+2))
	oriented = append(oriented, exif...)
	oriented = append(oriented, data[2:]...)
	p, err := s.Process(context.Background(), oriented)
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 40 || p.Height != 80 || p.ThumbnailWidth != 40 || p.ThumbnailHeight != 40 {
		t.Fatal("orientation/no-upscale", p)
	}
	normalizedCrop, err := s.CropThumbnail(context.Background(), p.Key, Crop{X: 0, Y: 40, Size: 40})
	if err != nil || normalizedCrop.ThumbnailWidth != 40 || normalizedCrop.ThumbnailHeight != 40 {
		t.Fatal("normalized oriented crop", normalizedCrop, err)
	}
	f, _ := s.Open(p.Key, false)
	raw, _ := os.ReadFile(f.Name())
	_ = f.Close()
	if bytes.Contains(raw, []byte("Exif")) {
		t.Fatal("source EXIF retained")
	}
	input := filepath.Join(t.TempDir(), "large.png")
	if out, err := exec.Command(s.convert, "-size", "2500x1000", "xc:red", input).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	raw, _ = os.ReadFile(input)
	large, err := s.Process(context.Background(), raw)
	if err != nil || large.Width != 2400 || large.ThumbnailWidth != 640 || large.ThumbnailHeight != 640 {
		t.Fatal("resize", large, err)
	}
	orphan := filepath.Join(s.root, "00000000000000000000000000000000")
	_ = os.Mkdir(orphan, 0700)
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(orphan, past, past)
	if err := s.Sweep(map[string]bool{p.Key: true, large.Key: true, normalizedCrop.Key: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphan remained", err)
	}
	if _, err := s.Open(p.Key, false); err != nil {
		t.Fatal("live media removed", err)
	}
}

// A phone-sized compressed JPEG is small on the wire but needs a >64 MiB
// ImageMagick pixel cache. Keep this realistic dimension separate from tiny fixtures.
func TestPhoneResolutionJPEG(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "IMG_4144.jpeg")
	if out, err := exec.Command(s.convert, "-size", "4284x5712", "gradient:", source).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > MaxBytes {
		t.Fatal("fixture exceeds upload limit")
	}
	photo, err := s.Process(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if photo.Width != 1800 || photo.Height != 2400 || photo.ThumbnailWidth != 640 || photo.ThumbnailHeight != 640 {
		t.Fatal(photo)
	}
}

func TestHEIFOriginJPEGAndContentDetection(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Simulate Photos exporting a HEIF source as JPEG, retaining only actual bytes
	// as the server's authority (filenames and multipart MIME do not reach Process).
	source := filepath.Join(t.TempDir(), "source.heic")
	if err := os.WriteFile(source, decoderFixture, 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "export.jpeg")
	if out, err := exec.Command(s.convert, "HEIC:"+source+"[0]", "JPEG:"+target).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if format, err := Format(raw); err != nil || format != "JPEG" {
		t.Fatal(format, err)
	}
	for _, raw := range [][]byte{decoderFixture, raw} {
		photo, err := s.Process(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.Open(photo.Key, false)
		if err != nil {
			t.Fatal(err)
		}
		_, format, err := image.Decode(f)
		_ = f.Close()
		if err != nil || format != "jpeg" {
			t.Fatal(format, err)
		}
	}
}

func TestHEIFBrands(t *testing.T) {
	box := func(brands ...string) []byte {
		raw := make([]byte, 16+4*(len(brands)-1))
		binary.BigEndian.PutUint32(raw[:4], uint32(len(raw)))
		copy(raw[4:8], "ftyp")
		copy(raw[8:12], brands[0])
		for i, brand := range brands[1:] {
			copy(raw[16+4*i:], brand)
		}
		return raw
	}
	for _, brands := range [][]string{{"heic"}, {"mif1", "heic"}, {"heix", "mif1"}} {
		if got, err := Format(box(brands...)); err != nil || got != "HEIC" {
			t.Fatal(brands, got, err)
		}
	}
	for _, raw := range [][]byte{box("avif", "mif1"), box("mif1", "avis"), box("mp42"), []byte("not a JPEG"), box("heic")[:15]} {
		if _, err := Format(raw); err == nil {
			t.Fatal("invalid content accepted")
		}
	}
}

func TestValidationReasonsAndPixelLimit(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "over-limit.jpg")
	if out, err := exec.Command(s.convert, "-size", "8000x6400", "xc:red", source).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	oversizedDimensions, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw  []byte
		want error
	}{
		{nil, ErrSize}, {make([]byte, MaxBytes+1), ErrSize},
		{[]byte("<svg/>"), ErrFormat}, {[]byte{255, 216, 255, 0}, ErrDecode},
		{oversizedDimensions, ErrDimensions},
	} {
		_, err := s.Process(context.Background(), tc.raw)
		if !errors.Is(err, tc.want) || !errors.Is(err, ErrImage) {
			t.Fatalf("got %v, want %v", err, tc.want)
		}
	}
	for _, exitCode := range []int{1, 25, 24} {
		err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", exitCode)).Run()
		got := decoderFailure(context.Background(), "convert", "JPEG", err)
		if errors.Is(got, ErrImage) != (exitCode == 1) {
			t.Fatalf("exit %d misclassified: %v", exitCode, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := decoderFailure(ctx, "identify", "JPEG", context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatal(got)
	}
}

func TestMaximumResolutionJPEG(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "50mp.jpeg")
	if out, err := exec.Command(s.convert, "-size", "5000x10000", "gradient:", source).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	photo, err := s.Process(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if photo.Width != 1200 || photo.Height != 2400 || photo.ThumbnailWidth != 640 || photo.ThumbnailHeight != 640 {
		t.Fatal(photo)
	}
}
