package users

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AvatarURLSQL resolves app-managed media first without exposing its storage key.
// alias must be an internal SQL alias, never user input.
func AvatarURLSQL(alias string) string {
	if alias != "" {
		alias += "."
	}
	return "COALESCE('/api/v1/users/'||" + alias + "id::text||'/avatar/'||" + alias + "custom_avatar_id::text," + alias + "avatar_url)"
}

var userColumns = `id::text,email,nickname,` + AvatarURLSQL("") + `,created_at,instagram_url,tiktok_url,youtube_url,custom_avatar_id IS NOT NULL`

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Upsert(ctx context.Context, identity Identity, nickname string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `INSERT INTO users(google_subject,email,nickname,avatar_url)
        VALUES ($1,$2,$3,NULLIF($4,''))
        ON CONFLICT (google_subject) DO UPDATE SET email=EXCLUDED.email,
            avatar_url=EXCLUDED.avatar_url, updated_at=now()
        RETURNING `+userColumns, identity.Subject, identity.Email, nickname, identity.AvatarURL))
}
func (r *Repository) Get(ctx context.Context, id string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, id))
}
func (r *Repository) UpdateNickname(ctx context.Context, id, nickname string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `UPDATE users SET nickname=$2,updated_at=now() WHERE id=$1
        RETURNING `+userColumns, id, nickname))
}
func scan(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Nickname, &u.AvatarURL, &u.CreatedAt, &u.InstagramURL, &u.TikTokURL, &u.YouTubeURL, &u.HasCustomAvatar)
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
	u.InstagramURL = safeSocial("instagram", u.InstagramURL)
	u.TikTokURL = safeSocial("tiktok", u.TikTokURL)
	u.YouTubeURL = safeSocial("youtube", u.YouTubeURL)
	return u, err
}

// GetPublic selects only approved public statistics, never private counts/identity.
func (r *Repository) GetPublic(ctx context.Context, id string) (DriverProfile, error) {
	var p DriverProfile
	err := r.pool.QueryRow(ctx, `SELECT u.id::text,u.nickname,`+AvatarURLSQL("u")+`,u.created_at,u.instagram_url,u.tiktok_url,u.youtube_url,
        (SELECT count(*) FROM setups s WHERE s.owner_id=u.id AND s.visibility='public' AND s.moderation_status='approved')
        FROM users u WHERE u.id=$1`, id).Scan(&p.ID, &p.Nickname, &p.AvatarURL, &p.MemberSince, &p.InstagramURL, &p.TikTokURL, &p.YouTubeURL, &p.PublicSetupCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return DriverProfile{}, ErrNotFound
	}
	p.MemberSince = p.MemberSince.UTC()
	p.InstagramURL = safeSocial("instagram", p.InstagramURL)
	p.TikTokURL = safeSocial("tiktok", p.TikTokURL)
	p.YouTubeURL = safeSocial("youtube", p.YouTubeURL)
	return p, err
}

func (r *Repository) updateProfile(ctx context.Context, id string, nickname *string, setInstagram bool, instagram *string, setTikTok bool, tiktok *string, setYouTube bool, youtube *string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `UPDATE users SET nickname=COALESCE($2,nickname),
        instagram_url=CASE WHEN $3 THEN $4 ELSE instagram_url END,
        tiktok_url=CASE WHEN $5 THEN $6 ELSE tiktok_url END,
        youtube_url=CASE WHEN $7 THEN $8 ELSE youtube_url END,updated_at=now()
        WHERE id=$1 RETURNING `+userColumns, id, nickname, setInstagram, instagram, setTikTok, tiktok, setYouTube, youtube))
}
