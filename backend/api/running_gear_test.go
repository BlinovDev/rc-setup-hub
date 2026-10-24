package api_test

import (
	"fmt"
	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunningGearRequestContract(t *testing.T) {
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
	for _, side := range []string{"front", "rear"} {
		for _, tc := range []struct {
			block string
			valid bool
		}{
			{`null`, true}, {`{}`, true}, {`{"hex":{"offset_mm":0},"wheel":{"offset_mm":-4},"tire":{"model":"Tire"}}`, true},
			{`{"wheel":{"offset_mm":-2147483648}}`, true}, {`{"wheel":{"offset_mm":2147483647}}`, true},
			{`{"hex":{"offset_mm":-2.123456789}}`, true},
			{`{"wheel":{"offset_mm":1.5}}`, false}, {`{"wheel":{"offset_mm":2147483648}}`, false},
			{`{"hex":{"width_mm":1}}`, false}, {`{"wheel":{"manufacturer_id":"bad"}}`, false},
			{`{"tire":{"manufacturer_id":"abcdefab-cdef-4abc-8def-abcdefabcdef"}}`, true},
			{`{"tire":{"model":"` + strings.Repeat("界", 200) + `"}}`, true},
			{`{"tire":{"model":"` + strings.Repeat("界", 201) + `"}}`, false},
			{`{"wheel":{"model":"bad\u0000text"}}`, false},
			{`{"hex":null,"wheel":null,"tire":null}`, true},
		} {
			for _, method := range []string{"POST", "PATCH"} {
				data := fmt.Sprintf(`{"running_gear":{"%s":%s}}`, side, tc.block)
				path, body := "/api/v1/setups", `{"title":"Mount","visibility":"private","data":`+data+`}`
				if method == "PATCH" {
					path += "/11111111-1111-4111-8111-111111111111"
					body = `{"data":` + data + `}`
				}
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract"})
				if valid, errs := validate.ValidateHttpRequest(req); valid != tc.valid {
					t.Fatalf("%s %s %s valid=%v want=%v: %+v", method, side, tc.block[:min(len(tc.block), 70)], valid, tc.valid, errs)
				}
			}
		}
	}
	for _, method := range []string{"POST", "PATCH"} {
		path, body := "/api/v1/setups", `{"title":"Mount","visibility":"private","data":{},"running_gear_manufacturers":null}`
		if method == "PATCH" {
			path += "/11111111-1111-4111-8111-111111111111"
			body = `{"running_gear_manufacturers":null}`
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract"})
		if valid, _ := validate.ValidateHttpRequest(req); valid {
			t.Fatal("read-only projection accepted", method)
		}
	}
}
