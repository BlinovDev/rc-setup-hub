package users_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type profileGoogle struct{}

func (profileGoogle) AuthorizationURL(state, nonce, verifier string) string {
	return "https://accounts.google.com/auth?state=" + url.QueryEscape(state)
}
func (profileGoogle) Exchange(context.Context, string, string, string) (users.Identity, error) {
	return users.Identity{Subject: "caller-private-subject", Email: "caller@example.com", Name: "Caller"}, nil
}

func TestPublicProfileHTTP(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	service := users.NewService(users.NewRepository(pool))
	target, err := service.Login(ctx, users.Identity{Subject: "target-private-subject", Email: "target-private@example.com", Name: "Driver", AvatarURL: "https://example.com/avatar"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode}
	authentication := auth.NewHandler(cfg, profileGoogle{}, service)
	router := web.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), pool.Ping)
	authentication.Register(router, func(r chi.Router) { users.NewHandler(service).RegisterAPI(r, auth.CurrentUser) })
	request := func(router http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	cookieNamed := func(w *httptest.ResponseRecorder, name string) *http.Cookie {
		t.Helper()
		for _, c := range w.Result().Cookies() {
			if c.Name == name {
				return c
			}
		}
		t.Fatalf("missing %s", name)
		return nil
	}
	start := request(router, "/auth/google", nil)
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := request(router, "/auth/google/callback?code=fake&state="+url.QueryEscape(location.Query().Get("state")), cookieNamed(start, "rc_oauth_state"))
	if callback.Code != 303 {
		t.Fatal("login", callback.Code, callback.Body)
	}
	cookie := cookieNamed(callback, "rc_session")
	for _, tc := range []struct {
		name, id string
		cookie   *http.Cookie
		status   int
	}{
		{"existing", target.ID, cookie, 200}, {"invalid", "invalid", cookie, 400},
		{"missing", "00000000-0000-0000-0000-000000000000", cookie, 404}, {"unauthenticated", target.ID, nil, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(router, "/api/v1/users/"+tc.id, tc.cookie)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body)
			}
			for _, private := range []string{"email", "google_subject", "target-private", "caller-private", "rc_session"} {
				if strings.Contains(w.Body.String(), private) {
					t.Fatal("private data exposed", w.Body)
				}
			}
			if tc.status == 200 {
				var got users.PublicProfile
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.ID != target.ID || got.Nickname != target.Nickname || got.AvatarURL == nil || *got.AvatarURL != "https://example.com/avatar" {
					t.Fatal("incorrect profile", got)
				}
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(w.Body.Bytes(), &fields)
				if len(fields) != 3 {
					t.Fatal("unexpected public fields", fields)
				}
			}
		})
	}
	// Authentication continues using a healthy pool while the profile query fails.
	closed, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	failedRouter := web.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), pool.Ping)
	authentication.Register(failedRouter, func(r chi.Router) {
		users.NewHandler(users.NewService(users.NewRepository(closed))).RegisterAPI(r, auth.CurrentUser)
	})
	w := request(failedRouter, "/api/v1/users/"+target.ID, cookie)
	if w.Code != 500 || w.Body.String() != "{\"error\":\"user profile unavailable\"}\n" {
		t.Fatal("unsafe database error", w.Code, w.Body)
	}
}
