package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
)

func TestLinkLengthsRequestContract(t *testing.T) {
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
	for _, axle := range []string{"front", "rear"} {
		for _, tc := range []struct {
			link  string
			valid bool
		}{
			{`{"name":"upper_arm","length_mm":42.123456789}`, true},
			{`{"name":"lower_arm"}`, true}, {`{"name":"custom unknown","length_mm":null}`, true},
			{`{"name":"steering_tie_rod","length_mm":16.2}`, true},
			{`{"name":"Legacy label 保留","length_mm":0.1}`, true},
			{`{"name":"upper_arm","length_mm":0}`, false}, {`{"name":"lower_arm","length_mm":-1}`, false},
			{`{"name":"x","length_mm":"16.2"}`, false}, {`{"length_mm":42}`, false},
			{`{"name":"","length_mm":42}`, false}, {`{"name":"x","length_mm":42,"kind":"arm"}`, false},
		} {
			for _, method := range []string{"POST", "PATCH"} {
				path := "/api/v1/setups"
				if method == "PATCH" {
					path += "/11111111-1111-4111-8111-111111111111"
				}
				data := `"data":{"suspension":{"` + axle + `":{"camber_deg":0,"link_lengths":[` + tc.link + `]}}}`
				body := "{" + data + "}"
				if method == "POST" {
					body = `{"title":"Links","visibility":"private",` + data + `}`
				}
				request := httptest.NewRequest(method, path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract"})
				valid, errs := check.ValidateHttpRequest(request)
				if valid != tc.valid {
					t.Fatalf("%s %s %s: valid=%v errors=%+v", method, axle, tc.link, valid, errs)
				}
			}
		}
	}
}

func TestFixedStandardLengthRequests(t *testing.T) {
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
	for _, axle := range []string{"front", "rear"} {
		for _, field := range []string{"upper_arm_length_mm", "lower_arm_length_mm", "steering_tie_rod_length_mm"} {
			for _, value := range []string{"null", "16.2123456789", "0", "-1", `"16.2"`} {
				validExpected := value == "null" || (value == "16.2123456789" && (axle == "front" || field != "steering_tie_rod_length_mm"))
				request := httptest.NewRequest("PATCH", "/api/v1/setups/11111111-1111-4111-8111-111111111111", strings.NewReader(`{"data":{"suspension":{"`+axle+`":{"`+field+`":`+value+`}}}}`))
				request.Header.Set("Content-Type", "application/json")
				request.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract"})
				if valid, errs := check.ValidateHttpRequest(request); valid != validExpected {
					t.Fatalf("%s %s %s: %v %+v", axle, field, value, valid, errs)
				}
			}
		}
	}
}
