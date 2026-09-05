-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN bond_characteristics TEXT DEFAULT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN bond_characteristics;
-- +goose StatementEnd
