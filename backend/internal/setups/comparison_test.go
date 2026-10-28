package setups

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
)

func TestComparisonIDs(t *testing.T) {
	a, b := "abcdefab-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	for _, pair := range [][2]string{{"", b}, {a, ""}, {"bad", b}, {a, "bad"}, {a, a}, {strings.ToUpper(a), a}} {
		if _, err := comparisonIDs(pair[0], pair[1]); err == nil {
			t.Fatal("invalid pair accepted", pair)
		}
	}
	ids, err := comparisonIDs(strings.ToUpper(a), b)
	if err != nil || !reflect.DeepEqual(ids, []string{a, b}) {
		t.Fatal(ids, err)
	}
}

func TestVisibleSelectionComparisonMatrix(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	records := append(append([]Setup{}, f.public...), f.friends, f.private)
	for _, owner := range []string{f.friend.ID, f.other.ID} {
		for _, visibility := range []Visibility{Friends, Private} {
			s, err := createReviewedFixture(t, f.service, ctx, owner, CreateInput{Title: "Additional visible candidate", Visibility: visibility, Data: &DataV1{}})
			if err != nil {
				t.Fatal(err)
			}
			records = append(records, s)
		}
	}
	for _, tc := range []struct {
		name, viewer, status string
		reverse              bool
	}{
		{"guest", "", "accepted", false}, {"owner", f.owner.ID, "pending", false},
		{"pending", f.friend.ID, "pending", false}, {"accepted", f.friend.ID, "accepted", false},
		{"accepted reverse", f.friend.ID, "accepted", true}, {"rejected", f.friend.ID, "", false},
		{"removed", f.friend.ID, "", false}, {"unrelated owner", f.other.ID, "accepted", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.pool.Exec(ctx, `DELETE FROM friendships`); err != nil {
				t.Fatal(err)
			}
			if tc.status != "" {
				a, b := f.owner.ID, f.friend.ID
				if tc.reverse {
					a, b = b, a
				}
				if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,$3)`, a, b, tc.status); err != nil {
					t.Fatal(err)
				}
			}
			expected := []Setup{}
			for _, record := range records {
				allowed := record.Visibility == Public || (tc.viewer != "" && record.OwnerID == tc.viewer) ||
					(tc.viewer == f.friend.ID && record.OwnerID == f.owner.ID && tc.status == "accepted" && record.Visibility == Friends)
				if allowed {
					expected = append(expected, record)
				}
				anchor := f.public[2]
				if record.ID == anchor.ID {
					anchor = f.public[0]
				}
				comparison, err := f.service.Compare(ctx, tc.viewer, record.ID, anchor.ID)
				detail, detailErr := f.service.Get(ctx, tc.viewer, record.ID)
				if allowed {
					if err != nil || comparison.Left.ID != record.ID || comparison.Right.ID != anchor.ID || detailErr != nil || detail.ID != record.ID {
						t.Fatalf("read parity %s: %+v %v %v", record.ID, comparison, err, detailErr)
					}
				} else if !errors.Is(err, ErrNotFound) || !errors.Is(detailErr, ErrNotFound) || comparison.Left.ID != "" || comparison.Right.ID != "" {
					t.Fatalf("inaccessible/partial comparison: %+v %v %v", comparison, err, detailErr)
				}
			}
			page, err := f.service.SearchVisible(ctx, tc.viewer, SearchInput{})
			if err != nil || !reflect.DeepEqual(searchIDs(page.Items), sortedIDs(expected)) {
				t.Fatalf("selector leakage/order: %v want %v: %v", searchIDs(page.Items), sortedIDs(expected), err)
			}
			public, err := f.service.Search(ctx, SearchInput{})
			if err != nil || len(public.Items) != len(f.public) {
				t.Fatal("Explore widened", public, err)
			}
		})
	}
	// Selecting while accepted does not create a permission capability.
	if _, err := f.pool.Exec(ctx, `DELETE FROM friendships`); err != nil {
		t.Fatal(err)
	}
	relation, err := friendships.NewRepository(f.pool).Create(ctx, f.owner.ID, f.friend.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = friendships.NewRepository(f.pool).Accept(ctx, f.friend.ID, relation.ID); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.SearchVisible(ctx, f.friend.ID, SearchInput{Q: "RD2 Carpet"})
	if err != nil || len(page.Items) != 2 {
		t.Fatal("accepted selection", page, err)
	}
	metadata, err := f.repo.getAccess(ctx, f.friends.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := friendships.NewRepository(f.pool).Delete(ctx, f.friend.ID, relation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.getAuthorized(ctx, f.friends.ID, metadata, f.friend.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale friendship detail authorization", err)
	}
	if _, err := f.service.Compare(ctx, f.friend.ID, f.friends.ID, f.public[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("removed friendship comparison", err)
	}
	// Cross-owner writes remain denied even when a public setup is selectable.
	if _, err := f.service.Patch(ctx, f.friend.ID, f.public[0].ID, PatchInput{Title: NullableString{Present: true, Value: stringPtr("forbidden")}}); !errors.Is(err, ErrNotFound) {
		t.Fatal("selector granted write", err)
	}
	if err := f.service.Delete(ctx, f.friend.ID, f.public[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("selector granted delete", err)
	}
}

func TestVisibleSelectionFiltersPaginationAndRevocation(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'accepted')`, f.owner.ID, f.friend.ID); err != nil {
		t.Fatal(err)
	}
	// Include all timestamps tied to exercise the UUID keyset tiebreaker.
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET created_at='2026-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	all, err := f.service.SearchVisible(ctx, f.owner.ID, SearchInput{})
	if err != nil {
		t.Fatal(err)
	}
	var cursor *string
	ids := []string{}
	for i := 0; i < 10; i++ {
		page, err := f.service.SearchVisible(ctx, f.owner.ID, SearchInput{Limit: stringPtr("1"), Cursor: cursor})
		if err != nil || len(page.Items) != 1 {
			t.Fatal(page, err)
		}
		ids = append(ids, page.Items[0].ID)
		cursor = page.NextCursor
		if cursor == nil {
			break
		}
	}
	if !reflect.DeepEqual(ids, searchIDs(all.Items)) {
		t.Fatal("pagination lost/duplicated rows", ids, searchIDs(all.Items))
	}
	for _, tc := range []struct {
		input SearchInput
		count int
	}{
		{SearchInput{Q: "  aNtOn-bLiNoV  "}, 5}, {SearchInput{Q: "%"}, 1}, {SearchInput{Q: "_"}, 1},
		{SearchInput{Q: "' OR true --"}, 0}, {SearchInput{Q: "email-marker@example.com"}, 0},
		{SearchInput{Q: "technical-marker"}, 0}, {SearchInput{Q: "notes-marker"}, 0},
		{SearchInput{BrandID: &f.yokomo.ID, ModelID: &f.rmx.ID}, 0}, {SearchInput{ModelID: &f.rd.ID}, 3},
	} {
		page, err := f.service.SearchVisible(ctx, f.owner.ID, tc.input)
		if err != nil || len(page.Items) != tc.count {
			t.Fatalf("filter %+v: %v %v", tc.input, page, err)
		}
	}
	// Both endpoints recheck committed visibility/deletion rather than trusting a selection/cursor.
	for _, visibility := range []Visibility{Friends, Private} {
		if _, err := f.pool.Exec(ctx, `UPDATE setups SET visibility=$2 WHERE id=$1`, f.public[0].ID, visibility); err != nil {
			t.Fatal(err)
		}
		if result, err := f.service.Compare(ctx, "", f.public[0].ID, f.public[1].ID); !errors.Is(err, ErrNotFound) || result.Left.ID != "" {
			t.Fatal("visibility changed after selection", result, err)
		}
		page, err := f.service.SearchVisible(ctx, "", SearchInput{ModelID: &f.rd.ID})
		if err != nil || len(page.Items) != 0 || page.NextCursor != nil {
			t.Fatal("private pagination leak", page, err)
		}
	}
	if err := f.service.Delete(ctx, f.owner.ID, f.public[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Compare(ctx, f.owner.ID, f.public[0].ID, f.public[1].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted comparison", err)
	}
	// No SQL/data decoding on the permitted side when the other side is inaccessible.
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET data='[]'::jsonb,schema_version=2 WHERE id=$1`, f.public[1].ID); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{f.public[1].ID, f.private.ID}, {f.private.ID, f.public[1].ID}, {f.public[1].ID, "00000000-0000-0000-0000-000000000000"}} {
		if result, err := f.service.Compare(ctx, "", pair[0], pair[1]); !errors.Is(err, ErrNotFound) || result.Left.ID != "" {
			t.Fatal("partial authorization leaked schema", result, err)
		}
	}
	var index string
	if err := f.pool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE indexname='setups_created_id_idx'`).Scan(&index); err != nil || !strings.Contains(index, "created_at DESC, id DESC") {
		t.Fatal("keyset index missing", index, err)
	}
}

func TestComparisonFullDataHistoricalChassisAndZero(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	var data DataV1
	raw := `{"suspension":{"front":{"camber_deg":-5.25,"toe_deg":0,"top_arm_spacing":{"front_mm":0,"rear_mm":5.2},"lower_arm_spacing":{"rear_mm":0},"lower_arm_mount":{"manufacturer_id":"` + f.yokomo.ID + `","configuration_notes":"Front config"}},"rear":{"toe_deg":null,"lower_arm_mount":{"model":"Rear mount"}}},"shocks":{"front":{"piston":{"holes":4,"hole_diameter_mm":1.25,"notes":"Piston notes"},"seal":{"type":"x-ring","description":"Seal"},"oil_cst":250,"spring":{"color":"purple"}},"rear":{"model":"Rear shock"}},"front_steering_knuckle":{"manufacturer_id":"` + f.yokomo.ID + `","description":"Knuckle"},"rear_differential":{"type":"lsd","manufacturer_id":"` + f.yokomo.ID + `","model":"Diff","tuning_notes":"Diff notes"},"running_gear":{"front":{"hex":{"offset_mm":0},"wheel":{"manufacturer_id":"` + f.yokomo.ID + `","model":"Wheel","offset_mm":0},"tire":{"model":"Front tire"}},"rear":{"hex":{"offset_mm":-0.3},"wheel":{"offset_mm":-4},"tire":{"manufacturer_id":"` + f.yokomo.ID + `","model":"Rear tire"}}},"electronics":{"motor":"Motor","esc":"ESC","servo":"Servo","gyro":"Gyro","radio":"Radio"}}`
	if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
		t.Fatal(err)
	}
	left, err := createReviewedFixture(t, f.service, ctx, f.owner.ID, CreateInput{Title: "Complete", Visibility: Public, ChassisModelID: &f.rd.ID, Data: &data, Notes: stringPtr("Setup notes")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.catalog.SetBrandActive(ctx, f.yokomo.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, right := range []Setup{f.public[2], f.public[3]} {
		result, err := f.service.Compare(ctx, "", left.ID, right.ID)
		if err != nil || !reflect.DeepEqual(result.Left, left) || !reflect.DeepEqual(result.Right, right) {
			t.Fatal("complete projection/order changed", result, err)
		}
		reversed, err := f.service.Compare(ctx, "", right.ID, left.ID)
		if err != nil || !reflect.DeepEqual(reversed.Right, left) {
			t.Fatal("reversed order", reversed, err)
		}
		if result.Left.Chassis == nil || result.Left.Chassis.ModelID != f.rd.ID {
			t.Fatal("historical chassis lost")
		}
		if right.ChassisModelID == nil && result.Right.Chassis != nil {
			t.Fatal("custom chassis invented")
		}
		body, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{`"toe_deg":0`, `"front_mm":0`, `"rear_mm":0`, `"offset_mm":0`, `"rear":{"lower_arm_mount"`, `"notes":"Setup notes"`} {
			if !strings.Contains(string(body), fragment) {
				t.Fatal("optional/zero data lost", fragment, string(body))
			}
		}
		if strings.Contains(string(body), `"caster_deg":0`) || strings.Contains(string(body), `"toe_deg":null`) {
			t.Fatal("missing measurement changed", string(body))
		}
	}
	// No comparison operation changes persisted setup data or timestamps.
	loaded, err := f.service.Get(ctx, f.owner.ID, left.ID)
	if err != nil || !reflect.DeepEqual(left, loaded) {
		t.Fatal("read modified setup", loaded, err)
	}
}
