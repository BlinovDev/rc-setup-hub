package setups

import (
	"context"
	"errors"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	repo    *Repository
	catalog *chassis.Service
}

func NewService(repo *Repository, catalog *chassis.Service) *Service {
	return &Service{repo: repo, catalog: catalog}
}
func validID(id string) error {
	var uuid pgtype.UUID
	if err := uuid.Scan(id); err != nil || !uuid.Valid {
		return invalid("invalid setup ID")
	}
	return nil
}
func (s *Service) validateChassis(ctx context.Context, id *string) error {
	if id == nil {
		return nil
	}
	model, err := s.catalog.GetModel(ctx, *id)
	if errors.Is(err, chassis.ErrNotFound) || errors.Is(err, chassis.ErrInvalidID) {
		return ErrInvalidChassis
	}
	if err != nil {
		return err
	}
	if !model.IsActive || !model.BrandActive {
		return ErrInvalidChassis
	}
	return nil
}
func (s *Service) Create(ctx context.Context, ownerID string, input CreateInput) (Setup, error) {
	if input.Data == nil {
		return Setup{}, invalid("data must be a schema-v1 JSON object")
	}
	setup := Setup{Title: input.Title, ChassisModelID: input.ChassisModelID, Visibility: input.Visibility, Notes: input.Notes, Data: *input.Data, SchemaVersion: SchemaVersion}
	if err := normalize(&setup); err != nil {
		return Setup{}, err
	}
	if err := s.validateChassis(ctx, setup.ChassisModelID); err != nil {
		return Setup{}, err
	}
	return s.repo.Create(ctx, ownerID, setup)
}
func (s *Service) Get(ctx context.Context, ownerID, id string) (Setup, error) {
	if err := validID(id); err != nil {
		return Setup{}, err
	}
	return s.repo.GetOwned(ctx, ownerID, id)
}
func (s *Service) List(ctx context.Context, ownerID string) ([]Setup, error) {
	return s.repo.ListOwned(ctx, ownerID)
}
func (s *Service) Delete(ctx context.Context, ownerID, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	return s.repo.DeleteOwned(ctx, ownerID, id)
}
func sameChassis(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// UUID-equivalent spellings should not invalidate an unchanged historical reference.
	var x, y pgtype.UUID
	return x.Scan(*a) == nil && y.Scan(*b) == nil && x == y
}
func (s *Service) Patch(ctx context.Context, ownerID, id string, input PatchInput) (Setup, error) {
	setup, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return Setup{}, err
	}
	expected := setup.UpdatedAt
	if input.Title.Present {
		if err := requiredPatch("title", input.Title.Value); err != nil {
			return Setup{}, err
		}
		setup.Title = *input.Title.Value
	}
	if input.Visibility.Present {
		if err := requiredPatch("visibility", input.Visibility.Value); err != nil {
			return Setup{}, err
		}
		setup.Visibility = Visibility(*input.Visibility.Value)
	}
	if input.Notes.Present {
		setup.Notes = input.Notes.Value
	}
	if input.Data.Present {
		if input.Data.Value == nil {
			return Setup{}, invalid("data cannot be null")
		}
		setup.Data = *input.Data.Value
	}
	if input.ChassisModelID.Present {
		if !sameChassis(setup.ChassisModelID, input.ChassisModelID.Value) {
			if err := s.validateChassis(ctx, input.ChassisModelID.Value); err != nil {
				return Setup{}, err
			}
		}
		setup.ChassisModelID = input.ChassisModelID.Value
	}
	if err := normalize(&setup); err != nil {
		return Setup{}, err
	}
	return s.repo.UpdateOwned(ctx, ownerID, id, setup, expected)
}
