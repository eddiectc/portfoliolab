-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS market_data_overrides (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol      TEXT    NOT NULL,
    date        TEXT    NOT NULL,  -- YYYY-MM-DD
    data_type   TEXT    NOT NULL,  -- 'stock', 'fx', or 'nav'
    price       TEXT    NOT NULL,
    currency    TEXT    NOT NULL,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(symbol, date, data_type)
);

CREATE INDEX idx_market_data_overrides_symbol ON market_data_overrides(symbol);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_market_data_overrides_symbol;
DROP TABLE IF EXISTS market_data_overrides;
-- +goose StatementEnd
