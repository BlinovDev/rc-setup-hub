package users

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
)

func TestAvatarDurableCleanupAndOrphanSafety(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "avatars")
	store, err := photos.New(root)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepository(pool))
	svc.EnableAvatars(store)
	u, err := svc.Login(ctx, Identity{Subject: "cleanup", Email: "cleanup@example.test", Name: "Cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 8, 10))); err != nil {
		t.Fatal(err)
	}
	u, err = svc.UploadAvatar(ctx, u.ID, raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var key string
	if err := pool.QueryRow(ctx, `SELECT custom_avatar_key FROM users WHERE id=$1`, u.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	// A filesystem failure must retain the DB outbox entry; no profile rollback.
	if err := os.Rename(root, root+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("blocked directory"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := svc.DeleteAvatar(ctx, u.ID)
	if err != nil || got.HasCustomAvatar {
		t.Fatal(got, err)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_avatar_deletions WHERE storage_key=$1`, key).Scan(&queued); err != nil || queued != 1 {
		t.Fatal(queued, err)
	}
	if err := svc.ReconcileAvatars(ctx); err == nil {
		t.Fatal("cleanup failure silently accepted")
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root+"-saved", root); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(NewRepository(pool))
	restarted.EnableAvatars(store)
	if err := restarted.ReconcileAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, key)); !os.IsNotExist(err) {
		t.Fatal("superseded publication survived restart", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_avatar_deletions`).Scan(&queued); err != nil || queued != 0 {
		t.Fatal(queued, err)
	}
	// Uncertain/precommit orphan publication is retained during the grace period,
	// then swept only after a successful DB inventory. Active media is kept.
	orphan, err := store.Process(ctx, raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ReconcileAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	if f, err := store.Open(orphan.Key, true); err != nil {
		t.Fatal("recent publication swept", err)
	} else {
		_ = f.Close()
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, orphan.Key), old, old); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ReconcileAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, orphan.Key)); !os.IsNotExist(err) {
		t.Fatal("orphan not swept", err)
	}
	u, err = restarted.UploadAvatar(ctx, u.ID, raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT custom_avatar_key FROM users WHERE id=$1`, u.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, key, "thumbnail.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ReconcileAvatars(ctx); err == nil {
		t.Fatal("missing referenced avatar ignored")
	}
	// A failed inventory must not sweep aged files.
	orphan, err = store.Process(ctx, raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, orphan.Key), old, old); err != nil {
		t.Fatal(err)
	}
	pool.Close()
	if err := restarted.ReconcileAvatars(ctx); err == nil {
		t.Fatal("failed DB inventory ignored")
	}
	if _, err := os.Stat(filepath.Join(root, orphan.Key)); err != nil {
		t.Fatal("swept without DB inventory", err)
	}
}
