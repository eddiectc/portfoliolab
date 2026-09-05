-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN sector TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN sector;
-- +goose StatementEnd
