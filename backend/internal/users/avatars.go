package users

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/jackc/pgx/v5"
)

// Avatar constraints are independent of setup galleries. The shared decoder also
// enforces 50 MP, resource/time limits, metadata stripping and actual format checks.
const MaxAvatarBytes = 10 * 1024 * 1024

var ErrInvalidAvatar = errors.New("avatar must be a supported image of at most 10 MiB and 50 megapixels")

// EnableAvatars requires a store rooted in MEDIA_DIR/avatars, not the photo root.
func (s *Service) EnableAvatars(store *photos.Store) {
	s.avatars = store
	s.avatarSlots = make(chan struct{}, 2)
}

func (s *Service) UploadAvatar(ctx context.Context, ownerID string, raw []byte) (User, error) {
	if s.avatars == nil {
		return User{}, errors.New("avatar storage unavailable")
	}
	if _, err := s.repo.Get(ctx, ownerID); err != nil {
		return User{}, err
	}
	if len(raw) == 0 || len(raw) > MaxAvatarBytes {
		return User{}, ErrInvalidAvatar
	}
	img, err := s.avatars.Process(ctx, raw)
	if err != nil {
		return User{}, err
	}
	u, err := s.repo.replaceAvatar(ctx, ownerID, img.Key)
	if errors.Is(err, ErrNotFound) {
		_ = s.avatars.Remove(img.Key)
	}
	// Any other DB error may be an uncertain commit. Retain publication until
	// successful inventory reconciliation, rather than deleting referenced bytes.
	if err == nil {
		s.cleanupAvatars(ctx)
	}
	return u, err
}
func (s *Service) DeleteAvatar(ctx context.Context, ownerID string) (User, error) {
	u, err := s.repo.replaceAvatar(ctx, ownerID, "")
	if err == nil && s.avatars != nil {
		s.cleanupAvatars(ctx)
	}
	return u, err
}

func (r *Repository) replaceAvatar(ctx context.Context, id, key string) (User, error) {
	return scan(r.pool.QueryRow(ctx, `UPDATE users SET custom_avatar_key=NULLIF($2,''),
        custom_avatar_id=CASE WHEN $2='' THEN NULL ELSE gen_random_uuid() END,updated_at=now()
        WHERE id=$1 RETURNING `+userColumns, id, key))
}
func (r *Repository) avatarKey(ctx context.Context, id, avatarID string) (string, error) {
	var key string
	err := r.pool.QueryRow(ctx, `SELECT custom_avatar_key FROM users WHERE id=$1 AND custom_avatar_id=$2`, id, avatarID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}
func (s *Service) OpenAvatar(ctx context.Context, id, avatarID string) (*os.File, error) {
	if s.avatars == nil {
		return nil, errors.New("avatar storage unavailable")
	}
	key, err := s.repo.avatarKey(ctx, id, avatarID)
	if err != nil {
		return nil, err
	}
	f, openErr := s.avatars.Open(key, true)
	// Recheck even an open failure: concurrent cleanup of a replaced avatar is
	// a revoked URL (404), while missing still-referenced media is a server error.
	current, err := s.repo.avatarKey(ctx, id, avatarID)
	if err != nil || current != key {
		if f != nil {
			_ = f.Close()
		}
		if err == nil {
			err = ErrNotFound
		}
		return nil, err
	}
	if openErr != nil {
		return nil, openErr
	}
	return f, nil
}

func (s *Service) cleanupAvatars(ctx context.Context) {
	if _, err := s.repo.cleanupAvatarPage(ctx, s.avatars, ""); err != nil {
		slog.Warn("avatar cleanup deferred; queued deletions retained")
	}
}
func (r *Repository) cleanupAvatarPage(ctx context.Context, store *photos.Store, after string) (string, error) {
	rows, err := r.pool.Query(ctx, `SELECT storage_key FROM user_avatar_deletions WHERE storage_key>$1 ORDER BY storage_key LIMIT 100`, after)
	if err != nil {
		return "", err
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return "", err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	failed := false
	for _, key := range keys {
		if err := store.Remove(key); err != nil {
			failed = true
			continue
		}
		if _, err := r.pool.Exec(ctx, `DELETE FROM user_avatar_deletions WHERE storage_key=$1`, key); err != nil {
			return "", err
		}
	}
	next := ""
	if len(keys) == 100 {
		next = keys[len(keys)-1]
	}
	if failed {
		return next, errors.New("avatar cleanup failed; queued deletions retained")
	}
	return next, nil
}

// ReconcileAvatars retries all queued deletions, validates referenced media, then
// sweeps only old unreferenced publications. Never sweep after a DB/read failure.
func (s *Service) ReconcileAvatars(ctx context.Context) error {
	if s.avatars == nil {
		return errors.New("avatar storage unavailable")
	}
	failed := false
	for after := ""; ; {
		next, err := s.repo.cleanupAvatarPage(ctx, s.avatars, after)
		failed = failed || err != nil
		if next == "" {
			break
		}
		after = next
	}
	if failed {
		return errors.New("avatar cleanup failed; queued deletions retained")
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT custom_avatar_key FROM users WHERE custom_avatar_key IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return err
		}
		known[key] = true
		f, err := s.avatars.Open(key, true)
		if err != nil {
			return errors.New("referenced avatar missing or unreadable; restore media before startup")
		}
		_ = f.Close()
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return s.avatars.Sweep(known)
}
