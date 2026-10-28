package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
)

func photoPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 20, 10))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestPhotoPersistenceConcurrencyAndCleanup(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	owner, err := users.NewService(users.NewRepository(pool)).Login(ctx, users.Identity{Subject: "photos", Email: "photos@example.test", Name: "Photos"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := photos.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, chassis.NewService(chassis.NewRepository(pool)), friendships.NewRepository(pool))
	service.EnablePhotos(store)
	setup, err := service.Create(ctx, owner.ID, CreateInput{Title: "Photos", Visibility: Public, Data: realisticData(t)})
	if err != nil {
		t.Fatal(err)
	}
	raw := photoPNG(t)
	requestID := "11111111-1111-4111-8111-111111111111"
	first, err := service.uploadPhoto(ctx, owner.ID, setup.ID, requestID, raw)
	if err != nil || !first.IsCover {
		t.Fatal(first, err)
	}
	repeated, err := service.uploadPhoto(ctx, owner.ID, setup.ID, requestID, raw)
	if err != nil || repeated.ID != first.ID {
		t.Fatal("retry duplicated", repeated, err)
	}
	if _, err := service.uploadPhoto(ctx, owner.ID, setup.ID, requestID, []byte("different")); err != ErrConflict {
		t.Fatal("changed request accepted", err)
	}
	for i := 2; i <= 9; i++ {
		if _, err := service.uploadPhoto(ctx, owner.ID, setup.ID, fmt.Sprintf("00000000-0000-4000-8000-%012d", i), raw); err != nil {
			t.Fatal(err)
		}
	}
	// Preprocess before contention so the DB count/lock, not processor saturation, is exercised.
	var wg sync.WaitGroup
	results := make(chan error, 5)
	for i := 10; i < 15; i++ {
		img, err := store.Process(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(i int, img photos.Image) {
			defer wg.Done()
			_, keep, err := repo.addPhoto(ctx, owner.ID, setup.ID, fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "digest", img)
			if !keep {
				_ = store.Remove(img.Key)
			}
			results <- err
		}(i, img)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if _, ok := err.(*ValidationError); !ok {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("concurrent limit", success)
	}
	gallery, err := repo.listPhotos(ctx, owner.ID, setup.ID)
	if err != nil || len(gallery) != 10 || gallery[0].ID != first.ID {
		t.Fatal("order/count", gallery, err)
	}
	for _, read := range []func() (Setup, error){func() (Setup, error) { return service.Get(ctx, owner.ID, setup.ID) }, func() (Setup, error) { return repo.GetOwned(ctx, owner.ID, setup.ID) }} {
		s, err := read()
		if err != nil || s.CoverPhoto == nil || s.CoverPhoto.ID != first.ID {
			t.Fatal("cover projection", s, err)
		}
	}
	list, err := service.List(ctx, owner.ID)
	if err != nil || list[0].CoverPhoto.ID != first.ID {
		t.Fatal("list cover", err)
	}
	visible, err := service.ListUser(ctx, owner.ID, owner.ID)
	if err != nil || visible[0].CoverPhoto.ID != first.ID {
		t.Fatal("profile cover", err)
	}
	approveFixture(t, pool, setup.ID)
	search, err := service.Search(ctx, SearchInput{})
	if err != nil || search.Items[0].CoverPhoto.ID != first.ID {
		t.Fatal("search cover", err)
	}
	changed, err := service.Patch(ctx, owner.ID, setup.ID, PatchInput{Data: OptionalData{Present: true, Value: &DataV1{}}})
	if err != nil || changed.CoverPhoto.ID != first.ID {
		t.Fatal("technical edit lost photos", err)
	}
	if _, err := repo.mutatePhoto(ctx, owner.ID, setup.ID, gallery[5].ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.mutatePhoto(ctx, owner.ID, setup.ID, gallery[5].ID, true); err != nil {
		t.Fatal(err)
	}
	gallery, err = repo.listPhotos(ctx, owner.ID, setup.ID)
	if err != nil || !gallery[0].IsCover {
		t.Fatal("cover fallback", gallery, err)
	}
	if err := service.Delete(ctx, owner.ID, setup.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM setup_photos`).Scan(&count); err != nil || count != 0 {
		t.Fatal("cascade", count, err)
	}
	if _, err := store.Open(first.key, false); !os.IsNotExist(err) {
		t.Fatal("setup delete left files", err)
	}
}
func TestPhotoHTTPAuthorizationAndRetry(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	friendRepo := friendships.NewRepository(pool)
	service := NewService(repo, chassis.NewService(chassis.NewRepository(pool)), friendRepo)
	root := t.TempDir()
	store, err := photos.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service.EnablePhotos(store)
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
	provider := &fakeGoogle{identity: users.Identity{Subject: "owner", Email: "owner@example.test", Name: "Owner"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	cookie := login(t, router)
	setup := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Photos","visibility":"public","data":{}}`, cookie), 201)
	base := "/api/v1/setups/" + setup.ID + "/photos"
	upload := func(cookie *http.Cookie, key string, raw []byte, origin string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("request_id", key)
		part, _ := writer.CreateFormFile("file", "../../malicious.svg")
		_, _ = part.Write(raw)
		_ = writer.Close()
		req := httptest.NewRequest("POST", base, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	raw := photoPNG(t)
	key := "11111111-1111-4111-8111-111111111111"
	if w := upload(nil, key, raw, ""); w.Code != 401 {
		t.Fatal(w.Code, w.Body)
	}
	if w := upload(cookie, key, raw, "https://evil.test"); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	w := upload(cookie, key, raw, "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	gallery, err := repo.listPhotos(ctx, setup.OwnerID, setup.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := gallery[0]
	cropPath := base + "/" + p.ID + "/crop"
	cropBody := `{"expected_revision":1,"crop":{"x":0,"y":0,"size":5}}`
	if w := request(router, "PUT", cropPath, cropBody, nil); w.Code != 401 {
		t.Fatal("unauthenticated crop", w.Code)
	}
	foreign := httptest.NewRequest("PUT", cropPath, bytes.NewBufferString(cropBody))
	foreign.AddCookie(cookie)
	foreign.Header.Set("Content-Type", "application/json")
	foreign.Header.Set("Origin", "https://evil.test")
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, foreign)
	if denied.Code != 403 {
		t.Fatal("crop origin", denied.Code)
	}
	for _, bad := range []string{
		`null`, `{}`, `{"expected_revision":1}`, `{"expected_revision":null,"crop":null}`,
		`{"expected_revision":1.2,"crop":null}`, `{"expected_revision":1,"crop":{"x":0,"y":0,"size":1.5}}`,
		`{"expected_revision":1,"crop":{"x":0,"size":5}}`, `{"expected_revision":1,"crop":{"x":null,"y":0,"size":5}}`,
		`{"expected_revision":1,"crop":{"x":"0","y":0,"size":5}}`, `{"expected_revision":1,"crop":{"x":0,"y":0,"size":5,"width":20}}`,
		`{"expected_revision":1,"crop":{"x":9223372036854775808,"y":0,"size":5}}`,
		`{"expected_revision":1,"crop":{"x":19,"y":0,"size":5}}`, `{"expected_revision":1,"crop":null,"reset":true}`,
	} {
		if w := request(router, "PUT", cropPath, bad, cookie); w.Code != 400 {
			t.Fatal("invalid crop", bad, w.Code, w.Body)
		}
	}
	w = request(router, "PUT", cropPath, cropBody, cookie)
	if w.Code != 200 {
		t.Fatal("crop", w.Code, w.Body)
	}
	var updated Photo
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil || updated.ThumbnailRevision != 2 || updated.ThumbnailWidth != 5 || updated.Crop == nil {
		t.Fatal("crop response", updated, err)
	}
	if w := request(router, "PUT", cropPath, cropBody, cookie); w.Code != 200 {
		t.Fatal("crop retry", w.Code, w.Body)
	}
	if w := request(router, "PUT", cropPath, `{"expected_revision":1,"crop":null}`, cookie); w.Code != 409 {
		t.Fatal("stale crop", w.Code, w.Body)
	}
	p, err = repo.visiblePhoto(ctx, setup.OwnerID, setup.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.URL, p.ThumbnailURL} {
		if w := request(router, "GET", path, "", nil); w.Code != 401 {
			t.Fatal(w.Code)
		}
		w := request(router, "GET", path, "", cookie)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store, private" || w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("ETag") != "" || w.Header().Get("Last-Modified") != "" {
			t.Fatal(w.Code, w.Header())
		}
	}
	if w := upload(cookie, key, raw, ""); w.Code != 201 {
		t.Fatal("retry", w.Code, w.Body)
	}
	for _, bad := range [][]byte{[]byte("<svg/>"), []byte("bad"), make([]byte, photos.MaxBytes+1)} {
		if w := upload(cookie, "22222222-2222-4222-8222-222222222222", bad, ""); w.Code != 400 {
			t.Fatal("bad upload", w.Code, w.Body)
		}
	}
	provider.identity = users.Identity{Subject: "other", Email: "other@example.test", Name: "Other"}
	otherCookie := login(t, router)
	if w := request(router, "PUT", cropPath, `not JSON`, otherCookie); w.Code != 404 {
		t.Fatal("foreign crop before parsing", w.Code)
	}
	var otherID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE google_subject='other'`).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if w := upload(otherCookie, key, []byte("bad"), ""); w.Code != 404 {
		t.Fatal("owner before decode", w.Code)
	}
	for _, method := range []string{"PUT", "DELETE"} {
		path := base + "/" + p.ID
		if method == "PUT" {
			path += "/cover"
		}
		if w := request(router, method, path, "", otherCookie); w.Code != 404 {
			t.Fatal("foreign mutation", w.Code)
		}
	}
	approveFixture(t, pool, setup.ID)
	for _, path := range []string{p.URL, p.ThumbnailURL} {
		if w := request(router, "GET", path, "", otherCookie); w.Code != 200 {
			t.Fatal("public", w.Code)
		}
	}
	for _, visibility := range []Visibility{Private, Friends} {
		if _, err := service.Patch(ctx, setup.OwnerID, setup.ID, PatchInput{Visibility: NullableString{Present: true, Value: stringPtr(string(visibility))}}); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{p.URL, p.ThumbnailURL, base} {
			if w := request(router, "GET", path, "", otherCookie); w.Code != 404 {
				t.Fatal("visibility revoked", visibility, w.Code)
			}
		}
	}
	friendship, err := friendRepo.Create(ctx, setup.OwnerID, otherID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := friendRepo.Accept(ctx, otherID, friendship.ID); err != nil {
		t.Fatal(err)
	}
	approveFixture(t, pool, setup.ID)
	for _, path := range []string{p.URL, p.ThumbnailURL} {
		if w := request(router, "GET", path, "", otherCookie); w.Code != 200 {
			t.Fatal("friend read", w.Code)
		}
	}
	if err := friendRepo.Delete(ctx, otherID, friendship.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.URL, p.ThumbnailURL} {
		if w := request(router, "GET", path, "", otherCookie); w.Code != 404 {
			t.Fatal("unfriend", w.Code)
		}
	}
	// A failed cleanup stays in the durable queue, then converges on retry.
	if _, err := repo.mutatePhoto(ctx, setup.OwnerID, setup.ID, p.ID, true); err != nil {
		t.Fatal(err)
	}
	// Metadata deletion is committed before files disappear; unauthorized/missing read is 404.
	if w := request(router, "GET", p.URL, "", cookie); w.Code != 404 {
		t.Fatal(w.Code)
	}
	// Block the storage path deliberately to exercise a retryable cleanup failure.
	moved := root + "-away"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcilePhotos(ctx); err == nil {
		t.Fatal("cleanup failure not reported")
	}
	var queued int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM setup_photo_deletions").Scan(&queued); err != nil || queued != 2 {
		t.Fatal("cleanup lost queue", queued, err)
	}
	_ = os.Remove(root)
	if err := os.Rename(moved, root); err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcilePhotos(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, p.key)); !os.IsNotExist(err) {
		t.Fatal("delete left files", err)
	}
	if w := upload(cookie, key, raw, ""); w.Code != 409 {
		t.Fatal("deleted retry resurrected", w.Code, w.Body)
	}
	loaded, err := service.Get(ctx, setup.OwnerID, setup.ID)
	if err != nil || loaded.CoverPhoto != nil {
		t.Fatal("final cover", err)
	}
	// Both native HEIF and its JPEG export must survive multipart MIME/name
	// mismatches: the helper declares an SVG filename and octet-stream MIME.
	heifPath := "../photos/testdata/phone-format.heic"
	heif, err := os.ReadFile(heifPath)
	if err != nil {
		t.Fatal(err)
	}
	jpegPath := filepath.Join(t.TempDir(), "export.jpeg")
	if out, err := exec.Command("convert", "HEIC:"+heifPath+"[0]", "JPEG:"+jpegPath).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	jpeg, err := os.ReadFile(jpegPath)
	if err != nil {
		t.Fatal(err)
	}
	for i, raw := range [][]byte{heif, jpeg} {
		key := fmt.Sprintf("22222222-2222-4222-8222-%012d", i)
		w := upload(cookie, key, raw, "")
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body)
		}
		var photo Photo
		if err := json.Unmarshal(w.Body.Bytes(), &photo); err != nil {
			t.Fatal(err)
		}
		if photo.ContentType != "image/jpeg" {
			t.Fatal(photo)
		}
		if retry := upload(cookie, key, raw, ""); retry.Code != 201 {
			t.Fatal(retry.Code, retry.Body)
		}
	}
	w = upload(cookie, "33333333-3333-4333-8333-333333333333", []byte("not an image"), "")
	if w.Code != 400 || !bytes.Contains(w.Body.Bytes(), []byte("unsupported image format")) {
		t.Fatal(w.Code, w.Body)
	}
}

// Coherent full pg_dump/media-copy restore into a second isolated PostgreSQL and
// separate persistent directory. No production/staging data or live host is used.
func TestPhotoBackupRestore(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	repo := NewRepository(pool)
	owner, err := users.NewService(users.NewRepository(pool)).Login(ctx, users.Identity{Subject: "backup", Email: "backup@example.test", Name: "Backup"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := photos.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, chassis.NewService(chassis.NewRepository(pool)), friendships.NewRepository(pool))
	service.EnablePhotos(store)
	setup, err := service.Create(ctx, owner.ID, CreateInput{Title: "Backup", Visibility: Private, Data: realisticData(t)})
	if err != nil {
		t.Fatal(err)
	}
	p, err := service.uploadPhoto(ctx, owner.ID, setup.ID, "11111111-1111-4111-8111-111111111111", photoPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	revision := p.ThumbnailRevision
	p, err = service.applyCrop(ctx, owner.ID, setup.ID, p.ID, CropInput{&revision, &photos.Crop{X: 0, Y: 0, Size: 5}})
	if err != nil {
		t.Fatal(err)
	}
	dump, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "host", "postgres:17-alpine", "pg_dump", "--format=c", "--no-owner", "--no-acl", "-d", pool.Config().ConnConfig.ConnString()).Output()
	if err != nil {
		t.Fatal("backup", err)
	}
	restoredPool := dbtest.New(t)
	restore := exec.CommandContext(ctx, "docker", "run", "--rm", "-i", "--network", "host", "postgres:17-alpine", "pg_restore", "--clean", "--if-exists", "--no-owner", "--no-acl", "-d", restoredPool.Config().ConnConfig.ConnString())
	restore.Stdin = bytes.NewReader(dump)
	if out, err := restore.CombinedOutput(); err != nil {
		t.Fatal("restore", err, string(out))
	}
	restoredRoot := t.TempDir()
	restoredStore, err := photos.New(restoredRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(restoredRoot, p.key), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"full.jpg", "thumbnail.jpg"} {
		raw, err := os.ReadFile(filepath.Join(root, p.key, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(restoredRoot, p.key, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(restoredRoot, p.thumbnailKey), 0700); err != nil {
		t.Fatal(err)
	}
	cropped, err := os.ReadFile(filepath.Join(root, p.thumbnailKey, "thumbnail.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restoredRoot, p.thumbnailKey, "thumbnail.jpg"), cropped, 0600); err != nil {
		t.Fatal(err)
	}
	restored := NewService(NewRepository(restoredPool), chassis.NewService(chassis.NewRepository(restoredPool)), friendships.NewRepository(restoredPool))
	restored.EnablePhotos(restoredStore)
	if err := service.Delete(ctx, owner.ID, setup.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := restored.Get(ctx, owner.ID, setup.ID)
	if err != nil || loaded.CoverPhoto == nil || loaded.CoverPhoto.ID != p.ID || loaded.CoverPhoto.ThumbnailRevision != p.ThumbnailRevision || loaded.CoverPhoto.Crop == nil || *loaded.CoverPhoto.Crop != *p.Crop {
		t.Fatal("restored metadata", loaded, err)
	}
	for _, thumb := range []bool{false, true} {
		mediaKey := p.key
		if thumb {
			mediaKey = p.thumbnailKey
		}
		f, err := restoredStore.Open(mediaKey, thumb)
		if err != nil {
			t.Fatal("restored media", err)
		}
		_ = f.Close()
	}
	if err := restored.ReconcilePhotos(ctx); err != nil {
		t.Fatal(err)
	}
}
