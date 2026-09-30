package setups

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterAPI mounts authenticated setup routes; mutations remain owner-only.
func (h *Handler) RegisterAPI(r chi.Router) {
	r.Post("/setups", h.create)
	r.Get("/setups/{id}", h.get)
	r.Patch("/setups/{id}", h.patch)
	r.Delete("/setups/{id}", h.delete)
	r.Get("/me/setups", h.list)
	r.Get("/users/{user_id}/setups", h.listUser)
}

func (h *Handler) listUser(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	setups, err := h.service.ListUser(r.Context(), id, chi.URLParam(r, "user_id"))
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, setups)
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, err error) {
	var validation *ValidationError
	status, message := 500, "setup operation failed"
	switch {
	case errors.As(err, &validation):
		status, message = 400, validation.Error()
	case errors.Is(err, ErrInvalidChassis):
		status, message = 400, ErrInvalidChassis.Error()
	case errors.Is(err, ErrNotFound):
		status, message = 404, ErrNotFound.Error()
	case errors.Is(err, ErrConflict):
		status, message = 409, ErrConflict.Error()
	}
	respond(w, status, struct {
		Error string `json:"error"`
	}{message})
}
func owner(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		respond(w, 401, struct {
			Error string `json:"error"`
		}{"authentication required"})
		return "", false
	}
	return user.ID, true
}
func input(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, invalid("Content-Type must be application/json"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	if err := decodeStrict(r.Body, target); err != nil {
		failure(w, invalid("invalid JSON body or unknown request field"))
		return false
	}
	return true
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	var body *CreateInput
	if !input(w, r, &body) {
		return
	}
	if body == nil {
		failure(w, invalid("body must be a JSON object"))
		return
	}
	setup, err := h.service.Create(r.Context(), id, *body)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/setups/"+setup.ID)
	respond(w, http.StatusCreated, setup)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	setup, err := h.service.Get(r.Context(), id, chi.URLParam(r, "id"))
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, setup)
}
func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	var body *PatchInput
	if !input(w, r, &body) {
		return
	}
	if body == nil {
		failure(w, invalid("body must be a JSON object"))
		return
	}
	setup, err := h.service.Patch(r.Context(), id, chi.URLParam(r, "id"), *body)
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, setup)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), id, chi.URLParam(r, "id")); err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	result, err := h.service.List(r.Context(), id)
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, result)
}
