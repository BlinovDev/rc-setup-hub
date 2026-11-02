package users

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// DriverProfile extends the compact profile used in search/friendship projections.
type DriverProfile struct {
	PublicProfile
	MemberSince      time.Time `json:"member_since"`
	PublicSetupCount int64     `json:"public_setup_count"`
	InstagramURL     *string   `json:"instagram_url"`
	TikTokURL        *string   `json:"tiktok_url"`
	YouTubeURL       *string   `json:"youtube_url"`
}

var ErrInvalidUserID = errors.New("invalid user UUID")

// GetPublic returns only the public application profile, without friendship state.
func (s *Service) GetPublic(ctx context.Context, id string) (DriverProfile, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(id); err != nil || !uuid.Valid {
		return DriverProfile{}, ErrInvalidUserID
	}
	return s.repo.GetPublic(ctx, uuid.String())
}
