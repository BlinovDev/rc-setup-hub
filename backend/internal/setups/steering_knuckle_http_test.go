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

func TestSteeringKnuckleHTTPPersistence(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(repo, catalog, friendships.NewRepository(pool))
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("d"), 32), SameSite: http.SameSiteLaxMode}
	provider := &fakeGoogle{identity: users.Identity{Subject: "knuckle-owner", Email: "diff@example.com", Name: "Knuckle"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	cookie := login(t, router)
	brand, err := catalog.CreateBrand(ctx, "Knuckle manufacturer")
	if err != nil {
		t.Fatal(err)
	}
	otherBrand, err := catalog.CreateBrand(ctx, "Other manufacturer")
	if err != nil {
		t.Fatal(err)
	}
	data := realisticData(t)
	data.RearDifferential = &RearDifferential{ManufacturerID: &brand.ID, Model: "Existing differential"}
	data.FrontSteeringKnuckle = &SteeringKnuckle{ManufacturerID: &brand.ID, Description: " Part #42\nAluminum "}
	body := func(data *DataV1) string {
		t.Helper()
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	create := func(raw string) string {
		return `{"title":"Knuckle","visibility":"private","notes":"Existing notes","data":` + raw + `}`
	}
	created := setupResponse(t, request(router, "POST", "/api/v1/setups", create(body(data)), cookie), 201)
	path := "/api/v1/setups/" + created.ID
	want := created.Data
	if want.FrontSteeringKnuckle == nil || want.FrontSteeringKnuckle.Description != "Part #42\nAluminum" {
		t.Fatal("normalization", want)
	}
	check := func(want DataV1) {
		t.Helper()
		got := setupResponse(t, request(router, "GET", path, "", cookie), 200)
		if got.SchemaVersion != 1 || !reflect.DeepEqual(got.Data, want) || got.Notes == nil || *got.Notes != "Existing notes" {
			t.Fatal("detail changed", got)
		}
		var display *DifferentialManufacturer
		if want.FrontSteeringKnuckle != nil && want.FrontSteeringKnuckle.ManufacturerID != nil {
			id := want.FrontSteeringKnuckle.ManufacturerID
			name := brand.Name
			if *id == otherBrand.ID {
				name = otherBrand.Name
			}
			display = &DifferentialManufacturer{ID: *id, Name: name}
		}
		if !reflect.DeepEqual(got.FrontSteeringKnuckleManufacturer, display) {
			t.Fatal("historical manufacturer display", got.FrontSteeringKnuckleManufacturer, display)
		}
		loaded, err := repo.GetOwned(ctx, created.OwnerID, created.ID)
		if err != nil || !reflect.DeepEqual(loaded.Data, want) || !reflect.DeepEqual(loaded.FrontSteeringKnuckleManufacturer, display) {
			t.Fatal("repository round trip", loaded, err)
		}
		var stored DataV1
		var raw []byte
		if err := pool.QueryRow(ctx, "SELECT data FROM setups WHERE id=$1", created.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := decodeStrict(bytes.NewReader(raw), &stored); err != nil || !reflect.DeepEqual(stored, want) {
			t.Fatal("stored JSONB", string(raw), err)
		}
		for _, target := range []string{"/api/v1/me/setups", "/api/v1/users/" + created.OwnerID + "/setups"} {
			w := request(router, "GET", target, "", cookie)
			var list []Setup
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 1 || !reflect.DeepEqual(list[0].Data, want) || !reflect.DeepEqual(list[0].FrontSteeringKnuckleManufacturer, display) {
				t.Fatal("list round trip", w.Code, w.Body)
			}
		}
	}
	check(want)
	if created.FrontSteeringKnuckleManufacturer == nil || created.FrontSteeringKnuckleManufacturer.ID != brand.ID || created.FrontSteeringKnuckleManufacturer.Name != brand.Name {
		t.Fatal("create manufacturer display", created)
	}
	snapshot := func() string {
		t.Helper()
		var raw string
		if err := pool.QueryRow(ctx, "SELECT row_to_json(s)::text FROM setups s WHERE id=$1", created.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	reject := func(raw string) {
		t.Helper()
		before := snapshot()
		for _, method := range []string{"POST", "PATCH"} {
			target, requestBody := path, `{"data":`+raw+`}`
			if method == "POST" {
				target = "/api/v1/setups"
				requestBody = create(raw)
			}
			w := request(router, method, target, requestBody, cookie)
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"error"`) {
				t.Fatal("invalid request", method, w.Code, w.Body)
			}
			if snapshot() != before {
				t.Fatal("rejected write changed row")
			}
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM setups").Scan(&count); err != nil || count != 1 {
			t.Fatal("rejected create persisted", count, err)
		}
	}
	beforeForgery := snapshot()
	for _, method := range []string{"POST", "PATCH"} {
		target, raw := path, `{"front_steering_knuckle_manufacturer":{"id":"`+brand.ID+`","name":"Forged"}}`
		if method == "POST" {
			target = "/api/v1/setups"
			raw = `{"title":"Forged","visibility":"private","data":{},"front_steering_knuckle_manufacturer":{"id":"` + brand.ID + `","name":"Forged"}}`
		}
		if w := request(router, method, target, raw, cookie); w.Code != 400 {
			t.Fatal("read-only display accepted", method, w.Code, w.Body)
		}
		if snapshot() != beforeForgery {
			t.Fatal("forged display changed row")
		}
	}
	for _, raw := range []string{
		`{"front_steering_knuckle":{"manufacturer_id":"bad"}}`, `{"front_steering_knuckle":{"manufacturer_id":42}}`,
		`{"front_steering_knuckle":{"manufacturer_id":"00000000-0000-0000-0000-000000000000"}}`,
		`{"front_steering_knuckle":{"description":42}}`, `{"front_steering_knuckle":{"description":"bad\u0000text"}}`,
		`{"front_steering_knuckle":{"description":"` + strings.Repeat("😀", 201) + `"}}`,
		`{"front_steering_knuckle":{"rear":{}}}`, `{"rear_steering_knuckle":{}}`,
		`{"front_steering_knuckle_manufacturer":{"id":"` + brand.ID + `","name":"Forged"}}`,
	} {
		reject(raw)
	}

	if _, err := catalog.SetBrandActive(ctx, otherBrand.ID, false); err != nil {
		t.Fatal(err)
	}
	reject(`{"front_steering_knuckle":{"manufacturer_id":"` + otherBrand.ID + `"}}`)
	// Disabling a selected brand preserves reads and unchanged references, including UUID spelling.
	if _, err := catalog.SetBrandActive(ctx, brand.ID, false); err != nil {
		t.Fatal(err)
	}
	check(want)
	setupResponse(t, request(router, "PATCH", path, `{"title":"Unrelated edit"}`, cookie), 200)
	check(want)
	for _, description := range []string{"Updated part", strings.Repeat("😀", 200)} {
		replacement := realisticData(t)
		replacement.RearDifferential = &RearDifferential{ManufacturerID: &brand.ID, Model: "Existing differential"}
		upper := strings.ToUpper(brand.ID)
		replacement.FrontSteeringKnuckle = &SteeringKnuckle{ManufacturerID: &upper, Description: description}
		updated := setupResponse(t, request(router, "PATCH", path, `{"data":`+body(replacement)+`}`, cookie), 200)
		if updated.Data.FrontSteeringKnuckle.ManufacturerID == nil || *updated.Data.FrontSteeringKnuckle.ManufacturerID != brand.ID {
			t.Fatal("historical update", updated)
		}
		check(updated.Data)
	}

	// Historical display is resolved from the catalog, not copied into JSONB.
	brand, err = catalog.RenameBrand(ctx, brand.ID, "Renamed manufacturer")
	if err != nil {
		t.Fatal(err)
	}
	current := setupResponse(t, request(router, "GET", path, "", cookie), 200)
	check(current.Data)
	// New selection of an inactive historical brand is still rejected on create.
	if w := request(router, "POST", "/api/v1/setups", create(body(data)), cookie); w.Code != 400 {
		t.Fatal("inactive new selection", w.Code, w.Body)
	}
	// Owner authorization still precedes validation and does not expose private data.
	provider.identity = users.Identity{Subject: "diff-other", Email: "diff-other@example.com", Name: "Other"}
	otherCookie := login(t, router)
	before := snapshot()
	for _, method := range []string{"GET", "PATCH"} {
		raw := ""
		if method == "PATCH" {
			raw = `{"data":{"front_steering_knuckle":{"manufacturer_id":"bad"}}}`
		}
		if w := request(router, method, path, raw, otherCookie); w.Code != 404 {
			t.Fatal("non-owner", method, w.Code, w.Body)
		}
	}
	if snapshot() != before {
		t.Fatal("non-owner modified row")
	}
	if w := request(router, "POST", "/api/v1/setups", create(body(data)), nil); w.Code != 401 {
		t.Fatal("unauthenticated", w.Code)
	}
	// Changing to a different active catalog brand is allowed, even from historical data.
	if _, err := catalog.SetBrandActive(ctx, otherBrand.ID, true); err != nil {
		t.Fatal(err)
	}
	selected := setupResponse(t, request(router, "PATCH", path, `{"data":{"front_steering_knuckle":{"manufacturer_id":"`+otherBrand.ID+`"}}}`, cookie), 200)
	check(DataV1{FrontSteeringKnuckle: &SteeringKnuckle{ManufacturerID: &otherBrand.ID}})
	if selected.Data.FrontSteeringKnuckle.Description != "" {
		t.Fatal("replacement retained old description")
	}
	// Replacement clears a reference; adding it again requires an active brand.
	for _, raw := range []string{`{"front_steering_knuckle":{"description":"Description only"}}`, `{"front_steering_knuckle":{}}`, `{"front_steering_knuckle":null}`, `{"front_steering_knuckle":{"manufacturer_id":null,"description":null}}`, realisticJSON} {
		updated := setupResponse(t, request(router, "PATCH", path, `{"data":`+raw+`}`, cookie), 200)
		var expected DataV1
		if err := decodeStrict(strings.NewReader(raw), &expected); err != nil {
			t.Fatal(err)
		}
		if err := normalizeData(&expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(updated.Data, expected) {
			t.Fatal("replacement did not clear omitted knuckle fields")
		}
		check(expected)
		reject(`{"front_steering_knuckle":{"manufacturer_id":"` + brand.ID + `"}}`)
	}
	// A legacy schema-v1 record stays unchanged through a metadata-only edit.
	legacy := snapshot()
	var beforeData, afterData string
	if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&beforeData); err != nil {
		t.Fatal(err)
	}
	setupResponse(t, request(router, "PATCH", path, `{"title":"Legacy edit"}`, cookie), 200)
	if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&afterData); err != nil || beforeData != afterData || strings.Contains(afterData, "front_steering_knuckle") {
		t.Fatal("legacy changed", legacy, err)
	}
	check(*realisticData(t))
	// Every optional field also works independently on create, with no inferred defaults.
	for _, raw := range []string{
		`{"front_steering_knuckle":{"manufacturer_id":"` + otherBrand.ID + `"}}`,
		`{"front_steering_knuckle":{"description":"Description only"}}`,
		`{"front_steering_knuckle":{}}`, `{"front_steering_knuckle":null}`, `{}`,
	} {
		created := setupResponse(t, request(router, "POST", "/api/v1/setups", create(raw), cookie), 201)
		var expected DataV1
		if err := decodeStrict(strings.NewReader(raw), &expected); err != nil {
			t.Fatal(err)
		}
		if err := normalizeData(&expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(created.Data, expected) {
			t.Fatal("independent create", created.Data, expected)
		}
		loaded := setupResponse(t, request(router, "GET", "/api/v1/setups/"+created.ID, "", cookie), 200)
		if !reflect.DeepEqual(loaded.Data, expected) {
			t.Fatal("independent read", loaded.Data, expected)
		}
	}

}
