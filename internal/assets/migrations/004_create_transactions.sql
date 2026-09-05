-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS transactions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL,
    date                TEXT    NOT NULL,
    type                TEXT    NOT NULL,
    symbol              TEXT    NOT NULL,
    quantity            TEXT    NOT NULL,
    price               TEXT    NOT NULL,
    currency            TEXT    NOT NULL,
    net_cash            TEXT,
    external_system     TEXT,
    external_reference  TEXT,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_transactions_account_id ON transactions(account_id);
CREATE INDEX idx_transactions_date ON transactions(date DESC);
CREATE INDEX idx_transactions_symbol ON transactions(symbol);
CREATE INDEX idx_transactions_type ON transactions(type);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_transactions_type;
DROP INDEX IF EXISTS idx_transactions_symbol;
DROP INDEX IF EXISTS idx_transactions_date;
DROP INDEX IF EXISTS idx_transactions_account_id;
DROP TABLE IF EXISTS transactions;
-- +goose StatementEnd
