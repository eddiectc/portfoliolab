-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS symbol_mappings (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    internal_symbol     TEXT    NOT NULL UNIQUE,
    market_data_symbol  TEXT    NOT NULL,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS broker_symbol_mappings (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol_mapping_id   INTEGER NOT NULL,
    broker_name         TEXT    NOT NULL,
    broker_symbol       TEXT    NOT NULL,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (symbol_mapping_id) REFERENCES symbol_mappings(id) ON DELETE CASCADE,
    UNIQUE(broker_name, broker_symbol)
);

CREATE INDEX IF NOT EXISTS idx_broker_symbol_mappings_mapping_id
    ON broker_symbol_mappings(symbol_mapping_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_broker_symbol_mappings_mapping_id;
DROP TABLE IF EXISTS broker_symbol_mappings;
DROP TABLE IF EXISTS symbol_mappings;
-- +goose StatementEnd
