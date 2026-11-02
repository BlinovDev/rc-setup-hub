package users

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
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
