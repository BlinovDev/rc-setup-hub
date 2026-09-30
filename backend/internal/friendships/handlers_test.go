package friendships_test

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

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/setups"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

type fakeGoogle struct{ identity users.Identity }

func (f *fakeGoogle) AuthorizationURL(state, nonce, verifier string) string {
	return "https://accounts.google.com/auth?state=" + url.QueryEscape(state)
}
func (f *fakeGoogle) Exchange(context.Context, string, string, string) (users.Identity, error) {
	return f.identity, nil
}
func cookieNamed(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func TestFriendshipHTTPAndVisibility(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	userService := users.NewService(users.NewRepository(pool))
	repo := friendships.NewRepository(pool)
	friendshipHandler := friendships.NewHandler(friendships.NewService(repo))
	catalog := chassis.NewService(chassis.NewRepository(pool))
	setupHandler := setups.NewHandler(setups.NewService(setups.NewRepository(pool), catalog, repo))
	provider := &fakeGoogle{}
	cfg := config.Auth{GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode}
	authentication := auth.NewHandler(cfg, provider, userService)
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, friendshipHandler.RegisterAPI, setupHandler.RegisterAPI, func(r chi.Router) { users.NewHandler(userService).RegisterAPI(r, auth.CurrentUser) })
	req := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("HTTP %d want %d: %s", w.Code, want, w.Body)
		}
	}
	decode := func(w *httptest.ResponseRecorder, want int, target any) {
		t.Helper()
		check(w, want)
		if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
	type account struct {
		user   users.User
		cookie *http.Cookie
	}
	login := func(name string) account {
		t.Helper()
		provider.identity = users.Identity{Subject: name, Email: "email-marker-" + strings.ToLower(name) + "@example.com", Name: name, AvatarURL: "https://example.com/avatar"}
		start := req("GET", "/auth/google", "", nil)
		location, err := url.Parse(start.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		callback := req("GET", "/auth/google/callback?code=fake&state="+url.QueryEscape(location.Query().Get("state")), "", cookieNamed(t, start, "rc_oauth_state"))
		check(callback, 303)
		cookie := cookieNamed(t, callback, "rc_session")
		var u users.User
		decode(req("GET", "/api/v1/me", "", cookie), 200, &u)
		return account{u, cookie}
	}
	a, b, c, d, e := login("Driver-A"), login("Driver-B"), login("Driver-C"), login("Driver-D"), login("Driver-E")
	unknown := "00000000-0000-0000-0000-000000000000"
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/users/search?q=Driver"}, {"GET", "/api/v1/friendships"}, {"POST", "/api/v1/friendships"}, {"POST", "/api/v1/friendships/" + unknown + "/accept"}, {"DELETE", "/api/v1/friendships/" + unknown}} {
		check(req(route.method, route.path, `{}`, nil), 401)
	}
	noSecrets := func(w *httptest.ResponseRecorder) {
		t.Helper()
		for _, field := range []string{"email", "google_subject", "rc_session"} {
			if strings.Contains(w.Body.String(), field) {
				t.Fatal("private field leaked", w.Body)
			}
		}
	}
	for _, query := range []string{"", "  ", "email-marker-driver-b", "%", "_"} {
		w := req("GET", "/api/v1/users/search?q="+url.QueryEscape(query), "", a.cookie)
		check(w, 200)
		if w.Body.String() != "[]\n" {
			t.Fatal("empty/email/literal search", w.Body)
		}
	}
	var profiles []users.PublicProfile
	w := req("GET", "/api/v1/users/search?q="+url.QueryEscape(" iVeR-b "), "", a.cookie)
	decode(w, 200, &profiles)
	noSecrets(w)
	if len(profiles) != 1 || profiles[0].ID != b.user.ID || profiles[0].Nickname != b.user.Nickname || profiles[0].AvatarURL == nil {
		t.Fatal("substring search", profiles)
	}
	decode(req("GET", "/api/v1/users/search?q=Driver", "", a.cookie), 200, &profiles)
	if len(profiles) != 4 {
		t.Fatal("self excluded", profiles)
	}
	for _, profile := range profiles {
		if profile.ID == a.user.ID {
			t.Fatal("self returned")
		}
	}
	for i := 24; i >= 0; i-- {
		if _, err := userService.Login(ctx, users.Identity{Subject: fmt.Sprintf("limit%d", i), Email: fmt.Sprintf("limit%d@example.com", i), Name: fmt.Sprintf("Limit%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	w = req("GET", "/api/v1/users/search?q=LiMiT", "", a.cookie)
	decode(w, 200, &profiles)
	noSecrets(w)
	if len(profiles) != 20 {
		t.Fatal("search limit", len(profiles))
	}
	for i, p := range profiles {
		if p.Nickname != fmt.Sprintf("Limit%02d", i) {
			t.Fatal("search order", i, p)
		}
	}
	if repeated := req("GET", "/api/v1/users/search?q=limit", "", a.cookie); repeated.Body.String() != w.Body.String() {
		t.Fatal("unstable search order")
	}
	check(req("GET", "/api/v1/users/search?q="+strings.Repeat("x", 65), "", a.cookie), 400)
	create := func(from, to account) friendships.Relationship {
		t.Helper()
		var r friendships.Relationship
		decode(req("POST", "/api/v1/friendships", `{"user_id":"`+to.user.ID+`"}`, from.cookie), 201, &r)
		return r
	}
	acceptPath := func(id string) string { return "/api/v1/friendships/" + id + "/accept" }
	deletePath := func(id string) string { return "/api/v1/friendships/" + id }
	list := func(user account) friendships.List {
		t.Helper()
		w := req("GET", "/api/v1/friendships", "", user.cookie)
		var list friendships.List
		decode(w, 200, &list)
		noSecrets(w)
		return list
	}
	initial := list(a)
	if initial.Incoming == nil || initial.Outgoing == nil || initial.Accepted == nil || len(initial.Incoming)+len(initial.Outgoing)+len(initial.Accepted) != 0 {
		t.Fatal("empty list", initial)
	}
	for _, body := range []string{`{"user_id":"` + a.user.ID + `"}`, `{"user_id":"invalid"}`, `{}`, `null`, `[]`, `{`, `{} {}`, `{"user_id":"` + b.user.ID + `","requester_id":"` + c.user.ID + `"}`} {
		check(req("POST", "/api/v1/friendships", body, a.cookie), 400)
	}
	check(req("POST", "/api/v1/friendships", `{"user_id":"`+unknown+`"}`, a.cookie), 404)
	for _, method := range []string{"POST", "DELETE"} {
		path := deletePath(unknown)
		if method == "POST" {
			path = acceptPath(unknown)
		}
		check(req(method, path, "", a.cookie), 404)
		path = strings.Replace(path, unknown, "invalid", 1)
		check(req(method, path, "", a.cookie), 400)
	}
	// Primary visibility integration uses only real HTTP friendship transitions.
	var examples []setups.Setup
	for _, visibility := range []string{"public", "friends", "private"} {
		var s setups.Setup
		decode(req("POST", "/api/v1/setups", `{"title":"`+visibility+`","visibility":"`+visibility+`","data":{}}`, a.cookie), 201, &s)
		examples = append(examples, s)
	}
	setupPath := func(s setups.Setup) string { return "/api/v1/setups/" + s.ID }
	assertAccess := func(friendsAllowed bool) {
		t.Helper()
		check(req("GET", setupPath(examples[0]), "", b.cookie), 200)
		want := 404
		if friendsAllowed {
			want = 200
		}
		check(req("GET", setupPath(examples[1]), "", b.cookie), want)
		check(req("GET", setupPath(examples[2]), "", b.cookie), 404)
	}
	assertAccess(false)
	relationship := create(a, b)
	if relationship.RequesterID != a.user.ID || relationship.AddresseeID != b.user.ID || relationship.Status != friendships.Pending {
		t.Fatal("request owner/state", relationship)
	}
	assertAccess(false)
	check(req("POST", "/api/v1/friendships", `{"user_id":"`+b.user.ID+`"}`, a.cookie), 409)
	check(req("POST", "/api/v1/friendships", `{"user_id":"`+a.user.ID+`"}`, b.cookie), 409)
	outgoing, incoming := list(a), list(b)
	if len(outgoing.Outgoing) != 1 || outgoing.Outgoing[0].User.ID != b.user.ID || len(incoming.Incoming) != 1 || incoming.Incoming[0].User.ID != a.user.ID {
		t.Fatal("pending directions", outgoing, incoming)
	}
	check(req("POST", acceptPath(relationship.ID), "", a.cookie), 404)
	check(req("POST", acceptPath(relationship.ID), "", c.cookie), 404)
	check(req("DELETE", deletePath(relationship.ID), "", c.cookie), 404)
	var accepted friendships.Relationship
	decode(req("POST", acceptPath(relationship.ID), "", b.cookie), 200, &accepted)
	if accepted.Status != friendships.Accepted || !accepted.UpdatedAt.After(relationship.UpdatedAt) {
		t.Fatal("accepted timestamps", accepted)
	}
	check(req("POST", acceptPath(relationship.ID), "", b.cookie), 409)
	check(req("POST", acceptPath(relationship.ID), "", a.cookie), 404)
	check(req("POST", acceptPath(relationship.ID), "", c.cookie), 404)
	check(req("POST", "/api/v1/friendships", `{"user_id":"`+b.user.ID+`"}`, a.cookie), 409)
	assertAccess(true)
	var visible []setups.Setup
	decode(req("GET", "/api/v1/users/"+a.user.ID+"/setups", "", b.cookie), 200, &visible)
	if len(visible) != 2 {
		t.Fatal("accepted setup list", visible)
	}
	for _, s := range examples {
		check(req("PATCH", setupPath(s), `{"title":"Forbidden"}`, b.cookie), 404)
		check(req("DELETE", setupPath(s), "", b.cookie), 404)
	}
	check(req("DELETE", deletePath(relationship.ID), "", c.cookie), 404)
	check(req("DELETE", deletePath(relationship.ID), "", b.cookie), 204)
	assertAccess(false)
	decode(req("GET", "/api/v1/users/"+a.user.ID+"/setups", "", b.cookie), 200, &visible)
	if len(visible) != 1 || visible[0].ID != examples[0].ID {
		t.Fatal("removed list", visible)
	}
	// Pending cancel/reject and accepted removal by either participant.
	for _, tc := range []struct {
		name     string
		accepted bool
		deleter  account
	}{{"cancel", false, a}, {"reject", false, b}, {"remove requester", true, a}, {"remove addressee", true, b}} {
		t.Run(tc.name, func(t *testing.T) {
			r := create(a, b)
			if tc.accepted {
				check(req("POST", acceptPath(r.ID), "", b.cookie), 200)
			}
			check(req("DELETE", deletePath(r.ID), "", c.cookie), 404)
			check(req("DELETE", deletePath(r.ID), "", tc.deleter.cookie), 204)
			check(req("DELETE", deletePath(r.ID), "", tc.deleter.cookie), 404)
			if _, err := repo.GetParticipant(ctx, a.user.ID, r.ID); err != friendships.ErrNotFound {
				t.Fatal("row remains", err)
			}
		})
	}
	// Listing combines both accepted orientations with incoming and outgoing pending.
	out := create(a, b)
	in := create(c, a)
	acceptedOut := create(a, d)
	check(req("POST", acceptPath(acceptedOut.ID), "", d.cookie), 200)
	acceptedIn := create(e, a)
	check(req("POST", acceptPath(acceptedIn.ID), "", a.cookie), 200)
	grouped := list(a)
	if len(grouped.Incoming) != 1 || len(grouped.Outgoing) != 1 || len(grouped.Accepted) != 2 {
		t.Fatal("groups", grouped)
	}
	if grouped.Incoming[0].ID != in.ID || grouped.Incoming[0].User.ID != c.user.ID || grouped.Outgoing[0].ID != out.ID || grouped.Outgoing[0].User.ID != b.user.ID || grouped.Accepted[0].ID != acceptedIn.ID || grouped.Accepted[0].User.ID != e.user.ID || grouped.Accepted[1].ID != acceptedOut.ID || grouped.Accepted[1].User.ID != d.user.ID {
		t.Fatal("other profile/order", grouped)
	}
	firstList := req("GET", "/api/v1/friendships", "", a.cookie)
	secondList := req("GET", "/api/v1/friendships", "", a.cookie)
	if firstList.Body.String() != secondList.Body.String() {
		t.Fatal("unstable list")
	}
	// Inaccessible corrupt documents still remain concealed after friendship state changes.
	if _, err := pool.Exec(ctx, `UPDATE setups SET schema_version=2,data='[]'::jsonb WHERE id=$1`, examples[1].ID); err != nil {
		t.Fatal(err)
	}
	missing := req("GET", "/api/v1/setups/"+unknown, "", b.cookie)
	hidden := req("GET", setupPath(examples[1]), "", b.cookie)
	check(hidden, 404)
	if hidden.Body.String() != missing.Body.String() {
		t.Fatal("setup boundary weakened")
	}
	pool.Close()
	for _, path := range []string{"/api/v1/friendships", "/api/v1/users/search?q=Driver"} {
		w := req("GET", path, "", a.cookie)
		check(w, 500)
		if strings.Contains(w.Body.String(), "closed pool") {
			t.Fatal("database error leaked", w.Body)
		}
	}
}
