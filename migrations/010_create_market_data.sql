-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS market_data (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol      TEXT    NOT NULL,
    price       TEXT    NOT NULL,
    currency    TEXT    NOT NULL,
    data_type   TEXT    NOT NULL DEFAULT 'stock',  -- 'stock' or 'fx'
    source      TEXT    NOT NULL DEFAULT 'yahoo',
    date        TEXT,  -- NULL = latest/current, YYYY-MM-DD = historical snapshot
    fetched_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(symbol, source, date)
);

CREATE INDEX idx_market_data_symbol ON market_data(symbol);
CREATE INDEX idx_market_data_symbol_date ON market_data(symbol, date);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_market_data_symbol_date;
DROP INDEX IF EXISTS idx_market_data_symbol;
DROP TABLE IF EXISTS market_data;
-- +goose StatementEnd
