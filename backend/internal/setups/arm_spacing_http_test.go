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

// Real PostgreSQL HTTP coverage exercises create/PATCH, detail and both full lists,
// and inspects JSONB directly to ensure rejected writes and removals are durable.
func TestArmSpacingHTTPPersistence(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	service := NewService(repo, chassis.NewService(chassis.NewRepository(pool)), friendships.NewRepository(pool))
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("s"), 32), SameSite: http.SameSiteLaxMode}
	provider := &fakeGoogle{identity: users.Identity{Subject: "spacing-owner", Email: "spacing@example.com", Name: "Spacing"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	cookie := login(t, router)
	data := `{"suspension":{"front":{"camber_deg":-6.8,"link_lengths":[{"name":"upper","length_mm":16.2}],"top_arm_spacing":{"front_mm":0,"rear_mm":5.2},"lower_arm_spacing":{"front_mm":0,"rear_mm":0}},"rear":{"top_arm_spacing":{"rear_mm":0.123456789},"lower_arm_spacing":{"front_mm":1.2,"rear_mm":null}}},"electronics":{"motor":"Existing motor"}}`
	created := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Spacing","visibility":"public","data":`+data+`}`, cookie), 201)
	path := "/api/v1/setups/" + created.ID
	check := func(want DataV1) {
		t.Helper()
		detail := setupResponse(t, request(router, "GET", path, "", cookie), 200)
		if !reflect.DeepEqual(detail.Data, want) {
			t.Fatalf("detail mismatch: %+v", detail.Data)
		}
		loaded, err := repo.GetOwned(ctx, created.OwnerID, created.ID)
		if err != nil || !reflect.DeepEqual(loaded.Data, want) {
			t.Fatal("repository round trip", loaded, err)
		}
		for _, listPath := range []string{"/api/v1/me/setups", "/api/v1/users/" + created.OwnerID + "/setups"} {
			w := request(router, "GET", listPath, "", cookie)
			var items []Setup
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items) != 1 || !reflect.DeepEqual(items[0].Data, want) {
				t.Fatal("list round trip", w.Code, w.Body)
			}
		}
	}
	check(created.Data)
	var front, rear float64
	if err := pool.QueryRow(ctx, `SELECT (data #>> '{suspension,front,top_arm_spacing,front_mm}')::float8,(data #>> '{suspension,front,top_arm_spacing,rear_mm}')::float8 FROM setups WHERE id=$1`, created.ID).Scan(&front, &rear); err != nil || front != 0 || rear != 5.2 {
		t.Fatal("stored example", front, rear, err)
	}
	changed := setupResponse(t, request(router, "PATCH", path, `{"title":"Title only"}`, cookie), 200)
	if !reflect.DeepEqual(changed.Data, created.Data) {
		t.Fatal("title edit lost data")
	}
	snapshot := func() string {
		t.Helper()
		var raw string
		if err := pool.QueryRow(ctx, `SELECT row_to_json(s)::text FROM setups s WHERE id=$1`, created.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := snapshot()
	for _, bad := range []string{`{"front_mm":-1}`, `{"rear_mm":-0.1}`, `{"front_mm":"5.2"}`, `{"rear_mm":true}`, `{"rear_mm":1e999}`, `{"front_mm":NaN}`, `{"rear_mm":Infinity}`, `{"unknown":1}`, `[]`} {
		badData := `{"suspension":{"front":{"top_arm_spacing":` + bad + `}}}`
		for _, method := range []string{"POST", "PATCH"} {
			body := `{"data":` + badData + `}`
			target := path
			if method == "POST" {
				target = "/api/v1/setups"
				body = `{"title":"Bad","visibility":"private","data":` + badData + `}`
			}
			w := request(router, method, target, body, cookie)
			if w.Code != 400 {
				t.Fatal("invalid input accepted", method, bad, w.Code, w.Body)
			}
			if snapshot() != before {
				t.Fatal("rejected request wrote existing row")
			}
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM setups").Scan(&count); err != nil || count != 1 {
		t.Fatal("invalid create wrote row", count, err)
	}
	// Remove top front block; clear one lower value while retaining the other blocks/data.
	replacement := changed.Data
	replacement.Suspension.Front.TopArmSpacing = nil
	replacement.Suspension.Front.LowerArmSpacing.FrontMM = nil
	raw, err := json.Marshal(replacement)
	if err != nil {
		t.Fatal(err)
	}
	patched := setupResponse(t, request(router, "PATCH", path, `{"data":`+string(raw)+`}`, cookie), 200)
	check(patched.Data)
	if patched.Data.Suspension.Front.TopArmSpacing != nil || patched.Data.Suspension.Front.LowerArmSpacing.RearMM == nil || *patched.Data.Suspension.Front.LowerArmSpacing.RearMM != 0 || patched.Data.Suspension.Rear.TopArmSpacing == nil || patched.Data.Electronics.Motor != "Existing motor" || *patched.Data.Suspension.Front.CamberDeg != -6.8 || (patched.Data.Suspension.Front.LinkLengths[0].LengthMM == nil || *patched.Data.Suspension.Front.LinkLengths[0].LengthMM != 16.2) {
		t.Fatal("remove/clear lost sibling data", patched)
	}
	var present bool
	if err := pool.QueryRow(ctx, `SELECT data #> '{suspension,front}' ? 'top_arm_spacing' FROM setups WHERE id=$1`, created.ID).Scan(&present); err != nil || present {
		t.Fatal("removed block still in JSONB", err)
	}
	empty := setupResponse(t, request(router, "PATCH", path, `{"data":{"suspension":{"front":{"top_arm_spacing":null},"rear":{"lower_arm_spacing":{"front_mm":null,"rear_mm":null}}}}}`, cookie), 200)
	if empty.Data.Suspension != nil {
		t.Fatal("empty parents remain")
	}
	check(empty.Data)
	// An old v1 document receives no defaults and survives an unrelated edit unchanged.
	old := setupResponse(t, request(router, "PATCH", path, `{"data":`+realisticJSON+`}`, cookie), 200)
	var oldJSON string
	if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&oldJSON); err != nil {
		t.Fatal(err)
	}
	setupResponse(t, request(router, "PATCH", path, `{"notes":"Unrelated"}`, cookie), 200)
	var afterJSON string
	if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&afterJSON); err != nil || oldJSON != afterJSON || strings.Contains(afterJSON, "arm_spacing") {
		t.Fatal("legacy document changed", err)
	}
	check(old.Data)
}
