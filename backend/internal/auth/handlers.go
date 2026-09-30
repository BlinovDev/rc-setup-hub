package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"golang.org/x/oauth2"
)

type Handler struct {
	provider      Provider
	users         *users.Service
	sessions      *Sessions
	origins       []string
	backendOrigin string
}

func NewHandler(c config.Auth, provider Provider, service *users.Service) *Handler {
	redirect, _ := url.Parse(c.GoogleRedirectURL)
	return &Handler{provider: provider, users: service, sessions: NewSessions(c), origins: c.AllowedOrigins,
		backendOrigin: redirect.Scheme + "://" + redirect.Host}
}
func (h *Handler) Register(r chi.Router) {
	r.Get("/auth/google", h.login)
	r.Get("/auth/google/callback", h.callback)
	r.Route("/api/v1", func(r chi.Router) {
		// Empty allowlists must not use chi/cors' default wildcard behavior.
		if len(h.origins) > 0 {
			r.Use(cors.Handler(cors.Options{AllowedOrigins: h.origins,
				AllowedMethods: []string{"GET", "PATCH", "POST", "OPTIONS"}, AllowedHeaders: []string{"Content-Type"}, AllowCredentials: true}))
		}
		r.Use(h.RequireUser)
		r.Use(h.checkOrigin)
		r.Post("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			h.sessions.Logout(w, r)
			w.WriteHeader(http.StatusNoContent)
		})
		r.Get("/me", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, currentUser(r)) })
		r.Patch("/me", h.updateNickname)
	})
}
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, struct {
		Error string `json:"error"`
	}{message})
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	nonce, err := randomToken()
	if err != nil {
		fail(w, 500, "authentication unavailable")
		return
	}
	verifier := oauth2.GenerateVerifier()
	state, err := h.sessions.Start(w, nonce, verifier)
	if err != nil {
		fail(w, 500, "authentication unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, h.provider.AuthorizationURL(state, nonce, verifier), http.StatusFound)
}
func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	f, err := h.sessions.Consume(w, r, r.URL.Query().Get("state"))
	if err != nil {
		fail(w, 400, "invalid or expired OAuth state")
		return
	}
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		fail(w, 400, "Google authorization was not completed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	identity, err := h.provider.Exchange(ctx, r.URL.Query().Get("code"), f.nonce, f.verifier)
	if err != nil {
		fail(w, 401, "Google identity verification failed")
		return
	}
	user, err := h.users.Login(ctx, identity)
	if err != nil {
		if errors.Is(err, users.ErrEmailConflict) {
			fail(w, 409, "email already in use")
			return
		}
		fail(w, 500, "authentication unavailable")
		return
	}
	if err := h.sessions.Issue(w, r, user.ID); err != nil {
		fail(w, 500, "authentication unavailable")
		return
	}
	http.Redirect(w, r, "/api/v1/me", http.StatusSeeOther)
}

type userContextKey struct{}

func currentUser(r *http.Request) users.User { return r.Context().Value(userContextKey{}).(users.User) }

// CurrentUser returns the application user resolved by RequireUser.
func CurrentUser(r *http.Request) (users.User, bool) {
	user, ok := r.Context().Value(userContextKey{}).(users.User)
	return user, ok
}

// RequireUser resolves an existing session and returns 401 when authentication is absent.
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := h.sessions.UserID(r)
		if err != nil {
			fail(w, 401, "authentication required")
			return
		}
		user, err := h.users.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, users.ErrNotFound) {
				h.sessions.Logout(w, r)
				fail(w, 401, "authentication required")
				return
			}
			fail(w, 500, "profile unavailable")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}
func (h *Handler) checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			origin := r.Header.Get("Origin")
			allowed := origin == h.backendOrigin
			for _, candidate := range h.origins {
				if origin == candidate {
					allowed = true
				}
			}
			// Non-browser API tools omit Origin. Reject cross-site browser requests even when Origin is absent.
			if (origin != "" && !allowed) || (origin == "" && r.Header.Get("Sec-Fetch-Site") == "cross-site") {
				fail(w, 403, "origin not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (h *Handler) updateNickname(w http.ResponseWriter, r *http.Request) {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		fail(w, 415, "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input struct {
		Nickname string `json:"nickname"`
	}
	if err := decoder.Decode(&input); err != nil {
		fail(w, 400, "invalid JSON body")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		fail(w, 400, "invalid JSON body")
		return
	}
	user, err := h.users.UpdateNickname(r.Context(), currentUser(r).ID, input.Nickname)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrInvalidNickname):
			fail(w, 400, users.ErrInvalidNickname.Error())
		case errors.Is(err, users.ErrNicknameConflict):
			fail(w, 409, users.ErrNicknameConflict.Error())
		default:
			fail(w, 500, "profile update unavailable")
		}
		return
	}
	respond(w, 200, user)
}
