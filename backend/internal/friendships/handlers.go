package friendships

import (
	"encoding/json"
	"errors"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"io"
	"mime"
	"net/http"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }
func (h *Handler) RegisterAPI(r chi.Router) {
	r.Get("/friendships", h.list)
	r.Post("/friendships", h.create)
	r.Post("/friendships/{id}/accept", h.accept)
	r.Delete("/friendships/{id}", h.delete)
}
func respond(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if code != 204 {
		_ = json.NewEncoder(w).Encode(value)
	}
}
func failure(w http.ResponseWriter, err error) {
	code, message := 500, "friendship operation failed"
	switch {
	case errors.Is(err, ErrInvalidID), errors.Is(err, ErrSelfRequest):
		code, message = 400, err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrUserNotFound):
		code, message = 404, err.Error()
	case errors.Is(err, ErrConflict):
		code, message = 409, ErrConflict.Error()
	}
	respond(w, code, struct {
		Error string `json:"error"`
	}{message})
}
func caller(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		respond(w, 401, struct {
			Error string `json:"error"`
		}{"authentication required"})
		return "", false
	}
	return user.ID, true
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var body *struct {
		UserID string `json:"user_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err != nil || media != "application/json" || decoder.Decode(&body) != nil || body == nil || !errors.Is(decoder.Decode(new(json.RawMessage)), io.EOF) {
		respond(w, 400, struct {
			Error string `json:"error"`
		}{"expected one JSON object with user_id"})
		return
	}
	relationship, err := h.service.Create(r.Context(), id, body.UserID)
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 201, relationship)
}
func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}
	relationship, err := h.service.Accept(r.Context(), id, chi.URLParam(r, "id"))
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, relationship)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), id, chi.URLParam(r, "id")); err != nil {
		failure(w, err)
		return
	}
	respond(w, 204, nil)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}
	list, err := h.service.List(r.Context(), id)
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, list)
}
