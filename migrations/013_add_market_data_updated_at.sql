-- +goose Up
-- +goose StatementBegin
ALTER TABLE market_data ADD COLUMN updated_at TEXT NOT NULL DEFAULT (datetime('now'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite does not support DROP COLUMN before version 3.35.0.
-- Leave the column in place for compatibility.
-- +goose StatementEnd
