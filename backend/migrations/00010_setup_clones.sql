-- +goose Up
ALTER TABLE setups ADD COLUMN source_setup_id UUID REFERENCES setups(id) ON DELETE SET NULL;
CREATE INDEX setups_source_setup_idx ON setups(source_setup_id) WHERE source_setup_id IS NOT NULL;
-- Receipts survive source deletion and clone deletion (a tombstone prevents resurrection).
CREATE TABLE setup_clone_requests (
 owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 request_id UUID NOT NULL,
 source_id UUID NOT NULL,
 clone_id UUID REFERENCES setups(id) ON DELETE SET NULL,
 PRIMARY KEY(owner_id,request_id)
);
-- +goose Down
DROP TABLE setup_clone_requests;
ALTER TABLE setups DROP COLUMN source_setup_id;
