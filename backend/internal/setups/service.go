package setups

import (
	"context"
	"errors"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/chassis"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/friendships"
	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	uploadSlots chan struct{}
	photos      *photos.Store
	repo        *Repository
	catalog     *chassis.Service
	friends     *friendships.Repository
}

func NewService(repo *Repository, catalog *chassis.Service, friends *friendships.Repository) *Service {
	return &Service{repo: repo, catalog: catalog, friends: friends}
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
func (s *Service) validateManufacturer(ctx context.Context, id *string, component string) error {
	if id == nil {
		return nil
	}
	brand, err := s.catalog.GetBrand(ctx, *id)
	if errors.Is(err, chassis.ErrNotFound) || errors.Is(err, chassis.ErrInvalidID) {
		return invalid(component + " manufacturer must exist and be active")
	}
	if err != nil {
		return err
	}
	if !brand.IsActive {
		return invalid(component + " manufacturer must exist and be active")
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
	for _, ref := range manufacturerReferences(setup.Data) {
		if err := s.validateManufacturer(ctx, ref.id, ref.component); err != nil {
			return Setup{}, err
		}
	}
	return s.repo.Create(ctx, ownerID, setup)
}
func (s *Service) Get(ctx context.Context, callerID, id string) (Setup, error) {
	if err := validID(id); err != nil {
		return Setup{}, err
	}
	access, err := s.repo.getAccess(ctx, id)
	if err != nil {
		return Setup{}, err
	}
	accepted := false
	if !CanView(callerID, access.OwnerID, access.Visibility, false) && CanView(callerID, access.OwnerID, access.Visibility, true) {
		accepted, err = s.friends.AreAccepted(ctx, callerID, access.OwnerID)
		if err != nil {
			return Setup{}, err
		}
	}
	if !CanView(callerID, access.OwnerID, access.Visibility, accepted) {
		return Setup{}, ErrNotFound
	}
	return s.repo.getAuthorized(ctx, id, access)
}

func (s *Service) ListUser(ctx context.Context, callerID, ownerID string) ([]Setup, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(ownerID); err != nil || !uuid.Valid {
		return nil, invalid("invalid user ID")
	}
	ownerID = uuid.String()
	accepted := false
	if callerID != ownerID {
		var err error
		accepted, err = s.friends.AreAccepted(ctx, callerID, ownerID)
		if err != nil {
			return nil, err
		}
	}
	return s.repo.ListVisible(ctx, ownerID, VisibleVisibilities(callerID, ownerID, accepted))
}
func (s *Service) List(ctx context.Context, ownerID string) ([]Setup, error) {
	return s.repo.ListOwned(ctx, ownerID)
}
func (s *Service) Delete(ctx context.Context, ownerID, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	if err := s.repo.DeleteOwned(ctx, ownerID, id); err != nil {
		return err
	}
	if s.photos != nil {
		s.cleanupPhotos(ctx)
	}
	return nil
}
func sameCatalogID(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// UUID-equivalent spellings should not invalidate an unchanged historical reference.
	var x, y pgtype.UUID
	return x.Scan(*a) == nil && y.Scan(*b) == nil && x == y
}
func (s *Service) Patch(ctx context.Context, ownerID, id string, input PatchInput) (Setup, error) {
	if err := validID(id); err != nil {
		return Setup{}, err
	}
	setup, err := s.repo.GetOwned(ctx, ownerID, id)
	if err != nil {
		return Setup{}, err
	}
	expected := setup.UpdatedAt
	previousManufacturers := manufacturerReferences(setup.Data)
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
		if !sameCatalogID(setup.ChassisModelID, input.ChassisModelID.Value) {
			if err := s.validateChassis(ctx, input.ChassisModelID.Value); err != nil {
				return Setup{}, err
			}
		}
		setup.ChassisModelID = input.ChassisModelID.Value
	}
	if err := normalize(&setup); err != nil {
		return Setup{}, err
	}
	// Compare the same component slot, never a brand used elsewhere in the document.
	for i, ref := range manufacturerReferences(setup.Data) {
		if !sameCatalogID(previousManufacturers[i].id, ref.id) {
			if err := s.validateManufacturer(ctx, ref.id, ref.component); err != nil {
				return Setup{}, err
			}
		}
	}
	return s.repo.UpdateOwned(ctx, ownerID, id, setup, expected)
}
