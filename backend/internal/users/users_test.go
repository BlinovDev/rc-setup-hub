package users

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
)

func TestInitialNickname(t *testing.T) {
	for _, tc := range []struct {
		identity Identity
		want     string
	}{
		{Identity{Name: " Alice Driver "}, "Alice-Driver"},
		{Identity{Email: "driver@example.com"}, "driver"},
		{Identity{Name: "!!!"}, "driver"},
		{Identity{Name: strings.Repeat("a", 100)}, strings.Repeat("a", 40)},
	} {
		if got := initialNickname(tc.identity); got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
	}
}
func TestUserPersistence(t *testing.T) {
	pool := dbtest.New(t)
	service := NewService(NewRepository(pool))
	ctx := context.Background()
	identity := Identity{Subject: "subject-1", Email: " FIRST@EXAMPLE.COM ", Name: "Alice Driver", AvatarURL: "https://example.com/avatar"}
	first, err := service.Login(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if first.Email != "first@example.com" || first.Nickname != "Alice-Driver" || first.AvatarURL == nil {
		t.Fatalf("first user: %+v", first)
	}
	repeated, err := service.Login(ctx, identity)
	if err != nil || repeated.ID != first.ID {
		t.Fatalf("repeat identity: %+v %v", repeated, err)
	}
	changed, err := service.UpdateNickname(ctx, first.ID, "  My nickname  ")
	if err != nil || changed.Nickname != "My nickname" {
		t.Fatal(changed, err)
	}
	identity.Email = "Changed@Example.com"
	identity.AvatarURL = "https://example.com/new-avatar"
	changed, err = service.Login(ctx, identity)
	if err != nil || changed.ID != first.ID || changed.Email != "changed@example.com" || changed.Nickname != "My nickname" || *changed.AvatarURL != identity.AvatarURL {
		t.Fatal(changed, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE google_subject=$1", identity.Subject).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate user", count, err)
	}
	second, err := service.Login(ctx, Identity{Subject: "subject-2", Email: "second@example.com", Name: "My nickname"})
	if err != nil || strings.EqualFold(second.Nickname, changed.Nickname) || second.Nickname == "" {
		t.Fatal("nickname collision handling", second, err)
	}
	if _, err := service.UpdateNickname(ctx, second.ID, "MY NICKNAME"); !errors.Is(err, ErrNicknameConflict) {
		t.Fatal("nickname conflict", err)
	}
	for _, nickname := range []string{"", "  ", strings.Repeat("a", 65), "bad\x00name"} {
		if _, err := service.UpdateNickname(ctx, first.ID, nickname); !errors.Is(err, ErrInvalidNickname) {
			t.Fatalf("nickname %q: %v", nickname, err)
		}
	}
	_, err = service.Login(ctx, Identity{Subject: "subject-3", Email: "CHANGED@EXAMPLE.COM", Name: "Another"})
	if !errors.Is(err, ErrEmailConflict) {
		t.Fatal("email conflict not rejected", err)
	}
	base, err := service.Login(ctx, Identity{Subject: "collision-1", Email: "collision1@example.com", Name: "Collision"})
	if err != nil {
		t.Fatal(err)
	}
	suffixed, err := service.Login(ctx, Identity{Subject: "collision-2", Email: "collision2@example.com", Name: "COLLISION"})
	if err != nil || strings.EqualFold(base.Nickname, suffixed.Nickname) || !strings.HasPrefix(suffixed.Nickname, "COLLISION-") {
		t.Fatalf("automatic suffix failed: %+v %v", suffixed, err)
	}
	if _, err := service.Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Login(ctx, Identity{Subject: "concurrent", Email: "concurrent@example.com", Name: "Concurrent"})
			failures <- err
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE google_subject='concurrent'").Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent subject duplicated", count, err)
	}
}
