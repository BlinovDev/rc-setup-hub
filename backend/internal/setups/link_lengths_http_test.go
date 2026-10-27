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
	"github.com/BlinovDev/rc-setup-hub/backend/internal/config"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

func TestLinkLengthsPersistenceHTTPAndCompare(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	cfg := config.Auth{AppURL: &url.URL{Scheme: "http", Host: "localhost:5173"}, GoogleRedirectURL: "http://localhost:8080/auth/google/callback", SessionSecret: bytes.Repeat([]byte("l"), 32), SameSite: http.SameSiteLaxMode}
	provider := &fakeGoogle{}
	authentication := auth.NewHandler(cfg, provider, users.NewService(users.NewRepository(f.pool)))
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), f.pool.Ping)
	handler := NewHandler(f.service)
	authentication.RegisterWithPublic(router, []func(chi.Router){handler.RegisterPublicAPI}, handler.RegisterPrivateAPI)
	signIn := func(subject string, u users.User) *http.Cookie {
		t.Helper()
		provider.identity = users.Identity{Subject: subject, Email: u.Email, Name: u.Nickname}
		return login(t, router)
	}
	owner, friend := signIn("search-owner", f.owner), signIn("search-friend", f.friend)
	if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, f.owner.ID, f.friend.ID); err != nil {
		t.Fatal(err)
	}
	data := *realisticData(t)
	data.Suspension.Front.LinkLengths = []LinkLength{{Name: "upper_arm", LengthMM: floatPtr(42.123456789)}, {Name: "lower_arm"}, {Name: "steering_tie_rod", LengthMM: floatPtr(16.2)}, {Name: "Legacy upper label", LengthMM: floatPtr(99.9)}, {Name: "Old duplicate", LengthMM: floatPtr(12)}, {Name: "Old duplicate", LengthMM: floatPtr(13)}, {Name: "Custom 未知"}}
	data.Suspension.Rear.LinkLengths = []LinkLength{{Name: "upper_arm"}, {Name: "lower_arm", LengthMM: floatPtr(50.555555555)}, {Name: "Rear custom", LengthMM: floatPtr(22.2)}}
	data.Suspension.Front.TopArmSpacing = &ArmSpacing{FrontMM: floatPtr(0), RearMM: floatPtr(5.2)}
	data.RunningGear = &RunningGear{Front: &AxleRunningGear{Hex: &Hex{OffsetMM: floatPtr(0)}}}
	data.RearDifferential = &RearDifferential{Model: "Keep differential", TuningNotes: "Keep tuning notes"}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var records []Setup
	for _, visibility := range []Visibility{Public, Friends, Private} {
		created := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Links","visibility":"`+string(visibility)+`","notes":"Keep notes","data":`+string(raw)+`}`, owner), 201)
		if !reflect.DeepEqual(created.Data, data) {
			t.Fatal("create lost link/angle/sibling data", created.Data)
		}
		path := "/api/v1/setups/" + created.ID
		loaded := setupResponse(t, request(router, "GET", path, "", owner), 200)
		if !reflect.DeepEqual(created, loaded) {
			t.Fatal("detail roundtrip changed")
		}
		// Title-only editing of stored schema-v1 data must never rename/reorder links.
		patched := setupResponse(t, request(router, "PATCH", path, `{"title":"Retitled"}`, owner), 200)
		if !reflect.DeepEqual(patched.Data, data) || patched.Notes == nil || *patched.Notes != "Keep notes" {
			t.Fatal("unrelated PATCH lost data")
		}
		var stored DataV1
		var storedRaw []byte
		if err := f.pool.QueryRow(ctx, `SELECT data FROM setups WHERE id=$1`, created.ID).Scan(&storedRaw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(storedRaw, &stored); err != nil || !reflect.DeepEqual(stored, data) {
			t.Fatal("JSONB lost order/precision", err)
		}
		var lengthExists bool
		if err := f.pool.QueryRow(ctx, `SELECT (data #> '{suspension,front,link_lengths,1}') ? 'length_mm' FROM setups WHERE id=$1`, created.ID).Scan(&lengthExists); err != nil || lengthExists {
			t.Fatal("unknown length fabricated", lengthExists, err)
		}
		records = append(records, patched)
	}
	// Reordered arrays and differing custom values remain raw named arrays for Compare.
	replacement := records[0].Data
	front := *replacement.Suspension.Front
	rear := *replacement.Suspension.Rear
	suspension := *replacement.Suspension
	replacement.Suspension = &suspension
	replacement.Suspension.Front = &front
	replacement.Suspension.Rear = &rear
	replacement.Suspension.Front.LinkLengths = []LinkLength{{Name: "Custom 未知", LengthMM: floatPtr(17.7)}, {Name: "steering_tie_rod", LengthMM: floatPtr(16.3)}, {Name: "lower_arm"}, {Name: "upper_arm", LengthMM: floatPtr(42.123456789)}}
	right, err := f.service.Create(ctx, f.other.ID, CreateInput{Title: "Other links", Visibility: Public, Data: &replacement})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		want   [3]int
	}{
		{"guest", nil, [3]int{200, 404, 404}}, {"owner", owner, [3]int{200, 200, 200}}, {"friend", friend, [3]int{200, 200, 404}},
	} {
		for i, left := range records {
			w := request(router, "GET", "/api/v1/setups/compare?left_id="+left.ID+"&right_id="+right.ID, "", tc.cookie)
			if w.Code != tc.want[i] || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(tc.name, left.Visibility, w.Code, w.Body)
			}
			if w.Code == 200 {
				var result Comparison
				if json.Unmarshal(w.Body.Bytes(), &result) != nil || !reflect.DeepEqual(result.Left.Data, data) || !reflect.DeepEqual(result.Right.Data, replacement) {
					t.Fatal("Compare dropped/reordered/filled links", w.Body)
				}
				if result.Left.Data.Suspension.Front.LinkLengths[1].LengthMM != nil || *result.Left.Data.Suspension.Rear.ToeDeg != 0 || result.Left.Data.Suspension.Rear.CasterDeg != nil {
					t.Fatal("missing/zero semantics changed")
				}
			} else if strings.Contains(w.Body.String(), right.Title) || strings.Contains(w.Body.String(), "link_lengths") {
				t.Fatal("partial comparison leak", w.Body)
			}
		}
	}
	// Failed full-data replacements cannot overwrite existing technical data.
	for _, links := range []string{`[{"name":"upper_arm","length_mm":0}]`, `[{"name":"lower_arm","length_mm":-1}]`, `[{"name":"bad","length_mm":"16.2"}]`, `[{"name":" "}]`} {
		w := request(router, "PATCH", "/api/v1/setups/"+records[0].ID, `{"data":{"suspension":{"front":{"link_lengths":`+links+`}}}}`, owner)
		if w.Code != 400 {
			t.Fatal("invalid update accepted", w.Code, w.Body)
		}
		actual, err := f.service.Get(ctx, f.owner.ID, records[0].ID)
		if err != nil || !reflect.DeepEqual(actual.Data, data) {
			t.Fatal("failed PATCH changed data", err)
		}
	}
	// Name-only/null, rename, removal and full replacement preserve all unrelated fields.
	replacement.Suspension.Front.LinkLengths = []LinkLength{{Name: "Renamed custom"}, {Name: "steering_tie_rod", LengthMM: floatPtr(16.2123456789)}}
	updatedRaw, err := json.Marshal(replacement)
	if err != nil {
		t.Fatal(err)
	}
	updated := setupResponse(t, request(router, "PATCH", "/api/v1/setups/"+records[0].ID, `{"data":`+string(updatedRaw)+`}`, owner), 200)
	if !reflect.DeepEqual(updated.Data, replacement) {
		t.Fatal("rename/removal lost unrelated data")
	}
	// Existing valid free-text/numeric rows are not rewritten by a title update.
	legacy := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Legacy","visibility":"public","data":`+realisticJSON+`}`, owner), 201)
	legacyBefore := legacy.Data
	legacy = setupResponse(t, request(router, "PATCH", "/api/v1/setups/"+legacy.ID, `{"title":"Legacy title"}`, owner), 200)
	if !reflect.DeepEqual(legacy.Data, legacyBefore) || legacy.Data.Suspension.Front.LinkLengths[0].Name != "camber_link" {
		t.Fatal("legacy rewrite")
	}
	// Read a preexisting raw JSONB null without coalescing it to zero.
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET data=jsonb_set(data,'{suspension,front,link_lengths}',$2::jsonb) WHERE id=$1`, legacy.ID, `[{"name":"Legacy unknown","length_mm":null},{"name":"Original custom","length_mm":12.3456789}]`); err != nil {
		t.Fatal(err)
	}
	legacy = setupResponse(t, request(router, "GET", "/api/v1/setups/"+legacy.ID, "", nil), 200)
	if legacy.Data.Suspension.Front.LinkLengths[0].LengthMM != nil || *legacy.Data.Suspension.Front.LinkLengths[1].LengthMM != 12.3456789 {
		t.Fatal("stored null/decimal changed")
	}
	legacy = setupResponse(t, request(router, "PATCH", "/api/v1/setups/"+legacy.ID, `{"title":"Still legacy"}`, owner), 200)
	compared, err := f.service.Compare(ctx, "", legacy.ID, right.ID)
	if err != nil || !reflect.DeepEqual(compared.Left.Data, legacy.Data) {
		t.Fatal("legacy null comparison", err)
	}
	for _, raw := range []string{`{}`, `{"suspension":{"front":{"camber_deg":0},"rear":{"toe_deg":-1.5}}}`, `{"suspension":{"front":{"link_lengths":[{"name":"upper_arm"},{"name":"steering_tie_rod","length_mm":16.2}]},"rear":{"link_lengths":[{"name":"lower_arm","length_mm":50}]}}}`} {
		optional := setupResponse(t, request(router, "POST", "/api/v1/setups", `{"title":"Optional","visibility":"public","data":`+raw+`}`, owner), 201)
		read, err := f.service.Compare(ctx, "", optional.ID, right.ID)
		if err != nil || !reflect.DeepEqual(read.Left.Data, optional.Data) {
			t.Fatal("angles/lengths/empty comparison", err)
		}
	}
	// Comparison authorization remains current after removal/deletion.
	if _, err := f.pool.Exec(ctx, `DELETE FROM friendships`); err != nil {
		t.Fatal(err)
	}
	if w := request(router, "GET", "/api/v1/setups/compare?left_id="+records[1].ID+"&right_id="+right.ID, "", friend); w.Code != 404 {
		t.Fatal("revoked link data leaked", w.Code)
	}
	if w := request(router, "PATCH", "/api/v1/setups/"+right.ID, `{"title":"Forbidden"}`, owner); w.Code != 404 {
		t.Fatal("Compare granted write", w.Code)
	}
}
