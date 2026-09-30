package setups

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/database/dbtest"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
	"github.com/jackc/pgx/v5/pgxpool"
)

type searchFixture struct {
	pool                 *pgxpool.Pool
	repo                 *Repository
	service              *Service
	catalog              *chassis.Service
	owner, friend, other users.User
	yokomo, mst          chassis.Brand
	rd, sd, rmx          chassis.Model
	public               []Setup
	friends, private     Setup
}

func newSearchFixture(t *testing.T) searchFixture {
	t.Helper()
	ctx := context.Background()
	f := searchFixture{pool: dbtest.New(t)}
	f.repo = NewRepository(f.pool)
	f.catalog = chassis.NewService(chassis.NewRepository(f.pool))
	f.service = NewService(f.repo, f.catalog, friendships.NewRepository(f.pool))
	userService := users.NewService(users.NewRepository(f.pool))
	makeUser := func(subject, email, name string) users.User {
		t.Helper()
		u, err := userService.Login(ctx, users.Identity{Subject: subject, Email: email, Name: name, AvatarURL: "https://example.com/avatar"})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	f.owner = makeUser("search-owner", "email-marker@example.com", "Anton-Blinov")
	f.friend = makeUser("search-friend", "friend@example.com", "Friend-Driver")
	f.other = makeUser("search-other", "other@example.com", "MST-Driver")
	makeBrand := func(name string) chassis.Brand {
		t.Helper()
		b, err := f.catalog.CreateBrand(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	makeModel := func(b chassis.Brand, name string) chassis.Model {
		t.Helper()
		m, err := f.catalog.CreateModel(ctx, b.ID, name)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	f.yokomo = makeBrand("Yokomo")
	f.mst = makeBrand("MST")
	f.rd = makeModel(f.yokomo, "RD2.0")
	f.sd = makeModel(f.yokomo, "SD2.0")
	f.rmx = makeModel(f.mst, "RMX")
	for i, tc := range []struct {
		title      string
		model      *string
		owner      string
		visibility Visibility
	}{
		{"RD2 Carpet", &f.rd.ID, f.owner.ID, Public}, {"SD2 Asphalt 100%", &f.sd.ID, f.owner.ID, Public}, {"MST RMX under_score", &f.rmx.ID, f.other.ID, Public}, {"Custom carpet", nil, f.owner.ID, Public},
		{"RD2 Carpet", &f.rd.ID, f.owner.ID, Friends}, {"RD2 Carpet", &f.rd.ID, f.owner.ID, Private},
	} {
		s, err := f.service.Create(ctx, tc.owner, CreateInput{Title: tc.title, ChassisModelID: tc.model, Visibility: tc.visibility, Notes: stringPtr("notes-marker"), Data: &DataV1{Electronics: &Electronics{Motor: "technical-marker"}}})
		if err != nil {
			t.Fatal(err)
		}
		s.CreatedAt = time.Date(2024, 1, i+1, 0, 0, 0, 123456000, time.UTC)
		if _, err := f.pool.Exec(ctx, `UPDATE setups SET created_at=$2 WHERE id=$1`, s.ID, s.CreatedAt); err != nil {
			t.Fatal(err)
		}
		switch tc.visibility {
		case Public:
			f.public = append(f.public, s)
		case Friends:
			f.friends = s
		case Private:
			f.private = s
		}
	}
	return f
}
func sortedIDs(rows []Setup) []string {
	rows = append([]Setup(nil), rows...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	ids := []string{}
	for _, s := range rows {
		ids = append(ids, s.ID)
	}
	return ids
}
func searchIDs(items []SearchItem) []string {
	ids := []string{}
	for _, s := range items {
		ids = append(ids, s.ID)
	}
	return ids
}
func TestPublicSearchRepository(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	unknown := "00000000-0000-0000-0000-000000000000"
	all := sortedIDs(f.public)
	for _, tc := range []struct {
		name  string
		input SearchInput
		want  []string
	}{
		{"all", SearchInput{}, all}, {"empty text", SearchInput{Q: "  "}, all},
		{"carpet", SearchInput{Q: "  CaRpEt  "}, []string{f.public[3].ID, f.public[0].ID}},
		{"nickname", SearchInput{Q: "bLiNoV"}, []string{f.public[3].ID, f.public[1].ID, f.public[0].ID}},
		{"rd2", SearchInput{Q: "rd2"}, []string{f.public[0].ID}},
		{"email excluded", SearchInput{Q: "email-marker"}, []string{}}, {"notes excluded", SearchInput{Q: "notes-marker"}, []string{}}, {"technical excluded", SearchInput{Q: "technical-marker"}, []string{}},
		{"literal percent", SearchInput{Q: "%"}, []string{f.public[1].ID}}, {"literal underscore", SearchInput{Q: "_"}, []string{f.public[2].ID}},
		{"SQL input stays literal", SearchInput{Q: "' OR true --"}, []string{}},
		{"Yokomo", SearchInput{BrandID: &f.yokomo.ID}, []string{f.public[1].ID, f.public[0].ID}},
		{"MST", SearchInput{BrandID: &f.mst.ID}, []string{f.public[2].ID}},
		{"RD2", SearchInput{ModelID: &f.rd.ID}, []string{f.public[0].ID}},
		{"both", SearchInput{BrandID: &f.yokomo.ID, ModelID: &f.rd.ID}, []string{f.public[0].ID}},
		{"mismatch", SearchInput{BrandID: &f.yokomo.ID, ModelID: &f.rmx.ID}, []string{}},
		{"unknown brand", SearchInput{BrandID: &unknown}, []string{}}, {"unknown model", SearchInput{ModelID: &unknown}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := f.service.Search(ctx, tc.input)
			if err != nil || !reflect.DeepEqual(searchIDs(page.Items), tc.want) || page.NextCursor != nil {
				t.Fatal(page, err, tc.want)
			}
		})
	}
	page, err := f.service.Search(ctx, SearchInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Visibility != Public || item.SchemaVersion != 1 {
			t.Fatal("search metadata", item)
		}
		if item.Owner.ID == "" || item.Owner.Nickname == "" || item.Owner.AvatarURL == nil {
			t.Fatal("owner summary", item)
		}
		if item.ChassisModelID == nil {
			if item.Chassis != nil {
				t.Fatal("custom chassis", item)
			}
		} else if item.Chassis == nil || item.Chassis.ModelID != *item.ChassisModelID || item.Chassis.BrandName == "" || item.Chassis.ModelName == "" {
			t.Fatal("chassis join", item)
		}
	}
	if _, err := f.catalog.SetModelActive(ctx, f.rd.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.catalog.SetBrandActive(ctx, f.yokomo.ID, false); err != nil {
		t.Fatal(err)
	}
	historical, err := f.service.Search(ctx, SearchInput{BrandID: &f.yokomo.ID, ModelID: &f.rd.ID})
	if err != nil || len(historical.Items) != 1 || historical.Items[0].Chassis.BrandName != "Yokomo" || historical.Items[0].Chassis.ModelName != "RD2.0" {
		t.Fatal("inactive history", historical, err)
	}
	// Unsupported and undecodable private/friends rows cannot enter the search scan.
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET schema_version=2,data='[]'::jsonb WHERE visibility<>'public'`); err != nil {
		t.Fatal(err)
	}
	page, err = f.service.Search(ctx, SearchInput{})
	if err != nil || !reflect.DeepEqual(searchIDs(page.Items), all) {
		t.Fatal("hidden rows reached search", page, err)
	}
	f.pool.Close()
	if _, err := f.service.Search(ctx, SearchInput{}); err == nil {
		t.Fatal("database error swallowed")
	}
}
func TestSearchKeysetPagination(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	expected := append([]Setup(nil), f.public...)
	for i := 0; i < 31; i++ {
		s, err := f.service.Create(ctx, f.owner.ID, CreateInput{Title: "Pagination carpet", ChassisModelID: &f.rd.ID, Visibility: Public, Data: &DataV1{}})
		if err != nil {
			t.Fatal(err)
		}
		// Groups share timestamps, including PostgreSQL's microsecond precision.
		s.CreatedAt = time.Date(2025, 1, 1+i/3, 0, 0, 0, 123456000, time.UTC)
		if _, err := f.pool.Exec(ctx, `UPDATE setups SET created_at=$2 WHERE id=$1`, s.ID, s.CreatedAt); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, s)
	}
	page, err := f.service.Search(ctx, SearchInput{})
	if err != nil || len(page.Items) != 20 || page.NextCursor == nil {
		t.Fatal("default page", page, err)
	}
	if want := sortedIDs(expected)[:20]; !reflect.DeepEqual(searchIDs(page.Items), want) {
		t.Fatal("first page order", page.Items, want)
	}
	decoded, err := decodeSearchCursor(*page.NextCursor)
	last := page.Items[19]
	if err != nil || decoded.ID != last.ID || !decoded.CreatedAt.Equal(last.CreatedAt) {
		t.Fatal("cursor not last returned row", decoded, err)
	}
	page, err = f.service.Search(ctx, SearchInput{Limit: stringPtr("50")})
	if err != nil || len(page.Items) != 35 || page.NextCursor != nil {
		t.Fatal("maximum/final", page, err)
	}
	walk := func(input SearchInput) []string {
		t.Helper()
		seen := map[string]bool{}
		ids := []string{}
		for i := 0; i < 100; i++ {
			page, err := f.service.Search(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range page.Items {
				if seen[item.ID] {
					t.Fatal("duplicate across pages", item.ID)
				}
				seen[item.ID] = true
				ids = append(ids, item.ID)
			}
			if page.NextCursor == nil {
				return ids
			}
			if len(page.Items) == 0 {
				t.Fatal("empty page has cursor")
			}
			input.Cursor = page.NextCursor
		}
		t.Fatal("pagination did not terminate")
		return nil
	}
	if got := walk(SearchInput{Limit: stringPtr("7")}); !reflect.DeepEqual(got, sortedIDs(expected)) {
		t.Fatal("missing/order", got, sortedIDs(expected))
	}
	filtered := []Setup{}
	for _, s := range expected {
		if s.ChassisModelID != nil && *s.ChassisModelID == f.rd.ID {
			filtered = append(filtered, s)
		}
	}
	if got := walk(SearchInput{Q: " CaRpEt ", BrandID: &f.yokomo.ID, ModelID: &f.rd.ID, Limit: stringPtr("2")}); !reflect.DeepEqual(got, sortedIDs(filtered)) {
		t.Fatal("filters not preserved", got, sortedIDs(filtered))
	}
	// A newer insert does not shift the second page or repeat page-one rows.
	first, err := f.service.Search(ctx, SearchInput{Limit: stringPtr("2")})
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := f.service.Create(ctx, f.owner.ID, CreateInput{Title: "New insert", Visibility: Public, Data: &DataV1{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE setups SET created_at='2030-01-01T00:00:00Z' WHERE id=$1`, inserted.ID); err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Search(ctx, SearchInput{Limit: stringPtr("2"), Cursor: first.NextCursor})
	if err != nil || !reflect.DeepEqual(searchIDs(second.Items), sortedIDs(expected)[2:4]) {
		t.Fatal("insert shifted next page", second, err)
	}
}
