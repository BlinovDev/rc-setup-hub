package setups

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/httpjson"
)

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	h.searchPage(w, r, "", false)
}
func (h *Handler) publicUserSetups(w http.ResponseWriter, r *http.Request) {
	h.searchPage(w, r, chi.URLParam(r, "user_id"), false)
}
func (h *Handler) visibleSearch(w http.ResponseWriter, r *http.Request) {
	h.searchPage(w, r, "", true)
}
func (h *Handler) searchPage(w http.ResponseWriter, r *http.Request, ownerID string, visible bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, invalid("invalid search query parameters"))
		return
	}
	for _, field := range []string{"q", "brand_id", "model_id", "limit", "cursor", "scope"} {
		if len(values[field]) > 1 {
			failure(w, invalid("duplicate "+field+" parameter"))
			return
		}
	}
	optional := func(field string) *string {
		if !values.Has(field) {
			return nil
		}
		value := values.Get(field)
		return &value
	}
	input := SearchInput{Q: values.Get("q"), Scope: optional("scope"), BrandID: optional("brand_id"), ModelID: optional("model_id"), Limit: optional("limit"), Cursor: optional("cursor")}
	var page SearchPage
	if visible {
		page, err = h.service.SearchVisible(r.Context(), viewer(r), input)
	} else if ownerID == "" {
		page, err = h.service.Search(r.Context(), input)
	} else {
		page, err = h.service.PublicUserSetups(r.Context(), ownerID, input)
	}
	if err != nil {
		failure(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, page)
}
