package setups

import (
	"context"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
)

// cropPhoto serializes generation/publication with cover, crop and deletion writes.
// keep reports uncertain commit outcomes; retain published files for reconciliation.
func (r *Repository) cropPhoto(ctx context.Context, ownerID, setupID, photoID string, body CropInput, generate func(Photo, photos.Crop) (photos.Image, error)) (Photo, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Photo{}, false, err
	}
	defer tx.Rollback(context.Background())
	if err = lockPhotoSetup(ctx, tx, ownerID, setupID); err != nil {
		return Photo{}, false, err
	}
	p, err := scanPhoto(tx.QueryRow(ctx, `SELECT `+photoColumns+` FROM setup_photos WHERE setup_id=$1 AND id=$2`, setupID, photoID))
	if err != nil {
		return Photo{}, false, err
	}
	if body.ExpectedRevision == nil || *body.ExpectedRevision < 0 || *body.ExpectedRevision > 2147483647 {
		return Photo{}, false, invalid("expected_revision must be an integer from 0 to 2147483647")
	}
	crop := photos.CenterCrop(p.Width, p.Height)
	if body.Crop != nil {
		crop = *body.Crop
	}
	if err = crop.Validate(p.Width, p.Height); err != nil {
		return Photo{}, false, invalid(err.Error())
	}
	same := p.Crop != nil && *p.Crop == crop
	expected := *body.ExpectedRevision
	if expected != p.ThumbnailRevision {
		// An immediately repeated identical apply is safe after a lost response.
		if expected < p.ThumbnailRevision && p.ThumbnailRevision-expected == 1 && same {
			return p, false, nil
		}
		return Photo{}, false, ErrConflict
	}
	if same {
		return p, false, nil
	}
	if p.ThumbnailRevision == 2147483647 {
		return Photo{}, false, ErrConflict
	}
	img, err := generate(p, crop)
	if err != nil {
		return Photo{}, false, err
	}
	updated, err := scanPhoto(tx.QueryRow(ctx, `UPDATE setup_photos SET crop=$3,thumbnail_revision=thumbnail_revision+1,thumbnail_key=$4,thumbnail_width=$5,thumbnail_height=$6 WHERE setup_id=$1 AND id=$2 RETURNING `+photoColumns, setupID, photoID, crop, img.Key, img.ThumbnailWidth, img.ThumbnailHeight))
	if err != nil {
		return Photo{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Photo{}, true, err
	}
	return updated, true, nil
}
