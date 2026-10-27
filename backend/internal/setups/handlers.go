package setups

import (
	"errors"
	"mime"
	"net/http"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/httpjson"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterAPI retains the authenticated-only registration for existing callers.
func (h *Handler) RegisterAPI(r chi.Router) {
	h.RegisterPublicAPI(r)
	h.RegisterPrivateAPI(r)
}

// RegisterPublicAPI is the explicit GET-only public allowlist; data checks stay in services/SQL.
func (h *Handler) RegisterPublicAPI(r chi.Router) {
	r.Get("/setups/search", h.search)
	r.Get("/setups/{id}", h.get)
	r.Get("/users/{user_id}/public-setups", h.publicUserSetups)
	if h.service.photos != nil {
		r.Get("/setups/{id}/photos", h.gallery)
		r.Get("/setups/{id}/photos/{photo_id}/{variant:image|thumbnail}", h.photoImage)
	}
}
func (h *Handler) RegisterPrivateAPI(r chi.Router) {
	r.Post("/setups", h.create)
	r.Patch("/setups/{id}", h.patch)
	r.Delete("/setups/{id}", h.delete)
	r.Get("/me/setups", h.list)
	r.Get("/users/{user_id}/setups", h.listUser)
	if h.service.photos != nil {
		r.Post("/setups/{id}/photos", h.upload)
		r.Put("/setups/{id}/photos/{photo_id}/cover", h.coverPhoto)
		r.Put("/setups/{id}/photos/{photo_id}/crop", h.cropPhoto)
		r.Delete("/setups/{id}/photos/{photo_id}", h.deletePhoto)
	}
}

// viewer never manufactures an owner identity for a guest.
func viewer(r *http.Request) string {
	user, _ := auth.CurrentUser(r)
	return user.ID
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
	httpjson.Write(w, 200, setups)
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
	httpjson.Error(w, status, message)
}
func owner(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		httpjson.Error(w, 401, "authentication required")
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
	httpjson.Write(w, http.StatusCreated, setup)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := viewer(r)
	setup, err := h.service.Get(r.Context(), id, chi.URLParam(r, "id"))
	if err != nil {
		failure(w, err)
		return
	}
	if setup.Visibility == Public {
		w.Header().Set("X-Robots-Tag", "index, follow")
	}
	httpjson.Write(w, 200, setup)
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
	httpjson.Write(w, 200, setup)
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
	httpjson.Write(w, 200, result)
}
