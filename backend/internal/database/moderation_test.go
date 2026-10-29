package database_test

import (
	"context"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
)

func TestModerationMigrationPreservesExistingPublicSetups(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	provider, err := database.MigrationProvider(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if _, err := provider.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO users(google_subject,email,nickname) VALUES('migration','migration@example.test','Migration') RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	// Zero, one and many photos are all grandfathered. No timestamp/data changes.
	for i := 0; i < 3; i++ {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO setups(owner_id,title,visibility,data,notes,created_at,updated_at) VALUES($1,'Existing','public','{"suspension":{"rear":{"toe_deg":0}}}','Notes','2020-01-01','2020-01-02') RETURNING id::text`, owner).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for n := 0; n < i; n++ {
			if _, err := pool.Exec(ctx, `INSERT INTO setup_photos(setup_id,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover) VALUES($1,replace(gen_random_uuid()::text,'-',''),20,10,20,10,$2)`, id, n == 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, v := range []string{"private", "friends"} {
		if _, err := pool.Exec(ctx, `INSERT INTO setups(owner_id,title,visibility,data) VALUES($1,'Hidden',$2,'{}')`, owner, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var approved, pending, photos int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE moderation_status='approved' AND visibility='public' AND created_at='2020-01-01' AND updated_at='2020-01-02' AND notes='Notes' AND data #>> '{suspension,rear,toe_deg}'='0'),count(*) FILTER(WHERE moderation_status='pending' AND visibility<>'public'),(SELECT count(*) FROM setup_photos) FROM setups`).Scan(&approved, &pending, &photos); err != nil || approved != 3 || pending != 2 || photos != 3 {
		t.Fatal(approved, pending, photos, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE setups SET moderation_status='invalid'`); err == nil {
		t.Fatal("invalid moderation accepted")
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal("migration not idempotent", err)
	}
}
