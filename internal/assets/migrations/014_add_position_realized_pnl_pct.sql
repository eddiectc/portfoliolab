-- +goose Up
-- +goose StatementBegin
-- Store realized_pnl_pct as a pre-computed column to avoid recomputing at read time.
ALTER TABLE positions ADD COLUMN realized_pnl_pct TEXT DEFAULT NULL;
-- Index for currency-based filtering in multi-currency portfolio views.
CREATE INDEX IF NOT EXISTS idx_positions_currency ON positions(currency);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_positions_currency;
ALTER TABLE positions DROP COLUMN realized_pnl_pct;
-- +goose StatementEnd
