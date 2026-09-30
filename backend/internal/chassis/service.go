package chassis

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }
func validateID(id string) error {
	var uuid pgtype.UUID
	if err := uuid.Scan(id); err != nil || !uuid.Valid {
		return ErrInvalidID
	}
	return nil
}
func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidName
		}
	}
	if strings.EqualFold(name, "Custom") {
		return "", ErrCustomName
	}
	return name, nil
}
func (s *Service) ListBrands(ctx context.Context) ([]Brand, error) {
	return s.repo.ListBrands(ctx, false)
}
func (s *Service) ActiveBrands(ctx context.Context) ([]Brand, error) {
	return s.repo.ListBrands(ctx, true)
}
func (s *Service) ListModels(ctx context.Context) ([]ModelState, error) {
	return s.repo.ListModels(ctx)
}
func (s *Service) CreateBrand(ctx context.Context, name string) (Brand, error) {
	name, err := validateName(name)
	if err != nil {
		return Brand{}, err
	}
	return s.repo.CreateBrand(ctx, name)
}
func (s *Service) RenameBrand(ctx context.Context, id, name string) (Brand, error) {
	if err := validateID(id); err != nil {
		return Brand{}, err
	}
	name, err := validateName(name)
	if err != nil {
		return Brand{}, err
	}
	return s.repo.RenameBrand(ctx, id, name)
}
func (s *Service) SetBrandActive(ctx context.Context, id string, active bool) (Brand, error) {
	if err := validateID(id); err != nil {
		return Brand{}, err
	}
	return s.repo.SetBrandActive(ctx, id, active)
}
func (s *Service) CreateModel(ctx context.Context, brandID, name string) (Model, error) {
	if err := validateID(brandID); err != nil {
		return Model{}, err
	}
	name, err := validateName(name)
	if err != nil {
		return Model{}, err
	}
	brand, err := s.repo.GetBrand(ctx, brandID)
	if err != nil {
		return Model{}, err
	}
	if !brand.IsActive {
		return Model{}, ErrInactiveBrand
	}
	return s.repo.CreateModel(ctx, brandID, name)
}
func (s *Service) RenameModel(ctx context.Context, id, name string) (Model, error) {
	if err := validateID(id); err != nil {
		return Model{}, err
	}
	name, err := validateName(name)
	if err != nil {
		return Model{}, err
	}
	return s.repo.RenameModel(ctx, id, name)
}
func (s *Service) SetModelActive(ctx context.Context, id string, active bool) (Model, error) {
	if err := validateID(id); err != nil {
		return Model{}, err
	}
	return s.repo.SetModelActive(ctx, id, active)
}

// GetModel returns historical values and independent activation flags for future setup validation.
func (s *Service) GetModel(ctx context.Context, id string) (ModelState, error) {
	if err := validateID(id); err != nil {
		return ModelState{}, err
	}
	return s.repo.GetModel(ctx, id)
}
func (s *Service) ActiveModels(ctx context.Context, brandID string) ([]Model, error) {
	if err := validateID(brandID); err != nil {
		return nil, err
	}
	brand, err := s.repo.GetBrand(ctx, brandID)
	if err != nil {
		return nil, err
	}
	if !brand.IsActive {
		return nil, ErrNotFound
	}
	return s.repo.ActiveModels(ctx, brandID)
}
