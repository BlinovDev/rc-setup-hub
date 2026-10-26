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

func TestRunningGearHTTPPersistence(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	catalog := chassis.NewService(chassis.NewRepository(pool))
	service := NewService(repo, catalog, friendships.NewRepository(pool))
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("g"), 32), SameSite: http.SameSiteLaxMode}
	provider := &fakeGoogle{identity: users.Identity{Subject: "gear-owner", Email: "gear@example.com", Name: "Gear owner"}}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), pool.Ping)
	authentication.Register(router, NewHandler(service).RegisterAPI)
	cookie := login(t, router)
	brand, err := catalog.CreateBrand(ctx, "Gear brand")
	if err != nil {
		t.Fatal(err)
	}
	encode := func(v any) string {
		t.Helper()
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	for _, side := range []string{"front", "rear"} {
		for _, part := range []string{"hex", "wheel", "tire"} {
			t.Run(side+"/"+part, func(t *testing.T) {
				block := `{"offset_mm":-2.123456789}`
				if part == "wheel" {
					block = fmt.Sprintf(`{"manufacturer_id":"%s","model":" Wheel ","offset_mm":0}`, brand.ID)
				}
				if part == "tire" {
					block = fmt.Sprintf(`{"manufacturer_id":"%s","model":" Tire "}`, brand.ID)
				}
				data := realisticData(t)
				gear := fmt.Sprintf(`{"running_gear":{"%s":{"%s":%s}}}`, side, part, block)
				var extra DataV1
				if err := decodeStrict(strings.NewReader(gear), &extra); err != nil {
					t.Fatal(err)
				}
				data.RunningGear = extra.RunningGear
				data.FrontSteeringKnuckle = &SteeringKnuckle{Description: "Existing knuckle"}
				data.Suspension.Front.LowerArmMount = &LowerArmMount{Model: "Existing mount"}
				kind := Gear
				data.RearDifferential = &RearDifferential{Type: &kind, Model: "Existing differential"}
				holes := int32(4)
				data.Shocks.Front.Piston = &ShockPiston{Holes: &holes}
				created := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Gear","visibility":"private","data":`+encode(data)+`}`, cookie), 201)
				path := "/api/v1/setups/" + created.ID
				check := func(want DataV1) {
					t.Helper()
					got := setupResponse(t, request(router, "GET", path, "", cookie), 200)
					if !reflect.DeepEqual(got.Data, want) {
						t.Fatalf("detail data lost: %s", encode(got.Data))
					}
					var manufacturer *string
					if want.RunningGear != nil {
						expectedAxle := want.RunningGear.Front
						if side == "rear" {
							expectedAxle = want.RunningGear.Rear
						}
						if expectedAxle != nil {
							if part == "wheel" && expectedAxle.Wheel != nil {
								manufacturer = expectedAxle.Wheel.ManufacturerID
							}
							if part == "tire" && expectedAxle.Tire != nil {
								manufacturer = expectedAxle.Tire.ManufacturerID
							}
						}
					}
					if manufacturer != nil {
						if got.RunningGearManufacturers == nil {
							t.Fatal("missing catalogue projection")
						}
						axle := got.RunningGearManufacturers.Front
						if side == "rear" {
							axle = got.RunningGearManufacturers.Rear
						}
						if axle == nil {
							t.Fatal("missing axle projection")
						}
						display := axle.Wheel
						if part == "tire" {
							display = axle.Tire
						}
						if display == nil || display.ID != brand.ID || display.Name != brand.Name {
							t.Fatal("wrong historical display", display)
						}
					}
					for _, target := range []string{"/api/v1/me/setups", "/api/v1/users/" + created.OwnerID + "/setups"} {
						w := request(router, "GET", target, "", cookie)
						var list []Setup
						if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil {
							t.Fatal("list", w.Code, w.Body)
						}
						found := false
						for _, item := range list {
							if item.ID == created.ID {
								found = true
								if !reflect.DeepEqual(item.Data, want) || !reflect.DeepEqual(item.RunningGearManufacturers, got.RunningGearManufacturers) {
									t.Fatal("list mismatch")
								}
							}
						}
						if !found {
							t.Fatal("missing list entry")
						}
					}
					var raw []byte
					if err := pool.QueryRow(ctx, "SELECT data FROM setups WHERE id=$1", created.ID).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					var stored DataV1
					if err := decodeStrict(bytes.NewReader(raw), &stored); err != nil || !reflect.DeepEqual(stored, want) {
						t.Fatal("JSONB", string(raw), err)
					}
				}
				check(created.Data)
				var changed DataV1
				if err := decodeStrict(strings.NewReader(encode(created.Data)), &changed); err != nil {
					t.Fatal(err)
				}
				a := changed.RunningGear.Front
				if side == "rear" {
					a = changed.RunningGear.Rear
				}
				switch part {
				case "hex":
					a.Hex.OffsetMM = floatPtr(0)
				case "wheel":
					offset := int32(-12)
					a.Wheel.OffsetMM = &offset
					a.Wheel.Model = "Changed wheel"
				case "tire":
					a.Tire.Model = "Changed tire"
				}
				created = setupResponse(t, request(router, "PATCH", path, `{"data":`+encode(changed)+`}`, cookie), 200)
				if !reflect.DeepEqual(created.Data, changed) {
					t.Fatal("update mismatch")
				}
				check(changed)
				snapshot := func() string {
					var raw string
					if err := pool.QueryRow(ctx, "SELECT row_to_json(s)::text FROM setups s WHERE id=$1", created.ID).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					return raw
				}
				before := snapshot()
				for _, bad := range []string{
					`{"running_gear":{"front":{"hex":{"width_mm":1}}}}`,
					`{"running_gear":{"rear":{"wheel":{"offset_mm":1.5}}}}`,
					`{"running_gear":{"front":{"wheel":{"offset_mm":2147483648}}}}`,
					`{"running_gear":{"rear":{"tire":{"manufacturer_id":"bad"}}}}`,
					`{"running_gear":{"front":{"wheel":{"manufacturer_id":"11111111-1111-4111-8111-111111111111"}}}}`,
					`{"running_gear":{"rear":{"tire":{"model":"bad\u0000value"}}}}`,
				} {
					for _, method := range []string{"POST", "PATCH"} {
						target, body := path, `{"data":`+bad+`}`
						if method == "POST" {
							target = "/api/v1/setups"
							body = `{"title":"Invalid","visibility":"private","data":` + bad + `}`
						}
						w := request(router, method, target, body, cookie)
						if w.Code != 400 {
							t.Fatal("invalid write", method, w.Code, w.Body)
						}
						if snapshot() != before {
							t.Fatal("rejected write changed setup")
						}
					}
				}
				for _, method := range []string{"POST", "PATCH"} {
					target, body := path, `{"running_gear_manufacturers":null}`
					if method == "POST" {
						target = "/api/v1/setups"
						body = `{"title":"Invalid","visibility":"private","data":{},"running_gear_manufacturers":null}`
					}
					if w := request(router, method, target, body, cookie); w.Code != 400 {
						t.Fatal("projection accepted", w.Code)
					}
				}
				if w := request(router, "PATCH", path, `{"title":"Changed"}`, nil); w.Code != 401 {
					t.Fatal("anonymous mutation", w.Code)
				}
				provider.identity = users.Identity{Subject: "gear-other", Email: "other@example.com", Name: "Other"}
				other := login(t, router)
				for _, method := range []string{"GET", "PATCH", "DELETE"} {
					if w := request(router, method, path, `{"title":"Stolen"}`, other); w.Code != 404 {
						t.Fatal("nonowner", method, w.Code)
					}
				}
				if snapshot() != before {
					t.Fatal("unauthorized write changed setup")
				}
				if _, err := pool.Exec(ctx, "UPDATE chassis_brands SET is_active=false,name='Renamed gear' WHERE id=$1", brand.ID); err != nil {
					t.Fatal(err)
				}
				brand.Name = "Renamed gear"
				check(created.Data)
				updated := setupResponse(t, request(router, "PATCH", path, `{"title":"Changed","data":`+encode(created.Data)+`}`, cookie), 200)
				check(updated.Data)
				if part != "hex" {
					// A historical inactive ID cannot be moved to a different component/axle.
					bad := fmt.Sprintf(`{"running_gear":{"%s":{"%s":{"manufacturer_id":"%s"}}}}`, map[string]string{"front": "rear", "rear": "front"}[side], part, brand.ID)
					if w := request(router, "PATCH", path, `{"data":`+bad+`}`, cookie); w.Code != 400 {
						t.Fatal("inactive moved reference accepted", w.Code)
					}
				}
				cleared := setupResponse(t, request(router, "PATCH", path, `{"data":{"running_gear":{"front":{"hex":{},"wheel":{},"tire":null},"rear":null}}}`, cookie), 200)
				if cleared.Data.RunningGear != nil || cleared.RunningGearManufacturers != nil {
					t.Fatal("empty blocks persisted")
				}
				if part != "hex" {
					if w := request(router, "PATCH", path, `{"data":`+encode(created.Data)+`}`, cookie); w.Code != 400 {
						t.Fatal("inactive reselection accepted", w.Code)
					}
				}
				// Replace, rather than deep-merge, the entire technical document.
				legacy := setupResponse(t, request(router, "PATCH", path, `{"data":{}}`, cookie), 200)
				if !reflect.DeepEqual(legacy.Data, DataV1{}) {
					t.Fatal("replacement retained fields")
				}
				if _, err := pool.Exec(ctx, "UPDATE chassis_brands SET is_active=true WHERE id=$1", brand.ID); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
