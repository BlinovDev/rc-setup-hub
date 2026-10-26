package users

import (
	"errors"
	"net/http"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/httpjson"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterAPI receives the existing authenticated-user accessor to avoid an auth/users import cycle.
func (h *Handler) RegisterAPI(r chi.Router, currentUser func(*http.Request) (User, bool)) {
	r.Get("/users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(r); !ok {
			httpjson.Error(w, 401, "authentication required")
			return
		}
		profile, err := h.service.GetPublic(r.Context(), chi.URLParam(r, "user_id"))
		if err != nil {
			code, message := 500, "user profile unavailable"
			switch {
			case errors.Is(err, ErrInvalidUserID):
				code, message = 400, ErrInvalidUserID.Error()
			case errors.Is(err, ErrNotFound):
				code, message = 404, ErrNotFound.Error()
			}
			httpjson.Error(w, code, message)
			return
		}
		httpjson.Write(w, http.StatusOK, profile)
	})
	r.Get("/users/search", func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			httpjson.Error(w, 401, "authentication required")
			return
		}
		profiles, err := h.service.Search(r.Context(), user.ID, r.URL.Query().Get("q"))
		if err != nil {
			code, message := 500, "user search failed"
			if errors.Is(err, ErrInvalidSearchQuery) {
				code, message = 400, ErrInvalidSearchQuery.Error()
			}
			httpjson.Error(w, code, message)
			return
		}
		httpjson.Write(w, http.StatusOK, profiles)
	})
}
