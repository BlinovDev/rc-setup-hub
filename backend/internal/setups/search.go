package setups

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

// SearchInput contains raw query values; nil distinguishes absent optional parameters.
type SearchInput struct {
	Q                               string
	BrandID, ModelID, Limit, Cursor *string
}

// SearchItem is a browse summary; complete technical data remains on the detail endpoint.
type SearchItem struct {
	CoverPhoto     *Photo              `json:"cover_photo"`
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	Visibility     Visibility          `json:"visibility"`
	ChassisModelID *string             `json:"chassis_model_id"`
	SchemaVersion  int                 `json:"schema_version"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	Owner          users.PublicProfile `json:"owner"`
	Chassis        *SearchChassis      `json:"chassis"`
}
type SearchChassis struct {
	ModelID   string `json:"model_id"`
	ModelName string `json:"model_name"`
	BrandID   string `json:"brand_id"`
	BrandName string `json:"brand_name"`
}
type SearchPage struct {
	Items      []SearchItem `json:"items"`
	NextCursor *string      `json:"next_cursor"`
}
type searchCursor struct {
	CreatedAt time.Time
	ID        string
}
type searchParameters struct {
	query            string
	ownerID          *string
	brandID, modelID *string
	limit            int
	cursor           *searchCursor
}

func encodeSearchCursor(cursor searchCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + cursor.ID))
}
func decodeSearchCursor(value string) (*searchCursor, error) {
	if len(value) == 0 || len(value) > 256 {
		return nil, invalid("invalid cursor")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != value {
		return nil, invalid("invalid cursor")
	}
	fields := strings.Split(string(raw), "|")
	if len(fields) != 2 {
		return nil, invalid("invalid cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, fields[0])
	if err != nil || createdAt.UTC().Year() < 1 || createdAt.UTC().Year() > 9999 {
		return nil, invalid("invalid cursor")
	}
	id, err := canonicalUUID(&fields[1], "cursor id")
	if err != nil {
		return nil, invalid("invalid cursor")
	}
	return &searchCursor{CreatedAt: createdAt.UTC(), ID: *id}, nil
}
func normalizeSearch(input SearchInput) (searchParameters, error) {
	params := searchParameters{query: strings.TrimSpace(input.Q), limit: 20}
	if !utf8.ValidString(params.query) || strings.ContainsRune(params.query, 0) || utf8.RuneCountInString(params.query) > 100 {
		return params, invalid("q must contain at most 100 characters without null bytes")
	}
	var err error
	params.brandID, err = canonicalUUID(input.BrandID, "brand_id")
	if err != nil {
		return params, err
	}
	params.modelID, err = canonicalUUID(input.ModelID, "model_id")
	if err != nil {
		return params, err
	}
	if input.Limit != nil {
		params.limit, err = strconv.Atoi(*input.Limit)
		if err != nil || params.limit < 1 || params.limit > 50 {
			return params, invalid("limit must be an integer from 1 to 50")
		}
	}
	if input.Cursor != nil {
		params.cursor, err = decodeSearchCursor(*input.Cursor)
		if err != nil {
			return params, err
		}
	}
	return params, nil
}

// Search is deliberately public-only, regardless of the caller's ownership or friendships.
func (s *Service) Search(ctx context.Context, input SearchInput) (SearchPage, error) {
	params, err := normalizeSearch(input)
	if err != nil {
		return SearchPage{}, err
	}
	items, err := s.repo.searchPublic(ctx, params)
	if err != nil {
		return SearchPage{}, err
	}
	return publicPage(items, params.limit), nil
}
func publicPage(items []SearchItem, limit int) SearchPage {
	page := SearchPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		cursor := encodeSearchCursor(searchCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		page.NextCursor = &cursor
	}
	return page
}

// PublicUserSetups is public-only even for the owner/friends; the existing ListUser
// endpoint retains its authenticated visibility-dependent array contract.
func (s *Service) PublicUserSetups(ctx context.Context, ownerID string, input SearchInput) (SearchPage, error) {
	params, err := normalizeSearch(input)
	if err != nil {
		return SearchPage{}, err
	}
	params.ownerID, err = canonicalUUID(&ownerID, "user_id")
	if err != nil {
		return SearchPage{}, err
	}
	items, err := s.repo.searchPublic(ctx, params)
	if err != nil {
		return SearchPage{}, err
	}
	return publicPage(items, params.limit), nil
}
