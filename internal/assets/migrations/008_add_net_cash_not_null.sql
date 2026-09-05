-- +goose Up
-- +goose StatementBegin
-- Enforce NOT NULL on net_cash at the database level.
-- Migration 005 already backfilled all null values with quantity * price,
-- so no data loss occurs.
--
-- SQLite does not support adding NOT NULL constraints to existing columns,
-- so we rebuild the table.

-- Create the new table with the NOT NULL constraint.
CREATE TABLE transactions_new (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL,
    date                TEXT    NOT NULL,
    type                TEXT    NOT NULL,
    symbol              TEXT    NOT NULL,
    quantity            TEXT    NOT NULL,
    price               TEXT    NOT NULL,
    currency            TEXT    NOT NULL,
    net_cash            TEXT    NOT NULL,
    external_system     TEXT,
    external_reference  TEXT,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

-- Copy data from the old table.
INSERT INTO transactions_new (
    id, account_id, date, type, symbol, quantity, price, currency,
    net_cash, external_system, external_reference, created_at, updated_at
)
SELECT
    id, account_id, date, type, symbol, quantity, price, currency,
    net_cash, external_system, external_reference, created_at, updated_at
FROM transactions;

-- Drop the old table and rename the new one.
DROP TABLE transactions;
ALTER TABLE transactions_new RENAME TO transactions;

-- Recreate indexes.
CREATE INDEX idx_transactions_account_id ON transactions(account_id);
CREATE INDEX idx_transactions_date ON transactions(date DESC);
CREATE INDEX idx_transactions_symbol ON transactions(symbol);
CREATE INDEX idx_transactions_type ON transactions(type);

-- Recreate the unique index for external references (from migration 006).
CREATE UNIQUE INDEX idx_transactions_external_ref
    ON transactions(external_system, external_reference)
    WHERE external_system IS NOT NULL AND external_reference IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Revert to the original schema with nullable net_cash.
CREATE TABLE transactions_old (
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

INSERT INTO transactions_old (
    id, account_id, date, type, symbol, quantity, price, currency,
    net_cash, external_system, external_reference, created_at, updated_at
)
SELECT
    id, account_id, date, type, symbol, quantity, price, currency,
    net_cash, external_system, external_reference, created_at, updated_at
FROM transactions;

DROP TABLE transactions;
ALTER TABLE transactions_old RENAME TO transactions;

CREATE INDEX idx_transactions_account_id ON transactions(account_id);
CREATE INDEX idx_transactions_date ON transactions(date DESC);
CREATE INDEX idx_transactions_symbol ON transactions(symbol);
CREATE INDEX idx_transactions_type ON transactions(type);

CREATE UNIQUE INDEX idx_transactions_external_ref
    ON transactions(external_system, external_reference)
    WHERE external_system IS NOT NULL AND external_reference IS NOT NULL;
-- +goose StatementEnd
