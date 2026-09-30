package setups

import (
	"net/http"
	"net/url"
)

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	if _, ok := owner(w, r); !ok {
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, invalid("invalid search query parameters"))
		return
	}
	for _, field := range []string{"q", "brand_id", "model_id", "limit", "cursor"} {
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
	page, err := h.service.Search(r.Context(), SearchInput{Q: values.Get("q"), BrandID: optional("brand_id"), ModelID: optional("model_id"), Limit: optional("limit"), Cursor: optional("cursor")})
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, http.StatusOK, page)
}
