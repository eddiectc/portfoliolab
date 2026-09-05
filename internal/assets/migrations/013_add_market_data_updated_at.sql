-- +goose Up
-- +goose StatementBegin
-- SQLite ALTER TABLE ADD COLUMN only accepts literal defaults, not expressions.
-- Use an empty string as default; application layer sets it on upsert.
ALTER TABLE market_data ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite does not support DROP COLUMN before version 3.35.0.
-- Leave the column in place for compatibility.
-- +goose StatementEnd
