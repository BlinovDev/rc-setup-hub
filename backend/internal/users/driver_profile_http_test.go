package users_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type driverHarness struct {
	pool    *pgxpool.Pool
	service *users.Service
	router  http.Handler
	cookie  *http.Cookie
	owner   users.User
}

func driverHTTP(t *testing.T) driverHarness {
	t.Helper()
	pool := dbtest.New(t)
	service := users.NewService(users.NewRepository(pool))
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
	authentication := auth.NewHandler(cfg, profileGoogle{}, service)
	router := web.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), pool.Ping)
	handler := users.NewHandler(service)
	authentication.RegisterWithPublic(router, []func(chi.Router){handler.RegisterPublicAPI}, func(r chi.Router) { handler.RegisterPrivateAPI(r, auth.CurrentUser) })
	request := func(path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	cookieNamed := func(w *httptest.ResponseRecorder, name string) *http.Cookie {
		for _, c := range w.Result().Cookies() {
			if c.Name == name {
				return c
			}
		}
		t.Fatalf("missing cookie %s", name)
		return nil
	}
	start := request("/auth/google", nil)
	location, _ := url.Parse(start.Header().Get("Location"))
	callback := request("/auth/google/callback?code=fake&state="+url.QueryEscape(location.Query().Get("state")), cookieNamed(start, "rc_oauth_state"))
	if callback.Code != 303 {
		t.Fatal(callback.Code, callback.Body)
	}
	h := driverHarness{pool: pool, service: service, router: router, cookie: cookieNamed(callback, "rc_session")}
	me := h.request("GET", "/api/v1/me", "", nil, true, "")
	if me.Code != 200 || json.Unmarshal(me.Body.Bytes(), &h.owner) != nil {
		t.Fatal(me.Code, me.Body)
	}
	return h
}
func (h driverHarness) request(method, path, contentType string, body io.Reader, signed bool, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if signed {
		r.AddCookie(h.cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, r)
	return w
}
func (h driverHarness) patch(body string) *httptest.ResponseRecorder {
	return h.request("PATCH", "/api/v1/me", "application/json", strings.NewReader(body), true, "")
}
func profileFrom(t *testing.T, w *httptest.ResponseRecorder) users.DriverProfile {
	t.Helper()
	var p users.DriverProfile
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &p) != nil {
		t.Fatal(w.Code, w.Body)
	}
	return p
}

func TestDriverProfileSocialAndStatsHTTP(t *testing.T) {
	h := driverHTTP(t)
	ctx := context.Background()
	path := "/api/v1/users/" + h.owner.ID
	initial := profileFrom(t, h.request("GET", path, "", nil, false, ""))
	if !initial.MemberSince.Equal(h.owner.CreatedAt) || initial.PublicSetupCount != 0 || initial.AvatarURL != nil {
		t.Fatal(initial)
	}
	if w := h.patch(`{"nickname":"Updated driver","instagram_url":"https://www.instagram.com/driver/","tiktok_url":"https://www.tiktok.com/@driver","youtube_url":"https://youtube.com/@driver"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	for _, signed := range []bool{false, true} {
		w := h.request("GET", path, "", nil, signed, "")
		p := profileFrom(t, w)
		if p.InstagramURL == nil || *p.InstagramURL != "https://www.instagram.com/driver" || p.TikTokURL == nil || p.YouTubeURL == nil || p.Nickname != "Updated driver" {
			t.Fatal(p)
		}
		for _, private := range []string{"email", "google_subject", "caller-private", "custom_avatar_key", "has_custom_avatar"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("public leak", w.Body)
			}
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Header())
		}
	}
	for _, body := range []string{`{}`, `null`, `{"nickname":null}`, `{"nickname":""}`, `{"avatar_url":"https://evil.test/a"}`, `{"owner_id":"other"}`, `{"instagram_url":42}`, `{"instagram_url":"javascript:alert(1)"}`, `{"tiktok_url":"https://vm.tiktok.com/foo"}`, `{"youtube_url":"https://youtube.com/redirect?q=evil"}`, `{"nickname":"must not persist","youtube_url":"https://evil.test/@driver"}`, `{"instagram_url":null} {}`} {
		if w := h.patch(body); w.Code != 400 {
			t.Fatal("invalid patch accepted", body, w.Code, w.Body)
		}
	}
	if w := h.patch(`{"nickname":"` + strings.Repeat("x", 9000) + `"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := h.request("PATCH", "/api/v1/me", "text/plain", strings.NewReader(`{"nickname":"x"}`), true, ""); w.Code != 415 {
		t.Fatal(w.Code)
	}
	for _, method := range []string{"PATCH", "PUT", "DELETE"} {
		endpoint := "/api/v1/me/avatar"
		if method == "PATCH" {
			endpoint = "/api/v1/me"
		}
		if w := h.request(method, endpoint, "application/json", strings.NewReader(`{}`), false, ""); w.Code != 401 {
			t.Fatal(method, w.Code)
		}
		if w := h.request(method, endpoint, "application/json", strings.NewReader(`{}`), true, "https://evil.test"); w.Code != 403 {
			t.Fatal(method, w.Code)
		}
	}
	if w := h.patch(`{"nickname":"Updated again","instagram_url":null,"tiktok_url":""}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	p := profileFrom(t, h.request("GET", path, "", nil, false, ""))
	if p.InstagramURL != nil || p.TikTokURL != nil || p.YouTubeURL == nil {
		t.Fatal("clear/omission", p)
	}
	other, err := h.service.Login(ctx, users.Identity{Subject: "other", Email: "other@example.test", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if w := h.request("PATCH", "/api/v1/users/"+other.ID, "application/json", strings.NewReader(`{"nickname":"stolen"}`), true, ""); w.Code < 400 {
		t.Fatal(w.Code)
	}
	otherAfter, err := h.service.Get(ctx, other.ID)
	if err != nil || otherAfter.Nickname != other.Nickname {
		t.Fatal(otherAfter, err)
	}
	if _, err := h.pool.Exec(ctx, `UPDATE users SET instagram_url='javascript:evil',tiktok_url='https://evil.test/a',youtube_url='https://youtube.com/redirect?q=evil' WHERE id=$1`, h.owner.ID); err != nil {
		t.Fatal(err)
	}
	p = profileFrom(t, h.request("GET", path, "", nil, false, ""))
	if p.InstagramURL != nil || p.TikTokURL != nil || p.YouTubeURL != nil {
		t.Fatal("unvalidated imported links leaked", p)
	}
	ids := map[string]string{}
	for _, visibility := range []string{"public", "private", "friends"} {
		for _, status := range []string{"approved", "pending", "rejected"} {
			var id string
			if err := h.pool.QueryRow(ctx, `INSERT INTO setups(owner_id,title,visibility,moderation_status,data) VALUES($1,$2,$3,$4,'{}') RETURNING id::text`, h.owner.ID, visibility+status, visibility, status).Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids[visibility+status] = id
		}
	}
	assertCount := func(want int64) {
		t.Helper()
		for _, signed := range []bool{false, true} {
			p := profileFrom(t, h.request("GET", path, "", nil, signed, ""))
			if p.PublicSetupCount != want {
				t.Fatal(p)
			}
		}
	}
	assertCount(1)
	for _, tc := range []struct {
		sql, id string
		want    int64
	}{
		{`UPDATE setups SET moderation_status='approved' WHERE id=$1`, ids["publicpending"], 2},
		{`UPDATE setups SET visibility='private' WHERE id=$1`, ids["publicapproved"], 1},
		{`UPDATE setups SET moderation_status='pending' WHERE id=$1`, ids["publicpending"], 0},
		{`UPDATE setups SET moderation_status='approved' WHERE id=$1`, ids["publicpending"], 1},
		{`DELETE FROM setups WHERE id=$1`, ids["publicpending"], 0},
	} {
		if _, err := h.pool.Exec(ctx, tc.sql, tc.id); err != nil {
			t.Fatal(err)
		}
		assertCount(tc.want)
	}
}

func avatarMultipart(t *testing.T, raw []byte, field string, count int) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for i := 0; i < count; i++ {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="`+field+`"; filename="untrusted.html"`)
		header.Set("Content-Type", "text/html")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}
func avatarPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 60, 100))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestDriverAvatarLifecycleHTTP(t *testing.T) {
	h := driverHTTP(t)
	ctx := context.Background()
	root := t.TempDir()
	store, err := photos.New(filepath.Join(root, "avatars"))
	if err != nil {
		t.Fatal(err)
	}
	h.service.EnableAvatars(store)
	// A setup photo with the same key cannot be opened/deleted by avatar operations.
	setupStore, err := photos.New(root)
	if err != nil {
		t.Fatal(err)
	}
	setupImage, err := setupStore.Process(ctx, avatarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	put := func(raw []byte, signed bool, origin string) *httptest.ResponseRecorder {
		body, typ := avatarMultipart(t, raw, "file", 1)
		return h.request("PUT", "/api/v1/me/avatar", typ, body, signed, origin)
	}
	for _, tc := range []struct {
		signed bool
		origin string
		status int
	}{{false, "", 401}, {true, "https://evil.test", 403}} {
		if w := put([]byte("not decoded"), tc.signed, tc.origin); w.Code != tc.status {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, raw := range [][]byte{nil, []byte("<svg onload='evil()'/>"), {0xff, 0xd8, 0xff}, bytes.Repeat([]byte{'x'}, users.MaxAvatarBytes+1)} {
		if w := put(raw, true, ""); w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, tc := range []struct {
		field string
		count int
	}{{"file", 0}, {"file", 2}, {"owner_id", 1}} {
		body, typ := avatarMultipart(t, avatarPNG(t), tc.field, tc.count)
		if w := h.request("PUT", "/api/v1/me/avatar", typ, body, true, ""); w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if w := h.request("PUT", "/api/v1/me/avatar", "application/json", strings.NewReader(`{}`), true, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}

	// All accepted formats pass the real avatar pipeline regardless of declared MIME.
	source := filepath.Join(t.TempDir(), "source.png")
	if err := os.WriteFile(source, avatarPNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	webp := source + ".webp"
	if out, err := exec.Command("convert", source, webp).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	webpRaw, err := os.ReadFile(webp)
	if err != nil {
		t.Fatal(err)
	}
	heifRaw, err := os.ReadFile("../photos/testdata/phone-format.heic")
	if err != nil {
		t.Fatal(err)
	}
	var jpegRaw bytes.Buffer
	if err := jpeg.Encode(&jpegRaw, image.NewRGBA(image.Rect(0, 0, 60, 100)), nil); err != nil {
		t.Fatal(err)
	}
	// Add an actual EXIF orientation tag; no input metadata may survive normalization.
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	oriented := append([]byte{}, jpegRaw.Bytes()[:2]...)
	oriented = append(oriented, 0xff, 0xe1, byte((len(exif)+2)>>8), byte(len(exif)+2))
	oriented = append(oriented, exif...)
	oriented = append(oriented, jpegRaw.Bytes()[2:]...)
	for _, tc := range []struct {
		name string
		raw  []byte
	}{{"JPEG", oriented}, {"WEBP", webpRaw}, {"HEIF", heifRaw}} {
		t.Run(tc.name, func(t *testing.T) {
			uploaded := put(tc.raw, true, "")
			var u users.User
			if uploaded.Code != 200 || json.Unmarshal(uploaded.Body.Bytes(), &u) != nil || u.AvatarURL == nil {
				t.Fatal(uploaded.Code, uploaded.Body)
			}
			served := h.request("GET", *u.AvatarURL, "", nil, false, "")
			cfg, format, err := image.DecodeConfig(bytes.NewReader(served.Body.Bytes()))
			if served.Code != 200 || err != nil || format != "jpeg" || cfg.Width != cfg.Height || cfg.Width > 640 || bytes.Contains(served.Body.Bytes(), []byte("Exif")) {
				t.Fatal(served.Code, cfg, format, err)
			}
		})
	}
	oversized := source + "-oversized.jpg"
	if out, err := exec.Command("convert", "-size", "8000x6400", "xc:red", oversized).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	overPixels, err := os.ReadFile(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if w := put(overPixels, true, ""); w.Code != 400 {
		t.Fatal("pixel bomb accepted", w.Code, w.Body)
	}
	var current users.User
	w := put(avatarPNG(t), true, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &current) != nil || !current.HasCustomAvatar || current.AvatarURL == nil {
		t.Fatal(w.Code, w.Body)
	}
	oldURL := *current.AvatarURL
	imageResponse := h.request("GET", oldURL, "", nil, false, "")
	cfg, format, err := image.DecodeConfig(imageResponse.Body)
	if err != nil || imageResponse.Code != 200 || format != "jpeg" || cfg.Width != 60 || cfg.Height != 60 {
		t.Fatal(imageResponse.Code, cfg, format, err)
	}
	if imageResponse.Header().Get("Cache-Control") != "no-store, private" || imageResponse.Header().Get("X-Content-Type-Options") != "nosniff" || imageResponse.Header().Get("ETag") != "" || imageResponse.Header().Get("Last-Modified") != "" {
		t.Fatal(imageResponse.Header())
	}
	public := profileFrom(t, h.request("GET", "/api/v1/users/"+h.owner.ID, "", nil, false, ""))
	if public.AvatarURL == nil || *public.AvatarURL != oldURL {
		t.Fatal(public)
	}
	r := httptest.NewRequest("GET", oldURL, nil)
	r.Header.Set("If-None-Match", "*")
	r.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).Format(http.TimeFormat))
	conditional := httptest.NewRecorder()
	h.router.ServeHTTP(conditional, r)
	if conditional.Code != 200 {
		t.Fatal(conditional.Code)
	}
	// Refresh Google identity without overwriting custom profile/media.
	loggedIn, err := h.service.Login(ctx, users.Identity{Subject: "caller-private-subject", Email: "caller@example.com", Name: "Caller", AvatarURL: "https://google.example/new.jpg"})
	if err != nil || *loggedIn.AvatarURL != oldURL {
		t.Fatal(loggedIn, err)
	}
	w = put(avatarPNG(t), true, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &current) != nil || *current.AvatarURL == oldURL {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.request("GET", oldURL, "", nil, false, ""); w.Code != 404 {
		t.Fatal("old URL not revoked", w.Code)
	}
	oldURL = *current.AvatarURL
	w = h.request("DELETE", "/api/v1/me/avatar", "", nil, true, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &current) != nil || current.HasCustomAvatar || current.AvatarURL == nil || *current.AvatarURL != "https://google.example/new.jpg" {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.request("GET", oldURL, "", nil, false, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := h.request("DELETE", "/api/v1/me/avatar", "", nil, true, ""); w.Code != 200 {
		t.Fatal("repeat delete", w.Code)
	}
	if _, err := h.service.Login(ctx, users.Identity{Subject: "caller-private-subject", Email: "caller@example.com", Name: "Caller"}); err != nil {
		t.Fatal(err)
	}
	got, err := h.service.Get(ctx, h.owner.ID)
	if err != nil || got.AvatarURL != nil {
		t.Fatal("placeholder fallback", got, err)
	}
	// Concurrent replacements leave only one live avatar; old publications queue safely.
	raw := avatarPNG(t)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.service.UploadAvatar(ctx, h.owner.ID, raw)
			failures <- err
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err = h.service.Get(ctx, h.owner.ID)
	if err != nil || !got.HasCustomAvatar {
		t.Fatal(got, err)
	}
	if w := h.request("GET", *got.AvatarURL, "", nil, false, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	var key string
	if err := h.pool.QueryRow(ctx, `SELECT custom_avatar_key FROM users WHERE id=$1`, h.owner.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	// Setup paths cannot substitute for avatar IDs, including identical random keys.
	if w := h.request("GET", "/api/v1/users/"+h.owner.ID+"/avatar/"+setupImage.Key, "", nil, false, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if _, err := h.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, h.owner.ID); err != nil {
		t.Fatal(err)
	}
	if w := h.request("GET", *got.AvatarURL, "", nil, false, ""); w.Code != 404 {
		t.Fatal("deleted account avatar readable", w.Code)
	}
	restarted := users.NewService(users.NewRepository(h.pool))
	restarted.EnableAvatars(store)
	if err := restarted.ReconcileAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "avatars", key)); !os.IsNotExist(err) {
		t.Fatal("deleted avatar files retained", err)
	}
	if f, err := setupStore.Open(setupImage.Key, true); err != nil {
		t.Fatal("setup photo affected", err)
	} else {
		_ = f.Close()
	}
	entries, err := os.ReadDir(filepath.Join(root, "avatars"))
	if err != nil || len(entries) != 0 {
		t.Fatal("orphaned avatar publications", len(entries), err)
	}
}
