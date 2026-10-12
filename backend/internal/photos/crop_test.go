package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCropBounds(t *testing.T) {
	for _, tc := range []struct {
		c     Crop
		valid bool
	}{
		{Crop{0, 0, 1}, true}, {Crop{20, 0, 40}, true}, {Crop{40, 0, 40}, true},
		{Crop{-1, 0, 1}, false}, {Crop{0, -1, 1}, false}, {Crop{0, 0, 0}, false},
		{Crop{0, 0, 41}, false}, {Crop{41, 0, 40}, false}, {Crop{0, 1, 40}, false},
		{Crop{int(^uint(0) >> 1), 0, 1}, false}, {Crop{0, 0, int(^uint(0) >> 1)}, false},
	} {
		if (tc.c.Validate(80, 40) == nil) != tc.valid {
			t.Fatalf("%+v", tc)
		}
	}
	if c := CenterCrop(81, 40); c != (Crop{20, 0, 40}) {
		t.Fatal(c)
	}
	if c := CenterCrop(40, 81); c != (Crop{0, 20, 40}) {
		t.Fatal(c)
	}
}

func TestSquareCropNormalizedSourceAndPublication(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dims := range []image.Point{{90, 120}, {90, 160}, {100, 100}, {120, 90}, {160, 90}, {3, 5}} {
		t.Run(dims.String(), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, dims.X, dims.Y))
			for y := 0; y < dims.Y; y++ {
				for x := 0; x < dims.X; x++ {
					if x < dims.X/2 {
						img.Set(x, y, color.RGBA{255, 0, 0, 255})
					} else {
						img.Set(x, y, color.RGBA{0, 0, 255, 255})
					}
				}
			}
			var b bytes.Buffer
			_ = png.Encode(&b, img)
			p, err := s.Process(context.Background(), b.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if p.ThumbnailWidth != min(dims.X, dims.Y) || p.ThumbnailWidth != p.ThumbnailHeight || p.Crop != CenterCrop(dims.X, dims.Y) {
				t.Fatal(p)
			}
			fullPath := filepath.Join(s.root, p.Key, "full.jpg")
			original, _ := os.ReadFile(fullPath)
			checksum := sha256.Sum256(original)
			side := max(1, min(dims.X/2, dims.Y))
			crop := Crop{dims.X - side, 0, side}
			changed, err := s.CropThumbnail(context.Background(), p.Key, crop)
			if err != nil {
				t.Fatal(err)
			}
			if changed.Key == p.Key || changed.ThumbnailWidth != side || changed.ThumbnailHeight != side {
				t.Fatal(changed)
			}
			f, err := s.Open(changed.Key, true)
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := image.Decode(f)
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if dims.X > 3 {
				r, _, blue, _ := decoded.At(side/2, side/2).RGBA()
				if blue < r {
					t.Fatal("crop did not select right blue region")
				}
			}
			after, _ := os.ReadFile(fullPath)
			if sha256.Sum256(after) != checksum {
				t.Fatal("full photo changed")
			}
			legacy, _ := os.ReadFile(filepath.Join(s.root, p.Key, "thumbnail.jpg"))
			for _, c := range []Crop{{-1, 0, 1}, {0, 0, dims.X + dims.Y}} {
				if _, err := s.CropThumbnail(context.Background(), p.Key, c); err == nil {
					t.Fatal("invalid crop accepted")
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := s.CropThumbnail(ctx, p.Key, crop); err == nil {
				t.Fatal("cancel accepted")
			}
			preserved, _ := os.ReadFile(filepath.Join(s.root, p.Key, "thumbnail.jpg"))
			if !bytes.Equal(legacy, preserved) {
				t.Fatal("failure overwrote current thumbnail")
			}
			// Crashed, unreferenced crop publications are swept; referenced version directories survive.
			past := time.Now().Add(-2 * time.Hour)
			_ = os.Chtimes(filepath.Join(s.root, changed.Key), past, past)
			if err := s.Sweep(map[string]bool{p.Key: true, changed.Key: true}); err != nil {
				t.Fatal(err)
			}
			if err := s.Sweep(map[string]bool{p.Key: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Open(changed.Key, true); !os.IsNotExist(err) {
				t.Fatal("abandoned crop not removed", err)
			}
		})
	}
}
