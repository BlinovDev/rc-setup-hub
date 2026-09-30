-- +goose Up
CREATE TABLE chassis_brands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX chassis_brands_name_unique ON chassis_brands (lower(name));

-- +goose Down
DROP TABLE chassis_brands;
