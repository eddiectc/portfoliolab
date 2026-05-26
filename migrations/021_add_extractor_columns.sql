-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_mappings ADD COLUMN data_source_url TEXT DEFAULT NULL;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_details ADD COLUMN extractor_as_of_date TEXT DEFAULT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_details DROP COLUMN extractor_as_of_date;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE symbol_mappings DROP COLUMN data_source_url;
-- +goose StatementEnd
