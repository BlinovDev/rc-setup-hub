-- +goose Up
ALTER TABLE users
    ADD COLUMN instagram_url TEXT CHECK (octet_length(instagram_url) <= 2048),
    ADD COLUMN tiktok_url TEXT CHECK (octet_length(tiktok_url) <= 2048),
    ADD COLUMN youtube_url TEXT CHECK (octet_length(youtube_url) <= 2048),
    ADD COLUMN custom_avatar_id UUID,
    ADD COLUMN custom_avatar_key TEXT UNIQUE CHECK (custom_avatar_key ~ '^[0-9a-f]{32}$'),
    ADD CONSTRAINT users_avatar_pair CHECK ((custom_avatar_id IS NULL) = (custom_avatar_key IS NULL));
CREATE TABLE user_avatar_deletions (
    storage_key TEXT PRIMARY KEY CHECK (storage_key ~ '^[0-9a-f]{32}$')
);
-- The queue survives user deletion and process crashes. Avatar files live in
-- MEDIA_DIR/avatars, never the setup-photo namespace.
-- +goose StatementBegin
CREATE FUNCTION queue_user_avatar_deletion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.custom_avatar_key IS NOT NULL THEN
        IF TG_OP = 'DELETE' OR OLD.custom_avatar_key IS DISTINCT FROM NEW.custom_avatar_key THEN
            INSERT INTO user_avatar_deletions(storage_key) VALUES (OLD.custom_avatar_key) ON CONFLICT DO NOTHING;
        END IF;
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER user_avatar_cleanup AFTER UPDATE OF custom_avatar_key OR DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION queue_user_avatar_deletion();

-- +goose Down
DROP TRIGGER user_avatar_cleanup ON users;
DROP FUNCTION queue_user_avatar_deletion();
DROP TABLE user_avatar_deletions;
ALTER TABLE users DROP CONSTRAINT users_avatar_pair,
    DROP COLUMN custom_avatar_key, DROP COLUMN custom_avatar_id,
    DROP COLUMN instagram_url, DROP COLUMN tiktok_url, DROP COLUMN youtube_url;
