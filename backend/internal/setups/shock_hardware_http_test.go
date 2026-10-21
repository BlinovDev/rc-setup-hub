package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
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

func TestShockHardwareHTTPPersistence(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	service := NewService(repo, chassis.NewService(chassis.NewRepository(pool)), friendships.NewRepository(pool))
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
	provider := &fakeGoogle{identity: users.Identity{Subject: "shock-owner", Email: "shock@example.com", Name: "Shock owner"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	cookie := login(t, router)
	holes := int32(4)
	ring, otherRing := ORing, XRing
	data := realisticData(t)
	data.RearDifferential = &RearDifferential{Model: "Existing differential"}
	data.Shocks.Front.Piston = &ShockPiston{Holes: &holes, HoleDiameterMM: floatPtr(1.23456789), Notes: " Front piston\nModified holes "}
	data.Shocks.Front.Seal = &ShockSeal{Type: &ring, Description: " Front seal model "}
	data.Shocks.Rear.Piston = &ShockPiston{Holes: &holes, HoleDiameterMM: floatPtr(0.123456789), Notes: "Rear piston"}
	data.Shocks.Rear.Seal = &ShockSeal{Type: &otherRing, Description: "Rear seal model"}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	createBody := func(raw string) string {
		return `{"title":"Shocks","visibility":"private","notes":"Existing notes","data":` + raw + `}`
	}
	created := setupResponse(t, request(router, "POST", "/api/v1/setups", createBody(string(raw)), cookie), 201)
	path := "/api/v1/setups/" + created.ID
	if err := normalizeData(data); err != nil {
		t.Fatal(err)
	}
	check := func(want DataV1) {
		t.Helper()
		got := setupResponse(t, request(router, "GET", path, "", cookie), 200)
		if got.SchemaVersion != 1 || got.Notes == nil || *got.Notes != "Existing notes" || !reflect.DeepEqual(got.Data, want) {
			t.Fatal("detail round trip", got)
		}
		loaded, err := repo.GetOwned(ctx, created.OwnerID, created.ID)
		if err != nil || !reflect.DeepEqual(loaded.Data, want) {
			t.Fatal("repository round trip", loaded, err)
		}
		var stored []byte
		if err := pool.QueryRow(ctx, "SELECT data FROM setups WHERE id=$1", created.ID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		var actual DataV1
		if err := decodeStrict(bytes.NewReader(stored), &actual); err != nil || !reflect.DeepEqual(actual, want) {
			t.Fatal("stored JSONB", string(stored), err)
		}
		for _, target := range []string{"/api/v1/me/setups", "/api/v1/users/" + created.OwnerID + "/setups"} {
			w := request(router, "GET", target, "", cookie)
			var items []Setup
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items) != 1 || !reflect.DeepEqual(items[0].Data, want) {
				t.Fatal("list round trip", w.Code, w.Body)
			}
		}
	}
	if !reflect.DeepEqual(created.Data, *data) {
		t.Fatal("create normalization", created)
	}
	check(*data)
	var storedHoles int32
	var storedDiameter float64
	if err := pool.QueryRow(ctx, `SELECT (data #>> '{shocks,front,piston,holes}')::int,(data #>> '{shocks,rear,piston,hole_diameter_mm}')::float8 FROM setups WHERE id=$1`, created.ID).Scan(&storedHoles, &storedDiameter); err != nil || storedHoles != 4 || storedDiameter != 0.123456789 {
		t.Fatal("numeric precision", storedHoles, storedDiameter, err)
	}
	snapshot := func() string {
		t.Helper()
		var raw string
		if err := pool.QueryRow(ctx, "SELECT row_to_json(s)::text FROM setups s WHERE id=$1", created.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := snapshot()
	for _, axle := range []string{"front", "rear"} {
		for _, bad := range []string{
			`{"piston":{"holes":0}}`, `{"piston":{"holes":-1}}`, `{"piston":{"holes":1.5}}`, `{"piston":{"holes":"4"}}`, `{"piston":{"holes":2147483648}}`,
			`{"piston":{"hole_diameter_mm":0}}`, `{"piston":{"hole_diameter_mm":-1}}`, `{"piston":{"hole_diameter_mm":"1.2"}}`, `{"piston":{"hole_diameter_mm":1e999}}`, `{"piston":{"hole_diameter_mm":NaN}}`,
			`{"piston":{"notes":"bad\u0000text"}}`, `{"piston":{"notes":"` + strings.Repeat("x", 10001) + `"}}`,
			`{"seal":{"type":"other"}}`, `{"seal":{"type":""}}`, `{"seal":{"description":"bad\u0000text"}}`, `{"seal":{"description":"` + strings.Repeat("x", 201) + `"}}`,
			`{"piston":{"unknown":1}}`, `{"seal":{"unknown":1}}`, `{"piston":[]}`, `{"seal":[]}`,
		} {
			badData := `{"shocks":{"` + axle + `":` + bad + `}}`
			for _, method := range []string{"POST", "PATCH"} {
				target, body := path, `{"data":`+badData+`}`
				if method == "POST" {
					target = "/api/v1/setups"
					body = createBody(badData)
				}
				w := request(router, method, target, body, cookie)
				if w.Code != 400 || !strings.Contains(w.Body.String(), `"error"`) {
					t.Fatal("invalid shock accepted", method, axle, w.Code, w.Body)
				}
				if snapshot() != before {
					t.Fatal("rejected input changed existing row")
				}
			}
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM setups").Scan(&count); err != nil || count != 1 {
		t.Fatal("rejected create wrote a row", count, err)
	}
	setupResponse(t, request(router, "PATCH", path, `{"title":"Unrelated edit"}`, cookie), 200)
	check(*data)
	// Independent front/rear edits remove selected fields without erasing a sibling or old shock data.
	data.Shocks.Front.Piston.Holes = nil
	data.Shocks.Front.Piston.HoleDiameterMM = floatPtr(1.111111119)
	data.Shocks.Front.Seal = nil
	data.Shocks.Rear.Piston = nil
	data.Shocks.Rear.Seal = &ShockSeal{Description: "Description without a type"}
	raw, err = json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	updated := setupResponse(t, request(router, "PATCH", path, `{"data":`+string(raw)+`}`, cookie), 200)
	if !reflect.DeepEqual(updated.Data, *data) {
		t.Fatal("partial hardware replacement changed sibling data")
	}
	check(*data)
	// Owners only; existing access gates precede domain validation of new technical data.
	provider.identity = users.Identity{Subject: "shock-other", Email: "shock-other@example.com", Name: "Other"}
	otherCookie := login(t, router)
	before = snapshot()
	for _, method := range []string{"GET", "PATCH"} {
		body := ""
		if method == "PATCH" {
			body = `{"data":{"shocks":{"front":{"piston":{"holes":0}}}}}`
		}
		if w := request(router, method, path, body, otherCookie); w.Code != 404 {
			t.Fatal("non-owner access", method, w.Code, w.Body)
		}
	}
	if snapshot() != before {
		t.Fatal("non-owner modified setup")
	}
	if w := request(router, "POST", "/api/v1/setups", createBody(`{}`), nil); w.Code != 401 {
		t.Fatal("unauthenticated create", w.Code)
	}
	// Empty/null blocks prune only newly empty parents. Legacy data remains byte-for-byte JSONB stable on metadata edits.
	for _, replacement := range []string{
		`{"shocks":{"front":{"piston":null},"rear":{"seal":{"type":null,"description":null}}}}`,
		`{"shocks":{"front":{"piston":{"holes":1}},"rear":{"seal":{"type":"o-ring"}}}}`,
		`{"shocks":{"front":{"piston":{},"seal":{}},"rear":{"seal":null}}}`,
		realisticJSON,
	} {
		var want DataV1
		if err := decodeStrict(strings.NewReader(replacement), &want); err != nil {
			t.Fatal(err)
		}
		if err := normalizeData(&want); err != nil {
			t.Fatal(err)
		}
		updated := setupResponse(t, request(router, "PATCH", path, `{"data":`+replacement+`}`, cookie), 200)
		if !reflect.DeepEqual(updated.Data, want) {
			t.Fatal("removal/re-add/legacy", updated)
		}
		check(want)
	}
	var oldJSON, newJSON string
	if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&oldJSON); err != nil {
		t.Fatal(err)
	}
	setupResponse(t, request(router, "PATCH", path, `{"title":"Legacy edit"}`, cookie), 200)
	if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&newJSON); err != nil || oldJSON != newJSON || strings.Contains(newJSON, "piston") || strings.Contains(newJSON, "seal") {
		t.Fatal("legacy modified", newJSON, err)
	}
	check(*realisticData(t))
}
