package setups

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSearchValidation(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	cursor := encodeSearchCursor(searchCursor{CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 123456000, time.UTC), ID: id})
	for _, tc := range []struct {
		name  string
		input SearchInput
		valid bool
		limit int
	}{
		{"default", SearchInput{}, true, 20},
		{"trim Unicode", SearchInput{Q: "  " + strings.Repeat("界", 100) + "  "}, true, 20},
		{"limit one", SearchInput{Limit: stringPtr("1")}, true, 1},
		{"maximum", SearchInput{Limit: stringPtr("50")}, true, 50},
		{"all filters", SearchInput{Q: " carpet ", BrandID: &id, ModelID: &id, Limit: stringPtr("2"), Cursor: &cursor}, true, 2},
		{"too long", SearchInput{Q: strings.Repeat("a", 101)}, false, 0},
		{"bad UTF8", SearchInput{Q: "\xff"}, false, 0},
		{"null byte", SearchInput{Q: "bad\x00query"}, false, 0},
		{"invalid brand", SearchInput{BrandID: stringPtr("invalid")}, false, 0},
		{"empty brand", SearchInput{BrandID: stringPtr("")}, false, 0},
		{"invalid model", SearchInput{ModelID: stringPtr("invalid")}, false, 0},
		{"zero", SearchInput{Limit: stringPtr("0")}, false, 0},
		{"negative", SearchInput{Limit: stringPtr("-1")}, false, 0},
		{"over maximum", SearchInput{Limit: stringPtr("51")}, false, 0},
		{"malformed limit", SearchInput{Limit: stringPtr("1.5")}, false, 0},
		{"overflow", SearchInput{Limit: stringPtr(strings.Repeat("9", 100))}, false, 0},
		{"empty limit", SearchInput{Limit: stringPtr("")}, false, 0},
		{"bad cursor", SearchInput{Cursor: stringPtr("garbage")}, false, 0},
		{"empty cursor", SearchInput{Cursor: stringPtr("")}, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, err := normalizeSearch(tc.input)
			if tc.valid {
				if err != nil || params.limit != tc.limit || params.query != strings.TrimSpace(tc.input.Q) {
					t.Fatal(params, err)
				}
			} else {
				var validation *ValidationError
				if !errors.As(err, &validation) {
					t.Fatal("expected validation", err)
				}
			}
		})
	}
	decoded, err := decodeSearchCursor(cursor)
	if err != nil || decoded.ID != id || encodeSearchCursor(*decoded) != cursor || decoded.CreatedAt.Nanosecond() != 123456000 {
		t.Fatal("cursor precision", decoded, err)
	}
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	for _, bad := range []string{"", cursor + "=", cursor + "\n", strings.Repeat("x", 257), encode(""), encode("2025-01-02T03:04:05Z"), encode("invalid|" + id), encode("9999-12-31T23:59:59-12:00|" + id), encode("0001-01-01T00:00:00+12:00|" + id), encode("2025-99-02T03:04:05Z|" + id), encode("2025-01-02T03:04:05Z|invalid"), encode("2025-01-02T03:04:05Z|"), encode("2025-01-02T03:04:05Z|" + id + "|extra"), encode(`{"created_at":"2025-01-02T03:04:05Z","id":"` + id + `","extra":1}`)} {
		if _, err := decodeSearchCursor(bad); err == nil {
			t.Fatal("accepted malformed cursor", bad)
		}
	}
}
