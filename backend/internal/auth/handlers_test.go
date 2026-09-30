package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
)

type fakeProvider struct {
	identity        users.Identity
	err             error
	calls           int
	nonce, verifier string
}

func (f *fakeProvider) AuthorizationURL(state, nonce, verifier string) string {
	f.nonce = nonce
	f.verifier = verifier
	return "https://accounts.google.com/auth?state=" + url.QueryEscape(state)
}
func (f *fakeProvider) Exchange(ctx context.Context, code, nonce, verifier string) (users.Identity, error) {
	f.calls++
	if nonce != f.nonce || verifier != f.verifier || code != "fake-code" {
		return users.Identity{}, errors.New("flow binding failed")
	}
	return f.identity, f.err
}
func testConfig() config.Auth {
	return config.Auth{SessionSecret: bytes.Repeat([]byte("s"), 32), GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
}
func request(router http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
func findCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}
func start(t *testing.T, router http.Handler) (string, *http.Cookie) {
	t.Helper()
	w := request(router, "GET", "/auth/google", "", nil)
	if w.Code != 302 {
		t.Fatal("login start", w.Code, w.Body)
	}
	target, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return target.Query().Get("state"), findCookie(t, w, stateCookie)
}
func signIn(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	state, cookie := start(t, router)
	w := request(router, "GET", "/auth/google/callback?code=fake-code&state="+url.QueryEscape(state), "", cookie)
	if w.Code != 303 {
		t.Fatal("callback", w.Code, w.Body)
	}
	return findCookie(t, w, sessionCookie)
}
func TestAuthHTTP(t *testing.T) {
	pool := dbtest.New(t)
	service := users.NewService(users.NewRepository(pool))
	fake := &fakeProvider{identity: users.Identity{Subject: "private-subject", Email: "driver@example.com", Name: "Driver"}}
	h := NewHandler(testConfig(), fake, service)
	var logs bytes.Buffer
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), pool.Ping)
	h.Register(router)
	if w := request(router, "GET", "/api/v1/me", "", nil); w.Code != 401 {
		t.Fatal("unauthenticated me", w.Code)
	}
	if w := request(router, "PATCH", "/api/v1/me", `{"nickname":"test"}`, nil); w.Code != 401 {
		t.Fatal("unauthenticated patch", w.Code)
	}
	cookie := signIn(t, router)
	if !cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("local cookie flags: %+v", cookie)
	}
	w := request(router, "GET", "/api/v1/me", "", cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "driver@example.com") || strings.Contains(w.Body.String(), "google_subject") || strings.Contains(w.Body.String(), "private-subject") {
		t.Fatal("profile exposure", w.Code, w.Body)
	}
	second, err := service.Login(context.Background(), users.Identity{Subject: "other-subject", Email: "other@example.com", Name: "Taken"})
	if err != nil {
		t.Fatal(err)
	}
	_ = second
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"nickname":"  Updated  "}`, 200},
		{`{"nickname":"taken"}`, 409},
		{`{"nickname":"   "}`, 400},
		{`{"nickname":"` + strings.Repeat("a", 65) + `"}`, 400},
		{`{"nickname":"ok","email":"forged@example.com"}`, 400},
		{`{"nickname":"ok"} {}`, 400},
		{`null`, 400},
		{`{"nickname":`, 400},
	} {
		w := request(router, "PATCH", "/api/v1/me", tc.body, cookie)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body)
		}
	}
	if w := request(router, "GET", "/api/v1/me", "", cookie); !strings.Contains(w.Body.String(), `"nickname":"Updated"`) {
		t.Fatal("trimmed nickname not persisted", w.Body)
	}
	forged := *cookie
	forged.Value += "tampered"
	if w := request(router, "GET", "/api/v1/me", "", &forged); w.Code != 401 {
		t.Fatal("tampered cookie allowed")
	}
	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("untrusted mutation origin allowed", w.Code)
	}
	preflight := httptest.NewRequest("OPTIONS", "/api/v1/me", nil)
	preflight.Header.Set("Origin", "http://localhost:5173")
	preflight.Header.Set("Access-Control-Request-Method", "PATCH")
	preflight.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, preflight)
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("credentialed CORS missing", w.Header())
	}
	w = request(router, "POST", "/api/v1/auth/logout", "", cookie)
	if w.Code != 204 || findCookie(t, w, sessionCookie).MaxAge != -1 {
		t.Fatal("logout", w.Code)
	}
	if w := request(router, "GET", "/api/v1/me", "", cookie); w.Code != 401 {
		t.Fatal("replayed logged-out session accepted")
	}
	if strings.Contains(logs.String(), "fake-code") || strings.Contains(logs.String(), "private-subject") {
		t.Fatal("authentication internals logged")
	}
}
func TestOAuthState(t *testing.T) {
	fake := &fakeProvider{err: errors.New("Google failure")}
	h := NewHandler(testConfig(), fake, nil)
	var logs bytes.Buffer
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), func(context.Context) error { return nil })
	h.Register(router)
	state, cookie := start(t, router)
	for _, path := range []string{"/auth/google/callback?code=fake-code", "/auth/google/callback?code=fake-code&state=wrong"} {
		if w := request(router, "GET", path, "", cookie); w.Code != 400 {
			t.Fatal("invalid state accepted", w.Code)
		}
	}
	path := "/auth/google/callback?code=fake-code&state=" + url.QueryEscape(state)
	if w := request(router, "GET", path, "", nil); w.Code != 400 {
		t.Fatal("missing browser binding accepted")
	}
	if w := request(router, "GET", path, "", cookie); w.Code != 401 {
		t.Fatal("state failed before provider", w.Code)
	}
	if w := request(router, "GET", path, "", cookie); w.Code != 400 {
		t.Fatal("state replay accepted", w.Code)
	}
	if fake.calls != 1 {
		t.Fatal("exchange called with invalid state", fake.calls)
	}
	state, cookie = start(t, router)
	h.sessions.now = func() time.Time { return time.Now().Add(stateTTL + time.Second) }
	if w := request(router, "GET", "/auth/google/callback?code=fake-code&state="+state, "", cookie); w.Code != 400 {
		t.Fatal("expired state accepted")
	}
}
