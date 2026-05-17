-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN geographic_allocations TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN geographic_allocations;
-- +goose StatementEnd
