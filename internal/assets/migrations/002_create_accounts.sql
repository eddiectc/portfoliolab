-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS accounts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL,
    portfolio_id INTEGER NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (portfolio_id) REFERENCES portfolios(id) ON DELETE CASCADE
);

CREATE INDEX idx_accounts_portfolio_id ON accounts(portfolio_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_accounts_portfolio_id;
DROP TABLE IF EXISTS accounts;
-- +goose StatementEnd
