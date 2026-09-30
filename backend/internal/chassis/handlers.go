package chassis

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"mime"
	"net/http"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	admintemplates "github.com/BlinovDev/rc-setup-hub/backend/templates/admin"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

type Handler struct {
	service   *Service
	templates *template.Template
}

func NewHandler(service *Service) (*Handler, error) {
	tmpl, err := template.ParseFS(admintemplates.Files, "*.html")
	if err != nil {
		return nil, err
	}
	return &Handler{service: service, templates: tmpl}, nil
}

// RegisterAPI is called inside the existing authenticated/CORS API group.
func (h *Handler) RegisterAPI(r chi.Router) {
	r.Get("/chassis/brands", h.activeBrands)
	r.Get("/chassis/brands/{brand_id}/models", h.activeModels)
}

// RegisterAdmin is called inside the existing authenticated/admin/CSRF group.
func (h *Handler) RegisterAdmin(r chi.Router) {
	r.Get("/chassis/brands", func(w http.ResponseWriter, r *http.Request) { h.page(w, r, false, 200, "") })
	r.Get("/chassis/models", func(w http.ResponseWriter, r *http.Request) { h.page(w, r, true, 200, "") })
	r.Post("/chassis/brands", h.createBrand)
	r.Post("/chassis/brands/{id}/update", h.renameBrand)
	r.Post("/chassis/brands/{id}/disable", h.disableBrand)
	r.Post("/chassis/brands/{id}/enable", h.enableBrand)
	r.Post("/chassis/models", h.createModel)
	r.Post("/chassis/models/{id}/update", h.renameModel)
	r.Post("/chassis/models/{id}/disable", h.disableModel)
	r.Post("/chassis/models/{id}/enable", h.enableModel)
}

type selection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func sendJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func errorStatus(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return 404
	case errors.Is(err, ErrDuplicate):
		return 409
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrCustomName), errors.Is(err, ErrInvalidID), errors.Is(err, ErrInactiveBrand):
		return 400
	default:
		return 500
	}
}
func apiError(w http.ResponseWriter, err error) {
	message := err.Error()
	status := errorStatus(err)
	if status == 500 {
		message = "catalog unavailable"
	}
	sendJSON(w, status, struct {
		Error string `json:"error"`
	}{message})
}
func (h *Handler) activeBrands(w http.ResponseWriter, r *http.Request) {
	brands, err := h.service.ActiveBrands(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	result := []selection{}
	for _, brand := range brands {
		result = append(result, selection{brand.ID, brand.Name})
	}
	sendJSON(w, 200, result)
}
func (h *Handler) activeModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.service.ActiveModels(r.Context(), chi.URLParam(r, "brand_id"))
	if err != nil {
		apiError(w, err)
		return
	}
	result := []selection{}
	for _, model := range models {
		result = append(result, selection{model.ID, model.Name})
	}
	sendJSON(w, 200, result)
}

type catalogPage struct {
	User                      users.User
	CSRFField                 template.HTML
	Brands                    []Brand
	Models                    []ModelState
	Error, InputName, BrandID string
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request, models bool, status int, message string) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	data := catalogPage{User: user, CSRFField: csrf.TemplateField(r), Error: message, InputName: r.PostForm.Get("name"), BrandID: r.PostForm.Get("brand_id")}
	var err error
	data.Brands, err = h.service.ListBrands(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	name := "brands.html"
	if models {
		name = "models.html"
		data.Models, err = h.service.ListModels(r.Context())
		if err != nil {
			http.Error(w, "Internal Server Error", 500)
			return
		}
	}
	var body bytes.Buffer
	if err := h.templates.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
func (h *Handler) result(w http.ResponseWriter, r *http.Request, models bool, err error) {
	if err != nil {
		status := errorStatus(err)
		if status == 500 {
			http.Error(w, "Internal Server Error", 500)
			return
		}
		if status == 404 {
			http.Error(w, "Catalog entry not found", 404)
			return
		}
		h.page(w, r, models, status, err.Error())
		return
	}
	path := "/admin/chassis/brands"
	if models {
		path = "/admin/chassis/models"
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" {
		http.Error(w, "Use an HTML form submission", 415)
		return false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", 400)
		return false
	}
	return true
}
func (h *Handler) createBrand(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	_, err := h.service.CreateBrand(r.Context(), r.PostForm.Get("name"))
	h.result(w, r, false, err)
}
func (h *Handler) renameBrand(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	_, err := h.service.RenameBrand(r.Context(), chi.URLParam(r, "id"), r.PostForm.Get("name"))
	h.result(w, r, false, err)
}
func (h *Handler) disableBrand(w http.ResponseWriter, r *http.Request) { h.brandActive(w, r, false) }
func (h *Handler) enableBrand(w http.ResponseWriter, r *http.Request)  { h.brandActive(w, r, true) }
func (h *Handler) brandActive(w http.ResponseWriter, r *http.Request, active bool) {
	if !parseForm(w, r) {
		return
	}
	_, err := h.service.SetBrandActive(r.Context(), chi.URLParam(r, "id"), active)
	h.result(w, r, false, err)
}
func (h *Handler) createModel(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	_, err := h.service.CreateModel(r.Context(), r.PostForm.Get("brand_id"), r.PostForm.Get("name"))
	h.result(w, r, true, err)
}
func (h *Handler) renameModel(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	if _, present := r.PostForm["brand_id"]; present {
		h.page(w, r, true, 400, "A model's brand cannot be changed")
		return
	}
	_, err := h.service.RenameModel(r.Context(), chi.URLParam(r, "id"), r.PostForm.Get("name"))
	h.result(w, r, true, err)
}
func (h *Handler) disableModel(w http.ResponseWriter, r *http.Request) { h.modelActive(w, r, false) }
func (h *Handler) enableModel(w http.ResponseWriter, r *http.Request)  { h.modelActive(w, r, true) }
func (h *Handler) modelActive(w http.ResponseWriter, r *http.Request, active bool) {
	if !parseForm(w, r) {
		return
	}
	_, err := h.service.SetModelActive(r.Context(), chi.URLParam(r, "id"), active)
	h.result(w, r, true, err)
}
