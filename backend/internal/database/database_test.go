package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestOpenInvalidURL(t *testing.T) {
	if _, err := database.Open(context.Background(), "postgres://user:secret@%invalid"); err == nil || err.Error() != "invalid DATABASE_URL" {
		t.Fatalf("expected sanitized configuration error, got %v", err)
	}
}

func TestOpenConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := database.Open(ctx, "postgres://user:private-password@127.0.0.1:1/test?sslmode=disable"); err == nil || err.Error() != "database connection failed" {
		t.Fatalf("expected sanitized connection error, got %v", err)
	}
}

func TestPostgreSQLSchema(t *testing.T) {
	pool := dbtest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	provider, err := database.MigrationProvider(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if results, err := provider.Up(ctx); err != nil || len(results) != 0 {
		t.Fatalf("up not idempotent: %v %v", results, err)
	}
	var version int64
	version, err = provider.GetDBVersion(ctx)
	if err != nil || version != 5 {
		t.Fatalf("migration version: %d %v", version, err)
	}

	// Every constraint case runs in a transaction, so failures cannot contaminate another case.
	cases := []struct{ name, sql, code string }{
		{"email case insensitive", "INSERT INTO users(google_subject,email,nickname) VALUES ('other','USER@EXAMPLE.COM','other')", "23505"},
		{"nickname case insensitive", "INSERT INTO users(google_subject,email,nickname) VALUES ('other','other@example.com','DRIVER')", "23505"},
		{"google subject unique", "INSERT INTO users(google_subject,email,nickname) VALUES ('subject','other@example.com','other')", "23505"},
		{"brand case insensitive", "INSERT INTO chassis_brands(name) VALUES ('YOKOMO')", "23505"},
		{"model case insensitive in brand", "INSERT INTO chassis_models(brand_id,name) SELECT id,'RD2.0' FROM chassis_brands WHERE name='Yokomo'", "23505"},
		{"same model different brand", "INSERT INTO chassis_models(brand_id,name) SELECT id,'rd2.0' FROM chassis_brands WHERE name='Other'", ""},
		{"friend self", "INSERT INTO friendships(requester_id,addressee_id,status) SELECT id,id,'pending' FROM users WHERE nickname='Driver'", "23514"},
		{"friend duplicate", "INSERT INTO friendships(requester_id,addressee_id,status) SELECT requester_id,addressee_id,'pending' FROM friendships", "23505"},
		{"friend reverse", "INSERT INTO friendships(requester_id,addressee_id,status) SELECT addressee_id,requester_id,'accepted' FROM friendships", "23505"},
		{"friend invalid status", "UPDATE friendships SET status='rejected'", "23514"},
		{"friend accepted", "UPDATE friendships SET status='accepted'", ""},
		{"friend null status", "UPDATE friendships SET status=NULL", "23502"},
		{"friend unknown requester", "UPDATE friendships SET requester_id='00000000-0000-0000-0000-000000000000'", "23503"},
		{"friend unknown addressee", "UPDATE friendships SET addressee_id='00000000-0000-0000-0000-000000000000'", "23503"},
		{"visibility invalid", "UPDATE setups SET visibility='other'", "23514"},
		{"visibility friends", "UPDATE setups SET visibility='friends'", ""},
		{"visibility private", "UPDATE setups SET visibility='private'", ""},
		{"visibility null", "UPDATE setups SET visibility=NULL", "23502"},
		{"version zero", "UPDATE setups SET schema_version=0", "23514"},
		{"version negative", "UPDATE setups SET schema_version=-1", "23514"},
		{"version null", "UPDATE setups SET schema_version=NULL", "23502"},
		{"future positive version", "UPDATE setups SET schema_version=2", ""},
		{"null chassis allowed", "UPDATE setups SET chassis_model_id=NULL", ""},
		{"unknown chassis", "UPDATE setups SET chassis_model_id='00000000-0000-0000-0000-000000000000'", "23503"},
		{"unknown owner", "UPDATE setups SET owner_id='00000000-0000-0000-0000-000000000000'", "23503"},
		{"unknown brand", "UPDATE chassis_models SET brand_id='00000000-0000-0000-0000-000000000000'", "23503"},
		{"brand delete restricted", "DELETE FROM chassis_brands WHERE name='Yokomo'", "23503"},
		{"invalid json", "UPDATE setups SET data='invalid'", "22P02"},
		{"null json", "UPDATE setups SET data=NULL", "23502"},
		{"email null", "UPDATE users SET email=NULL", "23502"},
		{"nickname null", "UPDATE users SET nickname=NULL", "23502"},
		{"subject null", "UPDATE users SET google_subject=NULL", "23502"},
		{"brand name null", "UPDATE chassis_brands SET name=NULL", "23502"},
		{"model name null", "UPDATE chassis_models SET name=NULL", "23502"},
		{"owner null", "UPDATE setups SET owner_id=NULL", "23502"},
		{"title null", "UPDATE setups SET title=NULL", "23502"},
		{"duplicate primary key", "INSERT INTO users(id,google_subject,email,nickname) SELECT id,'new','new@example.com','New' FROM users WHERE nickname='Driver'", "23505"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			seed(t, ctx, tx)
			_, err = tx.Exec(ctx, tc.sql)
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Fatalf("want SQLSTATE %s, got %v", tc.code, err)
			}
		})
	}
	for table, columns := range map[string][]string{
		"users":          {"id", "created_at", "updated_at"},
		"friendships":    {"id", "requester_id", "addressee_id", "created_at", "updated_at"},
		"chassis_brands": {"id", "is_active", "created_at", "updated_at"},
		"chassis_models": {"id", "brand_id", "is_active", "created_at", "updated_at"},
		"setups":         {"id", "created_at", "updated_at"},
	} {
		for _, column := range columns {
			t.Run(table+" nonnull "+column, func(t *testing.T) {
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				seed(t, ctx, tx)
				_, err = tx.Exec(ctx, "UPDATE "+table+" SET "+column+"=NULL")
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "23502" {
					t.Fatalf("expected NOT NULL violation, got %v", err)
				}
			})
		}
	}
	t.Run("defaults json deletion and historical catalog", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		seed(t, ctx, tx)
		var version int
		var active bool
		var created, updated time.Time
		var zero float64
		if err := tx.QueryRow(ctx, "SELECT schema_version, created_at, updated_at, (data #>> '{suspension,front,toe_deg}')::float8 FROM setups").Scan(&version, &created, &updated, &zero); err != nil {
			t.Fatal(err)
		}
		if version != 1 || created.IsZero() || updated.IsZero() || zero != 0 {
			t.Fatal("unexpected defaults/JSONB round trip")
		}
		if err := tx.QueryRow(ctx, "SELECT is_active FROM chassis_models").Scan(&active); err != nil || !active {
			t.Fatalf("model default: %v %v", active, err)
		}
		if err := tx.QueryRow(ctx, "SELECT is_active FROM chassis_brands WHERE name='Yokomo'").Scan(&active); err != nil || !active {
			t.Fatalf("brand default: %v %v", active, err)
		}
		execSQL(t, ctx, tx, "UPDATE chassis_brands SET is_active=false; UPDATE chassis_models SET is_active=false")
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM setups s JOIN chassis_models m ON m.id=s.chassis_model_id JOIN chassis_brands b ON b.id=m.brand_id").Scan(&count); err != nil || count != 1 {
			t.Fatalf("historical catalog lost: %d %v", count, err)
		}
		execSQL(t, ctx, tx, "DELETE FROM chassis_models")
		var isNull bool
		if err := tx.QueryRow(ctx, "SELECT chassis_model_id IS NULL FROM setups").Scan(&isNull); err != nil || !isNull {
			t.Fatalf("model deletion: %v %v", isNull, err)
		}
		execSQL(t, ctx, tx, "DELETE FROM users WHERE nickname='Driver'")
		if err := tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM setups)+(SELECT count(*) FROM friendships)").Scan(&count); err != nil || count != 0 {
			t.Fatalf("user cascade: %d %v", count, err)
		}
	})
	t.Run("schema metadata", func(t *testing.T) {
		var invalid int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('users','friendships','chassis_brands','chassis_models','setups') AND ((column_name='id' AND data_type <> 'uuid') OR (column_name IN ('created_at','updated_at') AND (data_type <> 'timestamp with time zone' OR is_nullable <> 'NO')) OR column_name IN ('role','is_admin','discipline'))`).Scan(&invalid); err != nil || invalid != 0 {
			t.Fatalf("invalid schema: %d %v", invalid, err)
		}
		for _, name := range []string{"users_email_unique", "users_nickname_unique", "friendships_requester_idx", "friendships_addressee_idx", "friendships_pair_unique", "chassis_brands_name_unique", "chassis_models_name_unique", "chassis_models_brand_idx", "setups_owner_idx", "setups_chassis_model_idx", "setups_visibility_idx", "setups_visibility_created_idx"} {
			var exists bool
			if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname=$1)", name).Scan(&exists); err != nil || !exists {
				t.Fatalf("index %s missing: %v", name, err)
			}
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexdef ILIKE '%USING gin%'").Scan(&invalid); err != nil || invalid != 0 {
			t.Fatal("unexpected GIN index", err)
		}
		var zone string
		if err := pool.QueryRow(ctx, "SHOW timezone").Scan(&zone); err != nil || zone != "UTC" {
			t.Fatalf("timezone %s %v", zone, err)
		}
	})
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('users','friendships','chassis_brands','chassis_models','setups')").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("down left tables: %d %v", remaining, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	pool.Close()
	if err := pool.Ping(ctx); err == nil {
		t.Fatal("closed pool reports healthy")
	}
}

func execSQL(t *testing.T, ctx context.Context, tx pgx.Tx, query string) {
	t.Helper()
	if _, err := tx.Exec(ctx, query); err != nil {
		t.Fatal(err)
	}
}

func seed(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	execSQL(t, ctx, tx, `INSERT INTO users(google_subject,email,nickname) VALUES ('subject','user@example.com','Driver'),('friend','friend@example.com','Friend');
    INSERT INTO chassis_brands(name) VALUES ('Yokomo'),('Other');
    INSERT INTO chassis_models(brand_id,name) SELECT id,'rd2.0' FROM chassis_brands WHERE name='Yokomo';
    INSERT INTO friendships(requester_id,addressee_id,status) SELECT a.id,b.id,'pending' FROM users a,users b WHERE a.nickname='Driver' AND b.nickname='Friend';
    INSERT INTO setups(owner_id,chassis_model_id,title,visibility,data) SELECT u.id,m.id,'Drift setup','public','{"suspension":{"front":{"toe_deg":0}}}' FROM users u,chassis_models m WHERE u.nickname='Driver';`)
}
