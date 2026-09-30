package admin

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/jackc/pgx/v5/pgxpool"
)

type readFixture struct {
	pool     *pgxpool.Pool
	repo     *Repository
	accounts []users.User
}

func newReadFixture(t *testing.T) readFixture {
	t.Helper()
	ctx := context.Background()
	f := readFixture{pool: dbtest.New(t)}
	f.repo = NewRepository(f.pool)
	empty, err := f.repo.Stats(ctx)
	if err != nil || empty != (Stats{}) {
		t.Fatal("empty stats", empty, err)
	}
	rows, err := f.repo.Users(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatal("empty users", rows, err)
	}
	service := users.NewService(users.NewRepository(f.pool))
	for i, identity := range []users.Identity{
		{Subject: "private-google-subject-admin", Email: "admin@example.com", Name: "Admin"},
		{Subject: "private-google-subject-other", Email: "other@example.com", Name: "Other"},
		{Subject: "private-google-subject-zero", Email: "zero@example.com", Name: "Zero"},
	} {
		u, err := service.Login(ctx, identity)
		if err != nil {
			t.Fatal(err)
		}
		day := 2
		if i == 0 {
			day = 1
		}
		u.CreatedAt = time.Date(2024, 1, day, 12, 30, 0, 0, time.UTC)
		if _, err := f.pool.Exec(ctx, `UPDATE users SET created_at=$2 WHERE id=$1`, u.ID, u.CreatedAt); err != nil {
			t.Fatal(err)
		}
		f.accounts = append(f.accounts, u)
	}
	u, err := service.UpdateNickname(ctx, f.accounts[2].ID, `Zero <script>alert("x")</script>`)
	if err != nil {
		t.Fatal(err)
	}
	f.accounts[2].Nickname = u.Nickname
	// All visibility states, including a non-v1/corrupt setup and an inactive chassis.
	var modelID string
	if err := f.pool.QueryRow(ctx, `WITH b AS (INSERT INTO chassis_brands(name,is_active) VALUES('Historical brand',false) RETURNING id)
 INSERT INTO chassis_models(brand_id,name,is_active) SELECT id,'Historical model',false FROM b RETURNING id::text`).Scan(&modelID); err != nil {
		t.Fatal(err)
	}
	for i, visibility := range []string{"public", "public", "friends", "private", "private", "private", "public", "private"} {
		owner := f.accounts[0].ID
		if i >= 6 {
			owner = f.accounts[1].ID
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO setups(owner_id,chassis_model_id,title,visibility,data,schema_version) VALUES($1,$2,'Fixture',$3,'[]'::jsonb,2)`, owner, modelID, visibility); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func TestAdminReadRepository(t *testing.T) {
	f := newReadFixture(t)
	ctx := context.Background()
	stats, err := f.repo.Stats(ctx)
	if err != nil || stats != (Stats{TotalUsers: 3, TotalSetups: 8, PublicSetups: 3}) {
		t.Fatal("stats", stats, err)
	}
	rows, err := f.repo.Users(ctx)
	if err != nil || len(rows) != 3 {
		t.Fatal(rows, err)
	}
	expected := append([]users.User(nil), f.accounts...)
	sort.Slice(expected, func(i, j int) bool {
		if expected[i].CreatedAt.Equal(expected[j].CreatedAt) {
			return expected[i].ID > expected[j].ID
		}
		return expected[i].CreatedAt.After(expected[j].CreatedAt)
	})
	counts := map[string]int64{f.accounts[0].ID: 6, f.accounts[1].ID: 2, f.accounts[2].ID: 0}
	for i, row := range rows {
		if row.ID != expected[i].ID || row.Nickname != expected[i].Nickname || row.Email != expected[i].Email || !row.CreatedAt.Equal(expected[i].CreatedAt) || row.CreatedAt.Location() != time.UTC || row.SetupCount != counts[row.ID] {
			t.Fatal("row/order/count", i, row, expected[i])
		}
	}
	f.pool.Close()
	if _, err := f.repo.Stats(ctx); err == nil {
		t.Fatal("stats DB error swallowed")
	}
	if _, err := f.repo.Users(ctx); err == nil {
		t.Fatal("users DB error swallowed")
	}
}
