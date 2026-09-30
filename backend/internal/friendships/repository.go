package friendships

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

const relationshipColumns = `id::text,requester_id::text,addressee_id::text,status,created_at,updated_at`

func scanRelationship(row pgx.Row) (Relationship, error) {
	var relationship Relationship
	err := row.Scan(&relationship.ID, &relationship.RequesterID, &relationship.AddresseeID, &relationship.Status, &relationship.CreatedAt, &relationship.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Relationship{}, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" && pgErr.ConstraintName == "friendships_pair_unique" {
			return Relationship{}, ErrConflict
		}
		if pgErr.Code == "23503" && pgErr.ConstraintName == "friendships_addressee_id_fkey" {
			return Relationship{}, ErrUserNotFound
		}
	}
	relationship.CreatedAt = relationship.CreatedAt.UTC()
	relationship.UpdatedAt = relationship.UpdatedAt.UTC()
	return relationship, err
}
func (r *Repository) Create(ctx context.Context, requesterID, addresseeID string) (Relationship, error) {
	return scanRelationship(r.pool.QueryRow(ctx, `INSERT INTO friendships(requester_id,addressee_id,status) VALUES($1,$2,'pending') RETURNING `+relationshipColumns, requesterID, addresseeID))
}
func (r *Repository) GetParticipant(ctx context.Context, callerID, id string) (Relationship, error) {
	return scanRelationship(r.pool.QueryRow(ctx, `SELECT `+relationshipColumns+` FROM friendships WHERE id=$1 AND (requester_id=$2 OR addressee_id=$2)`, id, callerID))
}

// Accept performs the state transition atomically, only for the pending addressee.
func (r *Repository) Accept(ctx context.Context, callerID, id string) (Relationship, error) {
	return scanRelationship(r.pool.QueryRow(ctx, `UPDATE friendships SET status='accepted',updated_at=now() WHERE id=$1 AND addressee_id=$2 AND status='pending' RETURNING `+relationshipColumns, id, callerID))
}
func (r *Repository) Delete(ctx context.Context, callerID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM friendships WHERE id=$1 AND (requester_id=$2 OR addressee_id=$2)`, id, callerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// list fetches the other participant's public profile in one query.
func (r *Repository) list(ctx context.Context, callerID string) ([]listedRelationship, error) {
	rows, err := r.pool.Query(ctx, `SELECT f.id::text,f.status,f.addressee_id=$1,u.id::text,u.nickname,u.avatar_url,f.created_at,f.updated_at
 FROM friendships f JOIN users u ON u.id=CASE WHEN f.requester_id=$1 THEN f.addressee_id ELSE f.requester_id END
 WHERE f.requester_id=$1 OR f.addressee_id=$1 ORDER BY f.updated_at DESC,f.id DESC`, callerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []listedRelationship{}
	for rows.Next() {
		var item listedRelationship
		if err := rows.Scan(&item.ID, &item.Status, &item.Incoming, &item.User.ID, &item.User.Nickname, &item.User.AvatarURL, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = item.CreatedAt.UTC()
		item.UpdatedAt = item.UpdatedAt.UTC()
		result = append(result, item)
	}
	return result, rows.Err()
}
