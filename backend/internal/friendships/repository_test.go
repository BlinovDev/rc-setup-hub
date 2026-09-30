package friendships

import (
	"context"
	"errors"
	"strings"
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

func TestFriendshipMutations(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	service := NewService(repo)
	userService := users.NewService(users.NewRepository(pool))
	makeUser := func(name string) users.User {
		t.Helper()
		u, err := userService.Login(ctx, users.Identity{Subject: name, Email: name + "@example.com", Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	a, b, c := makeUser("a"), makeUser("b"), makeUser("c")
	r, err := repo.Create(ctx, a.ID, b.ID)
	if err != nil || r.Status != Pending || r.RequesterID != a.ID || r.AddresseeID != b.ID {
		t.Fatal(r, err)
	}
	for _, pair := range [][2]string{{a.ID, b.ID}, {b.ID, a.ID}} {
		if _, err := repo.Create(ctx, pair[0], pair[1]); !errors.Is(err, ErrConflict) {
			t.Fatal("unordered duplicate", err)
		}
	}
	if _, err := repo.GetParticipant(ctx, c.ID, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("participant scope", err)
	}
	if _, err := repo.Accept(ctx, a.ID, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("requester accept", err)
	}
	if _, err := repo.Accept(ctx, c.ID, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("stranger accept", err)
	}
	accepted, err := service.Accept(ctx, b.ID, r.ID)
	if err != nil || accepted.Status != Accepted || !accepted.UpdatedAt.After(r.UpdatedAt) {
		t.Fatal("accept", accepted, err)
	}
	if _, err := service.Accept(ctx, b.ID, r.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("repeat accept", err)
	}
	if _, err := service.Accept(ctx, a.ID, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("accepted requester", err)
	}
	if err := repo.Delete(ctx, c.ID, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("stranger delete", err)
	}
	if err := repo.Delete(ctx, a.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetParticipant(ctx, a.ID, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("not deleted", err)
	}
	// Race opposite INSERTs: one wins, one maps the unique violation to conflict.
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, pair := range [][2]string{{a.ID, b.ID}, {b.ID, a.ID}} {
		go func(pair [2]string) { <-start; _, err := repo.Create(ctx, pair[0], pair[1]); results <- err }(pair)
	}
	close(start)
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM friendships").Scan(&count); err != nil || count != 1 || successes != 1 || conflicts != 1 {
		t.Fatal("opposite race", count, successes, conflicts, err)
	}
	// Service validation includes equivalent UUID spellings for self requests.
	if _, err := service.Create(ctx, a.ID, strings.ToUpper(a.ID)); !errors.Is(err, ErrSelfRequest) {
		t.Fatal("self request", err)
	}
	if _, err := service.Create(ctx, a.ID, "invalid"); !errors.Is(err, ErrInvalidID) {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, a.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrUserNotFound) {
		t.Fatal("missing target", err)
	}
}
