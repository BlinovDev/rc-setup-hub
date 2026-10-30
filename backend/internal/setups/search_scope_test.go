package setups

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestVisibleScopeRequiresSessionAndValidScope(t *testing.T) {
	service := &Service{}
	for _, scope := range []string{"mine", "friends"} {
		if _, err := service.SearchVisible(context.Background(), "", SearchInput{Scope: &scope}); !errors.Is(err, ErrSearchAuthentication) {
			t.Fatalf("guest scope %s: %v", scope, err)
		}
	}
	for _, scope := range []string{"", "unknown", "MINE"} {
		if _, err := normalizeSearch(SearchInput{Scope: &scope}); err == nil {
			t.Fatalf("accepted scope %q", scope)
		}
	}
}

func TestVisibleScopeFiltersBeforePaginationAndRechecksFriendship(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, f.owner.ID, f.friend.ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, viewer, scope string
		expected            []Setup
	}{
		{"mine includes private", f.owner.ID, "mine", []Setup{f.public[0], f.public[1], f.public[3], f.friends, f.private}},
		{"friend sees no private or unrelated", f.friend.ID, "friends", []Setup{f.public[0], f.public[1], f.public[3], f.friends}},
		{"own setups excluded from friends", f.owner.ID, "friends", nil},
		{"unrelated has no friends", f.other.ID, "friends", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := []string{}
			var cursor *string
			for i := 0; i < 10; i++ {
				page, err := f.service.SearchVisible(ctx, tc.viewer, SearchInput{Scope: &tc.scope, Limit: stringPtr("1"), Cursor: cursor})
				if err != nil {
					t.Fatal(err)
				}
				ids = append(ids, searchIDs(page.Items)...)
				cursor = page.NextCursor
				if cursor == nil {
					break
				}
			}
			if !reflect.DeepEqual(ids, sortedIDs(tc.expected)) {
				t.Fatalf("got %v want %v", ids, sortedIDs(tc.expected))
			}
		})
	}
	scope := "friends"
	page, err := f.service.SearchVisible(ctx, f.friend.ID, SearchInput{Scope: &scope, ModelID: &f.rd.ID, Limit: stringPtr("1")})
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("model and page: %+v %v", page, err)
	}
	for _, status := range []string{"pending", "accepted"} {
		if _, err := f.pool.Exec(ctx, `UPDATE friendships SET status=$1`, status); err != nil {
			t.Fatal(err)
		}
		found, err := f.service.SearchVisible(ctx, f.friend.ID, SearchInput{Scope: &scope, ModelID: &f.rd.ID})
		want := 0
		if status == "accepted" {
			want = 2
		}
		if err != nil || len(found.Items) != want {
			t.Fatalf("%s: %+v %v", status, found, err)
		}
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM friendships`); err != nil {
		t.Fatal(err)
	}
	found, err := f.service.SearchVisible(ctx, f.friend.ID, SearchInput{Scope: &scope, ModelID: &f.rd.ID, Cursor: page.NextCursor})
	if err != nil || len(found.Items) != 0 || found.NextCursor != nil {
		t.Fatalf("revoked cursor: %+v %v", found, err)
	}
}
