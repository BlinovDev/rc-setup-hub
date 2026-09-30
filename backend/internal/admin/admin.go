package admin

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	admintemplates "github.com/BlinovDev/rc-setup-hub/backend/templates/admin"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

type Handler struct {
	emails    []string
	template  *template.Template
	protect   func(http.Handler) http.Handler
	plaintext bool
}

// PageData makes the library-generated hidden CSRF field available to future forms.
// Templates can include {{.CSRFField}} inside an admin mutation form.
type PageData struct {
	User      users.User
	CSRFField template.HTML
}

func New(c config.Auth) (*Handler, error) {
	tmpl, err := template.ParseFS(admintemplates.Files, "index.html")
	if err != nil {
		return nil, err
	}
	redirect, err := url.Parse(c.GoogleRedirectURL)
	if err != nil {
		return nil, err
	}
	return &Handler{emails: append([]string(nil), c.AdminEmails...), template: tmpl, plaintext: redirect.Scheme == "http",
		protect: csrf.Protect(c.SessionSecret, csrf.CookieName("rc_admin_csrf"), csrf.Path("/admin"),
			csrf.Secure(c.CookieSecure), csrf.HttpOnly(true), csrf.SameSite(csrf.SameSiteLaxMode),
			csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "Forbidden", http.StatusForbidden) })))}, nil
}
func (h *Handler) Register(r chi.Router, authenticate func(http.Handler) http.Handler) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(authenticate)
		r.Use(h.RequireAdmin)
		r.Use(h.CSRF)
		r.Get("/", h.Page)
	})
}

// RequireAdmin checks only the authenticated application email against the environment allowlist.
func (h *Handler) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, ok := auth.CurrentUser(r)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		for _, email := range h.emails {
			if strings.EqualFold(strings.TrimSpace(user.Email), email) {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
	})
}

// CSRF protects only admin routes; local HTTP is explicitly marked for the library.
func (h *Handler) CSRF(next http.Handler) http.Handler {
	protected := h.protect(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.plaintext {
			r = csrf.PlaintextHTTPRequest(r)
		}
		protected.ServeHTTP(w, r)
	})
}

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var body bytes.Buffer
	// Buffer first so template failures cannot send partial HTML with status 200.
	if err := h.template.ExecuteTemplate(&body, "index.html", PageData{User: user, CSRFField: csrf.TemplateField(r)}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body.Bytes())
}
