package setups

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReviewItem contains only admin queue metadata; technical data loads on preview.
type ReviewItem struct {
	ID            string
	OwnerNickname string
	Title         string
	UpdatedAt     time.Time
}

// PendingReviews is used exclusively inside the authenticated admin route group.
func (s *Service) PendingReviews(ctx context.Context) ([]ReviewItem, error) {
	rows, err := s.repo.pool.Query(ctx, `SELECT s.id::text,u.nickname,s.title,s.updated_at FROM setups s JOIN users u ON u.id=s.owner_id WHERE s.visibility='public' AND s.moderation_status='pending' ORDER BY s.updated_at,s.id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ReviewItem{}
	for rows.Next() {
		var item ReviewItem
		if err := rows.Scan(&item.ID, &item.OwnerNickname, &item.Title, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ReviewPreview bypasses public visibility only for explicit admin preview routes.
func (s *Service) ReviewPreview(ctx context.Context, id string) (Setup, []Photo, error) {
	if err := validID(id); err != nil {
		return Setup{}, nil, err
	}
	tx, err := s.repo.pool.Begin(ctx)
	if err != nil {
		return Setup{}, nil, err
	}
	defer tx.Rollback(context.Background())
	// Parent share lock keeps the preview and gallery in one coherent revision.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM setups WHERE id=$1 AND visibility='public' FOR SHARE`, id).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return Setup{}, nil, err
	}
	setup, err := scan(tx.QueryRow(ctx, `SELECT `+displayColumns+` FROM setups s`+displayJoins+` WHERE s.id=$1`, id))
	if err != nil {
		return Setup{}, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT `+photoColumns+` FROM setup_photos WHERE setup_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return Setup{}, nil, err
	}
	gallery := []Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			rows.Close()
			return Setup{}, nil, err
		}
		gallery = append(gallery, p)
	}
	err = rows.Err()
	rows.Close()
	return setup, gallery, err
}

// ReviewPhoto has no public registration; admin middleware must authorize first.
func (s *Service) ReviewPhoto(ctx context.Context, setupID, photoID string, thumbnail bool) (*os.File, error) {
	if err := validID(setupID); err != nil {
		return nil, err
	}
	if err := validID(photoID); err != nil {
		return nil, err
	}
	p, err := scanPhoto(s.repo.pool.QueryRow(ctx, `SELECT p.id::text,p.setup_id::text,p.storage_key,p.width,p.height,p.thumbnail_width,p.thumbnail_height,p.is_cover,p.created_at,p.crop,p.thumbnail_revision,COALESCE(p.thumbnail_key,p.storage_key) FROM setup_photos p JOIN setups s ON s.id=p.setup_id WHERE s.id=$1 AND s.visibility='public' AND p.id=$2`, setupID, photoID))
	if err != nil {
		return nil, err
	}
	key := p.key
	if thumbnail {
		key = p.thumbnailKey
	}
	if s.photos == nil {
		return nil, ErrNotFound
	}
	f, err := s.photos.Open(key, thumbnail)
	if err != nil {
		return nil, ErrNotFound
	}
	return f, nil
}

// Review atomically consumes a pending revision. Photo writes lock the same parent;
// stale forms, concurrent decisions and edits can never approve unseen content.
func (s *Service) Review(ctx context.Context, id, reviewerID string, expected time.Time, decision ModerationStatus) error {
	if err := validID(id); err != nil {
		return err
	}
	if decision != Approved && decision != Rejected {
		return invalid("invalid moderation decision")
	}
	tag, err := s.repo.pool.Exec(ctx, `UPDATE setups SET moderation_status=$4,reviewed_by=$2,reviewed_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1 AND visibility='public' AND moderation_status='pending' AND updated_at=$3`, id, reviewerID, expected, decision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
