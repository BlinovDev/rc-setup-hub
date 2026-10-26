package setups

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

func TestPhotoMutationDoesNotSweepUnrelatedMedia(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	root := t.TempDir()
	store, err := photos.New(root)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(NewRepository(pool), chassis.NewService(chassis.NewRepository(pool)), friendships.NewRepository(pool))
	s.EnablePhotos(store)
	owner, err := users.NewService(users.NewRepository(pool)).Login(ctx, users.Identity{Subject: "cleanup", Email: "cleanup@example.test", Name: "Cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	setup, err := s.Create(ctx, owner.ID, CreateInput{Title: "Cleanup", Visibility: Public, Data: &DataV1{}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.uploadPhoto(ctx, owner.ID, setup.ID, "11111111-1111-4111-8111-111111111111", photoPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, "ffffffffffffffffffffffffffffffff")
	if err := os.Mkdir(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	expected := p.ThumbnailRevision
	if _, err := s.applyCrop(ctx, owner.ID, setup.ID, p.ID, CropInput{&expected, p.Crop}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("no-op crop swept unrelated media", err)
	}
	// More queued keys than one request may process. No files are needed for these
	// already-missing publications; their durable rows still need acknowledgement.
	for i := 0; i < 250; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO setup_photo_deletions(storage_key) VALUES($1)`, fmt.Sprintf("%032x", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(ctx, owner.ID, setup.ID); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM setup_photo_deletions`).Scan(&pending); err != nil || pending == 0 {
		t.Fatal("request drained the entire global queue", pending, err)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("delete swept unrelated media", err)
	}
	// A restarted service recovers the remaining queue and performs the full sweep.
	restarted := NewService(NewRepository(pool), nil, nil)
	restarted.EnablePhotos(store)
	if err := restarted.ReconcilePhotos(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM setup_photo_deletions`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("restart left queued files", pending, err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("background reconciliation left orphan", err)
	}
}

func TestPhotoCleanupContinuesAfterFileFailure(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	root := t.TempDir()
	store, err := photos.New(root)
	if err != nil {
		t.Fatal(err)
	}
	blocked := "00000000000000000000000000000000"
	dir := filepath.Join(root, blocked)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blocked"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO setup_photo_deletions(storage_key) VALUES($1)`, blocked); err != nil {
		t.Fatal(err)
	}
	// Cross multiple batches so a failed first key cannot starve later work.
	for i := 1; i <= 250; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO setup_photo_deletions(storage_key) VALUES($1)`, fmt.Sprintf("%032x", i)); err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(NewRepository(pool), nil, nil)
	s.EnablePhotos(store)
	if err := s.ReconcilePhotos(ctx); err == nil {
		t.Fatal("filesystem failure did not fail reconciliation")
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM setup_photo_deletions`).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("unrelated deletions blocked or failed row lost", pending, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcilePhotos(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM setup_photo_deletions`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("retry did not converge", pending, err)
	}
}
