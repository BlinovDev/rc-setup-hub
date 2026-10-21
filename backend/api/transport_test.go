package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/setups"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
)

func TestTransportContract(t *testing.T) {
	document, err := libopenapi.NewDocument(api.OpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Release()
	validate, errs := validator.NewValidator(document, config.WithFormatAssertions())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	defer validate.Release()
	id := "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	zero := 0.0
	spacing := 5.2
	profile := users.PublicProfile{ID: id, Nickname: "Driver"}
	setup := setups.Setup{ID: id, OwnerID: id, Title: "Track setup", Visibility: setups.Private, SchemaVersion: 1, CreatedAt: now, UpdatedAt: now, Data: setups.DataV1{Suspension: &setups.Suspension{Front: &setups.AxleSuspension{ToeDeg: &zero, TopArmSpacing: &setups.ArmSpacing{FrontMM: &zero, RearMM: &spacing}, LowerArmSpacing: &setups.ArmSpacing{RearMM: &zero}}}}}
	setup.Data.Suspension.Front.LowerArmMount = &setups.LowerArmMount{ManufacturerID: &id, Model: "Part #42", ConfigurationNotes: "Front inserts\nMount B"}
	setup.Data.Suspension.Rear = &setups.AxleSuspension{LowerArmMount: &setups.LowerArmMount{Model: "Rear part"}}
	setup.LowerArmMountManufacturers = &setups.LowerArmMountManufacturers{Front: &setups.DifferentialManufacturer{ID: id, Name: "Historical manufacturer"}}
	kind := setups.Gear
	setup.RearDifferentialManufacturer = &setups.DifferentialManufacturer{ID: id, Name: "Manufacturer"}
	setup.Data.RearDifferential = &setups.RearDifferential{Type: &kind, ManufacturerID: &id, Model: "Gear diff", TuningNotes: "Oil 10000\nTwo gears"}
	holes := int32(4)
	sealType := setups.XRing
	diameter := 1.23456789
	setup.Data.Shocks = &setups.Shocks{
		Front: &setups.Shock{Manufacturer: "Existing brand", Model: "Existing model", Spring: &setups.Spring{Color: "Red"}, OilCST: &spacing, Piston: &setups.ShockPiston{Holes: &holes, HoleDiameterMM: &diameter, Notes: "Drilled piston"}, Seal: &setups.ShockSeal{Type: &sealType, Description: "Seal model"}},
		Rear:  &setups.Shock{Piston: &setups.ShockPiston{HoleDiameterMM: &diameter}, Seal: &setups.ShockSeal{Description: "Custom seal description"}},
	}
	legacy := setup
	legacy.Data = setups.DataV1{}
	legacy.RearDifferentialManufacturer = nil
	legacy.LowerArmMountManufacturers = nil
	photo := &setups.Photo{ID: id, SetupID: id, Width: 100, Height: 50, ThumbnailWidth: 100, ThumbnailHeight: 50, ContentType: "image/jpeg", IsCover: true, CreatedAt: now, URL: "/api/v1/setups/" + id + "/photos/" + id + "/image", ThumbnailURL: "/api/v1/setups/" + id + "/photos/" + id + "/thumbnail"}
	photo.Crop = &photos.Crop{X: 25, Y: 0, Size: 50}
	photo.ThumbnailRevision = 1
	photo.ThumbnailWidth = 50
	photo.ThumbnailHeight = 50
	setup.CoverPhoto = photo
	historical := setup
	historical.ChassisModelID = &id
	historical.Chassis = &setups.SearchChassis{ModelID: id, ModelName: "RD2.0", BrandID: id, BrandName: "Yokomo"}
	relationship := friendships.Relationship{ID: id, RequesterID: id, AddresseeID: "22222222-2222-4222-8222-222222222222", Status: friendships.Pending, CreatedAt: now, UpdatedAt: now}
	item := friendships.Item{ID: id, User: profile, CreatedAt: now, UpdatedAt: now}
	summary := setups.SearchItem{CoverPhoto: photo, ID: id, Title: "Public setup", Visibility: setups.Public, SchemaVersion: 1, CreatedAt: now, UpdatedAt: now, Owner: profile}
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"/api/v1/setups/" + id + "/photos", []setups.Photo{*photo}},
		{"/api/v1/me", users.User{ID: id, Email: "driver@example.com", Nickname: "Driver", CreatedAt: now}},
		{"/api/v1/users/search", []users.PublicProfile{profile}},
		{"/api/v1/users/" + id, profile},
		{"/api/v1/setups/" + id, setup},
		{"/api/v1/me/setups", []setups.Setup{setup, historical, legacy}},
		{"/api/v1/setups/" + id, historical},
		{"/api/v1/users/" + id + "/setups", []setups.Setup{historical}},
		{"/api/v1/friendships", friendships.List{Incoming: []friendships.Item{item}, Outgoing: []friendships.Item{}, Accepted: []friendships.Item{}}},
		{"/api/v1/setups/search", setups.SearchPage{Items: []setups.SearchItem{summary}}},
		{"/api/v1/setups/search", setups.SearchPage{Items: []setups.SearchItem{}}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			body, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(200)
			_, _ = response.Write(body)
			if valid, errs := validate.ValidateHttpResponse(httptest.NewRequest("GET", tc.path, nil), response.Result()); !valid || len(errs) > 0 {
				t.Fatalf("response outside contract: %s: %+v", body, errs)
			}
		})
	}
	for _, body := range []string{`{"expected_revision":0,"crop":null}`, `{"expected_revision":1,"crop":{"x":0,"y":10,"size":20}}`} {
		req := httptest.NewRequest("PUT", "/api/v1/setups/"+id+"/photos/"+id+"/crop", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract-test-cookie"})
		if valid, errs := validate.ValidateHttpRequest(req); !valid || len(errs) > 0 {
			t.Fatalf("crop request: %+v", errs)
		}
		response := httptest.NewRecorder()
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(200)
		_ = json.NewEncoder(response).Encode(photo)
		if valid, errs := validate.ValidateHttpResponse(req, response.Result()); !valid || len(errs) > 0 {
			t.Fatalf("crop response: %+v", errs)
		}
	}
	t.Run("created relationship", func(t *testing.T) {
		response := httptest.NewRecorder()
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(201)
		if err := json.NewEncoder(response).Encode(relationship); err != nil {
			t.Fatal(err)
		}
		if valid, errs := validate.ValidateHttpResponse(httptest.NewRequest("POST", "/api/v1/friendships", nil), response.Result()); !valid || len(errs) > 0 {
			t.Fatalf("friendship response: %+v", errs)
		}
	})
	for _, tc := range []struct {
		method, body string
		valid        bool
	}{
		{"POST", `{"title":"Track","visibility":"private","chassis_model_id":null,"notes":null,"data":{"suspension":{"front":{"toe_deg":0}}}}`, true},
		{"POST", `{"title":"Track","visibility":"private","data":{"suspension":{"front":{"toe_deg":null}},"electronics":{"motor":null}}}`, true},
		{"POST", `{"title":"Track","visibility":"private","data":{}}`, true},
		{"POST", `{"title":"Track","visibility":"private","schema_version":2,"data":{}}`, false},
		{"POST", `{"title":"Track","visibility":"private","owner_id":"` + id + `","data":{}}`, false},
		{"POST", `{"title":"Track","visibility":"private","data":{"suspension":{"front":{"top_arm_spacing":{"front_mm":0,"rear_mm":5.2}},"rear":{"lower_arm_spacing":{"front_mm":null,"rear_mm":0}}}}}`, true},
		{"PATCH", `{"data":{"suspension":{"rear":{"top_arm_spacing":null,"lower_arm_spacing":{}}}}}`, true},
		{"PATCH", `{"data":{"suspension":{"front":{"top_arm_spacing":{"front_mm":-0.1}}}}}`, false},
		{"PATCH", `{"data":{"suspension":{"front":{"lower_arm_spacing":{"rear_mm":"5.2"}}}}}`, false},
		{"PATCH", `{"data":{"suspension":{"rear":{"top_arm_spacing":{"unknown":1}}}}}`, false},
		{"POST", `{"title":"Diff","visibility":"private","data":{"rear_differential":{"type":"gear","manufacturer_id":"` + id + `","model":"Model","tuning_notes":"Oil 10000"}}}`, true},
		{"POST", `{"title":"Diff","visibility":"private","data":{"rear_differential":{}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"type":"lsd"}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"type":"ball"}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"type":"spool"}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"manufacturer_id":"` + id + `"}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"model":"Model"}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"tuning_notes":"Notes"}}}`, true},
		{"PATCH", `{"data":{"rear_differential":null}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"type":null,"manufacturer_id":null,"model":null,"tuning_notes":null}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"type":"spul"}}}`, false},
		{"PATCH", `{"data":{"rear_differential":{"type":""}}}`, false},
		{"PATCH", `{"data":{"rear_differential":{"type":1}}}`, false},
		{"PATCH", `{"data":{"rear_differential":{"manufacturer_id":"bad"}}}`, false},
		{"PATCH", `{"data":{"rear_differential":{"manufacturer_id":1}}}`, false},
		{"PATCH", `{"data":{"rear_differential":{"model":"` + strings.Repeat("界", 200) + `","tuning_notes":"` + strings.Repeat("界", 10000) + `"}}}`, true},
		{"PATCH", `{"data":{"rear_differential":{"model":"` + strings.Repeat("x", 201) + `"}}}`, false},
		{"PATCH", `{"data":{"rear_differential":{"tuning_notes":"` + strings.Repeat("x", 10001) + `"}}}`, false},
		{"PATCH", `{"data":{"rear_differential":{"unknown":true}}}`, false},
		{"PATCH", `{"data":{"front_differential":{"type":"gear"}}}`, false},
		{"PATCH", `{"rear_differential_manufacturer":{"id":"` + id + `","name":"Forged"}}`, false},
		{"PATCH", `{}`, true}, {"PATCH", `{"title":"Changed"}`, true}, {"PATCH", `{"chassis_model_id":null,"notes":null}`, true},
		{"PATCH", `{"data":{}}`, true}, {"PATCH", `{"data":null}`, false}, {"PATCH", `{"title":null}`, false}, {"PATCH", `{"visibility":null}`, false},
		{"PATCH", `{"schema_version":2}`, false}, {"PATCH", `{"owner_id":"` + id + `"}`, false},
	} {
		t.Run(tc.method+tc.body[:min(len(tc.body), 150)], func(t *testing.T) {
			path := "/api/v1/setups"
			if tc.method == "PATCH" {
				path += "/" + id
			}
			request := httptest.NewRequest(tc.method, path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.AddCookie(&http.Cookie{Name: "rc_session", Value: "contract-test-cookie"})
			valid, errs := validate.ValidateHttpRequest(request)
			if valid != tc.valid {
				t.Fatalf("contract accepted=%v want=%v: %+v", valid, tc.valid, errs)
			}
		})
	}
}
