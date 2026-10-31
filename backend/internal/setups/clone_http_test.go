package setups

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

func TestCloneHTTPAuthorizationAndInput(t *testing.T) {
	f := newSearchFixture(t)
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode}
	provider := &fakeGoogle{identity: users.Identity{Subject: "search-other", Email: f.other.Email, Name: f.other.Nickname}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(f.pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), f.pool.Ping)
	h := NewHandler(f.service)
	authentication.RegisterWithPublic(router, []func(chi.Router){h.RegisterPublicAPI}, h.RegisterPrivateAPI)
	cookie := login(t, router)
	body := `{"request_id":"00000000-0000-4000-8000-000000000001"}`
	path := "/api/v1/setups/" + f.public[0].ID + "/clone"
	if w := request(router, "POST", path, body, nil); w.Code != 401 {
		t.Fatal(w.Code, w.Body)
	}
	badOrigin := httptest.NewRequest("POST", path, strings.NewReader(body))
	badOrigin.AddCookie(cookie)
	badOrigin.Header.Set("Content-Type", "application/json")
	badOrigin.Header.Set("Origin", "https://evil.test")
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, badOrigin)
	if denied.Code != 403 {
		t.Fatal("origin bypass", denied.Code)
	}
	for _, bad := range []string{`{}`, `null`, `{"request_id":"invalid"}`, `{"request_id":null}`, `{"request_id":"00000000-0000-4000-8000-000000000001","owner_id":"forged"}`, `{"request_id":"00000000-0000-4000-8000-000000000001","visibility":"public"}`} {
		if w := request(router, "POST", path, bad, cookie); w.Code != 400 {
			t.Fatal(bad, w.Code, w.Body)
		}
	}
	w := request(router, "POST", path, body, cookie)
	clone := setupResponse(t, w, 201)
	if w.Header().Get("Location") != "/api/v1/setups/"+clone.ID || w.Header().Get("Cache-Control") != "no-store" || clone.Visibility != Private || clone.OwnerID != f.other.ID {
		t.Fatal(w.Header(), clone)
	}
	var replay Setup
	w = request(router, "POST", path, body, cookie)
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &replay) != nil || replay.ID != clone.ID {
		t.Fatal("retry", w.Code, w.Body)
	}
	for _, id := range []string{f.friends.ID, f.private.ID, "00000000-0000-0000-0000-000000000000"} {
		w := request(router, "POST", "/api/v1/setups/"+id+"/clone", `{"request_id":"00000000-0000-4000-8000-000000000002"}`, cookie)
		if w.Code != 404 || w.Body.String() != "{\"error\":\"setup not found\"}\n" {
			t.Fatal(w.Code, w.Body)
		}
	}
	// A successful source GET is not a cloning capability after a visibility change.
	setupResponse(t, request(router, "GET", "/api/v1/setups/"+f.public[1].ID, "", cookie), 200)
	if _, err := f.pool.Exec(t.Context(), `UPDATE setups SET visibility='private' WHERE id=$1`, f.public[1].ID); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "POST", "/api/v1/setups/"+f.public[1].ID+"/clone", `{"request_id":"00000000-0000-4000-8000-000000000003"}`, cookie); w.Code != 404 {
		t.Fatal("stale source grant", w.Code, w.Body)
	}

}
