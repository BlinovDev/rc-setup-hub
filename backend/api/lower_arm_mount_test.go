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

func TestLowerArmMountRequestContract(t *testing.T) {
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
			{`null`, true}, {`{}`, true}, {`{"manufacturer_id":null,"model":null,"configuration_notes":null}`, true},
			{`{"manufacturer_id":"abcdefab-cdef-4abc-8def-abcdefabcdef"}`, true}, {`{"model":"Part"}`, true}, {`{"configuration_notes":"Notes\nTwo inserts"}`, true},
			{`{"model":"` + strings.Repeat("界", 200) + `","configuration_notes":"` + strings.Repeat("界", 10000) + `"}`, true},
			{`{"manufacturer_id":"bad"}`, false}, {`{"manufacturer_id":1}`, false}, {`{"model":false}`, false}, {`{"configuration_notes":{}}`, false},
			{`{"model":"` + strings.Repeat("界", 201) + `"}`, false}, {`{"configuration_notes":"` + strings.Repeat("界", 10001) + `"}`, false},
			{`{"model":"bad\u0000text"}`, false}, {`{"configuration_notes":"bad\u0000text"}`, false},
			{`{"inserts":[]}`, false}, {`{"mounts":[]}`, false}, {`{"geometry":{}}`, false}, {`[]`, false},
		} {
			for _, method := range []string{"POST", "PATCH"} {
				data := fmt.Sprintf(`{"suspension":{"%s":{"lower_arm_mount":%s}}}`, side, tc.block)
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
		path, body := "/api/v1/setups", `{"title":"Mount","visibility":"private","data":{},"lower_arm_mount_manufacturers":null}`
		if method == "PATCH" {
			path += "/11111111-1111-4111-8111-111111111111"
			body = `{"lower_arm_mount_manufacturers":null}`
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract"})
		if valid, _ := validate.ValidateHttpRequest(req); valid {
			t.Fatal("read-only projection accepted", method)
		}
	}
}
