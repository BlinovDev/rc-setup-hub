package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
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
func request(router http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
func cookieNamed(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing %s", name)
	return nil
}
func login(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	start := request(router, "GET", "/auth/google", "", nil)
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := request(router, "GET", "/auth/google/callback?code=fake&state="+url.QueryEscape(location.Query().Get("state")), "", cookieNamed(t, start, "rc_oauth_state"))
	if callback.Code != 303 {
		t.Fatal("callback", callback.Code, callback.Body)
	}
	return cookieNamed(t, callback, "rc_session")
}
func setupResponse(t *testing.T, w *httptest.ResponseRecorder, status int) Setup {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP %d want %d: %s", w.Code, status, w.Body)
	}
	if strings.Contains(w.Body.String(), "owner_id") {
		t.Fatal("owner ID exposed", w.Body)
	}
	var setup Setup
	if err := json.Unmarshal(w.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	return setup
}
func TestSetupHTTP(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(NewRepository(pool), catalog, friendships.NewRepository(pool))
	cfg := config.Auth{GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
	provider := &fakeGoogle{identity: users.Identity{Subject: "owner", Email: "owner@example.com", Name: "Owner"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	unknown := "00000000-0000-0000-0000-000000000000"
	for _, tc := range []struct{ method, path string }{{"POST", "/api/v1/setups"}, {"GET", "/api/v1/setups/" + unknown}, {"PATCH", "/api/v1/setups/" + unknown}, {"DELETE", "/api/v1/setups/" + unknown}, {"GET", "/api/v1/me/setups"}} {
		if w := request(router, tc.method, tc.path, `{}`, nil); w.Code != 401 {
			t.Fatal("unauthenticated", tc, w.Code)
		}
	}
	ownerCookie := login(t, router)
	me := request(router, "GET", "/api/v1/me", "", ownerCookie)
	var owner users.User
	if err := json.Unmarshal(me.Body.Bytes(), &owner); err != nil {
		t.Fatal(err)
	}
	body := `{"title":" Private setup ","chassis_model_id":null,"visibility":"private","notes":"Original notes","data":` + realisticJSON + `}`
	createdResponse := request(router, "POST", "/api/v1/setups", body, ownerCookie)
	created := setupResponse(t, createdResponse, 201)
	if created.SchemaVersion != 1 || created.ChassisModelID != nil || created.Title != "Private setup" || created.Visibility != Private || created.Data.Suspension.Rear.ToeDeg == nil || *created.Data.Suspension.Rear.ToeDeg != 0 {
		t.Fatal("create fields", created)
	}
	var storedOwner string
	if err := pool.QueryRow(ctx, "SELECT owner_id::text FROM setups WHERE id=$1", created.ID).Scan(&storedOwner); err != nil || storedOwner != owner.ID {
		t.Fatal("owner not from session", storedOwner, err)
	}
	path := "/api/v1/setups/" + created.ID
	if createdResponse.Header().Get("Location") != path {
		t.Fatal("creation location")
	}
	setupResponse(t, request(router, "GET", path, "", ownerCookie), 200)
	for _, invalidBody := range []string{
		`{"title":"x","visibility":"private","data":{},"owner_id":"forged"}`,
		`{"title":"x","visibility":"private","data":{},"schema_version":2}`,
		`{"title":"x","visibility":"invalid","data":{}}`,
		`{"title":" ","visibility":"private","data":{}}`,
		`{"title":"x","visibility":"private"}`,
		`{"title":"x","visibility":"private","data":null}`,
		`{"title":"x","visibility":"private","data":{"electronics":{"unknown":true}}}`,
		`{"title":"x","visibility":"private","data":{"shocks":{"front":{"oil_cst":0}}}}`,
		`{"title":"x","visibility":"private","data":{"suspension":{"front":{"link_lengths":[{"name":"","length_mm":1}]}}}}`,
		`null`, `[]`, `{`, `{} {}`,
	} {
		if w := request(router, "POST", "/api/v1/setups", invalidBody, ownerCookie); w.Code != 400 {
			t.Fatal("invalid create", invalidBody, w.Code, w.Body)
		}
	}
	for _, badPatch := range []string{`{"schema_version":2}`, `{"owner_id":"forged"}`, `{"title":null}`, `{"visibility":null}`, `{"data":null}`, `{"data":{"electronics":{"unknown":true}}}`, `null`, `{} {}`} {
		if w := request(router, "PATCH", path, badPatch, ownerCookie); w.Code != 400 {
			t.Fatal("invalid patch", badPatch, w.Code, w.Body)
		}
	}
	patched := setupResponse(t, request(router, "PATCH", path, `{"title":" Patched ","visibility":"public"}`, ownerCookie), 200)
	if patched.Title != "Patched" || patched.Visibility != Public || patched.Notes == nil || *patched.Notes != "Original notes" || patched.Data.Electronics == nil || !patched.UpdatedAt.After(created.UpdatedAt) {
		t.Fatal("PATCH erased omitted fields", patched)
	}
	patched = setupResponse(t, request(router, "PATCH", path, `{"notes":null,"data":{"electronics":{"motor":" Replacement motor "}}}`, ownerCookie), 200)
	if patched.Notes != nil || patched.Data.Suspension != nil || patched.Data.Electronics.Motor != "Replacement motor" || patched.SchemaVersion != 1 {
		t.Fatal("replace/clear", patched)
	}
	brand, err := catalog.CreateBrand(ctx, "Yokomo")
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, brand.ID, "RD2.0")
	if err != nil {
		t.Fatal(err)
	}
	validBody := `{"title":"RD2.0","visibility":"private","chassis_model_id":"` + model.ID + `","data":{}}`
	historical := setupResponse(t, request(router, "POST", "/api/v1/setups", validBody, ownerCookie), 201)
	if historical.ChassisModelID == nil || *historical.ChassisModelID != model.ID {
		t.Fatal("active chassis not selected")
	}
	invalidBody := strings.ReplaceAll(validBody, model.ID, unknown)
	if w := request(router, "POST", "/api/v1/setups", invalidBody, ownerCookie); w.Code != 400 {
		t.Fatal("unknown model create", w.Code, w.Body)
	}
	if _, err := catalog.SetModelActive(ctx, model.ID, false); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "POST", "/api/v1/setups", validBody, ownerCookie); w.Code != 400 {
		t.Fatal("inactive model create", w.Code)
	}
	historyPath := "/api/v1/setups/" + historical.ID
	setupResponse(t, request(router, "GET", historyPath, "", ownerCookie), 200)
	history := setupResponse(t, request(router, "PATCH", historyPath, `{"title":"History","notes":"Kept","data":{}}`, ownerCookie), 200)
	if history.ChassisModelID == nil || *history.ChassisModelID != model.ID {
		t.Fatal("inactive reference erased")
	}
	if w := request(router, "PATCH", path, `{"chassis_model_id":"`+model.ID+`"}`, ownerCookie); w.Code != 400 {
		t.Fatal("new inactive selection allowed", w.Code)
	}
	history = setupResponse(t, request(router, "PATCH", historyPath, `{"chassis_model_id":null}`, ownerCookie), 200)
	if history.ChassisModelID != nil {
		t.Fatal("null did not clear chassis")
	}
	if _, err := catalog.SetModelActive(ctx, model.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.SetBrandActive(ctx, brand.ID, false); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "POST", "/api/v1/setups", validBody, ownerCookie); w.Code != 400 {
		t.Fatal("inactive brand selection allowed", w.Code)
	}
	provider.identity = users.Identity{Subject: "other", Email: "other@example.com", Name: "Other"}
	otherCookie := login(t, router)
	foreign := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Foreign","visibility":"public","data":{}}`, otherCookie), 201)
	for _, method := range []string{"PATCH", "DELETE"} {
		if w := request(router, method, path, `{}`, otherCookie); w.Code != 404 {
			t.Fatal("non-owner public setup access", method, w.Code, w.Body)
		}
	}
	var list []Setup
	w := request(router, "GET", "/api/v1/me/setups", "", ownerCookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != historical.ID || list[1].ID != created.ID {
		t.Fatal("own list order", list)
	}
	for _, setup := range list {
		if setup.ID == foreign.ID {
			t.Fatal("foreign setup listed")
		}
	}
	// DELETE preflight must work for the separate authenticated web client.
	req := httptest.NewRequest("OPTIONS", "http://localhost:8080"+path, nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	preflight := httptest.NewRecorder()
	router.ServeHTTP(preflight, req)
	if !strings.Contains(preflight.Header().Get("Access-Control-Allow-Methods"), "DELETE") || preflight.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal("DELETE CORS", preflight.Header())
	}
	if w := request(router, "DELETE", path, "", ownerCookie); w.Code != 204 {
		t.Fatal("owner delete", w.Code, w.Body)
	}
	if w := request(router, "GET", path, "", ownerCookie); w.Code != 404 {
		t.Fatal("deleted setup read", w.Code)
	}
	if w := request(router, "PATCH", "/api/v1/setups/"+unknown, `{"title":"Missing"}`, ownerCookie); w.Code != 404 {
		t.Fatal("missing setup update", w.Code)
	}
	pool.Close()
	w = request(router, "GET", "/api/v1/me/setups", "", ownerCookie)
	if w.Code != 500 || strings.Contains(w.Body.String(), "closed pool") {
		t.Fatal("raw failure leaked", w.Code, w.Body)
	}
}
