package setups

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

func TestCropPersistenceRetriesRacesAndCleanup(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	owner, err := users.NewService(users.NewRepository(pool)).Login(ctx, users.Identity{Subject: "crop", Email: "crop@example.test", Name: "Crop"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := photos.New(root)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(repo, chassis.NewService(chassis.NewRepository(pool)), friendships.NewRepository(pool))
	s.EnablePhotos(store)
	setup, err := s.Create(ctx, owner.ID, CreateInput{Title: "Crop", Visibility: Public, Data: realisticData(t)})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.uploadPhoto(ctx, owner.ID, setup.ID, "11111111-1111-4111-8111-111111111111", photoPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.Crop == nil || *p.Crop != (photos.Crop{X: 5, Y: 0, Size: 10}) || p.ThumbnailRevision != 1 {
		t.Fatal("new default", p)
	}
	full, _ := os.ReadFile(filepath.Join(root, p.key, "full.jpg"))
	checksum := sha256.Sum256(full)
	apply := func(expected int, crop *photos.Crop) (Photo, error) {
		return s.applyCrop(ctx, owner.ID, setup.ID, p.ID, CropInput{&expected, crop})
	}
	changed, err := apply(1, &photos.Crop{X: 0, Y: 0, Size: 5})
	if err != nil || changed.ThumbnailRevision != 2 || changed.ThumbnailWidth != 5 {
		t.Fatal(changed, err)
	}
	retry, err := apply(1, &photos.Crop{X: 0, Y: 0, Size: 5})
	if err != nil || retry.thumbnailKey != changed.thumbnailKey || retry.ThumbnailRevision != 2 {
		t.Fatal("retry", retry, err)
	}
	if _, err := apply(1, &photos.Crop{X: 1, Y: 0, Size: 5}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale crop", err)
	}
	for _, crop := range []photos.Crop{{X: -1, Size: 1}, {X: 19, Size: 2}, {Size: 0}, {Size: 11}} {
		if _, err := apply(2, &crop); err == nil {
			t.Fatal("invalid crop", crop)
		}
	}
	// Round-trip into every joined cover projection, with no separate per-card metadata.
	loaded, err := s.Get(ctx, owner.ID, setup.ID)
	if err != nil || loaded.CoverPhoto.ThumbnailRevision != 2 || *loaded.CoverPhoto.Crop != *changed.Crop || loaded.CoverPhoto.ThumbnailURL != changed.ThumbnailURL {
		t.Fatal("projection", loaded, err)
	}
	list, err := s.List(ctx, owner.ID)
	if err != nil || list[0].CoverPhoto.ThumbnailRevision != 2 {
		t.Fatal("mine", err)
	}
	visible, err := s.ListUser(ctx, owner.ID, owner.ID)
	if err != nil || visible[0].CoverPhoto.ThumbnailRevision != 2 {
		t.Fatal("profile", err)
	}
	page, err := s.Search(ctx, SearchInput{})
	if err != nil || page.Items[0].CoverPhoto.ThumbnailRevision != 2 {
		t.Fatal("search", err)
	}
	// Different concurrent frames at one revision: exactly one wins; neither can overwrite the winner.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, x := range []int{1, 2} {
		wg.Add(1)
		go func(x int) { defer wg.Done(); _, err := apply(2, &photos.Crop{X: x, Y: 0, Size: 4}); results <- err }(x)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal(wins, conflicts)
	}
	current, err := repo.visiblePhoto(ctx, owner.ID, setup.ID, p.ID)
	if err != nil || current.ThumbnailRevision != 3 {
		t.Fatal(current, err)
	}
	if _, err := store.Open(changed.thumbnailKey, true); !os.IsNotExist(err) {
		t.Fatal("superseded version not cleaned", err)
	}
	// Failed generation and failed DB publication preserve metadata/current bytes.
	old, _ := os.ReadFile(filepath.Join(root, current.thumbnailKey, "thumbnail.jpg"))
	revision := 3
	if _, _, err := repo.cropPhoto(ctx, owner.ID, setup.ID, p.ID, CropInput{&revision, &photos.Crop{Size: 3}}, func(Photo, photos.Crop) (photos.Image, error) {
		return photos.Image{}, errors.New("injected generation failure")
	}); err == nil {
		t.Fatal("failure accepted")
	}
	if _, _, err := repo.cropPhoto(ctx, owner.ID, setup.ID, p.ID, CropInput{&revision, &photos.Crop{Size: 3}}, func(Photo, photos.Crop) (photos.Image, error) {
		return photos.Image{Key: "unsafe", ThumbnailWidth: 3, ThumbnailHeight: 3}, nil
	}); err == nil {
		t.Fatal("failed write accepted")
	}
	preserved, _ := repo.visiblePhoto(ctx, owner.ID, setup.ID, p.ID)
	raw, _ := os.ReadFile(filepath.Join(root, current.thumbnailKey, "thumbnail.jpg"))
	if preserved.ThumbnailRevision != 3 || !bytes.Equal(old, raw) {
		t.Fatal("failure corrupted current")
	}
	reset, err := apply(3, nil)
	if err != nil || reset.ThumbnailRevision != 4 || *reset.Crop != (photos.Crop{X: 5, Y: 0, Size: 10}) {
		t.Fatal("reset", reset, err)
	}
	// Cover updates retain crop/revision; technical PATCH never touches photos.
	cover, err := repo.mutatePhoto(ctx, owner.ID, setup.ID, p.ID, false)
	if err != nil || cover.ThumbnailRevision != 4 {
		t.Fatal("cover", err)
	}
	if _, err := s.Patch(ctx, owner.ID, setup.ID, PatchInput{Title: NullableString{Present: true, Value: ptrString("New title")}}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(root, p.key, "full.jpg"))
	if sha256.Sum256(after) != checksum {
		t.Fatal("full image changed")
	}
	// Legacy rows keep the original proportional file until first adjustment.
	if out, err := exec.Command("convert", filepath.Join(root, p.key, "full.jpg"), filepath.Join(root, p.key, "thumbnail.jpg")).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	_, err = pool.Exec(ctx, `UPDATE setup_photos SET crop=NULL,thumbnail_revision=0,thumbnail_key=NULL,thumbnail_width=20,thumbnail_height=10 WHERE id=$1`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := repo.visiblePhoto(ctx, owner.ID, setup.ID, p.ID)
	if legacy.Crop != nil || legacy.ThumbnailRevision != 0 || legacy.thumbnailKey != p.key {
		t.Fatal("legacy", legacy)
	}
	adjusted, err := apply(0, nil)
	if err != nil || adjusted.ThumbnailRevision != 1 || adjusted.ThumbnailWidth != 10 {
		t.Fatal("legacy adjustment", adjusted, err)
	}
	if err := s.ReconcilePhotos(ctx); err != nil {
		t.Fatal(err)
	}
	// Delete/crop serialization: generation holds the setup lock until commit, then deletion wins.
	started := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan error, 1)
	revision = 1
	go func() {
		_, _, err := repo.cropPhoto(ctx, owner.ID, setup.ID, p.ID, CropInput{&revision, &photos.Crop{Size: 2}}, func(photo Photo, crop photos.Crop) (photos.Image, error) {
			close(started)
			<-resume
			return store.CropThumbnail(ctx, photo.key, crop)
		})
		done <- err
	}()
	<-started
	deleted := make(chan error, 1)
	go func() { _, err := repo.mutatePhoto(ctx, owner.ID, setup.ID, p.ID, true); deleted <- err }()
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if _, err := apply(2, nil); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted crop resurrected", err)
	}
	if err := s.ReconcilePhotos(ctx); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("delete leaked media", entries)
	}
}

func ptrString(s string) *string { return &s }
