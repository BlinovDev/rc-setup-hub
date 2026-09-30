package users

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Upsert(ctx context.Context, identity Identity, nickname string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `INSERT INTO users(google_subject,email,nickname,avatar_url)
        VALUES ($1,$2,$3,NULLIF($4,''))
        ON CONFLICT (google_subject) DO UPDATE SET email=EXCLUDED.email,
            avatar_url=EXCLUDED.avatar_url, updated_at=now()
        RETURNING id::text,email,nickname,avatar_url,created_at`, identity.Subject, identity.Email, nickname, identity.AvatarURL))
}
func (r *Repository) Get(ctx context.Context, id string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT id::text,email,nickname,avatar_url,created_at FROM users WHERE id=$1`, id))
}
func (r *Repository) UpdateNickname(ctx context.Context, id, nickname string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `UPDATE users SET nickname=$2,updated_at=now() WHERE id=$1
        RETURNING id::text,email,nickname,avatar_url,created_at`, id, nickname))
}
func scan(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Nickname, &u.AvatarURL, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_nickname_unique":
			return User{}, ErrNicknameConflict
		case "users_email_unique":
			return User{}, ErrEmailConflict
		}
	}
	u.CreatedAt = u.CreatedAt.UTC()
	return u, err
}
