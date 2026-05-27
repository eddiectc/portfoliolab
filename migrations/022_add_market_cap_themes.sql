-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN market_cap_breakdown TEXT DEFAULT NULL;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN themes TEXT DEFAULT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN themes;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN market_cap_breakdown;
-- +goose StatementEnd
