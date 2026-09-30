package admin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Stats struct{ TotalUsers, TotalSetups, PublicSetups int64 }

// UserRow is an admin-only read projection, including email but no identity/session data.
type UserRow struct {
	ID         string
	Nickname   string
	Email      string
	CreatedAt  time.Time
	SetupCount int64
}
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	err := r.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users),count(*),count(*) FILTER (WHERE visibility='public') FROM setups`).Scan(&stats.TotalUsers, &stats.TotalSetups, &stats.PublicSetups)
	return stats, err
}

// Users includes zero-setup accounts and obtains every count in one aggregate query.
func (r *Repository) Users(ctx context.Context) ([]UserRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT u.id::text,u.nickname,u.email,u.created_at,count(s.id)
 FROM users u LEFT JOIN setups s ON s.owner_id=u.id
 GROUP BY u.id ORDER BY u.created_at DESC,u.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UserRow{}
	for rows.Next() {
		var user UserRow
		if err := rows.Scan(&user.ID, &user.Nickname, &user.Email, &user.CreatedAt, &user.SetupCount); err != nil {
			return nil, err
		}
		user.CreatedAt = user.CreatedAt.UTC()
		result = append(result, user)
	}
	return result, rows.Err()
}
