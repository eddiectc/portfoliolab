-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN risk_measures TEXT DEFAULT NULL;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN asset_class_allocation TEXT DEFAULT NULL;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN equity_derivatives_by_region TEXT DEFAULT NULL;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN currency_derivatives_allocation TEXT DEFAULT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN currency_derivatives_allocation;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN equity_derivatives_by_region;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN asset_class_allocation;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN risk_measures;
-- +goose StatementEnd
