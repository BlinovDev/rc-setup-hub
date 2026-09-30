-- +goose Up
CREATE TABLE setups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chassis_model_id UUID REFERENCES chassis_models(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    visibility TEXT NOT NULL CHECK (visibility IN ('public', 'friends', 'private')),
    data JSONB NOT NULL,
    notes TEXT,
    schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX setups_owner_idx ON setups (owner_id);
CREATE INDEX setups_chassis_model_idx ON setups (chassis_model_id);
CREATE INDEX setups_visibility_idx ON setups (visibility);
CREATE INDEX setups_visibility_created_idx ON setups (visibility, created_at DESC);

-- +goose Down
DROP TABLE setups;
