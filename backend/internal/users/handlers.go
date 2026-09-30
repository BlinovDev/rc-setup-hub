package users

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterAPI receives the existing authenticated-user accessor to avoid an auth/users import cycle.
func (h *Handler) RegisterAPI(r chi.Router, currentUser func(*http.Request) (User, bool)) {
	r.Get("/users/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		user, ok := currentUser(r)
		if !ok {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(struct {
				Error string `json:"error"`
			}{"authentication required"})
			return
		}
		profiles, err := h.service.Search(r.Context(), user.ID, r.URL.Query().Get("q"))
		if err != nil {
			code, message := 500, "user search failed"
			if errors.Is(err, ErrInvalidSearchQuery) {
				code, message = 400, ErrInvalidSearchQuery.Error()
			}
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(struct {
				Error string `json:"error"`
			}{message})
			return
		}
		_ = json.NewEncoder(w).Encode(profiles)
	})
}
