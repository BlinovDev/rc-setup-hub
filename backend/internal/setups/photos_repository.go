package setups

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/jackc/pgx/v5"
)

type Photo struct {
	Crop              *photos.Crop `json:"crop"`
	ThumbnailRevision int          `json:"thumbnail_revision"`
	thumbnailKey      string
	ID                string    `json:"id"`
	SetupID           string    `json:"setup_id"`
	Width             int       `json:"width"`
	Height            int       `json:"height"`
	ThumbnailWidth    int       `json:"thumbnail_width"`
	ThumbnailHeight   int       `json:"thumbnail_height"`
	ContentType       string    `json:"content_type"`
	IsCover           bool      `json:"is_cover"`
	CreatedAt         time.Time `json:"created_at"`
	URL               string    `json:"url"`
	ThumbnailURL      string    `json:"thumbnail_url"`
	key               string
}

func (p *Photo) urls() {
	p.ContentType = "image/jpeg"
	p.URL = "/api/v1/setups/" + p.SetupID + "/photos/" + p.ID + "/image"
	p.ThumbnailURL = "/api/v1/setups/" + p.SetupID + "/photos/" + p.ID + "/thumbnail?revision=" + strconv.Itoa(p.ThumbnailRevision)
	p.CreatedAt = p.CreatedAt.UTC()
}

const photoColumns = `id::text,setup_id::text,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover,created_at,crop,thumbnail_revision,COALESCE(thumbnail_key,storage_key)`

// coverPhotoJSON is shared by joined setup/detail and search projections. Keep
// storage keys private; urls() owns transport URLs, content type and UTC times.
const coverPhotoJSON = `CASE WHEN cp.id IS NULL THEN NULL ELSE jsonb_build_object('id',cp.id,'setup_id',cp.setup_id,'width',cp.width,'height',cp.height,'thumbnail_width',cp.thumbnail_width,'thumbnail_height',cp.thumbnail_height,'crop',cp.crop,'thumbnail_revision',cp.thumbnail_revision,'is_cover',cp.is_cover,'created_at',cp.created_at) END`

func decodeCoverPhoto(raw []byte) (*Photo, error) {
	var p *Photo
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p != nil {
		p.urls()
	}
	return p, nil
}

func scanPhoto(row pgx.Row) (Photo, error) {
	var p Photo
	var raw []byte
	err := row.Scan(&p.ID, &p.SetupID, &p.key, &p.Width, &p.Height, &p.ThumbnailWidth, &p.ThumbnailHeight, &p.IsCover, &p.CreatedAt, &raw, &p.ThumbnailRevision, &p.thumbnailKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if raw != nil {
		if err = json.Unmarshal(raw, &p.Crop); err != nil {
			return p, err
		}
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
	p, err := scanPhoto(tx.QueryRow(ctx, `INSERT INTO setup_photos(setup_id,storage_key,width,height,thumbnail_width,thumbnail_height,is_cover,crop,thumbnail_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1) RETURNING `+photoColumns, setupID, img.Key, img.Width, img.Height, img.ThumbnailWidth, img.ThumbnailHeight, count == 0, img.Crop))
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

func photoCaller(id string) any {
	if id == "" {
		return nil
	}
	return id
}

var photoVisibility = visibleSetupPredicate("$2")

func (r *Repository) photoAccess(ctx context.Context, callerID, setupID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT s.id::text FROM setups s WHERE s.id=$1 AND `+photoVisibility, setupID, photoCaller(callerID)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (r *Repository) visiblePhoto(ctx context.Context, callerID, setupID, photoID string) (Photo, error) {
	return scanPhoto(r.pool.QueryRow(ctx, `SELECT p.id::text,p.setup_id::text,p.storage_key,p.width,p.height,p.thumbnail_width,p.thumbnail_height,p.is_cover,p.created_at,p.crop,p.thumbnail_revision,COALESCE(p.thumbnail_key,p.storage_key) FROM setup_photos p JOIN setups s ON s.id=p.setup_id WHERE s.id=$1 AND `+photoVisibility+` AND p.id=$3`, setupID, photoCaller(callerID), photoID))
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
	if !remove && p.IsCover {
		return p, nil
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

const photoCleanupBatchSize = 100

// cleanupPhotoDeletions attempts one bounded page. The cursor advances past failed
// keys too, so a full background pass cannot starve unrelated queued deletions.
func (r *Repository) cleanupPhotoDeletions(ctx context.Context, store *photos.Store, after string) (string, error) {
	rows, err := r.pool.Query(ctx, `SELECT storage_key FROM setup_photo_deletions WHERE storage_key>$1 ORDER BY storage_key LIMIT $2`, after, photoCleanupBatchSize)
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
		if _, err := r.pool.Exec(ctx, `DELETE FROM setup_photo_deletions WHERE storage_key=$1`, key); err != nil {
			return "", err
		}
	}
	next := ""
	if len(keys) == photoCleanupBatchSize {
		next = keys[len(keys)-1]
	}
	if failed {
		// Filesystem errors can contain private paths; only return a safe summary.
		return next, errors.New("photo file cleanup failed; queued deletions retained")
	}
	return next, nil
}

func (r *Repository) reconcilePhotos(ctx context.Context, store *photos.Store) error {
	failed := false
	for after := ""; ; {
		next, err := r.cleanupPhotoDeletions(ctx, store, after)
		failed = failed || err != nil
		if next == "" {
			break
		}
		after = next
	}
	if failed {
		// Preserve fail-closed startup and never sweep after incomplete cleanup.
		return errors.New("photo cleanup failed; queued deletions retained")
	}
	rows, err := r.pool.Query(ctx, `SELECT storage_key,COALESCE(thumbnail_key,storage_key) FROM setup_photos`)
	if err != nil {
		return err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var key, thumbnailKey string
		if err := rows.Scan(&key, &thumbnailKey); err != nil {
			return err
		}
		known[key] = true
		known[thumbnailKey] = true
		for _, thumbnail := range []bool{false, true} {
			mediaKey := key
			if thumbnail {
				mediaKey = thumbnailKey
			}
			file, err := store.Open(mediaKey, thumbnail)
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
