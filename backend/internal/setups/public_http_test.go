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
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/admin"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

func TestAnonymousPublicHTTP(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(NewRepository(pool), catalog, friendships.NewRepository(pool))
	store, err := photos.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.EnablePhotos(store)
	userService := users.NewService(users.NewRepository(pool))
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("p"), 32), SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
	provider := &fakeGoogle{}
	authentication := auth.NewHandler(cfg, provider, userService)
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	setupHandler := NewHandler(service)
	profileHandler := users.NewHandler(userService)
	catalogHandler, err := chassis.NewHandler(catalog)
	if err != nil {
		t.Fatal(err)
	}
	friendHandler := friendships.NewHandler(friendships.NewService(friendships.NewRepository(pool)))
	// This is the same explicit route composition as cmd/server, not a permissive test router.
	authentication.RegisterWithPublic(router, []func(chi.Router){setupHandler.RegisterPublicAPI, profileHandler.RegisterPublicAPI, catalogHandler.RegisterAPI}, setupHandler.RegisterPrivateAPI, friendHandler.RegisterAPI, func(r chi.Router) { profileHandler.RegisterPrivateAPI(r, auth.CurrentUser) })
	administrator, err := admin.New(cfg, admin.NewRepository(pool))
	if err != nil {
		t.Fatal(err)
	}
	administrator.Register(router, authentication.RequireUser, catalogHandler.RegisterAdmin)
	type account struct {
		user   users.User
		cookie *http.Cookie
	}
	loginAs := func(name string) account {
		t.Helper()
		provider.identity = users.Identity{Subject: name + "-secret", Email: name + "@example.test", Name: name}
		cookie := login(t, router)
		w := request(router, "GET", "/api/v1/me", "", cookie)
		var user users.User
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &user) != nil {
			t.Fatal("login", w.Code, w.Body)
		}
		return account{user, cookie}
	}
	owner, friend, stranger, empty := loginAs("public-owner"), loginAs("public-friend"), loginAs("public-stranger"), loginAs("empty-owner")
	if _, err := pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, owner.user.ID, friend.user.ID); err != nil {
		t.Fatal(err)
	}
	brand, err := catalog.CreateBrand(ctx, "Public historical brand")
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, brand.ID, "Historical model")
	if err != nil {
		t.Fatal(err)
	}
	var records []Setup
	var images []Photo
	for i, visibility := range []Visibility{Public, Friends, Private} {
		s, err := service.Create(ctx, owner.user.ID, CreateInput{Title: fmt.Sprintf("Guest setup %d", i), Visibility: visibility, ChassisModelID: &model.ID, Data: realisticData(t)})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, s)
		p, err := service.uploadPhoto(ctx, owner.user.ID, s.ID, fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1), photoPNG(t))
		if err != nil {
			t.Fatal(err)
		}
		images = append(images, p)
	}
	if _, err := catalog.SetBrandActive(ctx, brand.ID, false); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "GET", "/api/v1/chassis/brands", "", nil); w.Code != 200 || w.Body.String() != "[]\n" {
		t.Fatal("inactive catalog leaked", w.Code, w.Body)
	}
	if w := request(router, "GET", "/api/v1/chassis/brands/"+brand.ID+"/models", "", nil); w.Code != 404 {
		t.Fatal("inactive catalog selector", w.Code)
	}
	if _, err := service.Create(ctx, empty.user.ID, CreateInput{Title: "Hidden only", Visibility: Private, Data: &DataV1{}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		want   [3]int
	}{
		{"guest", nil, [3]int{200, 404, 404}}, {"invalid session", &http.Cookie{Name: "rc_session", Value: "invalid"}, [3]int{200, 404, 404}},
		{"owner", owner.cookie, [3]int{200, 200, 200}}, {"friend", friend.cookie, [3]int{200, 200, 404}}, {"stranger", stranger.cookie, [3]int{200, 404, 404}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, s := range records {
				base := "/api/v1/setups/" + s.ID
				for _, path := range []string{base, base + "/photos", images[i].URL, images[i].ThumbnailURL} {
					w := request(router, "GET", path, "", tc.cookie)
					if w.Code != tc.want[i] {
						t.Fatalf("%s: %d %s", path, w.Code, w.Body)
					}
					if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
						t.Fatal("cacheable read", path)
					}
					if w.Header().Get("X-Robots-Tag") == "index, follow" && (path != base || i != 0) {
						t.Fatal("nonpublic/media indexing", path)
					}
					if w.Code == 200 && (path == images[i].URL || path == images[i].ThumbnailURL) && (w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("X-Content-Type-Options") != "nosniff") {
						t.Fatal("unsafe media headers")
					}
				}
			}
		})
	}
	publicDetail := request(router, "GET", "/api/v1/setups/"+records[0].ID, "", nil)
	if publicDetail.Header().Get("X-Robots-Tag") != "index, follow" {
		t.Fatal("public indexing eligibility missing")
	}
	var detail Setup
	if json.Unmarshal(publicDetail.Body.Bytes(), &detail) != nil || detail.Chassis == nil || detail.Chassis.ModelName != model.Name || detail.Data.Suspension.Rear.ToeDeg == nil || *detail.Data.Suspension.Rear.ToeDeg != 0 {
		t.Fatal("historical/zero public detail lost")
	}
	conditional := httptest.NewRequest("GET", images[0].URL, nil)
	conditional.Header.Set("If-None-Match", "*")
	conditional.Header.Set("If-Modified-Since", "Tue, 01 Jan 2030 00:00:00 GMT")
	cw := httptest.NewRecorder()
	router.ServeHTTP(cw, conditional)
	if cw.Code != 200 || cw.Body.Len() == 0 || cw.Header().Get("ETag") != "" || cw.Header().Get("Last-Modified") != "" {
		t.Fatal("conditional media caching", cw.Code, cw.Header())
	}
	missing := "/api/v1/setups/00000000-0000-4000-8000-000000000000"
	for _, suffix := range []string{"", "/photos", "/photos/00000000-0000-4000-8000-000000000000/image", "/photos/00000000-0000-4000-8000-000000000000/thumbnail"} {
		a := request(router, "GET", missing+suffix, "", nil)
		b := request(router, "GET", "/api/v1/setups/"+records[2].ID+suffix, "", nil)
		if a.Code != 404 || b.Code != 404 || a.Body.String() != b.Body.String() {
			t.Fatal("private/missing distinction", a.Code, b.Code)
		}
	}
	for _, person := range []account{owner, empty} {
		w := request(router, "GET", "/api/v1/users/"+person.user.ID, "", nil)
		var profile map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &profile) != nil || len(profile) != 3 || profile["id"] != person.user.ID || profile["nickname"] == nil {
			t.Fatal("unsafe profile", w.Code, w.Body)
		}
	}
	for _, person := range []account{empty, stranger} {
		w := request(router, "GET", "/api/v1/users/"+person.user.ID+"/public-setups", "", nil)
		if w.Code != 200 || w.Body.String() != "{\"items\":[],\"next_cursor\":null}\n" {
			t.Fatal("empty public profile", w.Code, w.Body)
		}
	}
	if w := request(router, "GET", "/api/v1/users/00000000-0000-4000-8000-000000000000", "", nil); w.Code != 404 {
		t.Fatal("missing profile", w.Code)
	}
	checkPage := func(path string, cookie *http.Cookie) SearchPage {
		t.Helper()
		w := request(router, "GET", path, "", cookie)
		var page SearchPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
			t.Fatal("page", w.Code, w.Body)
		}
		for _, word := range []string{"email", "google_subject", "storage_key", "thumbnail_key", "incoming", "accepted", "private_count"} {
			if strings.Contains(w.Body.String(), "\""+word+"\"") {
				t.Fatal("unsafe projection", word)
			}
		}
		return page
	}
	for _, cookie := range []*http.Cookie{nil, owner.cookie, friend.cookie} {
		for _, path := range []string{"/api/v1/setups/search", "/api/v1/users/" + owner.user.ID + "/public-setups"} {
			page := checkPage(path, cookie)
			if len(page.Items) != 1 || page.Items[0].ID != records[0].ID || page.Items[0].CoverPhoto == nil || page.Items[0].CoverPhoto.ID != images[0].ID {
				t.Fatal("public page/cover leakage", page)
			}
		}
	}
	// Authenticated visibility-dependent array endpoint is unchanged.
	for _, tc := range []struct {
		cookie *http.Cookie
		count  int
	}{{owner.cookie, 3}, {friend.cookie, 2}, {stranger.cookie, 1}} {
		w := request(router, "GET", "/api/v1/users/"+owner.user.ID+"/setups", "", tc.cookie)
		var result []Setup
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result) != tc.count {
			t.Fatal("authenticated profile parity", w.Code, w.Body)
		}
	}
	// Public-pagination tests use one-row pages so hidden records cannot affect a cursor.
	second, err := service.Create(ctx, owner.user.ID, CreateInput{Title: "Second public", Visibility: Public, Data: &DataV1{}})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := service.Create(ctx, stranger.user.ID, CreateInput{Title: "Foreign public", Visibility: Public, Data: &DataV1{}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/users/" + owner.user.ID + "/public-setups?limit=1"
	page := checkPage(path, nil)
	if len(page.Items) != 1 || page.Items[0].ID != second.ID || page.NextCursor == nil {
		t.Fatal("pagination", page)
	}
	page = checkPage(path+"&cursor="+url.QueryEscape(*page.NextCursor), nil)
	if len(page.Items) != 1 || page.Items[0].ID != records[0].ID || page.NextCursor != nil {
		t.Fatal("owner cursor escaped filter", foreign.ID, page)
	}
	for _, query := range []string{"limit=51", "limit=0", "limit=1&limit=2", "cursor=broken", "brand_id=bad", "q=%00"} {
		if w := request(router, "GET", "/api/v1/setups/search?"+query, "", nil); w.Code != 400 {
			t.Fatal("search validation", query, w.Code)
		}
	}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/me"}, {"GET", "/api/v1/me/setups"}, {"GET", "/api/v1/users/search?q=a"}, {"GET", "/api/v1/friendships"}, {"GET", "/api/v1/users/" + owner.user.ID + "/setups"},
		{"POST", "/api/v1/setups"}, {"PATCH", "/api/v1/setups/" + records[0].ID}, {"DELETE", "/api/v1/setups/" + records[0].ID}, {"POST", "/api/v1/auth/logout"}, {"PATCH", "/api/v1/me"}, {"POST", "/api/v1/friendships"}, {"POST", "/api/v1/friendships/" + owner.user.ID + "/accept"}, {"DELETE", "/api/v1/friendships/" + owner.user.ID},
		{"POST", "/api/v1/setups/" + records[0].ID + "/photos"}, {"PUT", "/api/v1/setups/" + records[0].ID + "/photos/" + images[0].ID + "/cover"}, {"PUT", "/api/v1/setups/" + records[0].ID + "/photos/" + images[0].ID + "/crop"}, {"DELETE", images[0].URL[:strings.LastIndex(images[0].URL, "/")]}, {"GET", "/admin/"},
	} {
		w := request(router, tc.method, tc.path, "", nil)
		if w.Code != 401 || w.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Fatal("protected route open", tc, w.Code, w.Body)
		}
	}
	// Anonymous HEAD does not silently gain access through a GET registration.
	if w := request(router, "HEAD", "/api/v1/setups/"+records[0].ID, "", nil); w.Code == 200 {
		t.Fatal("implicit HEAD allowlist")
	}
	// Origin protection and safe methods remain distinct after route splitting.
	r := httptest.NewRequest("PATCH", "/api/v1/setups/"+records[0].ID, strings.NewReader(`{"title":"bad"}`))
	r.AddCookie(owner.cookie)
	r.Header.Set("Origin", "https://evil.invalid")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("origin bypass", w.Code)
	}
	// More restrictive visibility immediately revokes known full/thumbnail/gallery/detail URLs.
	if _, err := pool.Exec(ctx, `UPDATE setups SET visibility='private' WHERE id=$1`, records[0].ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/setups/" + records[0].ID, "/api/v1/setups/" + records[0].ID + "/photos", images[0].URL, images[0].ThumbnailURL} {
		if w := request(router, "GET", path, "", nil); w.Code != 404 {
			t.Fatal("stale public URL", path, w.Code)
		}
	}
	// Friendship removal revokes authenticated media without changing public access.
	if _, err := pool.Exec(ctx, `DELETE FROM friendships WHERE requester_id=$1 AND addressee_id=$2`, owner.user.ID, friend.user.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/setups/" + records[1].ID, images[1].URL, images[1].ThumbnailURL} {
		if w := request(router, "GET", path, "", friend.cookie); w.Code != 404 {
			t.Fatal("removed friend still has access", w.Code)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE setups SET visibility='public' WHERE id=$1`, records[0].ID); err != nil {
		t.Fatal(err)
	}
	// Conditional requests cannot restore cached access or turn a revoked URL into 304.
	if err := service.Delete(ctx, owner.user.ID, records[0].ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/setups/" + records[0].ID, images[0].URL, images[0].ThumbnailURL} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("If-None-Match", "*")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 404 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("deleted URL", w.Code)
		}
	}
	// Do not silently downgrade real identity-store failures to a guest.
	pool.Close()
	if w := request(router, "GET", "/api/v1/setups/search", "", owner.cookie); w.Code != 500 {
		t.Fatal("identity lookup failure downgraded", w.Code)
	}
}
