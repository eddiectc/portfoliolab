-- +goose Up
-- +goose StatementBegin
ALTER TABLE positions ADD COLUMN fx_rate_used TEXT DEFAULT '';
ALTER TABLE positions ADD COLUMN fx_rate_fallback BOOLEAN DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE positions DROP COLUMN fx_rate_fallback;
ALTER TABLE positions DROP COLUMN fx_rate_used;
-- +goose StatementEnd
