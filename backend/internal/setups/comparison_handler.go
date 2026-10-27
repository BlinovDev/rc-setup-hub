package setups

import (
	"net/http"
	"net/url"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/httpjson"
)

func (h *Handler) compare(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, invalid("invalid comparison query parameters"))
		return
	}
	if len(values) != 2 || len(values["left_id"]) != 1 || len(values["right_id"]) != 1 {
		failure(w, invalid("comparison requires exactly left_id and right_id"))
		return
	}
	result, err := h.service.Compare(r.Context(), viewer(r), values.Get("left_id"), values.Get("right_id"))
	if err != nil {
		failure(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}
