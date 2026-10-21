package api_test

import (
	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSteeringKnuckleRequestContract(t *testing.T) {
	doc, err := libopenapi.NewDocument(api.OpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Release()
	validate, errs := validator.NewValidator(doc, config.WithFormatAssertions())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	defer validate.Release()
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{`{}`, true}, {`{"front_steering_knuckle":null}`, true}, {`{"front_steering_knuckle":{}}`, true},
		{`{"front_steering_knuckle":{"manufacturer_id":null,"description":null}}`, true},
		{`{"front_steering_knuckle":{"manufacturer_id":"abcdefab-cdef-4abc-8def-abcdefabcdef"}}`, true},
		{`{"front_steering_knuckle":{"description":"` + strings.Repeat("界", 200) + `"}}`, true},
		{`{"front_steering_knuckle":{"description":"Part #42\nAluminum"}}`, true},
		{`{"front_steering_knuckle":{"manufacturer_id":"bad"}}`, false},
		{`{"front_steering_knuckle":{"manufacturer_id":42}}`, false},
		{`{"front_steering_knuckle":{"description":42}}`, false},
		{`{"front_steering_knuckle":{"description":"` + strings.Repeat("界", 201) + `"}}`, false},
		{`{"front_steering_knuckle":{"description":"bad\u0000text"}}`, false},
		{`{"front_steering_knuckle":{"unknown":true}}`, false},
		{`{"rear_steering_knuckle":{}}`, false}, {`{"front_steering_knuckle":[]}`, false},
	} {
		for _, method := range []string{"POST", "PATCH"} {
			path, body := "/api/v1/setups", `{"title":"Knuckle","visibility":"private","data":`+tc.raw+`}`
			if method == "PATCH" {
				path += "/11111111-1111-4111-8111-111111111111"
				body = `{"data":` + tc.raw + `}`
			}
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract"})
			if valid, errs := validate.ValidateHttpRequest(req); valid != tc.valid {
				t.Fatalf("%s %s accepted=%v want=%v: %+v", method, tc.raw, valid, tc.valid, errs)
			}
		}
	}
}
