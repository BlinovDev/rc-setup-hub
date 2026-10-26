package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

func TestAnonymousReadBudget(t *testing.T) {
	limits := newPublicReadLimits()
	now := time.Now()
	limits.now = func() time.Time { return now }
	limits.last = now
	called := 0
	handler := limits.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Error("unbounded public context")
		}
		w.WriteHeader(200)
	}))
	for i := 0; i < 120; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "8.8.8.8")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" || called != 120 {
		t.Fatal("budget bypass", w.Code, called)
	}
	now = now.Add(time.Second)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("budget did not refill", w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/?q="+strings.Repeat("x", 2048), nil))
	if w.Code != 414 {
		t.Fatal("unbounded URI", w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 401 {
		t.Fatal("public mutation", w.Code)
	}
}
func TestAnonymousConcurrencyAndAuthenticatedIsolation(t *testing.T) {
	limits := newPublicReadLimits()
	for i := 0; i < cap(limits.slots); i++ {
		limits.slots <- struct{}{}
	}
	called := false
	handler := limits.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 429 || called {
		t.Fatal("concurrency guard failed")
	}
	limits.tokens = 0
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), userContextKey{}, users.User{ID: "owner"}))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !called || w.Code != 200 {
		t.Fatal("guest budget changed authenticated behavior")
	}
}
func TestAnonymousBudgetIsRaceSafe(t *testing.T) {
	limits := newPublicReadLimits()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); limits.take() }()
	}
	wg.Wait()
	if limits.tokens < 0 || limits.tokens > 120 {
		t.Fatal("invalid token balance")
	}
}
