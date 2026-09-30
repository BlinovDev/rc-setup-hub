package admin

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/gorilla/csrf"
)

func TestAdminCSRF(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "local HTTP"
		origin := "http://localhost:8080"
		if secure {
			name = "production HTTPS"
			origin = "https://example.com"
		}
		t.Run(name, func(t *testing.T) {
			c := settings()
			c.CookieSecure = secure
			c.GoogleRedirectURL = origin + "/auth/google/callback"
			h, err := New(c)
			if err != nil {
				t.Fatal(err)
			}
			form := template.Must(template.New("form").Parse(`<form method="post">{{.CSRFField}}</form>`))
			protected := h.CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if err := form.Execute(w, PageData{CSRFField: csrf.TemplateField(r)}); err != nil {
						t.Error(err)
					}
					return
				}
				w.WriteHeader(204)
			}))
			get := httptest.NewRecorder()
			protected.ServeHTTP(get, httptest.NewRequest("GET", origin+"/admin", nil))
			if get.Code != 200 {
				t.Fatal("GET requires mutation token", get.Code)
			}
			match := regexp.MustCompile(`value="([^"]+)"`).FindStringSubmatch(get.Body.String())
			if len(match) != 2 || match[1] == "" {
				t.Fatal("CSRF form helper missing", get.Body)
			}
			cookie := cookieNamed(t, get, "rc_admin_csrf")
			if cookie.Path != "/admin" || !cookie.HttpOnly || cookie.Secure != secure || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatal("CSRF cookie flags", cookie)
			}
			for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
				for _, tc := range []struct {
					name, token string
					status      int
				}{
					{"valid", match[1], 204}, {"missing", "", 403}, {"invalid", "invalid", 403},
				} {
					req := httptest.NewRequest(method, origin+"/admin/test-mutation", nil)
					req.AddCookie(cookie)
					req.Header.Set("Origin", origin)
					req.Header.Set("X-CSRF-Token", tc.token)
					w := httptest.NewRecorder()
					protected.ServeHTTP(w, req)
					if w.Code != tc.status {
						t.Fatalf("%s %s: %d %s", method, tc.name, w.Code, w.Body)
					}
				}
			}
			formRequest := httptest.NewRequest("POST", origin+"/admin/test-mutation", strings.NewReader(url.Values{"gorilla.csrf.Token": {match[1]}}.Encode()))
			formRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			formRequest.Header.Set("Origin", origin)
			formRequest.AddCookie(cookie)
			w := httptest.NewRecorder()
			protected.ServeHTTP(w, formRequest)
			if w.Code != 204 {
				t.Fatal("hidden form token rejected", w.Code, w.Body)
			}
			req := httptest.NewRequest("POST", origin+"/admin/test-mutation", nil)
			req.AddCookie(cookie)
			req.Header.Set("Origin", "https://evil.example")
			req.Header.Set("X-CSRF-Token", match[1])
			w = httptest.NewRecorder()
			protected.ServeHTTP(w, req)
			if w.Code != 403 {
				t.Fatal("foreign origin accepted", w.Code)
			}
			req = httptest.NewRequest("POST", origin+"/admin/test-mutation", nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("X-CSRF-Token", match[1])
			w = httptest.NewRecorder()
			protected.ServeHTTP(w, req)
			if w.Code != 403 {
				t.Fatal("token without matching cookie accepted", w.Code)
			}
		})
	}
}
