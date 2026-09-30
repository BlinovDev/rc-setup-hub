package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	cfg := testConfig()
	cfg.CookieSecure = true
	cfg.SameSite = http.SameSiteNoneMode
	sessions := NewSessions(cfg)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	if err := sessions.Issue(w, req, "user-id"); err != nil {
		t.Fatal(err)
	}
	cookie := findCookie(t, w, sessionCookie)
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteNoneMode {
		t.Fatal("production cookie flags", cookie)
	}
	req.AddCookie(cookie)
	if id, err := sessions.UserID(req); err != nil || id != "user-id" {
		t.Fatal(id, err)
	}
	if _, err := NewSessions(cfg).UserID(req); err == nil {
		t.Fatal("session survived process/store restart")
	}
	rotated := httptest.NewRecorder()
	if err := sessions.Issue(rotated, req, "user-id"); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.UserID(req); err == nil {
		t.Fatal("old session not rotated")
	}
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(findCookie(t, rotated, sessionCookie))
	sessions.now = func() time.Time { return time.Now().Add(sessionTTL + time.Second) }
	if _, err := sessions.UserID(req); err == nil {
		t.Fatal("expired session accepted")
	}
}
func TestStateSingleUseConcurrent(t *testing.T) {
	sessions := NewSessions(testConfig())
	w := httptest.NewRecorder()
	state, err := sessions.Start(w, "nonce", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	cookie := findCookie(t, w, stateCookie)
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/auth/google/callback", nil)
			req.AddCookie(cookie)
			_, err := sessions.Consume(httptest.NewRecorder(), req, state)
			results <- err == nil
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for success := range results {
		if success {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("single-use state consumed", successes, "times")
	}
}
