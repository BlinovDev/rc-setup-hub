-- +goose Up
CREATE TABLE chassis_models (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id UUID NOT NULL REFERENCES chassis_brands(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX chassis_models_name_unique ON chassis_models (brand_id, lower(name));
CREATE INDEX chassis_models_brand_idx ON chassis_models (brand_id);

-- +goose Down
DROP TABLE chassis_models;
