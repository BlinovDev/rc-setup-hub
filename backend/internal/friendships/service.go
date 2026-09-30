package friendships

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }
func uuidID(id string) (string, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(id); err != nil || !uuid.Valid {
		return "", ErrInvalidID
	}
	return uuid.String(), nil
}
func (s *Service) Create(ctx context.Context, callerID, targetID string) (Relationship, error) {
	targetID, err := uuidID(targetID)
	if err != nil {
		return Relationship{}, err
	}
	if callerID == targetID {
		return Relationship{}, ErrSelfRequest
	}
	return s.repo.Create(ctx, callerID, targetID)
}
func (s *Service) Accept(ctx context.Context, callerID, id string) (Relationship, error) {
	id, err := uuidID(id)
	if err != nil {
		return Relationship{}, err
	}
	relationship, err := s.repo.Accept(ctx, callerID, id)
	if !errors.Is(err, ErrNotFound) {
		return relationship, err
	}
	relationship, err = s.repo.GetParticipant(ctx, callerID, id)
	if err != nil {
		return Relationship{}, err
	}
	if relationship.AddresseeID != callerID {
		return Relationship{}, ErrNotFound
	}
	if relationship.Status == Accepted {
		return Relationship{}, ErrConflict
	}
	return Relationship{}, ErrNotFound
}
func (s *Service) Delete(ctx context.Context, callerID, id string) error {
	id, err := uuidID(id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, callerID, id)
}
func (s *Service) List(ctx context.Context, callerID string) (List, error) {
	rows, err := s.repo.list(ctx, callerID)
	result := List{Incoming: []Item{}, Outgoing: []Item{}, Accepted: []Item{}}
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		switch row.Status {
		case Accepted:
			result.Accepted = append(result.Accepted, row.Item)
		case Pending:
			if row.Incoming {
				result.Incoming = append(result.Incoming, row.Item)
			} else {
				result.Outgoing = append(result.Outgoing, row.Item)
			}
		default:
			return result, errors.New("unsupported friendship status")
		}
	}
	return result, nil
}
