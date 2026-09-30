package admin

import (
	"bytes"
	"context"
	"html/template"
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
)

type fakeGoogle struct{ identity users.Identity }

func (f fakeGoogle) AuthorizationURL(state, nonce, verifier string) string {
	return "https://accounts.google.com/auth?state=" + url.QueryEscape(state)
}
func (f fakeGoogle) Exchange(context.Context, string, string, string) (users.Identity, error) {
	return f.identity, nil
}
func settings() config.Auth {
	return config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, SessionSecret: bytes.Repeat([]byte("s"), 32), GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SameSite: http.SameSiteLaxMode}
}
func call(router http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
func cookieNamed(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}
func login(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	start := call(router, "/auth/google", nil)
	if start.Code != 302 {
		t.Fatal("start login", start.Code)
	}
	u, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := call(router, "/auth/google/callback?code=fake&state="+url.QueryEscape(u.Query().Get("state")), cookieNamed(t, start, "rc_oauth_state"))
	if callback.Code != 303 {
		t.Fatal("callback", callback.Code, callback.Body)
	}
	return cookieNamed(t, callback, "rc_session")
}
func TestAdminAccess(t *testing.T) {
	pool := dbtest.New(t)
	service := users.NewService(users.NewRepository(pool))
	for _, tc := range []struct {
		name, allow, email string
		status             int
	}{
		{"allowlisted", "  ADMIN@EXAMPLE.COM, ", "admin@example.com", 200},
		{"ordinary", "admin@example.com", "ordinary@example.com", 403},
		{"empty allowlist", "", "admin@example.com", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := settings()
			var err error
			c.AdminEmails, err = config.ParseAdminEmails(tc.allow)
			if err != nil {
				t.Fatal(err)
			}
			identity := users.Identity{Subject: tc.email, Email: tc.email, Name: "Driver"}
			user, err := service.Login(context.Background(), identity)
			if err != nil {
				t.Fatal(err)
			}
			user, err = service.UpdateNickname(context.Background(), user.ID, `<script>`+tc.email+`</script>`)
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			router := web.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), pool.Ping)
			authentication := auth.NewHandler(c, fakeGoogle{identity}, service)
			authentication.Register(router)
			h, err := New(c, NewRepository(pool))
			if err != nil {
				t.Fatal(err)
			}
			h.Register(router, authentication.RequireUser)
			for _, path := range []string{"/admin", "/admin/users"} {
				if w := call(router, path, nil); w.Code != 401 {
					t.Fatal("unauthenticated admin", path, w.Code)
				}
			}
			cookie := login(t, router)
			for _, path := range []string{"/admin", "/admin/users"} {
				if w := call(router, path, cookie); w.Code != tc.status {
					t.Fatalf("%s status %d want %d", path, w.Code, tc.status)
				}
			}
			w := call(router, "/admin", cookie)
			if w.Code != tc.status {
				t.Fatalf("admin status %d want %d: %s", w.Code, tc.status, w.Body)
			}
			if tc.status == 200 {
				for _, text := range []string{"RC Setup Hub", "Dashboard", "Users", "Chassis catalog", tc.email, template.HTMLEscapeString(user.Nickname)} {
					if !strings.Contains(w.Body.String(), text) {
						t.Fatalf("page missing %q: %s", text, w.Body)
					}
				}
				if strings.Contains(w.Body.String(), "<script>") || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
					t.Fatal("unsafe or incorrect HTML", w.Body)
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("admin page can be cached")
				}
				if w := call(router, "/admin/users", cookie); w.Code != 200 {
					t.Fatal("users route unavailable", w.Code)
				}
				// An execution error after some output must yield only a generic 500.
				h.template = template.Must(template.New("index.html").Parse(`private template prefix {{.MissingField}}`))
				failure := call(router, "/admin", cookie)
				if failure.Code != 500 || strings.Contains(failure.Body.String(), "private template") || strings.Contains(failure.Body.String(), "MissingField") {
					t.Fatal("template failure leaked or wrote 200", failure.Code, failure.Body)
				}
			}
		})
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name IN ('role','roles','is_admin')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("admin database role present", count, err)
	}
}
