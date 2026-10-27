-- +goose Up
-- Visible selection spans visibility buckets; retain keyset order across them.
CREATE INDEX setups_created_id_idx ON setups (created_at DESC, id DESC);

-- +goose Down
DROP INDEX setups_created_id_idx;
