package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
)

func TestComparisonContractBoundaries(t *testing.T) {
	document, err := libopenapi.NewDocument(api.OpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Release()
	check, errs := validator.NewValidator(document, config.WithFormatAssertions())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	defer check.Release()
	for _, query := range []string{"", "left_id=bad&right_id=bad", "left_id=11111111-1111-4111-8111-111111111111"} {
		request := httptest.NewRequest("GET", "/api/v1/setups/compare?"+query, nil)
		if valid, _ := check.ValidateHttpRequest(request); valid {
			t.Fatal("invalid comparison request accepted by contract", query)
		}
	}
	request := httptest.NewRequest("GET", "/api/v1/setups/compare?left_id=11111111-1111-4111-8111-111111111111&right_id=22222222-2222-4222-8222-222222222222", nil)
	for _, body := range []string{`{}`, `{"left":null,"right":null}`, `{"left":{},"right":{}}`, `{"items":[]}`} {
		response := httptest.NewRecorder()
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(200)
		response.WriteString(body)
		if valid, _ := check.ValidateHttpResponse(request, response.Result()); valid {
			t.Fatal("incomplete comparison response accepted", body)
		}
	}
	for _, status := range []int{400, 404, 500} {
		response := httptest.NewRecorder()
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(status)
		response.WriteString(`{"error":"setup not found"}`)
		if valid, errs := check.ValidateHttpResponse(request, response.Result()); !valid || len(errs) > 0 {
			t.Fatalf("error response outside contract %d: %+v", status, errs)
		}
	}
}
