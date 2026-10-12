package setups

import (
	"context"
	"errors"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/jackc/pgx/v5"
)

type Photo struct {
	ID              string    `json:"id"`
	SetupID         string    `json:"setup_id"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	ThumbnailWidth  int       `json:"thumbnail_width"`
	ThumbnailHeight int       `json:"thumbnail_height"`
	ContentType     string    `json:"content_type"`
	IsCover         bool      `json:"is_cover"`
	CreatedAt       time.Time `json:"created_at"`
	URL             string    `json:"url"`
	ThumbnailURL    string    `json:"thumbnail_url"`
	key             string
}

func (p *Photo) urls() {
	p.ContentType = "image/jpeg"
	p.URL = "/api/v1/setups/" + p.SetupID + "/photos/" + p.ID + "/image"
	p.ThumbnailURL = "/api/v1/setups/" + p.SetupID + "/photos/" + p.ID + "/thumbnail"
	p.CreatedAt = p.CreatedAt.UTC()
}

const photoColumns = `id::text,setup_id::text,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover,created_at`

func scanPhoto(row pgx.Row) (Photo, error) {
	var p Photo
	err := row.Scan(&p.ID, &p.SetupID, &p.key, &p.Width, &p.Height, &p.ThumbnailWidth, &p.ThumbnailHeight, &p.IsCover, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	p.urls()
	return p, err
}
func lockPhotoSetup(ctx context.Context, tx pgx.Tx, ownerID, setupID string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM setups WHERE id=$1 AND owner_id=$2 FOR UPDATE`, setupID, ownerID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func receipt(ctx context.Context, tx pgx.Tx, setupID, requestID, digest string) (Photo, bool, error) {
	var existingDigest string
	var photoID *string
	err := tx.QueryRow(ctx, `SELECT digest,photo_id::text FROM setup_photo_uploads WHERE setup_id=$1 AND request_id=$2`, setupID, requestID).Scan(&existingDigest, &photoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, false, nil
	}
	if err != nil {
		return Photo{}, false, err
	}
	if existingDigest != digest || photoID == nil {
		return Photo{}, true, ErrConflict
	}
	p, err := scanPhoto(tx.QueryRow(ctx, `SELECT `+photoColumns+` FROM setup_photos WHERE id=$1`, *photoID))
	return p, true, err
}
func (r *Repository) photoReceipt(ctx context.Context, ownerID, setupID, requestID, digest string) (Photo, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Photo{}, false, err
	}
	defer tx.Rollback(context.Background())
	if err := lockPhotoSetup(ctx, tx, ownerID, setupID); err != nil {
		return Photo{}, false, err
	}
	p, exists, err := receipt(ctx, tx, setupID, requestID, digest)
	return p, exists, err
}
func (r *Repository) addPhoto(ctx context.Context, ownerID, setupID, requestID, digest string, img photos.Image) (Photo, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Photo{}, false, err
	}
	defer tx.Rollback(context.Background())
	if err := lockPhotoSetup(ctx, tx, ownerID, setupID); err != nil {
		return Photo{}, false, err
	}
	if p, exists, err := receipt(ctx, tx, setupID, requestID, digest); exists || err != nil {
		return p, false, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM setup_photos WHERE setup_id=$1`, setupID).Scan(&count); err != nil {
		return Photo{}, false, err
	}
	if count >= 10 {
		return Photo{}, false, invalid("a setup may have at most 10 photos")
	}
	p, err := scanPhoto(tx.QueryRow(ctx, `INSERT INTO setup_photos(setup_id,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+photoColumns, setupID, img.Key, img.Width, img.Height, img.ThumbnailWidth, img.ThumbnailHeight, count == 0))
	if err != nil {
		return Photo{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO setup_photo_uploads(setup_id,request_id,digest,photo_id) VALUES($1,$2,$3,$4)`, setupID, requestID, digest, p.ID); err != nil {
		return Photo{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		// Commit outcome may be uncertain: never delete possibly referenced files.
		return Photo{}, true, err
	}
	return p, true, nil
}
func (r *Repository) listPhotos(ctx context.Context, callerID, setupID string) ([]Photo, error) {
	if err := r.photoAccess(ctx, callerID, setupID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+photoColumns+` FROM setup_photos WHERE setup_id=$1 ORDER BY created_at,id`, setupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	// Revalidate after loading metadata; more restrictive changes fail closed.
	if err := r.photoAccess(ctx, callerID, setupID); err != nil {
		return nil, err
	}
	return result, rows.Err()
}

const photoVisibility = `(s.owner_id=$2 OR s.visibility='public' OR (s.visibility='friends' AND EXISTS(SELECT 1 FROM friendships f WHERE f.status='accepted' AND ((f.requester_id=s.owner_id AND f.addressee_id=$2) OR (f.addressee_id=s.owner_id AND f.requester_id=$2)))))`

func (r *Repository) photoAccess(ctx context.Context, callerID, setupID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT s.id::text FROM setups s WHERE s.id=$1 AND `+photoVisibility, setupID, callerID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (r *Repository) visiblePhoto(ctx context.Context, callerID, setupID, photoID string) (Photo, error) {
	return scanPhoto(r.pool.QueryRow(ctx, `SELECT p.id::text,p.setup_id::text,p.storage_key,p.width,p.height,p.thumbnail_width,p.thumbnail_height,p.is_cover,p.created_at FROM setup_photos p JOIN setups s ON s.id=p.setup_id WHERE s.id=$1 AND `+photoVisibility+` AND p.id=$3`, setupID, callerID, photoID))
}
func (r *Repository) mutatePhoto(ctx context.Context, ownerID, setupID, photoID string, remove bool) (Photo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Photo{}, err
	}
	defer tx.Rollback(context.Background())
	if err := lockPhotoSetup(ctx, tx, ownerID, setupID); err != nil {
		return Photo{}, err
	}
	p, err := scanPhoto(tx.QueryRow(ctx, `SELECT `+photoColumns+` FROM setup_photos WHERE setup_id=$1 AND id=$2`, setupID, photoID))
	if err != nil {
		return Photo{}, err
	}
	if remove {
		if _, err = tx.Exec(ctx, `DELETE FROM setup_photos WHERE id=$1`, photoID); err != nil {
			return Photo{}, err
		}
		if p.IsCover {
			_, err = tx.Exec(ctx, `UPDATE setup_photos SET is_cover=true WHERE id=(SELECT id FROM setup_photos WHERE setup_id=$1 ORDER BY created_at,id LIMIT 1)`, setupID)
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE setup_photos SET is_cover=false WHERE setup_id=$1 AND is_cover`, setupID); err != nil {
			return Photo{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE setup_photos SET is_cover=true WHERE id=$1`, photoID)
		p.IsCover = true
	}
	if err != nil {
		return Photo{}, err
	}
	return p, tx.Commit(ctx)
}
func (r *Repository) reconcilePhotos(ctx context.Context, store *photos.Store) error {
	rows, err := r.pool.Query(ctx, `SELECT storage_key FROM setup_photo_deletions`)
	if err != nil {
		return err
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := store.Remove(key); err != nil {
			return err
		}
		if _, err := r.pool.Exec(ctx, `DELETE FROM setup_photo_deletions WHERE storage_key=$1`, key); err != nil {
			return err
		}
	}
	rows, err = r.pool.Query(ctx, `SELECT storage_key FROM setup_photos`)
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
		for _, thumbnail := range []bool{false, true} {
			file, err := store.Open(key, thumbnail)
			if err != nil {
				return errors.New("referenced photo file missing or unreadable; restore media before startup")
			}
			_ = file.Close()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return store.Sweep(known)
}
