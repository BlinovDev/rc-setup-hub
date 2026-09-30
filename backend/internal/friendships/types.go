package friendships

import (
	"errors"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/users"
)

type Status string

const (
	Pending  Status = "pending"
	Accepted Status = "accepted"
)

var (
	ErrInvalidID    = errors.New("invalid UUID")
	ErrSelfRequest  = errors.New("cannot send a friend request to yourself")
	ErrNotFound     = errors.New("friendship not found")
	ErrUserNotFound = errors.New("user not found")
	ErrConflict     = errors.New("relationship already exists or request already accepted")
)

type Relationship struct {
	ID          string    `json:"id"`
	RequesterID string    `json:"requester_id"`
	AddresseeID string    `json:"addressee_id"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Item struct {
	ID        string              `json:"id"`
	User      users.PublicProfile `json:"user"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}
type List struct {
	Incoming []Item `json:"incoming"`
	Outgoing []Item `json:"outgoing"`
	Accepted []Item `json:"accepted"`
}
type listedRelationship struct {
	Item
	Status   Status
	Incoming bool
}
