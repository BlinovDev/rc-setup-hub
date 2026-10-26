package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
)

func TestPublicReadTransportContract(t *testing.T) {
	document, err := libopenapi.NewDocument(api.OpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Release()
	validate, errs := validator.NewValidator(document, config.WithFormatAssertions())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	defer validate.Release()
	id := "11111111-1111-4111-8111-111111111111"
	for _, path := range []string{"/api/v1/setups/search?limit=1", "/api/v1/users/" + id, "/api/v1/users/" + id + "/public-setups?limit=50", "/api/v1/setups/" + id, "/api/v1/setups/" + id + "/photos", "/api/v1/setups/" + id + "/photos/" + id + "/image", "/api/v1/setups/" + id + "/photos/" + id + "/thumbnail", "/api/v1/chassis/brands", "/api/v1/chassis/brands/" + id + "/models"} {
		req := httptest.NewRequest("GET", path, nil) // Deliberately no cookie.
		if valid, errs := validate.ValidateHttpRequest(req); !valid || len(errs) > 0 {
			t.Fatalf("guest request %s: %+v", path, errs)
		}
		for _, status := range []int{414, 429} {
			w := httptest.NewRecorder()
			w.Header().Set("Content-Type", "application/json")
			if status == 429 {
				w.Header().Set("Retry-After", "1")
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "safe message"})
			if valid, errs := validate.ValidateHttpResponse(req, w.Result()); !valid || len(errs) > 0 {
				t.Fatalf("overload response %s %d: %+v", path, status, errs)
			}
		}
	}
	// Authentication remains the default, not an empty top-level security policy.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(401)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
	if valid, errs := validate.ValidateHttpResponse(req, w.Result()); !valid || len(errs) > 0 {
		t.Fatalf("guest me 401: %+v", errs)
	}
}
