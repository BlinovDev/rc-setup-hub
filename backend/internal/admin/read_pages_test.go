package admin

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminReadPages(t *testing.T) {
	f := newReadFixture(t)
	ctx := context.Background()
	c := settings()
	c.AdminEmails = []string{"admin@example.com"}
	service := users.NewService(users.NewRepository(f.pool))
	provider := &fakeGoogle{identity: users.Identity{Subject: "private-google-subject-admin", Email: "admin@example.com", Name: "Admin"}}
	authentication := auth.NewHandler(c, provider, service)
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), f.pool.Ping)
	authentication.Register(router)
	h, err := New(c, f.repo)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := chassis.NewHandler(chassis.NewService(chassis.NewRepository(f.pool)))
	if err != nil {
		t.Fatal(err)
	}
	h.Register(router, authentication.RequireUser, catalog.RegisterAdmin)
	for _, path := range []string{"/admin", "/admin/users"} {
		if w := call(router, path, nil); w.Code != 401 {
			t.Fatal("unauthenticated", path, w.Code)
		}
	}
	adminCookie := login(t, router)
	provider.identity = users.Identity{Subject: "private-google-subject-other", Email: "other@example.com", Name: "Other"}
	ordinaryCookie := login(t, router)
	for _, path := range []string{"/admin", "/admin/users"} {
		if w := call(router, path, ordinaryCookie); w.Code != 403 {
			t.Fatal("non-admin", path, w.Code)
		}
	}
	assertSafe := func(path string, cookie *http.Cookie) string {
		t.Helper()
		w := call(router, path, cookie)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatal("HTML response", path, w.Code, w.Header(), w.Body)
		}
		for _, private := range []string{"google_subject", "private-google-subject", string(c.SessionSecret), "access_token", "id_token", "rc_session", "<script>"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("private/unsafe HTML", path, private, w.Body)
			}
		}
		for _, link := range []string{`href="/admin/users"`, `href="/admin/chassis/brands"`, `href="/admin/chassis/models"`} {
			if !strings.Contains(w.Body.String(), link) {
				t.Fatal("navigation missing", path, link)
			}
		}
		return w.Body.String()
	}
	dashboard := assertSafe("/admin", adminCookie)
	for _, fragment := range []string{"<dt>Total users</dt><dd>3</dd>", "<dt>Total setups</dt><dd>8</dd>", "<dt>Public setups</dt><dd>3</dd>"} {
		if !strings.Contains(dashboard, fragment) {
			t.Fatal("dashboard count", fragment, dashboard)
		}
	}
	page := assertSafe("/admin/users", adminCookie)
	rows, err := f.repo.Users(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expected := append([]UserRow(nil), rows...)
	sort.Slice(expected, func(i, j int) bool {
		if expected[i].CreatedAt.Equal(expected[j].CreatedAt) {
			return expected[i].ID > expected[j].ID
		}
		return expected[i].CreatedAt.After(expected[j].CreatedAt)
	})
	last := -1
	for _, row := range expected {
		escaped := template.HTMLEscapeString(row.Nickname)
		position := strings.Index(page, "<td>"+escaped+"</td>")
		if position < 0 || position <= last {
			t.Fatal("HTML user/order", row, page)
		}
		last = position
		end := strings.Index(page[position:], "</tr>")
		if end < 0 {
			t.Fatal("missing row")
		}
		htmlRow := page[position : position+end]
		count := "0"
		if row.SetupCount == 6 {
			count = "6"
		} else if row.SetupCount == 2 {
			count = "2"
		}
		for _, value := range []string{template.HTMLEscapeString(row.Email), row.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), "<td>" + count + "</td>"} {
			if !strings.Contains(htmlRow, value) {
				t.Fatal("row field", row, value, htmlRow)
			}
		}
	}
	// Existing chassis routes still render through the same auth/admin/CSRF chain.
	assertSafe("/admin/chassis/brands", adminCookie)
	assertSafe("/admin/chassis/models", adminCookie)
	originalTemplate := h.template
	for _, tc := range []struct{ path, name string }{{"/admin", "index.html"}, {"/admin/users", "users.html"}} {
		h.template = template.Must(template.New(tc.name).Parse(`partial private HTML {{.MissingField}}`))
		w := call(router, tc.path, adminCookie)
		if w.Code != 500 || w.Body.String() != "Internal Server Error\n" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("template error/partial HTML", tc.path, w.Code, w.Body)
		}
	}
	h.template = originalTemplate
	// Keep session resolution healthy while the admin repository's connection pool fails.
	failedPool, err := pgxpool.NewWithConfig(ctx, f.pool.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	failedPool.Close()
	h.repo = NewRepository(failedPool)
	for _, path := range []string{"/admin", "/admin/users"} {
		w := call(router, path, adminCookie)
		if w.Code != 500 || w.Body.String() != "Internal Server Error\n" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("DB error leaked", path, w.Code, w.Body)
		}
	}
}
