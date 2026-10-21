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
	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
)

func TestLowerArmMountHTTPPersistence(t *testing.T) {
	for _, side := range []string{"front", "rear"} {
		t.Run(side, func(t *testing.T) {
			pool := dbtest.New(t)
			ctx := context.Background()
			repo := NewRepository(pool)
			catalog := chassis.NewService(chassis.NewRepository(pool))
			service := NewService(repo, catalog, friendships.NewRepository(pool))
			cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("m"), 32), SameSite: http.SameSiteLaxMode}
			provider := &fakeGoogle{identity: users.Identity{Subject: "mount-owner", Email: "mount@example.com", Name: "Mount owner"}}
			authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
			router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
			authentication.Register(router, NewHandler(service).RegisterAPI)
			cookie := login(t, router)
			brand, err := catalog.CreateBrand(ctx, "Mount brand")
			if err != nil {
				t.Fatal(err)
			}
			otherBrand, err := catalog.CreateBrand(ctx, "Other mount brand")
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]string{brand.ID: brand.Name, otherBrand.ID: otherBrand.Name}
			otherSide := "rear"
			if side == "rear" {
				otherSide = "front"
			}
			axle := func(data *DataV1, name string) *AxleSuspension {
				if data.Suspension == nil {
					return nil
				}
				if name == "front" {
					return data.Suspension.Front
				}
				return data.Suspension.Rear
			}
			encode := func(value any) string {
				t.Helper()
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				return string(raw)
			}
			create := func(data string) string {
				return `{"title":"Mount setup","visibility":"private","notes":"Existing notes","data":` + data + `}`
			}
			blockData := func(block string) string {
				return fmt.Sprintf(`{"suspension":{"%s":{"lower_arm_mount":%s}}}`, side, block)
			}
			data := realisticData(t)
			axle(data, side).LowerArmMount = &LowerArmMount{ManufacturerID: &brand.ID, Model: " Part #42 ", ConfigurationNotes: " Inserts A\nMount B "}
			axle(data, otherSide).LowerArmMount = &LowerArmMount{ManufacturerID: &otherBrand.ID, Model: "Other axle", ConfigurationNotes: "Other config"}
			axle(data, side).TopArmSpacing = &ArmSpacing{FrontMM: floatPtr(0), RearMM: floatPtr(5.23456789)}
			kind := Gear
			data.RearDifferential = &RearDifferential{Type: &kind, ManufacturerID: &brand.ID, Model: "Diff", TuningNotes: "Tuning"}
			holes := int32(6)
			seal := XRing
			data.Shocks.Front.Piston = &ShockPiston{Holes: &holes, HoleDiameterMM: floatPtr(1.23456789), Notes: "Piston"}
			data.Shocks.Rear.Seal = &ShockSeal{Type: &seal, Description: "Seal product"}
			created := setupResponse(t, request(router, "POST", "/api/v1/setups", create(encode(data)), cookie), 201)
			path := "/api/v1/setups/" + created.ID
			if mount := axle(&created.Data, side).LowerArmMount; mount.Model != "Part #42" || mount.ConfigurationNotes != "Inserts A\nMount B" {
				t.Fatal("normalization", mount)
			}
			check := func(want DataV1) {
				t.Helper()
				var display *LowerArmMountManufacturers
				var front, rear *DifferentialManufacturer
				for _, name := range []string{"front", "rear"} {
					a := axle(&want, name)
					if a != nil && a.LowerArmMount != nil && a.LowerArmMount.ManufacturerID != nil {
						id := *a.LowerArmMount.ManufacturerID
						value := &DifferentialManufacturer{ID: id, Name: names[id]}
						if name == "front" {
							front = value
						} else {
							rear = value
						}
					}
				}
				if front != nil || rear != nil {
					display = &LowerArmMountManufacturers{Front: front, Rear: rear}
				}
				assertSetup := func(got Setup) {
					t.Helper()
					if got.SchemaVersion != 1 || !reflect.DeepEqual(got.Data, want) || !reflect.DeepEqual(got.LowerArmMountManufacturers, display) || got.Notes == nil || *got.Notes != "Existing notes" {
						t.Fatalf("round trip got %+v display=%+v want %+v display=%+v", got, got.LowerArmMountManufacturers, want, display)
					}
				}
				assertSetup(setupResponse(t, request(router, "GET", path, "", cookie), 200))
				got, err := repo.GetOwned(ctx, created.OwnerID, created.ID)
				if err != nil {
					t.Fatal(err)
				}
				assertSetup(got)
				for _, target := range []string{"/api/v1/me/setups", "/api/v1/users/" + created.OwnerID + "/setups"} {
					w := request(router, "GET", target, "", cookie)
					var list []Setup
					if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 1 {
						t.Fatal("list", w.Code, w.Body)
					}
					assertSetup(list[0])
				}
				var raw []byte
				var stored DataV1
				if err := pool.QueryRow(ctx, "SELECT data FROM setups WHERE id=$1", created.ID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				if err := decodeStrict(bytes.NewReader(raw), &stored); err != nil || !reflect.DeepEqual(stored, want) {
					t.Fatal("JSONB", string(raw), err)
				}
				if bytes.Contains(raw, []byte("lower_arm_mount_manufacturers")) {
					t.Fatal("projection persisted")
				}
			}
			check(created.Data)
			if created.LowerArmMountManufacturers == nil || created.LowerArmMountManufacturers.Front == nil || created.LowerArmMountManufacturers.Rear == nil {
				t.Fatal("create projection", created)
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
					target, body := path, `{"data":`+raw+`}`
					if method == "POST" {
						target = "/api/v1/setups"
						body = create(raw)
					}
					w := request(router, method, target, body, cookie)
					if w.Code != 400 || !strings.Contains(w.Body.String(), `"error"`) {
						t.Fatal("invalid input", method, w.Code, w.Body)
					}
					if snapshot() != before {
						t.Fatal("invalid write changed row")
					}
				}
				var count int
				if err := pool.QueryRow(ctx, "SELECT count(*) FROM setups").Scan(&count); err != nil || count != 1 {
					t.Fatal("invalid create persisted", count, err)
				}
			}
			for _, block := range []string{`{"manufacturer_id":"bad"}`, `{"manufacturer_id":42}`, `{"manufacturer_id":"00000000-0000-0000-0000-000000000000"}`, `{"model":42}`, `{"model":"bad\u0000text"}`, `{"configuration_notes":"bad\u0000text"}`, `{"model":"` + strings.Repeat("😀", 201) + `"}`, `{"configuration_notes":"` + strings.Repeat("界", 10001) + `"}`, `{"inserts":[]}`, `{"mounts":[]}`, `{"geometry":{}}`, `[]`} {
				reject(blockData(block))
			}
			before := snapshot()
			for _, method := range []string{"POST", "PATCH"} {
				target, body := path, `{"lower_arm_mount_manufacturers":{"front":{"id":"`+brand.ID+`","name":"Forged"},"rear":null}}`
				if method == "POST" {
					target = "/api/v1/setups"
					body = `{"title":"Forged","visibility":"private","data":{},"lower_arm_mount_manufacturers":null}`
				}
				if w := request(router, method, target, body, cookie); w.Code != 400 {
					t.Fatal("forged projection", w.Code, w.Body)
				}
				if snapshot() != before {
					t.Fatal("forged projection changed row")
				}
			}
			if _, err := catalog.SetBrandActive(ctx, otherBrand.ID, false); err != nil {
				t.Fatal(err)
			}
			reject(blockData(`{"manufacturer_id":"` + otherBrand.ID + `"}`))
			if _, err := catalog.SetBrandActive(ctx, brand.ID, false); err != nil {
				t.Fatal(err)
			}
			check(created.Data)
			setupResponse(t, request(router, "PATCH", path, `{"title":"Unrelated edit"}`, cookie), 200)
			check(created.Data)
			// Unchanged historical references survive full data replacement and UUID spelling changes.
			current := setupResponse(t, request(router, "GET", path, "", cookie), 200)
			upper := strings.ToUpper(brand.ID)
			axle(&current.Data, side).LowerArmMount.ManufacturerID = &upper
			axle(&current.Data, side).LowerArmMount.Model = strings.Repeat("😀", 200)
			axle(&current.Data, side).LowerArmMount.ConfigurationNotes = strings.Repeat("界", 10000)
			updated := setupResponse(t, request(router, "PATCH", path, `{"data":`+encode(current.Data)+`}`, cookie), 200)
			if *axle(&updated.Data, side).LowerArmMount.ManufacturerID != brand.ID {
				t.Fatal("UUID not canonical")
			}
			check(updated.Data)
			// An inactive historical ID is not transferable to a different axle as a new selection.
			var moved DataV1
			if err := decodeStrict(strings.NewReader(encode(updated.Data)), &moved); err != nil {
				t.Fatal(err)
			}
			axle(&moved, otherSide).LowerArmMount.ManufacturerID = &brand.ID
			reject(encode(moved))
			// Names resolve from the catalog, not stale JSONB copies.
			brand, err = catalog.RenameBrand(ctx, brand.ID, "Renamed mount brand")
			if err != nil {
				t.Fatal(err)
			}
			names[brand.ID] = brand.Name
			check(updated.Data)
			provider.identity = users.Identity{Subject: "mount-other", Email: "other-mount@example.com", Name: "Other"}
			otherCookie := login(t, router)
			before = snapshot()
			for _, method := range []string{"GET", "PATCH", "DELETE"} {
				body := ""
				if method == "PATCH" {
					body = `{"data":` + blockData(`{"manufacturer_id":"bad"}`) + `}`
				}
				if w := request(router, method, path, body, otherCookie); w.Code != 404 {
					t.Fatal("non-owner", method, w.Code, w.Body)
				}
			}
			if snapshot() != before {
				t.Fatal("non-owner modified row")
			}
			if w := request(router, "POST", "/api/v1/setups", create(`{}`), nil); w.Code != 401 {
				t.Fatal("unauthenticated", w.Code)
			}
			if _, err := catalog.SetBrandActive(ctx, otherBrand.ID, true); err != nil {
				t.Fatal(err)
			}
			selected := setupResponse(t, request(router, "PATCH", path, `{"data":`+blockData(`{"manufacturer_id":"`+otherBrand.ID+`"}`)+`}`, cookie), 200)
			check(selected.Data)
			if m := axle(&selected.Data, side).LowerArmMount; m.Model != "" || m.ConfigurationNotes != "" {
				t.Fatal("replacement merged old fields")
			}
			for _, block := range []string{`{"model":"Model only"}`, `{"configuration_notes":"Notes only"}`, `{}`, `null`, `{"manufacturer_id":null,"model":null,"configuration_notes":null}`} {
				updated := setupResponse(t, request(router, "PATCH", path, `{"data":`+blockData(block)+`}`, cookie), 200)
				var want DataV1
				if err := decodeStrict(strings.NewReader(blockData(block)), &want); err != nil {
					t.Fatal(err)
				}
				if err := normalizeData(&want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(updated.Data, want) {
					t.Fatal("replacement", updated.Data, want)
				}
				check(want)
				reject(blockData(`{"manufacturer_id":"` + brand.ID + `"}`))
			}
			// Legacy shape/zeros remain unchanged through metadata-only edits.
			setupResponse(t, request(router, "PATCH", path, `{"data":`+realisticJSON+`}`, cookie), 200)
			var beforeData, afterData string
			if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&beforeData); err != nil {
				t.Fatal(err)
			}
			setupResponse(t, request(router, "PATCH", path, `{"title":"Legacy edit"}`, cookie), 200)
			if err := pool.QueryRow(ctx, "SELECT data::text FROM setups WHERE id=$1", created.ID).Scan(&afterData); err != nil || beforeData != afterData {
				t.Fatal("legacy changed", err)
			}
			check(*realisticData(t))
			for _, block := range []string{`{"manufacturer_id":"` + otherBrand.ID + `"}`, `{"model":"Model only"}`, `{"configuration_notes":"Notes only"}`, `{}`, `null`} {
				created := setupResponse(t, request(router, "POST", "/api/v1/setups", create(blockData(block)), cookie), 201)
				loaded := setupResponse(t, request(router, "GET", "/api/v1/setups/"+created.ID, "", cookie), 200)
				var want DataV1
				if err := decodeStrict(strings.NewReader(blockData(block)), &want); err != nil {
					t.Fatal(err)
				}
				if err := normalizeData(&want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(created.Data, want) || !reflect.DeepEqual(loaded.Data, want) {
					t.Fatal("independent create/read")
				}
			}
		})
	}
}
