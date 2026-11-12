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
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
)

func TestWeightDistributionCreateReadUpdateAndRemove(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(NewRepository(pool), catalog, friendships.NewRepository(pool))
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("w"), 32), SameSite: http.SameSiteLaxMode, AllowedOrigins: []string{"http://localhost:5173"}}
	provider := &fakeGoogle{identity: users.Identity{Subject: "weight-owner", Email: "weight-owner@example.com", Name: "Weight Owner"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	cookie := login(t, router)

	body := `{"title":"Weight test","visibility":"private","data":{"weight_distribution":{"front_left_g":380.25,"front_right_g":400,"rear_left_g":320,"rear_right_g":350.5}}}`
	created := request(router, http.MethodPost, "/api/v1/setups", body, cookie)
	setup := setupResponse(t, created, http.StatusCreated)
	if setup.ModerationStatus != Pending {
		t.Fatalf("new setup moderation status changed: %s", setup.ModerationStatus)
	}
	path := "/api/v1/setups/" + setup.ID
	assertWeight := func(response *httptest.ResponseRecorder, status int, want string) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("HTTP %d want %d: %s", response.Code, status, response.Body)
		}
		var decoded map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
		data, ok := decoded["data"].(map[string]any)
		if !ok {
			t.Fatalf("missing setup data: %s", response.Body)
		}
		measurement, ok := data["weight_distribution"].(map[string]any)
		if !ok {
			t.Fatalf("missing weight distribution: %s", response.Body)
		}
		got, err := json.Marshal(measurement)
		if err != nil || string(got) != want {
			t.Fatalf("weight distribution = %s, want %s (error %v)", got, want, err)
		}
	}
	want := `{"battery_installed":false,"body_installed":false,"front_left_g":380.25,"front_right_g":400,"rear_left_g":320,"rear_right_g":350.5}`
	assertWeight(created, http.StatusCreated, want)
	assertWeight(request(router, http.MethodGet, path, "", cookie), http.StatusOK, want)
	if _, err := pool.Exec(ctx, `UPDATE setups SET moderation_status='approved' WHERE id=$1`, setup.ID); err != nil {
		t.Fatal(err)
	}
	updated := request(router, http.MethodPatch, path, `{"data":{"weight_distribution":{"front_left_g":381,"front_right_g":400,"rear_left_g":320,"rear_right_g":350,"body_installed":false,"battery_installed":true}}}`, cookie)
	patched := setupResponse(t, updated, http.StatusOK)
	if patched.ModerationStatus != Approved {
		t.Fatalf("technical update changed moderation status: %s", patched.ModerationStatus)
	}
	assertWeight(updated, http.StatusOK, `{"battery_installed":true,"body_installed":false,"front_left_g":381,"front_right_g":400,"rear_left_g":320,"rear_right_g":350}`)
	removed := request(router, http.MethodPatch, path, `{"data":{"electronics":{"motor":"preserved"}}}`, cookie)
	setupResponse(t, removed, http.StatusOK)
	if strings.Contains(removed.Body.String(), "weight_distribution") || !strings.Contains(removed.Body.String(), `"motor":"preserved"`) {
		t.Fatalf("full-data removal or unrelated value failed: %s", removed.Body)
	}
	for _, invalid := range []string{
		`{"title":"partial","visibility":"private","data":{"weight_distribution":{"front_left_g":1,"body_installed":false,"battery_installed":false}}}`,
		`{"title":"zero","visibility":"private","data":{"weight_distribution":{"front_left_g":0,"front_right_g":1,"rear_left_g":1,"rear_right_g":1}}}`,
		`{"title":"negative","visibility":"private","data":{"weight_distribution":{"front_left_g":1,"front_right_g":-1,"rear_left_g":1,"rear_right_g":1}}}`,
	} {
		if response := request(router, http.MethodPost, "/api/v1/setups", invalid, cookie); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid weight distribution accepted: HTTP %d %s", response.Code, response.Body)
		}
	}
	if w := request(router, http.MethodGet, path, "", nil); w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "weight_distribution") {
		t.Fatalf("unauthorized read exposed measurement: HTTP %d %s", w.Code, w.Body)
	}
}
