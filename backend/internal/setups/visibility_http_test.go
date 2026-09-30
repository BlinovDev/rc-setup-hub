package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

func TestVisibilityHTTP(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(NewRepository(pool), catalog, friendships.NewRepository(pool))
	cfg := config.Auth{GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode}
	provider := &fakeGoogle{}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	type account struct {
		id     string
		cookie *http.Cookie
	}
	signIn := func(name string) account {
		t.Helper()
		provider.identity = users.Identity{Subject: name, Email: name + "@example.com", Name: name}
		cookie := login(t, router)
		w := request(router, "GET", "/api/v1/me", "", cookie)
		var user users.User
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
			t.Fatal(err)
		}
		return account{user.ID, cookie}
	}
	owner, friend, reverse, pending, other := signIn("owner"), signIn("friend"), signIn("reverse"), signIn("pending"), signIn("other")
	for _, f := range []struct{ a, b, status string }{{owner.id, friend.id, "accepted"}, {reverse.id, owner.id, "accepted"}, {owner.id, pending.id, "pending"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,$3)`, f.a, f.b, f.status); err != nil {
			t.Fatal(err)
		}
	}
	var setups []Setup
	for i, visibility := range []Visibility{Public, Friends, Private} {
		body := fmt.Sprintf(`{"title":"%s","visibility":"%s","data":{}}`, visibility, visibility)
		setup := setupResponse(t, request(router, "POST", "/api/v1/setups", body, owner.cookie), 201)
		setups = append(setups, setup)
		if _, err := pool.Exec(ctx, `UPDATE setups SET created_at=$2::timestamptz WHERE id=$1`, setup.ID, fmt.Sprintf("2020-01-%02dT00:00:00Z", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	listPath := "/api/v1/users/" + owner.id + "/setups"
	assertList := func(cookie *http.Cookie, path string, want []string) {
		t.Helper()
		w := request(router, "GET", path, "", cookie)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		var list []Setup
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if len(list) != len(want) {
			t.Fatalf("list %s want %v", w.Body, want)
		}
		for i, s := range list {
			if s.ID != want[i] {
				t.Fatalf("list order/visibility %s want %v", w.Body, want)
			}
		}
	}
	for _, caller := range []struct {
		name    string
		account account
		codes   [3]int
		list    []string
	}{
		{"owner", owner, [3]int{200, 200, 200}, []string{setups[2].ID, setups[1].ID, setups[0].ID}},
		{"accepted outgoing", friend, [3]int{200, 200, 404}, []string{setups[1].ID, setups[0].ID}},
		{"accepted incoming", reverse, [3]int{200, 200, 404}, []string{setups[1].ID, setups[0].ID}},
		{"unrelated", other, [3]int{200, 404, 404}, []string{setups[0].ID}},
		{"pending", pending, [3]int{200, 404, 404}, []string{setups[0].ID}},
		{"unauthenticated", account{}, [3]int{401, 401, 401}, nil},
	} {
		t.Run(caller.name, func(t *testing.T) {
			for i, s := range setups {
				t.Run(string(s.Visibility), func(t *testing.T) {
					w := request(router, "GET", "/api/v1/setups/"+s.ID, "", caller.account.cookie)
					if w.Code != caller.codes[i] {
						t.Fatalf("got %d want %d: %s", w.Code, caller.codes[i], w.Body)
					}
					if w.Code == 200 {
						got := setupResponse(t, w, 200)
						if got.ID != s.ID {
							t.Fatal("wrong setup")
						}
					}
				})
			}
			if caller.account.cookie == nil {
				for _, path := range []string{listPath, "/api/v1/me/setups"} {
					if w := request(router, "GET", path, "", nil); w.Code != 401 {
						t.Fatal("unauthenticated list", w.Code)
					}
				}
			} else {
				assertList(caller.account.cookie, listPath, caller.list)
			}
		})
	}
	assertList(owner.cookie, "/api/v1/me/setups", []string{setups[2].ID, setups[1].ID, setups[0].ID})
	// UUID spelling must not make an owner appear unrelated on the user list.
	assertList(owner.cookie, "/api/v1/users/"+strings.ToUpper(owner.id)+"/setups", []string{setups[2].ID, setups[1].ID, setups[0].ID})
	unknown := "00000000-0000-0000-0000-000000000000"
	missing := request(router, "GET", "/api/v1/setups/"+unknown, "", other.cookie)
	private := request(router, "GET", "/api/v1/setups/"+setups[2].ID, "", other.cookie)
	if missing.Code != 404 || private.Code != 404 || missing.Body.String() != private.Body.String() {
		t.Fatal("inaccessible detail reveals existence", missing, private)
	}
	assertList(other.cookie, "/api/v1/users/"+unknown+"/setups", nil)
	if w := request(router, "GET", "/api/v1/users/invalid/setups", "", owner.cookie); w.Code != 400 {
		t.Fatal("invalid target", w.Code)
	}
	// The pending fixture becomes accepted without implementing any friendship API.
	friendsPath := "/api/v1/setups/" + setups[1].ID
	if _, err := pool.Exec(ctx, `UPDATE friendships SET status='accepted',updated_at=now() WHERE requester_id=$1 AND addressee_id=$2`, owner.id, pending.id); err != nil {
		t.Fatal(err)
	}
	setupResponse(t, request(router, "GET", friendsPath, "", pending.cookie), 200)
	assertList(pending.cookie, listPath, []string{setups[1].ID, setups[0].ID})
	if _, err := pool.Exec(ctx, `DELETE FROM friendships WHERE requester_id=$1 AND addressee_id=$2`, owner.id, pending.id); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "GET", friendsPath, "", pending.cookie); w.Code != 404 {
		t.Fatal("removed friendship still grants access", w.Code)
	}
	assertList(pending.cookie, listPath, []string{setups[0].ID})
	// Inaccessible documents must be denied before schema checks or typed decoding.
	for _, corruption := range []struct {
		name    string
		version int
		data    string
	}{
		{"unsupported schema", 2, `{}`},
		{"undecodable data", 1, `{"electronics":{"motor":42}}`},
	} {
		for _, setup := range setups[1:] {
			t.Run(corruption.name+"/"+string(setup.Visibility), func(t *testing.T) {
				if _, err := pool.Exec(ctx, `UPDATE setups SET schema_version=$2,data=$3::jsonb WHERE id=$1`, setup.ID, corruption.version, corruption.data); err != nil {
					t.Fatal(err)
				}
				path := "/api/v1/setups/" + setup.ID
				denied := []account{other, pending}
				if setup.Visibility == Private {
					denied = append(denied, friend, reverse)
				}
				for _, caller := range denied {
					missing := request(router, "GET", "/api/v1/setups/"+unknown, "", caller.cookie)
					inaccessible := request(router, "GET", path, "", caller.cookie)
					if missing.Code != 404 || inaccessible.Code != 404 || missing.Body.String() != inaccessible.Body.String() || missing.Header().Get("Content-Type") != inaccessible.Header().Get("Content-Type") || missing.Header().Get("Cache-Control") != inaccessible.Header().Get("Cache-Control") {
						t.Fatalf("protected corrupt document revealed: %d %s", inaccessible.Code, inaccessible.Body)
					}
					if _, err := service.Get(ctx, caller.id, setup.ID); !errors.Is(err, ErrNotFound) {
						t.Fatalf("unauthorized service read: %v", err)
					}
					for _, method := range []string{"PATCH", "DELETE"} {
						if w := request(router, method, path, `{"title":"Forbidden"}`, caller.cookie); w.Code != 404 {
							t.Fatal("corrupt non-owner mutation", method, w.Code, w.Body)
						}
					}
				}
				_, err := service.Get(ctx, owner.id, setup.ID)
				if corruption.version == 2 {
					if !errors.Is(err, ErrUnsupportedSchema) {
						t.Fatalf("owner schema error lost: %v", err)
					}
				} else {
					var decodeErr *json.UnmarshalTypeError
					if !errors.As(err, &decodeErr) {
						t.Fatalf("owner decode error lost: %v", err)
					}
				}
				if w := request(router, "GET", path, "", owner.cookie); w.Code != 500 || w.Body.String() != "{\"error\":\"setup operation failed\"}\n" {
					t.Fatal("owner internal error response", w.Code, w.Body)
				}
				if _, err := pool.Exec(ctx, `UPDATE setups SET schema_version=1,data='{}'::jsonb WHERE id=$1`, setup.ID); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	// Restored documents remain readable by owners, public readers and accepted friends.
	setupResponse(t, request(router, "GET", "/api/v1/setups/"+setups[0].ID, "", other.cookie), 200)
	setupResponse(t, request(router, "GET", friendsPath, "", friend.cookie), 200)
	// Friends and public readers must never acquire mutation permissions.
	for _, caller := range []account{friend, reverse, other} {
		for _, s := range setups {
			for _, method := range []string{"PATCH", "DELETE"} {
				if w := request(router, method, "/api/v1/setups/"+s.ID, `{"title":"Forbidden"}`, caller.cookie); w.Code != 404 {
					t.Fatal("non-owner mutation", method, w.Code, w.Body)
				}
			}
		}
	}
	for _, s := range setups {
		path := "/api/v1/setups/" + s.ID
		patched := setupResponse(t, request(router, "PATCH", path, `{"title":"Owner updated"}`, owner.cookie), 200)
		if patched.Title != "Owner updated" {
			t.Fatal("owner patch")
		}
		if w := request(router, "DELETE", path, "", owner.cookie); w.Code != 204 {
			t.Fatal("owner delete", w.Code, w.Body)
		}
	}
	// Error responses never disclose database details.
	pool.Close()
	for _, path := range []string{friendsPath, listPath} {
		w := request(router, "GET", path, "", other.cookie)
		if w.Code != 500 || strings.Contains(w.Body.String(), "closed pool") {
			t.Fatal("internal error leak", w.Code, w.Body)
		}
	}
}
