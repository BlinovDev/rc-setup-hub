package users

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrInvalidUserID = errors.New("invalid user UUID")

// GetPublic returns only the public application profile, without friendship state.
func (s *Service) GetPublic(ctx context.Context, id string) (PublicProfile, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(id); err != nil || !uuid.Valid {
		return PublicProfile{}, ErrInvalidUserID
	}
	return s.repo.GetPublic(ctx, uuid.String())
}
