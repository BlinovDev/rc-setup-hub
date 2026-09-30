package chassis

import (
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("catalog entry not found")
	ErrDuplicate     = errors.New("that name is already in use")
	ErrInactiveBrand = errors.New("cannot create a model under an inactive brand")
	ErrInvalidName   = errors.New("name must contain 1 to 100 characters without control characters")
	ErrCustomName    = errors.New("Custom is a client-only option, not a catalog entry")
	ErrInvalidID     = errors.New("invalid catalog ID")
)

type Brand struct {
	ID, Name             string
	IsActive             bool
	CreatedAt, UpdatedAt time.Time
}
type Model struct {
	ID, BrandID, Name    string
	IsActive             bool
	CreatedAt, UpdatedAt time.Time
}

// ModelState preserves historical catalog data and both activation flags for setup validation.
type ModelState struct {
	Model
	BrandName   string
	BrandActive bool
}
