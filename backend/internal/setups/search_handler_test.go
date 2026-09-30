package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
)

func TestSearchHTTP(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	provider := &fakeGoogle{}
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(f.pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), f.pool.Ping)
	authentication.Register(router, NewHandler(f.service).RegisterAPI, friendships.NewHandler(friendships.NewService(friendships.NewRepository(f.pool))).RegisterAPI)
	signIn := func(subject, email string) *http.Cookie {
		t.Helper()
		provider.identity = users.Identity{Subject: subject, Email: email, Name: subject}
		return login(t, router)
	}
	ownerCookie := signIn("search-owner", "email-marker@example.com")
	friendCookie := signIn("search-friend", "friend@example.com")
	otherCookie := signIn("search-other", "other@example.com")
	check := func(wCode, want int, body string) {
		t.Helper()
		if wCode != want {
			t.Fatalf("HTTP %d want %d: %s", wCode, want, body)
		}
	}
	search := func(query string, cookie *http.Cookie) SearchPage {
		t.Helper()
		w := request(router, "GET", "/api/v1/setups/search"+query, "", cookie)
		check(w.Code, 200, w.Body.String())
		for _, forbidden := range []string{`"email"`, `"google_subject"`, `"data"`, `"notes"`, `"session"`, `"oauth"`, "email-marker", "technical-marker", "notes-marker", "rc_session"} {
			if strings.Contains(w.Body.String(), forbidden) {
				t.Fatal("search privacy", w.Body)
			}
		}
		var page SearchPage
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Items == nil {
			t.Fatal("items must be an array")
		}
		for _, item := range page.Items {
			if item.Visibility != Public {
				t.Fatal("visibility leak", item)
			}
		}
		return page
	}
	w := request(router, "GET", "/api/v1/setups/search", "", nil)
	check(w.Code, 401, w.Body.String())
	for _, query := range []string{"?limit=0", "?limit=-1", "?limit=51", "?limit=abc", "?limit=", "?limit=99999999999999999999", "?cursor=garbage", "?cursor=", "?brand_id=bad", "?model_id=bad", "?brand_id=", "?model_id=", "?q=" + strings.Repeat("a", 101), "?q=%00", "?q=%FF", "?q=%zz", "?q=one&q=two", "?limit=1&limit=2"} {
		w := request(router, "GET", "/api/v1/setups/search"+query, "", ownerCookie)
		check(w.Code, 400, w.Body.String())
	}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", sortedIDs(f.public)}, {"?q=+", sortedIDs(f.public)},
		{"?q=+CaRpEt+", []string{f.public[3].ID, f.public[0].ID}},
		{"?q=bLiNoV", []string{f.public[3].ID, f.public[1].ID, f.public[0].ID}},
		{"?q=email-marker", []string{}}, {"?q=notes-marker", []string{}}, {"?q=technical-marker", []string{}},
		{"?q=%25", []string{f.public[1].ID}}, {"?q=_", []string{f.public[2].ID}},
		{"?brand_id=" + f.yokomo.ID, []string{f.public[1].ID, f.public[0].ID}},
		{"?model_id=" + f.rd.ID, []string{f.public[0].ID}},
		{"?brand_id=" + f.mst.ID, []string{f.public[2].ID}},
		{"?brand_id=" + f.yokomo.ID + "&model_id=" + f.rmx.ID, []string{}},
		{"?brand_id=00000000-0000-0000-0000-000000000000", []string{}},
		{"?model_id=00000000-0000-0000-0000-000000000000", []string{}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			page := search(tc.query, ownerCookie)
			if !reflect.DeepEqual(searchIDs(page.Items), tc.want) || page.NextCursor != nil {
				t.Fatal(page, tc.want)
			}
		})
	}
	empty := request(router, "GET", "/api/v1/setups/search?q=no-such-match", "", ownerCookie)
	if empty.Body.String() != "{\"items\":[],\"next_cursor\":null}\n" {
		t.Fatal("empty contract", empty.Body)
	}
	publicOnly := func() {
		t.Helper()
		for _, cookie := range []*http.Cookie{ownerCookie, friendCookie, otherCookie} {
			page := search("?q=rd2&model_id="+f.rd.ID, cookie)
			if len(page.Items) != 1 || page.Items[0].ID != f.public[0].ID {
				t.Fatal("non-public search leak", page)
			}
		}
	}
	publicOnly()
	// Pending and accepted relationships cannot broaden discovery, even for the owner.
	w = request(router, "POST", "/api/v1/friendships", `{"user_id":"`+f.friend.ID+`"}`, ownerCookie)
	check(w.Code, 201, w.Body.String())
	var relationship friendships.Relationship
	if err := json.Unmarshal(w.Body.Bytes(), &relationship); err != nil {
		t.Fatal(err)
	}
	publicOnly()
	w = request(router, "POST", "/api/v1/friendships/"+relationship.ID+"/accept", "", friendCookie)
	check(w.Code, 200, w.Body.String())
	publicOnly()
	// Focused regressions: detail remains visibility-aware, mutations remain owner-only.
	setupResponse(t, request(router, "GET", "/api/v1/setups/"+f.public[0].ID, "", otherCookie), 200)
	setupResponse(t, request(router, "GET", "/api/v1/setups/"+f.friends.ID, "", friendCookie), 200)
	w = request(router, "GET", "/api/v1/setups/"+f.private.ID, "", friendCookie)
	check(w.Code, 404, w.Body.String())
	for _, cookie := range []*http.Cookie{friendCookie, otherCookie} {
		for _, s := range []Setup{f.public[0], f.friends, f.private} {
			for _, method := range []string{"PATCH", "DELETE"} {
				w := request(router, method, "/api/v1/setups/"+s.ID, `{"title":"Forbidden"}`, cookie)
				check(w.Code, 404, w.Body.String())
			}
		}
	}
	setupResponse(t, request(router, "PATCH", "/api/v1/setups/"+f.private.ID, `{"notes":"Owner update"}`, ownerCookie), 200)
	for _, path := range []string{"/api/v1/me/setups", "/api/v1/users/" + f.owner.ID + "/setups"} {
		w := request(router, "GET", path, "", ownerCookie)
		check(w.Code, 200, w.Body.String())
		var rows []Setup
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || len(rows) != 5 {
			t.Fatal("owner list regression", rows, err)
		}
	}
	w = request(router, "GET", "/api/v1/users/"+f.owner.ID+"/setups", "", friendCookie)
	check(w.Code, 200, w.Body.String())
	var visible []Setup
	if err := json.Unmarshal(w.Body.Bytes(), &visible); err != nil || len(visible) != 4 {
		t.Fatal("friend list regression", visible, err)
	}
	if _, err := f.catalog.SetModelActive(ctx, f.rd.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.catalog.SetBrandActive(ctx, f.yokomo.ID, false); err != nil {
		t.Fatal(err)
	}
	historical := search("?brand_id="+f.yokomo.ID+"&model_id="+f.rd.ID, otherCookie)
	if len(historical.Items) != 1 || historical.Items[0].Chassis.ModelName != "RD2.0" || historical.Items[0].Chassis.BrandName != "Yokomo" {
		t.Fatal("historical names/filter", historical)
	}
	// Corrupt hidden rows do not reach the summary query or reveal themselves.
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET schema_version=2,data='[]'::jsonb WHERE visibility<>'public'`); err != nil {
		t.Fatal(err)
	}
	publicOnly()
	missing := request(router, "GET", "/api/v1/setups/00000000-0000-0000-0000-000000000000", "", otherCookie)
	private := request(router, "GET", "/api/v1/setups/"+f.private.ID, "", otherCookie)
	if missing.Code != 404 || private.Code != 404 || missing.Body.String() != private.Body.String() {
		t.Fatal("hardened detail regression", private.Body)
	}
	// HTTP defaults, cursor parsing and final-page null; every expected item appears once.
	expected := append([]Setup(nil), f.public...)
	for i := 0; i < 21; i++ {
		s, err := f.service.Create(ctx, f.owner.ID, CreateInput{Title: fmt.Sprintf("Paging %02d", i), Visibility: Public, Data: &DataV1{}})
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, s)
	}
	first := search("", otherCookie)
	if len(first.Items) != 20 || first.NextCursor == nil {
		t.Fatal("HTTP default limit", first)
	}
	second := search("?cursor="+url.QueryEscape(*first.NextCursor), otherCookie)
	if len(second.Items) != 5 || second.NextCursor != nil {
		t.Fatal("HTTP last page", second)
	}
	combined := append(searchIDs(first.Items), searchIDs(second.Items)...)
	if !reflect.DeepEqual(combined, sortedIDs(expected)) {
		t.Fatal("HTTP pagination missing/order/duplicates", combined)
	}
	single := search("?limit=1", ownerCookie)
	if len(single.Items) != 1 || single.NextCursor == nil {
		t.Fatal("custom limit", single)
	}
	limited := search("?limit=50", ownerCookie)
	if len(limited.Items) != 25 || limited.NextCursor != nil {
		t.Fatal("max limit", limited)
	}
	// Owner deletion still works even for its now-corrupt private document.
	w = request(router, "DELETE", "/api/v1/setups/"+f.private.ID, "", ownerCookie)
	check(w.Code, 204, w.Body.String())
	f.pool.Close()
	w = request(router, "GET", "/api/v1/setups/search", "", ownerCookie)
	check(w.Code, 500, w.Body.String())
	if strings.Contains(w.Body.String(), "closed pool") {
		t.Fatal("DB error leaked", w.Body)
	}
}
