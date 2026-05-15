-- +goose Up
-- +goose StatementBegin
-- Allow symbol mappings to be marked as user-defined benchmarks for portfolio performance comparison.
ALTER TABLE symbol_mappings ADD COLUMN is_benchmark BOOLEAN NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_mappings DROP COLUMN is_benchmark;
-- +goose StatementEnd
