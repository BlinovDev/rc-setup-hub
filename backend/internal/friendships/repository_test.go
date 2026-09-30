package friendships

import (
	"context"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

func TestAcceptedPair(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	service := users.NewService(users.NewRepository(pool))
	a, err := service.Login(ctx, users.Identity{Subject: "a", Email: "a@example.com", Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.Login(ctx, users.Identity{Subject: "b", Email: "b@example.com", Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	check := func(want bool) {
		t.Helper()
		for _, pair := range [][2]string{{a.ID, b.ID}, {b.ID, a.ID}} {
			got, err := repo.AreAccepted(ctx, pair[0], pair[1])
			if err != nil || got != want {
				t.Fatalf("accepted=%v want %v err=%v", got, want, err)
			}
		}
	}
	check(false)
	for _, pair := range [][2]string{{a.ID, b.ID}, {b.ID, a.ID}} {
		if _, err := pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'pending')`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
		check(false)
		if _, err := pool.Exec(ctx, `UPDATE friendships SET status='accepted',updated_at=now()`); err != nil {
			t.Fatal(err)
		}
		check(true)
		if _, err := pool.Exec(ctx, `DELETE FROM friendships`); err != nil {
			t.Fatal(err)
		}
		check(false)
	}
	pool.Close()
	if _, err := repo.AreAccepted(ctx, a.ID, b.ID); err == nil {
		t.Fatal("database error swallowed")
	}
}
