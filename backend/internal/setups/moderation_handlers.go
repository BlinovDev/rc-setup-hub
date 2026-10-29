package setups

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	admintemplates "github.com/BlinovDev/rc-setup-hub/backend/templates/admin"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

type ModerationHandler struct {
	service   *Service
	templates *template.Template
}
type reviewPage struct {
	User          users.User
	CSRFField     template.HTML
	Reviews       []ReviewItem
	Setup         *Setup
	Photos        []Photo
	TechnicalData string
}

func NewModerationHandler(service *Service) (*ModerationHandler, error) {
	tmpl, err := template.ParseFS(admintemplates.Files, "*.html")
	if err != nil {
		return nil, err
	}
	return &ModerationHandler{service: service, templates: tmpl}, nil
}

// RegisterAdmin must be called within the existing admin session/allowlist/CSRF group.
func (h *ModerationHandler) RegisterAdmin(r chi.Router) {
	r.Get("/moderation", h.Moderation)
	r.Get("/moderation/{id}", h.Preview)
	r.Get("/moderation/{id}/photos/{photo_id}/{variant:image|thumbnail}", h.ReviewImage)
	r.Post("/moderation/{id}/approve", h.Approve)
	r.Post("/moderation/{id}/reject", h.Reject)
}
func (h *ModerationHandler) render(w http.ResponseWriter, name string, data reviewPage) {
	var body bytes.Buffer
	if err := h.templates.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}

func reviewError(w http.ResponseWriter, err error) {
	status := 500
	var validation *ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		status = 404
	case errors.Is(err, ErrConflict):
		status = 409
	case errors.As(err, &validation):
		status = 400
	}
	http.Error(w, http.StatusText(status), status)
}
func (h *ModerationHandler) Moderation(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.PendingReviews(r.Context())
	if err != nil {
		reviewError(w, err)
		return
	}
	user, _ := auth.CurrentUser(r)
	h.render(w, "moderation.html", reviewPage{User: user, Reviews: items, CSRFField: csrf.TemplateField(r)})
}
func (h *ModerationHandler) Preview(w http.ResponseWriter, r *http.Request) {
	setup, photos, err := h.service.ReviewPreview(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		reviewError(w, err)
		return
	}
	raw, err := json.MarshalIndent(setup.Data, "", "  ")
	if err != nil {
		reviewError(w, err)
		return
	}
	user, _ := auth.CurrentUser(r)
	h.render(w, "review.html", reviewPage{User: user, Setup: &setup, Photos: photos, TechnicalData: string(raw), CSRFField: csrf.TemplateField(r)})
}
func (h *ModerationHandler) Approve(w http.ResponseWriter, r *http.Request) { h.decide(w, r, Approved) }
func (h *ModerationHandler) Reject(w http.ResponseWriter, r *http.Request)  { h.decide(w, r, Rejected) }
func (h *ModerationHandler) decide(w http.ResponseWriter, r *http.Request, decision ModerationStatus) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	values := r.PostForm["expected_updated_at"]
	if len(values) != 1 {
		http.Error(w, "Bad Request", 400)
		return
	}
	expected, err := time.Parse(time.RFC3339Nano, values[0])
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	user, _ := auth.CurrentUser(r)
	if err := h.service.Review(r.Context(), chi.URLParam(r, "id"), user.ID, expected, decision); err != nil {
		reviewError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/moderation", http.StatusSeeOther)
}
func (h *ModerationHandler) ReviewImage(w http.ResponseWriter, r *http.Request) {
	f, err := h.service.ReviewPhoto(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "photo_id"), chi.URLParam(r, "variant") == "thumbnail")
	if err != nil {
		reviewError(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "inline")
	r = r.Clone(r.Context())
	r.Header.Del("If-None-Match")
	r.Header.Del("If-Modified-Since")
	http.ServeContent(w, r, "photo.jpg", time.Time{}, f)
}
