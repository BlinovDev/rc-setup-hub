-- +goose Up
ALTER TABLE setup_photos
 ADD COLUMN crop JSONB,
 ADD COLUMN thumbnail_revision INTEGER NOT NULL DEFAULT 0 CHECK (thumbnail_revision >= 0),
 ADD COLUMN thumbnail_key TEXT CHECK (thumbnail_key ~ '^[0-9a-f]{32}$'),
 ADD CONSTRAINT photo_crop_bounds CHECK (crop IS NULL OR (
  jsonb_typeof(crop)='object' AND crop ?& ARRAY['x','y','size']
  AND jsonb_typeof(crop->'x')='number' AND jsonb_typeof(crop->'y')='number' AND jsonb_typeof(crop->'size')='number'
  AND (crop->>'x')::numeric = trunc((crop->>'x')::numeric)
  AND (crop->>'y')::numeric = trunc((crop->>'y')::numeric)
  AND (crop->>'size')::numeric = trunc((crop->>'size')::numeric)
  AND (crop->>'x')::numeric >= 0 AND (crop->>'y')::numeric >= 0 AND (crop->>'size')::numeric >= 1
  AND (crop->>'x')::numeric + (crop->>'size')::numeric <= width
  AND (crop->>'y')::numeric + (crop->>'size')::numeric <= height
 ));
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION queue_photo_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO setup_photo_deletions(storage_key) VALUES(OLD.storage_key) ON CONFLICT DO NOTHING;
 IF OLD.thumbnail_key IS NOT NULL THEN
  INSERT INTO setup_photo_deletions(storage_key) VALUES(OLD.thumbnail_key) ON CONFLICT DO NOTHING;
 END IF;
 RETURN OLD;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION queue_old_thumbnail() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.thumbnail_key IS NOT NULL AND OLD.thumbnail_key IS DISTINCT FROM NEW.thumbnail_key THEN
  INSERT INTO setup_photo_deletions(storage_key) VALUES(OLD.thumbnail_key) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER queue_old_thumbnail AFTER UPDATE OF thumbnail_key ON setup_photos FOR EACH ROW EXECUTE FUNCTION queue_old_thumbnail();

-- +goose Down
-- Cropped thumb files need archived media before rollback; legacy thumbnail.jpg remains intact.
DROP TRIGGER queue_old_thumbnail ON setup_photos;
DROP FUNCTION queue_old_thumbnail();
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION queue_photo_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO setup_photo_deletions(storage_key) VALUES(OLD.storage_key) ON CONFLICT DO NOTHING;
 RETURN OLD;
END $$;
-- +goose StatementEnd
ALTER TABLE setup_photos DROP CONSTRAINT photo_crop_bounds, DROP COLUMN crop, DROP COLUMN thumbnail_revision, DROP COLUMN thumbnail_key;
