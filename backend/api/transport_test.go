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
	"github.com/BlinovDev/rc-setup-hub/backend/internal/setups"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
)

func TestTransportContract(t *testing.T) {
	document, err := libopenapi.NewDocument(api.OpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Release()
	validate, errs := validator.NewValidator(document)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	defer validate.Release()
	id := "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	zero := 0.0
	profile := users.PublicProfile{ID: id, Nickname: "Driver"}
	setup := setups.Setup{ID: id, OwnerID: id, Title: "Track setup", Visibility: setups.Private, SchemaVersion: 1, CreatedAt: now, UpdatedAt: now, Data: setups.DataV1{Suspension: &setups.Suspension{Front: &setups.AxleSuspension{ToeDeg: &zero}}}}
	relationship := friendships.Relationship{ID: id, RequesterID: id, AddresseeID: "22222222-2222-4222-8222-222222222222", Status: friendships.Pending, CreatedAt: now, UpdatedAt: now}
	item := friendships.Item{ID: id, User: profile, CreatedAt: now, UpdatedAt: now}
	summary := setups.SearchItem{ID: id, Title: "Public setup", Visibility: setups.Public, SchemaVersion: 1, CreatedAt: now, UpdatedAt: now, Owner: profile}
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"/api/v1/me", users.User{ID: id, Email: "driver@example.com", Nickname: "Driver", CreatedAt: now}},
		{"/api/v1/users/search", []users.PublicProfile{profile}},
		{"/api/v1/setups/" + id, setup},
		{"/api/v1/me/setups", []setups.Setup{setup}},
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
		{"PATCH", `{}`, true}, {"PATCH", `{"title":"Changed"}`, true}, {"PATCH", `{"chassis_model_id":null,"notes":null}`, true},
		{"PATCH", `{"data":{}}`, true}, {"PATCH", `{"data":null}`, false}, {"PATCH", `{"title":null}`, false}, {"PATCH", `{"visibility":null}`, false},
		{"PATCH", `{"schema_version":2}`, false}, {"PATCH", `{"owner_id":"` + id + `"}`, false},
	} {
		t.Run(tc.method+tc.body, func(t *testing.T) {
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
