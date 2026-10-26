package httpjson

import (
	"net/http/httptest"
	"testing"
)

func TestWrite(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		value  any
		want   string
	}{
		{"object", 200, struct {
			Name string `json:"name"`
		}{"<private>"}, "{\"name\":\"\\u003cprivate\\u003e\"}\n"},
		{"empty list", 200, []string{}, "[]\n"},
		{"null", 200, nil, "null\n"},
		{"created", 201, map[string]int{"revision": 0}, "{\"revision\":0}\n"},
		{"no content", 204, "must not be encoded", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			w.Header().Set("Location", "/api/v1/setups/id")
			w.Header().Set("Cache-Control", "public")
			Write(w, tc.status, tc.value)
			if w.Code != tc.status || w.Body.String() != tc.want {
				t.Fatalf("got %d %q, want %d %q", w.Code, w.Body.String(), tc.status, tc.want)
			}
			if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Location") != "/api/v1/setups/id" {
				t.Fatalf("response headers changed: %v", w.Header())
			}
		})
	}
}

func TestError(t *testing.T) {
	for _, status := range []int{400, 401, 404, 409, 500, 503} {
		w := httptest.NewRecorder()
		Error(w, status, "safe message")
		if w.Code != status || w.Body.String() != "{\"error\":\"safe message\"}\n" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("error response changed: %d %q %v", w.Code, w.Body.String(), w.Header())
		}
	}
}
