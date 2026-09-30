package friendships

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// AreAccepted checks the unordered pair; pending requests never grant access.
func (r *Repository) AreAccepted(ctx context.Context, a, b string) (bool, error) {
	var accepted bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (
        SELECT 1 FROM friendships
        WHERE LEAST(requester_id,addressee_id)=LEAST($1::uuid,$2::uuid)
          AND GREATEST(requester_id,addressee_id)=GREATEST($1::uuid,$2::uuid)
          AND status='accepted'
    )`, a, b).Scan(&accepted)
	return accepted, err
}
