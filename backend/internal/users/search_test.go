package users

import (
	"context"
	"fmt"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"testing"
)

func TestNicknameSearch(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	service := NewService(NewRepository(pool))
	makeUser := func(name, email string) User {
		t.Helper()
		u, err := service.Login(ctx, Identity{Subject: email, Email: email, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	self := makeUser("DriverSelf", "self@example.com")
	makeUser("Anton-Blinov", "only-email-marker@example.com")
	for i := 24; i >= 0; i-- {
		makeUser(fmt.Sprintf("Driver%02d", i), fmt.Sprintf("d%02d@example.com", i))
	}
	for _, query := range []string{"", "  ", "only-email-marker", "%", "_"} {
		got, err := service.Search(ctx, self.ID, query)
		if err != nil || len(got) != 0 {
			t.Fatal("empty/email/literal search", query, got, err)
		}
	}
	got, err := service.Search(ctx, self.ID, " bLiNo ")
	if err != nil || len(got) != 1 || got[0].Nickname != "Anton-Blinov" {
		t.Fatal("substring/case", got, err)
	}
	got, err = service.Search(ctx, self.ID, "DRIVER")
	if err != nil || len(got) != 20 {
		t.Fatal("limit", len(got), err)
	}
	for i, u := range got {
		if u.ID == self.ID || u.Nickname != fmt.Sprintf("Driver%02d", i) {
			t.Fatal("exclusion/order", i, u)
		}
	}
	repeated, err := service.Search(ctx, self.ID, "driver")
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		if got[i].ID != repeated[i].ID {
			t.Fatal("unstable order")
		}
	}
	if _, err := service.Search(ctx, self.ID, "bad\x00query"); err != ErrInvalidSearchQuery {
		t.Fatal(err)
	}
	pool.Close()
	if _, err := service.Search(ctx, self.ID, "driver"); err == nil {
		t.Fatal("database error swallowed")
	}
}
