package chassis

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/admin"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
)

type fakeGoogle struct{ identity users.Identity }

func (f *fakeGoogle) AuthorizationURL(state, nonce, verifier string) string {
	return "https://accounts.google.com/auth?state=" + url.QueryEscape(state)
}
func (f *fakeGoogle) Exchange(context.Context, string, string, string) (users.Identity, error) {
	return f.identity, nil
}
func get(router http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "http://localhost:8080"+path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
func findCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}
func login(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	start := get(router, "/auth/google")
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	result := get(router, "/auth/google/callback?code=fake&state="+url.QueryEscape(location.Query().Get("state")), findCookie(t, start, "rc_oauth_state"))
	if result.Code != 303 {
		t.Fatal("login", result.Code, result.Body)
	}
	return findCookie(t, result, "rc_session")
}
func post(router http.Handler, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "http://localhost:8080"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://localhost:8080")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
func selections(t *testing.T, w *httptest.ResponseRecorder) []selection {
	t.Helper()
	if w.Code != 200 {
		t.Fatal("selection", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "is_active") || strings.Contains(w.Body.String(), "created_at") {
		t.Fatal("internal catalog fields exposed", w.Body)
	}
	var result []selection
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestCatalogHTTP(t *testing.T) {
	pool := dbtest.New(t)
	service := NewService(NewRepository(pool))
	ctx := context.Background()
	cfg := config.Auth{GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode, AdminEmails: []string{"admin@example.com"}, AllowedOrigins: []string{"http://localhost:5173"}}
	provider := &fakeGoogle{identity: users.Identity{Subject: "admin", Email: "admin@example.com", Name: "Admin"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	authentication.Register(router, handler.RegisterAPI)
	adminHandler, err := admin.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	adminHandler.Register(router, authentication.RequireUser, handler.RegisterAdmin)
	for _, path := range []string{"/api/v1/chassis/brands", "/api/v1/chassis/brands/00000000-0000-0000-0000-000000000000/models", "/admin/chassis/brands"} {
		if w := get(router, path); w.Code != 401 {
			t.Fatal("unauthenticated catalog", path, w.Code)
		}
	}
	adminCookie := login(t, router)
	formPage := get(router, "/admin/chassis/brands", adminCookie)
	if formPage.Code != 200 {
		t.Fatal(formPage.Code, formPage.Body)
	}
	csrfCookie := findCookie(t, formPage, "rc_admin_csrf")
	match := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(formPage.Body.String())
	if len(match) != 2 {
		t.Fatal("CSRF form field missing", formPage.Body)
	}
	token := match[1]
	send := func(path string, fields url.Values, status int) *httptest.ResponseRecorder {
		t.Helper()
		fields.Set("gorilla.csrf.Token", token)
		w := post(router, path, fields, adminCookie, csrfCookie)
		if w.Code != status {
			t.Fatalf("POST %s: %d want %d: %s", path, w.Code, status, w.Body)
		}
		return w
	}
	if w := post(router, "/admin/chassis/brands", url.Values{"name": {"Blocked"}}, adminCookie, csrfCookie); w.Code != 403 {
		t.Fatal("missing CSRF allowed", w.Code)
	}
	if w := post(router, "/admin/chassis/brands", url.Values{"name": {"Blocked"}, "gorilla.csrf.Token": {"invalid"}}, adminCookie, csrfCookie); w.Code != 403 {
		t.Fatal("invalid CSRF allowed", w.Code)
	}
	send("/admin/chassis/brands", url.Values{"name": {" Yokomo "}}, 303)
	brands, err := service.ListBrands(ctx)
	if err != nil || len(brands) != 1 || brands[0].Name != "Yokomo" {
		t.Fatal(brands, err)
	}
	brand := brands[0]
	duplicate := send("/admin/chassis/brands", url.Values{"name": {"YOKOMO"}}, 409)
	if !strings.Contains(duplicate.Body.String(), "already in use") || strings.Contains(duplicate.Body.String(), "SQLSTATE") || strings.Contains(duplicate.Body.String(), "chassis_brands_name_unique") {
		t.Fatal("raw duplicate error", duplicate.Body)
	}
	send("/admin/chassis/brands", url.Values{"name": {" "}}, 400)
	send("/admin/chassis/models", url.Values{"brand_id": {brand.ID}, "name": {" RD2.0 "}}, 303)
	models, err := service.ListModels(ctx)
	if err != nil || len(models) != 1 || models[0].Name != "RD2.0" {
		t.Fatal(models, err)
	}
	model := models[0]
	duplicate = send("/admin/chassis/models", url.Values{"brand_id": {brand.ID}, "name": {"rd2.0"}}, 409)
	if !strings.Contains(duplicate.Body.String(), "already in use") || strings.Contains(duplicate.Body.String(), "SQLSTATE") {
		t.Fatal("model duplicate error", duplicate.Body)
	}
	send("/admin/chassis/brands/"+brand.ID+"/update", url.Values{"name": {" Yokomo Racing "}}, 303)
	send("/admin/chassis/models/"+model.ID+"/update", url.Values{"name": {" RD2.0 Limited "}}, 303)
	send("/admin/chassis/models/"+model.ID+"/update", url.Values{"name": {"Moved"}, "brand_id": {brand.ID}}, 400)
	provider.identity = users.Identity{Subject: "ordinary", Email: "ordinary@example.com", Name: "Ordinary"}
	ordinaryCookie := login(t, router)
	for _, path := range []string{"/admin/chassis/brands", "/admin/chassis/models", "/admin/chassis/brands/" + brand.ID + "/update", "/admin/chassis/brands/" + brand.ID + "/disable", "/admin/chassis/brands/" + brand.ID + "/enable", "/admin/chassis/models/" + model.ID + "/update", "/admin/chassis/models/" + model.ID + "/disable", "/admin/chassis/models/" + model.ID + "/enable"} {
		if w := post(router, path, url.Values{"name": {"Not permitted"}, "gorilla.csrf.Token": {token}}, ordinaryCookie, csrfCookie); w.Code != 403 {
			t.Fatal("ordinary mutation allowed", path, w.Code)
		}
	}
	activeBrands := selections(t, get(router, "/api/v1/chassis/brands", ordinaryCookie))
	if len(activeBrands) != 1 || activeBrands[0].Name != "Yokomo Racing" {
		t.Fatal(activeBrands)
	}
	path := "/api/v1/chassis/brands/" + brand.ID + "/models"
	activeModels := selections(t, get(router, path, ordinaryCookie))
	if len(activeModels) != 1 || activeModels[0].Name != "RD2.0 Limited" {
		t.Fatal(activeModels)
	}
	send("/admin/chassis/models/"+model.ID+"/disable", url.Values{}, 303)
	if w := get(router, path, ordinaryCookie); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("inactive model leaked", w.Code, w.Body)
	}
	send("/admin/chassis/models/"+model.ID+"/enable", url.Values{}, 303)
	send("/admin/chassis/brands/"+brand.ID+"/disable", url.Values{}, 303)
	if list := selections(t, get(router, "/api/v1/chassis/brands", ordinaryCookie)); len(list) != 0 {
		t.Fatal("inactive brand leaked", list)
	}
	if w := get(router, path, ordinaryCookie); w.Code != 404 {
		t.Fatal("inactive parent API", w.Code, w.Body)
	}
	state, err := service.GetModel(ctx, model.ID)
	if err != nil || !state.IsActive || state.BrandActive {
		t.Fatal("model cascade-disabled", state, err)
	}
	send("/admin/chassis/models", url.Values{"brand_id": {brand.ID}, "name": {"Blocked"}}, 400)
	send("/admin/chassis/brands/"+brand.ID+"/enable", url.Values{}, 303)
	if list := selections(t, get(router, path, ordinaryCookie)); len(list) != 1 {
		t.Fatal("reenable model selection", list)
	}
	missing := "00000000-0000-0000-0000-000000000000"
	for _, path := range []string{"/admin/chassis/brands/" + missing + "/disable", "/admin/chassis/models/" + missing + "/update"} {
		send(path, url.Values{"name": {"Missing"}}, 404)
	}
	if w := get(router, "/api/v1/chassis/brands/"+missing+"/models", ordinaryCookie); w.Code != 404 {
		t.Fatal("unknown brand API", w.Code)
	}
	// An authenticated cross-origin client retains the exact existing CORS policy.
	req := httptest.NewRequest("GET", "/api/v1/chassis/brands", nil)
	req.AddCookie(ordinaryCookie)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal("catalog CORS", w.Header())
	}
	pool.Close()
	w = get(router, "/api/v1/chassis/brands", ordinaryCookie)
	if w.Code != 500 || strings.Contains(w.Body.String(), "closed pool") {
		t.Fatal("DB failure details leaked", w.Code, w.Body)
	}
}
