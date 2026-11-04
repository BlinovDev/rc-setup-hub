package database_test

import (
	"context"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
)

func TestDriverProfileMigrationPreservesExistingUsers(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	provider, err := database.MigrationProvider(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO users(google_subject,email,nickname,avatar_url,created_at) VALUES('profile-migration','driver@example.test','Driver','https://google.example/avatar','2020-01-02T03:04:05Z') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var valid bool
	if err := pool.QueryRow(ctx, `SELECT google_subject='profile-migration' AND email='driver@example.test' AND nickname='Driver' AND avatar_url='https://google.example/avatar' AND created_at='2020-01-02T03:04:05Z' AND instagram_url IS NULL AND tiktok_url IS NULL AND youtube_url IS NULL AND custom_avatar_id IS NULL AND custom_avatar_key IS NULL FROM users WHERE id=$1`, id).Scan(&valid); err != nil || !valid {
		t.Fatal(valid, err)
	}
	for _, sql := range []string{
		`UPDATE users SET custom_avatar_key='../../unsafe' WHERE id=$1`,
		`UPDATE users SET custom_avatar_id=gen_random_uuid() WHERE id=$1`,
		`UPDATE users SET custom_avatar_key=repeat('a',32) WHERE id=$1`,
		`UPDATE users SET instagram_url=repeat('a',2049) WHERE id=$1`,
	} {
		if _, err := pool.Exec(ctx, sql, id); err == nil {
			t.Fatal("unsafe storage value accepted", sql)
		}
	}
	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT avatar_url='https://google.example/avatar' AND nickname='Driver' FROM users WHERE id=$1`, id).Scan(&valid); err != nil || !valid {
		t.Fatal(valid, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal("round trip", err)
	}
}
