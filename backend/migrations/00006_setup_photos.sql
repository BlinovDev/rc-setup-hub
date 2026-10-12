-- +goose Up
CREATE TABLE setup_photos (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 setup_id UUID NOT NULL REFERENCES setups(id) ON DELETE CASCADE,
 storage_key TEXT NOT NULL UNIQUE CHECK (storage_key ~ '^[0-9a-f]{32}$'),
 width INTEGER NOT NULL CHECK(width BETWEEN 1 AND 2400),
 height INTEGER NOT NULL CHECK(height BETWEEN 1 AND 2400),
 thumbnail_width INTEGER NOT NULL CHECK(thumbnail_width BETWEEN 1 AND 640),
 thumbnail_height INTEGER NOT NULL CHECK(thumbnail_height BETWEEN 1 AND 640),
 is_cover BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX setup_photos_order ON setup_photos(setup_id,created_at,id);
CREATE UNIQUE INDEX setup_photos_cover ON setup_photos(setup_id) WHERE is_cover;
CREATE TABLE setup_photo_uploads (
 setup_id UUID NOT NULL REFERENCES setups(id) ON DELETE CASCADE,
 request_id UUID NOT NULL,
 digest TEXT NOT NULL,
 photo_id UUID REFERENCES setup_photos(id) ON DELETE SET NULL,
 PRIMARY KEY(setup_id,request_id)
);
CREATE TABLE setup_photo_deletions (storage_key TEXT PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
-- +goose StatementBegin
CREATE FUNCTION queue_photo_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO setup_photo_deletions(storage_key) VALUES(OLD.storage_key) ON CONFLICT DO NOTHING;
 RETURN OLD;
END $$;
-- +goose StatementEnd
CREATE TRIGGER queue_photo_delete AFTER DELETE ON setup_photos FOR EACH ROW EXECUTE FUNCTION queue_photo_delete();
-- +goose StatementBegin
CREATE FUNCTION check_photo_invariants() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target UUID; total INTEGER; covers INTEGER;
BEGIN
 target := COALESCE(NEW.setup_id,OLD.setup_id);
 SELECT count(*),count(*) FILTER(WHERE is_cover) INTO total,covers FROM setup_photos WHERE setup_id=target;
 IF total>10 OR (total>0 AND covers<>1) THEN RAISE EXCEPTION 'invalid setup photo count/cover' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER photo_invariants AFTER INSERT OR UPDATE OR DELETE ON setup_photos DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_photo_invariants();

-- +goose Down
DROP TABLE setup_photo_uploads;
DROP TABLE setup_photos;
DROP FUNCTION check_photo_invariants();
DROP FUNCTION queue_photo_delete();
DROP TABLE setup_photo_deletions;
