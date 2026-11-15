package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/auth"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

// TestReadEndpointLatency measures real migrated PostgreSQL, session lookup,
// authorization, projection and JSON encoding. It excludes fixture setup and
// network/browser rendering; the generous budget catches second-scale regressions.
func TestReadEndpointLatency(t *testing.T) {
	for _, populated := range []bool{false, true} {
		name := "small_without_statistics"
		if populated {
			name = "populated_analyzed"
		}
		t.Run(name, func(t *testing.T) { testReadEndpointLatency(t, populated) })
	}
}
func testReadEndpointLatency(t *testing.T, populated bool) {
	f := newSearchFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, f.owner.ID, f.other.ID); err != nil {
		t.Fatal(err)
	}
	if populated {
		// Mix authorized and hidden rows and populate every manufacturer join. These
		// disposable fixtures model a growing catalog without timing setup work.
		if _, err := f.pool.Exec(ctx, `INSERT INTO chassis_brands(name) SELECT 'Perf brand '||n FROM generate_series(1,250) n`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO setups(owner_id,title,visibility,moderation_status,data,chassis_model_id)
 SELECT CASE WHEN n<=50 THEN $1::uuid ELSE $2::uuid END,'Perf setup '||n,
 (ARRAY['public','friends','private'])[1+n%3], 'approved',
 jsonb_build_object('rear_differential',jsonb_build_object('manufacturer_id',$3::text),
 'suspension',jsonb_build_object(
   'front',jsonb_build_object('lower_arm_mount',jsonb_build_object('manufacturer_id',$3::text)),
   'rear',jsonb_build_object('lower_arm_mount',jsonb_build_object('manufacturer_id',$3::text))),
 'front_steering_knuckle',jsonb_build_object('manufacturer_id',$3::text),
 'running_gear',jsonb_build_object(
   'front',jsonb_build_object('wheel',jsonb_build_object('manufacturer_id',$3::text),'tire',jsonb_build_object('manufacturer_id',$3::text)),
   'rear',jsonb_build_object('wheel',jsonb_build_object('manufacturer_id',$3::text),'tire',jsonb_build_object('manufacturer_id',$3::text)))), $4::uuid
 FROM generate_series(1,1000) n`, f.owner.ID, f.other.ID, f.yokomo.ID, f.rd.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `ANALYZE`); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("p"), 32), SameSite: http.SameSiteLaxMode}
	provider := &fakeGoogle{}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(f.pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), f.pool.Ping)
	handler := NewHandler(f.service)
	catalog, err := chassis.NewHandler(f.catalog)
	if err != nil {
		t.Fatal(err)
	}
	profile := users.NewHandler(users.NewService(users.NewRepository(f.pool)))
	friends := friendships.NewHandler(friendships.NewService(friendships.NewRepository(f.pool)))
	authentication.RegisterWithPublic(router, []func(chi.Router){handler.RegisterPublicAPI, catalog.RegisterAPI, profile.RegisterPublicAPI}, handler.RegisterPrivateAPI, friends.RegisterAPI)
	provider.identity = users.Identity{Subject: "search-owner", Email: f.owner.Email, Name: f.owner.Nickname}
	cookie := login(t, router)

	for _, tc := range []struct {
		name, path string
		guest      bool
	}{
		{"home", "/api/v1/setups/search", false},
		{"home_guest", "/api/v1/setups/search", true},
		{"my_setups", "/api/v1/me/setups", false},
		{"compare_selector", "/api/v1/setups/visible", false},
		{"compare_friends", "/api/v1/setups/visible?scope=friends", false},
		{"filtered_home", "/api/v1/setups/search?q=carpet&brand_id=" + f.yokomo.ID, false},
		{"compare_guest", "/api/v1/setups/compare?left_id=" + f.public[0].ID + "&right_id=" + f.public[1].ID, true},
		{"compare_mine", "/api/v1/setups/visible?scope=mine", false},
		{"compare", "/api/v1/setups/compare?left_id=" + f.public[0].ID + "&right_id=" + f.public[1].ID, false},
		{"detail", "/api/v1/setups/" + f.public[0].ID, false},
		{"profile_setups", "/api/v1/users/" + f.owner.ID + "/setups", false},
		{"public_profile_setups", "/api/v1/users/" + f.owner.ID + "/public-setups", true},
		{"profile", "/api/v1/users/" + f.owner.ID, true},
		{"friendships", "/api/v1/friendships", false},
		{"session", "/api/v1/me", false},
		{"brands", "/api/v1/chassis/brands", false},
		{"models", "/api/v1/chassis/brands/" + f.yokomo.ID + "/models", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := cookie
			if tc.guest {
				session = nil
			}
			samples := make([]time.Duration, 20)
			for i := range samples {
				start := time.Now()
				w := request(router, "GET", tc.path, "", session)
				samples[i] = time.Since(start)
				if w.Code != 200 {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body)
				}
			}
			cold := samples[0]
			sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
			t.Logf("first=%s median=%s p95=%s max=%s", cold, samples[len(samples)/2], samples[18], samples[19])
			if cold > 500*time.Millisecond || samples[18] > 500*time.Millisecond {
				t.Errorf("first=%s or p95=%s exceeds local 500ms read budget", cold, samples[18])
			}
		})
	}
}

// A projection may enrich each setup but cannot multiply it. Without uniqueness
// information for UUID-as-text joins PostgreSQL estimates exponential fan-out,
// triggering expensive JIT even for this six-row database before auto-analyze.
func TestSetupProjectionCardinality(t *testing.T) {
	f := newSearchFixture(t)
	var raw []byte
	err := f.pool.QueryRow(context.Background(), `EXPLAIN (FORMAT JSON) SELECT `+displayColumns+` FROM setups s`+displayJoins+` WHERE s.owner_id=$1 AND `+visibleSetupPredicate("$2")+` ORDER BY s.created_at DESC,s.id DESC`, f.owner.ID, f.owner.ID).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var plans []struct {
		Plan struct {
			Rows int `json:"Plan Rows"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected one plan, got %d", len(plans))
	}
	if plans[0].Plan.Rows > len(f.public)+2 {
		t.Fatalf("projection estimates %d rows for only %d setups; manufacturer joins must not multiply cardinality", plans[0].Plan.Rows, len(f.public)+2)
	}
}
