package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
)

func TestShockHardwareRequestContract(t *testing.T) {
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
	cases := []struct {
		shock string
		valid bool
	}{
		{`{}`, true},
		{`{"piston":null,"seal":null}`, true},
		{`{"piston":{},"seal":{}}`, true},
		{`{"piston":{"holes":4}}`, true},
		{`{"piston":{"holes":2147483647}}`, true},
		{`{"piston":{"hole_diameter_mm":0.123456789}}`, true},
		{`{"piston":{"notes":"Drilled piston\nThree holes"}}`, true},
		{`{"seal":{"type":"o-ring"}}`, true},
		{`{"seal":{"type":"x-ring"}}`, true},
		{`{"seal":{"description":"Specific product"}}`, true},
		{`{"piston":{"holes":null,"hole_diameter_mm":null,"notes":null},"seal":{"type":null,"description":null}}`, true},
		{`{"piston":{"notes":"` + strings.Repeat("界", 10000) + `"},"seal":{"description":"` + strings.Repeat("界", 200) + `"}}`, true},
		{`{"piston":{"holes":0}}`, false},
		{`{"piston":{"holes":-1}}`, false},
		{`{"piston":{"holes":1.5}}`, false},
		{`{"piston":{"holes":"4"}}`, false},
		{`{"piston":{"holes":true}}`, false},
		{`{"piston":{"holes":2147483648}}`, false},
		{`{"piston":{"hole_diameter_mm":0}}`, false},
		{`{"piston":{"hole_diameter_mm":-1}}`, false},
		{`{"piston":{"hole_diameter_mm":"1.2"}}`, false},
		{`{"piston":{"notes":"bad\u0000text"}}`, false},
		{`{"piston":{"notes":"` + strings.Repeat("界", 10001) + `"}}`, false},
		{`{"seal":{"type":"other"}}`, false},
		{`{"seal":{"type":""}}`, false},
		{`{"seal":{"type":true}}`, false},
		{`{"seal":{"description":"bad\u0000text"}}`, false},
		{`{"seal":{"description":"` + strings.Repeat("界", 201) + `"}}`, false},
		{`{"piston":{"unknown":1}}`, false},
		{`{"seal":{"unknown":1}}`, false},
		{`{"piston":[]}`, false},
		{`{"seal":[]}`, false},
	}
	for _, axle := range []string{"front", "rear"} {
		for _, method := range []string{"POST", "PATCH"} {
			for i, tc := range cases {
				t.Run(fmt.Sprintf("%s/%s/%d", axle, method, i), func(t *testing.T) {
					path, body := "/api/v1/setups", `{"data":{"shocks":{"`+axle+`":`+tc.shock+`}}}`
					if method == "PATCH" {
						path += "/" + id
					} else {
						body = `{"title":"Shock hardware","visibility":"private","data":{"shocks":{"` + axle + `":` + tc.shock + `}}}`
					}
					request := httptest.NewRequest(method, path, strings.NewReader(body))
					request.Header.Set("Content-Type", "application/json")
					request.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract-test-cookie"})
					valid, errs := validate.ValidateHttpRequest(request)
					if valid != tc.valid {
						t.Fatalf("accepted=%v want=%v: %+v", valid, tc.valid, errs)
					}
				})
			}
		}
	}
}
