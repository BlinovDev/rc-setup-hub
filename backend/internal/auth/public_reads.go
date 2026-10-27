package auth

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/httpjson"
)

// A shared process budget bounds anonymous work without trusting client-supplied
// forwarding headers or keeping an attacker-controlled IP map. Authenticated
// traffic retains its existing behavior. Edge per-IP/distributed controls are separate.
type publicReadLimits struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
	now    func() time.Time
	slots  chan struct{}
}

func newPublicReadLimits() *publicReadLimits {
	return &publicReadLimits{tokens: 120, last: time.Now(), now: time.Now, slots: make(chan struct{}, 16)}
}
func (l *publicReadLimits) take() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.tokens = min(120, l.tokens+max(0, now.Sub(l.last).Seconds())*30)
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
func (l *publicReadLimits) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Even an accidentally registered public mutation must fail closed.
		if r.Method != http.MethodGet {
			httpjson.Error(w, 401, "authentication required")
			return
		}
		if _, authenticated := CurrentUser(r); authenticated {
			next.ServeHTTP(w, r)
			return
		}
		if len(r.RequestURI) > 2048 {
			httpjson.Error(w, http.StatusRequestURITooLong, "public request URI too long")
			return
		}
		reject := func() {
			w.Header().Set("Retry-After", "1")
			httpjson.Error(w, http.StatusTooManyRequests, "public reads busy; retry later")
		}
		if !l.take() {
			reject()
			return
		}
		select {
		case l.slots <- struct{}{}:
			defer func() { <-l.slots }()
		default:
			reject()
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		controller := http.NewResponseController(w)
		_ = controller.SetWriteDeadline(l.now().Add(15 * time.Second))
		defer controller.SetWriteDeadline(time.Time{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
