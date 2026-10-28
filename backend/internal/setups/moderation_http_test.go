package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

func TestModerationHTTPAccessReviewAndRevocation(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	store, err := photos.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.service.EnablePhotos(store)
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("m"), 32), SameSite: http.SameSiteLaxMode, AdminEmails: []string{f.other.Email}}
	provider := &fakeGoogle{}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(f.pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), f.pool.Ping)
	handler := NewHandler(f.service)
	authentication.RegisterWithPublic(router, []func(chi.Router){handler.RegisterPublicAPI}, handler.RegisterPrivateAPI)
	administrator, err := admin.New(cfg, admin.NewRepository(f.pool))
	if err != nil {
		t.Fatal(err)
	}
	moderation, err := NewModerationHandler(f.service)
	if err != nil {
		t.Fatal(err)
	}
	administrator.Register(router, authentication.RequireUser, moderation.RegisterAdmin)
	signIn := func(subject string, u users.User) *http.Cookie {
		provider.identity = users.Identity{Subject: subject, Email: u.Email, Name: u.Nickname}
		return login(t, router)
	}
	owner, friend, adminCookie := signIn("search-owner", f.owner), signIn("search-friend", f.friend), signIn("search-other", f.other)
	if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, f.owner.ID, f.friend.ID); err != nil {
		t.Fatal(err)
	}
	setup := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Moderated <script>unsafe</script>","visibility":"public","notes":"<img src=x onerror=bad>","data":{"suspension":{"rear":{"toe_deg":0}}}}`, owner), 201)
	if setup.ModerationStatus != Pending {
		t.Fatal("create not pending", setup)
	}
	p, err := f.service.uploadPhoto(ctx, f.owner.ID, setup.ID, "00000000-0000-4000-8000-000000000001", photoPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/setups/" + setup.ID
	preview := "/admin/moderation/" + setup.ID
	adminImage := preview + "/photos/" + p.ID + "/image"
	adminThumb := preview + "/photos/" + p.ID + "/thumbnail"
	assertAccess := func(public bool) {
		t.Helper()
		for _, viewer := range []struct {
			name    string
			cookie  *http.Cookie
			allowed bool
		}{{"guest", nil, public}, {"owner", owner, true}, {"friend", friend, true}, {"stranger-admin", adminCookie, public}} {
			for _, path := range []string{base, base + "/photos", p.URL, p.ThumbnailURL, "/api/v1/setups/compare?left_id=" + setup.ID + "&right_id=" + f.public[0].ID} {
				want := 404
				if viewer.allowed {
					want = 200
				}
				w := request(router, "GET", path, "", viewer.cookie)
				if w.Code != want || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
					t.Fatal(viewer.name, path, w.Code, w.Body)
				}
				if path == base && (!public || !viewer.allowed) && w.Header().Get("X-Robots-Tag") == "index, follow" {
					t.Fatal("pending indexed")
				}
				if !viewer.allowed && strings.Contains(w.Body.String(), "unsafe") {
					t.Fatal("inaccessible setup leaked")
				}
			}
			for _, path := range []string{"/api/v1/setups/search?q=Moderated", "/api/v1/users/" + f.owner.ID + "/public-setups?q=Moderated", "/api/v1/setups/visible?q=Moderated"} {
				w := request(router, "GET", path, "", viewer.cookie)
				var page SearchPage
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
					t.Fatal(w.Code, w.Body)
				}
				want := 0
				if public || (strings.Contains(path, "/visible?") && viewer.allowed) {
					want = 1
				}
				if len(page.Items) != want {
					t.Fatal("summary visibility", viewer.name, path, page)
				}
			}
		}
		// Authenticated profile arrays share the SQL rule with detail/compare/media.
		for _, v := range []struct {
			cookie  *http.Cookie
			allowed bool
		}{{owner, true}, {friend, true}, {adminCookie, public}} {
			w := request(router, "GET", "/api/v1/users/"+f.owner.ID+"/setups", "", v.cookie)
			var list []Setup
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil {
				t.Fatal(w.Code, w.Body)
			}
			found := false
			for _, item := range list {
				found = found || item.ID == setup.ID
			}
			if found != v.allowed {
				t.Fatal("profile leaked", w.Body)
			}
		}
	}
	assertAccess(false)
	for _, path := range []string{"/admin/moderation", preview, adminImage, adminThumb} {
		for _, v := range []struct {
			cookie *http.Cookie
			want   int
		}{{nil, 401}, {owner, 403}, {adminCookie, 200}} {
			w := request(router, "GET", path, "", v.cookie)
			if w.Code != v.want || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("admin boundary", path, w.Code, w.Body)
			}
			if v.want == 200 && !strings.Contains(path, "/photos/") && (strings.Contains(w.Body.String(), "<script>unsafe") || strings.Contains(w.Body.String(), "<img src=x")) {
				t.Fatal("unescaped preview")
			}
		}
	}
	// CSRF and revision token are independent. A valid token cannot approve stale content.
	type form struct {
		token    string
		cookie   *http.Cookie
		expected string
	}
	getForm := func() form {
		t.Helper()
		w := request(router, "GET", preview, "", adminCookie)
		token := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
		expected := regexp.MustCompile(`name="expected_updated_at" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
		if w.Code != 200 || len(token) != 2 || len(expected) != 2 {
			t.Fatal("review form", w.Code, w.Body)
		}
		var cookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "rc_admin_csrf" {
				cookie = c
			}
		}
		if cookie == nil {
			t.Fatal("missing CSRF cookie")
		}
		return form{token[1], cookie, expected[1]}
	}
	post := func(action string, form form, token, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", preview+"/"+action, strings.NewReader(url.Values{"expected_updated_at": {form.expected}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", token)
		req.Header.Set("Origin", origin)
		req.AddCookie(adminCookie)
		req.AddCookie(form.cookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	form1 := getForm()
	for _, v := range []struct{ token, origin string }{{"", "http://localhost:8080"}, {form1.token, "https://evil.test"}} {
		if w := post("approve", form1, v.token, v.origin); w.Code != 403 {
			t.Fatal("CSRF bypass", w.Code)
		}
	}
	if _, err := f.service.Patch(ctx, f.owner.ID, setup.ID, PatchInput{Title: NullableString{Present: true, Value: stringPtr("Moderated edited")}}); err != nil {
		t.Fatal(err)
	}
	if w := post("approve", form1, form1.token, "http://localhost:8080"); w.Code != 409 {
		t.Fatal("stale review", w.Code, w.Body)
	}
	form2 := getForm()
	if w := post("approve", form2, form2.token, "http://localhost:8080"); w.Code != 303 || w.Header().Get("Location") != "/admin/moderation" {
		t.Fatal("approve", w.Code, w.Body)
	}
	if w := post("reject", form2, form2.token, "http://localhost:8080"); w.Code != 409 {
		t.Fatal("duplicate decision", w.Code)
	}
	assertAccess(true)
	// Photo upload atomically revokes old full/thumbnail URLs, search and Compare.
	if _, err := f.service.uploadPhoto(ctx, f.owner.ID, setup.ID, "00000000-0000-4000-8000-000000000002", photoPNG(t)); err != nil {
		t.Fatal(err)
	}
	assertAccess(false)
	form3 := getForm()
	if w := post("reject", form3, form3.token, "http://localhost:8080"); w.Code != 303 {
		t.Fatal("reject", w.Code, w.Body)
	}
	assertAccess(false)
	// Known URLs cannot regain access via conditional requests or a revision query.
	for _, path := range []string{p.URL, p.ThumbnailURL + "&revision=1"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("If-None-Match", "*")
		req.Header.Set("If-Modified-Since", "Wed, 28 Oct 2026 00:00:00 GMT")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 404 {
			t.Fatal("cached bypass", w.Code)
		}
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM friendships WHERE requester_id=$1 AND addressee_id=$2`, f.owner.ID, f.friend.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{base, base + "/photos", p.URL, p.ThumbnailURL} {
		if w := request(router, "GET", path, "", friend); w.Code != 404 {
			t.Fatal("friendship revocation", path, w.Code)
		}
	}
	// The pending/rejected moderation fields cannot be forged through owner writes.
	for _, method := range []string{"POST", "PATCH"} {
		path := base
		if method == "POST" {
			path = "/api/v1/setups"
		}
		body := fmt.Sprintf(`{"title":"Forged","visibility":"public","data":{},"moderation_status":"approved"}`)
		if w := request(router, method, path, body, owner); w.Code != 400 {
			t.Fatal("forged approval", method, w.Code)
		}
	}
}
