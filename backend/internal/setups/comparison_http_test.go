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
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

func TestComparisonAndVisibleHTTP(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("c"), 32), SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
	provider := &fakeGoogle{}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(f.pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), f.pool.Ping)
	handler := NewHandler(f.service)
	authentication.RegisterWithPublic(router, []func(chi.Router){handler.RegisterPublicAPI}, handler.RegisterPrivateAPI)
	signIn := func(subject string, u users.User) *http.Cookie {
		t.Helper()
		provider.identity = users.Identity{Subject: subject, Email: u.Email, Name: u.Nickname}
		return login(t, router)
	}
	owner, friend, other := signIn("search-owner", f.owner), signIn("search-friend", f.friend), signIn("search-other", f.other)
	if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, f.owner.ID, f.friend.ID); err != nil {
		t.Fatal(err)
	}
	path := func(left, right string) string {
		return "/api/v1/setups/compare?left_id=" + left + "&right_id=" + right
	}
	for _, tc := range []struct {
		name        string
		cookie      *http.Cookie
		count       int
		permissions [3]int
	}{
		{"guest", nil, 4, [3]int{200, 404, 404}}, {"invalid session", &http.Cookie{Name: "rc_session", Value: "invalid"}, 4, [3]int{200, 404, 404}},
		{"owner", owner, 6, [3]int{200, 200, 200}}, {"friend", friend, 5, [3]int{200, 200, 404}}, {"other", other, 4, [3]int{200, 404, 404}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := request(router, "GET", "/api/v1/setups/visible", "", tc.cookie)
			var page SearchPage
			if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != tc.count {
				t.Fatal("selector", response.Code, response.Body)
			}
			for _, private := range []string{"email", "google_subject", "notes", "data", "storage_key", "thumbnail_key", "total", "private_count"} {
				if strings.Contains(response.Body.String(), `"`+private+`"`) {
					t.Fatal("unsafe selector projection", private, response.Body)
				}
			}
			for i, record := range []Setup{f.public[0], f.friends, f.private} {
				for _, pair := range [][2]string{{record.ID, f.public[2].ID}, {f.public[2].ID, record.ID}} {
					w := request(router, "GET", path(pair[0], pair[1]), "", tc.cookie)
					if w.Code != tc.permissions[i] || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Robots-Tag") == "index, follow" {
						t.Fatal("compare permission/headers", w.Code, w.Header(), w.Body)
					}
					if w.Code == 200 {
						var result Comparison
						if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Left.ID != pair[0] || result.Right.ID != pair[1] {
							t.Fatal("pair order", w.Body)
						}
						for _, private := range []string{"email", "google_subject", "storage_key", "thumbnail_key"} {
							if strings.Contains(w.Body.String(), `"`+private+`"`) {
								t.Fatal("unsafe comparison", private)
							}
						}
					} else {
						missing := request(router, "GET", path(pair[0], "00000000-0000-0000-0000-000000000000"), "", tc.cookie)
						if missing.Code != 404 || missing.Body.String() != w.Body.String() {
							t.Fatal("inaccessible/missing distinction", missing.Code, missing.Body, w.Body)
						}
					}
				}
			}
			explore := request(router, "GET", "/api/v1/setups/search", "", tc.cookie)
			if explore.Code != 200 || json.Unmarshal(explore.Body.Bytes(), &page) != nil || len(page.Items) != 4 {
				t.Fatal("Explore widened", explore.Code, explore.Body)
			}
		})
	}
	for _, query := range []string{"", "left_id=" + f.public[0].ID, "left_id=&right_id=" + f.public[1].ID,
		"left_id=bad&right_id=" + f.public[1].ID, "left_id=" + f.public[0].ID + "&right_id=bad",
		"left_id=" + f.public[0].ID + "&right_id=" + strings.ToUpper(f.public[0].ID),
		"left_id=" + f.public[0].ID + "&right_id=" + f.public[1].ID + "&left_id=" + f.public[2].ID,
		"left_id=" + f.public[0].ID + "&right_id=" + f.public[1].ID + "&extra=third", "left_id=%zz&right_id=bad",
	} {
		w := request(router, "GET", "/api/v1/setups/compare?"+query, "", nil)
		if w.Code != 400 {
			t.Fatal("invalid comparison", query, w.Code, w.Body)
		}
	}
	spoofed := request(router, "GET", "/api/v1/setups/visible?viewer_id="+f.owner.ID+"&owner_id="+f.owner.ID, "", nil)
	var safe SearchPage
	if spoofed.Code != 200 || json.Unmarshal(spoofed.Body.Bytes(), &safe) != nil || len(safe.Items) != 4 {
		t.Fatal("caller-supplied identity changed scope", spoofed.Code, spoofed.Body)
	}
	for _, query := range []string{"limit=0", "limit=51", "q=%00", "q=" + strings.Repeat("a", 101), "limit=1&limit=2", "q=a&q=b", "brand_id=bad", "cursor=bad", "cursor=", "q=%zz"} {
		if w := request(router, "GET", "/api/v1/setups/visible?"+query, "", nil); w.Code != 400 {
			t.Fatal("invalid selection", query, w.Code, w.Body)
		}
	}
	for _, scope := range []string{"mine", "friends"} {
		if w := request(router, "GET", "/api/v1/setups/visible?scope="+scope, "", nil); w.Code != 401 {
			t.Fatal("guest personal scope", w.Code, w.Body)
		}
	}
	for _, query := range []string{"scope=", "scope=unknown", "scope=mine&scope=friends"} {
		if w := request(router, "GET", "/api/v1/setups/visible?"+query, "", owner); w.Code != 400 {
			t.Fatal("invalid scope", w.Code, w.Body)
		}
	}
	for _, method := range []string{"PATCH", "DELETE"} {
		if w := request(router, method, "/api/v1/setups/"+f.public[0].ID, `{"title":"forbidden"}`, friend); w.Code != 404 {
			t.Fatal("foreign mutation", method, w.Code, w.Body)
		}
		if w := request(router, method, "/api/v1/setups/"+f.public[0].ID, `{"title":"forbidden"}`, nil); w.Code != 401 {
			t.Fatal("anonymous mutation", method, w.Code, w.Body)
		}
	}
	foreign := httptest.NewRequest("PATCH", "/api/v1/setups/"+f.public[0].ID, strings.NewReader(`{"title":"forbidden"}`))
	foreign.AddCookie(owner)
	foreign.Header.Set("Origin", "https://evil.invalid")
	foreign.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, foreign)
	if w.Code != 403 {
		t.Fatal("origin bypass", w.Code)
	}
	// Comparison remains read-only; explicit cloning is a separate authenticated mutation.
	var receipts int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM setup_clone_requests`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatal("comparison created clone receipts", receipts, err)
	}
	body := `{"request_id":"00000000-0000-4000-8000-000000000001"}`
	for _, id := range []string{f.friends.ID, f.private.ID} {
		w := request(router, "POST", "/api/v1/setups/"+id+"/clone", body, friend)
		if w.Code != 404 {
			t.Fatal("comparison granted non-public cloning", w.Code, w.Body)
		}
	}
	if w := request(router, "POST", "/api/v1/setups/"+f.public[0].ID+"/clone", body, nil); w.Code != 401 {
		t.Fatal("comparison granted anonymous cloning", w.Code, w.Body)
	}
	for _, method := range []string{"POST", "PATCH", "DELETE", "HEAD"} {
		for _, p := range []string{"/api/v1/setups/visible", path(f.public[0].ID, f.public[1].ID)} {
			if w := request(router, method, p, "", nil); w.Code == 200 || w.Code == 201 {
				t.Fatal("non-GET public route", method, p, w.Code)
			}
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET visibility='private' WHERE id=$1`, f.public[0].ID); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "GET", path(f.public[0].ID, f.public[1].ID), "", nil); w.Code != 404 || strings.Contains(w.Body.String(), f.public[1].Title) {
		t.Fatal("partial response after revocation", w.Code, w.Body)
	}
	if err := f.service.Delete(ctx, f.owner.ID, f.public[1].ID); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "GET", path(f.public[0].ID, f.public[1].ID), "", owner); w.Code != 404 {
		t.Fatal("deleted pair", w.Code, w.Body)
	}
	f.pool.Close()
	if w := request(router, "GET", path(f.public[2].ID, f.public[3].ID), "", owner); w.Code != 500 || strings.Contains(w.Body.String(), "postgres") {
		t.Fatal("identity failure downgraded/leaked", w.Code, w.Body)
	}
}
