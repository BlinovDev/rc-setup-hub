-- +goose Up
ALTER TABLE setups
    ADD COLUMN moderation_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (moderation_status IN ('pending','approved','rejected')),
    ADD COLUMN reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN reviewed_at TIMESTAMPTZ;
-- Preserve every existing public setup, including those without photos.
UPDATE setups SET moderation_status='approved',reviewed_at=now() WHERE visibility='public';
CREATE INDEX setups_pending_review_idx ON setups(updated_at,id)
    WHERE visibility='public' AND moderation_status='pending';

-- Metadata and moderation commit or roll back together. This also invalidates
-- approval for photos edited while hidden, preventing a later visibility bypass.
-- +goose StatementBegin
CREATE FUNCTION invalidate_setup_photo_approval() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND ROW(NEW.storage_key,NEW.is_cover,NEW.crop,NEW.thumbnail_revision,NEW.thumbnail_key)
        IS NOT DISTINCT FROM ROW(OLD.storage_key,OLD.is_cover,OLD.crop,OLD.thumbnail_revision,OLD.thumbnail_key) THEN
        RETURN NULL;
    END IF;
    UPDATE setups SET moderation_status='pending',reviewed_by=NULL,reviewed_at=NULL,
        updated_at=clock_timestamp() WHERE id=COALESCE(NEW.setup_id,OLD.setup_id);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER setup_photos_invalidate_approval AFTER INSERT OR UPDATE OR DELETE ON setup_photos
    FOR EACH ROW EXECUTE FUNCTION invalidate_setup_photo_approval();

-- +goose Down
DROP TRIGGER setup_photos_invalidate_approval ON setup_photos;
DROP FUNCTION invalidate_setup_photo_approval();
DROP INDEX setups_pending_review_idx;
ALTER TABLE setups DROP COLUMN reviewed_at, DROP COLUMN reviewed_by, DROP COLUMN moderation_status;
