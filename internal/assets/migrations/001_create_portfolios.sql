-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS portfolios (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    currency   TEXT    NOT NULL DEFAULT 'USD',
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS portfolios;
-- +goose StatementEnd
