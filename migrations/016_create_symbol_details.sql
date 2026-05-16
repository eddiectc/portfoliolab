-- +goose Up
-- +goose StatementBegin
CREATE TABLE symbol_details (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    internal_symbol     TEXT    NOT NULL UNIQUE,
    short_name          TEXT,
    long_name           TEXT,
    exchange            TEXT,
    currency            TEXT,
    quote_type          TEXT,
    top_holdings        TEXT,
    sector_weightings   TEXT,
    aggregate_positions TEXT,
    fund_profile        TEXT,
    equity_valuation    TEXT,
    fetched_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_symbol_details_internal_symbol
    ON symbol_details(internal_symbol);

CREATE INDEX idx_symbol_details_fetched_at
    ON symbol_details(fetched_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_symbol_details_fetched_at;
DROP INDEX IF EXISTS idx_symbol_details_internal_symbol;
DROP TABLE IF EXISTS symbol_details;
-- +goose StatementEnd
