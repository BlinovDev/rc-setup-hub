-- +goose Up
-- JSONB manufacturer references are text. Preserve their existing comparison
-- semantics while exposing uniqueness to the planner: eight joins on id::text
-- otherwise multiply row estimates and can trigger JIT for tiny setup lists.
CREATE UNIQUE INDEX chassis_brands_id_text_idx ON chassis_brands ((id::text));

-- +goose Down
DROP INDEX chassis_brands_id_text_idx;
