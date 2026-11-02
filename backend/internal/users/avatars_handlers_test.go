package users

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/go-chi/chi/v5"
)

type forbiddenAvatarBody struct{}

func (forbiddenAvatarBody) Read([]byte) (int, error) { panic("busy upload must not consume body") }
func (forbiddenAvatarBody) Close() error             { return nil }

func TestAvatarUploadCapacityBeforeParsing(t *testing.T) {
	s := &Service{avatars: &photos.Store{}, avatarSlots: make(chan struct{}, 2)}
	s.avatarSlots <- struct{}{}
	s.avatarSlots <- struct{}{}
	r := httptest.NewRequest("PUT", "/api/v1/me/avatar", forbiddenAvatarBody{})
	w := httptest.NewRecorder()
	NewHandler(s).uploadAvatar(w, r, "owner")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "busy") {
		t.Fatal(w.Code, w.Body)
	}
	if len(s.avatarSlots) != 2 {
		t.Fatal("capacity changed")
	}
}

// Invalid resource identifiers must be rejected before storage/DB access, even
// when pgtype would accept a 32-hex storage key as a compact UUID.
func TestAvatarRejectsNonCanonicalUUIDsBeforeLookup(t *testing.T) {
	valid := "11111111-1111-4111-8111-111111111111"
	for _, ids := range [][2]string{{strings.ReplaceAll(valid, "-", ""), valid}, {valid, strings.ReplaceAll(valid, "-", "")}, {"invalid", valid}, {valid, "invalid"}} {
		router := chi.NewRouter()
		NewHandler(NewService(nil)).RegisterPublicAPI(router)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/users/"+ids[0]+"/avatar/"+ids[1], nil))
		if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid user or avatar UUID") {
			t.Fatal(ids, w.Code, w.Body)
		}
	}
}
