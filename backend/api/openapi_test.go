package api

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/pb33f/go-yaml"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi-validator/schema_validation"
)

func TestOpenAPIContract(t *testing.T) {
	source, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source, OpenAPI) {
		t.Fatal("embedded contract differs from source")
	}
	document, err := libopenapi.NewDocument(source)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Release()
	if _, err := document.BuildV3Model(); err != nil {
		t.Fatal("OpenAPI references/model", err)
	}
	if valid, errs := schema_validation.ValidateOpenAPIDocument(document); !valid || len(errs) > 0 {
		t.Fatalf("OpenAPI validation failed: %+v", errs)
	}
	// Structural checks supplement the maintained OpenAPI validator; they are not a YAML/schema validator.
	var contract struct {
		OpenAPI    string                               `yaml:"openapi"`
		Security   []map[string][]string                `yaml:"security"`
		Paths      map[string]map[string]map[string]any `yaml:"paths"`
		Components struct {
			Schemas map[string]map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(source, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.OpenAPI != "3.1.0" {
		t.Fatal("expected OpenAPI 3.1")
	}
	if len(contract.Security) != 1 || contract.Security[0]["CookieAuth"] == nil {
		t.Fatal("cookie authentication must be the default")
	}
	for path, methods := range map[string][]string{
		"/health": {"get"}, "/openapi.yaml": {"get"}, "/auth/google": {"get"}, "/auth/google/callback": {"get"},
		"/api/v1/users/{user_id}": {"get"}, "/api/v1/auth/logout": {"post"}, "/api/v1/me": {"get", "patch"}, "/api/v1/users/search": {"get"},
		"/api/v1/friendships": {"get", "post"}, "/api/v1/friendships/{id}/accept": {"post"}, "/api/v1/friendships/{id}": {"delete"},
		"/api/v1/chassis/brands": {"get"}, "/api/v1/chassis/brands/{brand_id}/models": {"get"},
		"/api/v1/setups": {"post"}, "/api/v1/setups/{id}": {"get", "patch", "delete"},
		"/api/v1/me/setups": {"get"}, "/api/v1/users/{user_id}/setups": {"get"}, "/api/v1/setups/search": {"get"},
	} {
		for _, method := range methods {
			op, exists := contract.Paths[path][method]
			if !exists {
				t.Errorf("missing %s %s", method, path)
				continue
			}
			if strings.HasPrefix(path, "/api/v1/") {
				if security, overridden := op["security"]; overridden {
					t.Errorf("%s %s overrides cookie security: %v", method, path, security)
				}
			}
		}
	}
	for _, name := range []string{"User", "PublicProfile", "ChassisBrand", "ChassisModel", "Setup", "SetupDataV1", "Suspension", "AxleSuspension", "LinkLength", "Shocks", "Shock", "Spring", "Electronics", "SetupSearchItem", "SetupSearchChassis", "SetupSearchPage", "FriendshipRelationship", "FriendshipItem", "FriendshipList", "APIError"} {
		if _, exists := contract.Components.Schemas[name]; !exists {
			t.Errorf("missing schema %s", name)
		}
	}
	for name, schema := range contract.Components.Schemas {
		properties, _ := schema["properties"].(map[string]any)
		for key := range properties {
			switch strings.ToLower(key) {
			case "google_subject", "access_token", "id_token", "refresh_token", "session", "session_id", "session_secret", "admin_emails", "auth_state":
				t.Errorf("private field %s in %s", key, name)
			case "email":
				if name != "User" {
					t.Errorf("email in public schema %s", name)
				}
			}
		}
	}
	schemas := contract.Components.Schemas
	props := func(name string) map[string]any { t.Helper(); return schemas[name]["properties"].(map[string]any) }
	if got := fmt.Sprint(props("Setup")["schema_version"].(map[string]any)["const"]); got != "1" {
		t.Fatal("detail schema version is not fixed to 1")
	}
	for _, name := range []string{"CreateSetup", "PatchSetup"} {
		for _, forbidden := range []string{"owner_id", "schema_version"} {
			if _, ok := props(name)[forbidden]; ok {
				t.Fatalf("%s accepts %s", name, forbidden)
			}
		}
		if schemas[name]["additionalProperties"] != false {
			t.Fatalf("%s permits unknown fields", name)
		}
	}
	if props("Setup")["owner_id"] == nil || props("Setup")["chassis"] == nil {
		t.Fatal("Setup lacks direct-link owner/chassis metadata")
	}
	if _, required := schemas["PatchSetup"]["required"]; required {
		t.Fatal("PATCH fields must be optional")
	}
	for _, field := range []string{"chassis_model_id", "notes"} {
		if !strings.Contains(fmt.Sprint(props("PatchSetup")[field].(map[string]any)["type"]), "null") {
			t.Fatalf("PATCH %s cannot be cleared", field)
		}
	}
	if got := props("AxleSuspension")["toe_deg"].(map[string]any); !strings.Contains(fmt.Sprint(got["type"]), "number") || !strings.Contains(fmt.Sprint(got["type"]), "null") {
		t.Fatal("toe_deg must accept zero and unknown values")
	}
}
