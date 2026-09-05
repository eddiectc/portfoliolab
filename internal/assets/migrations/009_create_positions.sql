-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS positions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id        INTEGER NOT NULL,
    symbol            TEXT    NOT NULL,
    currency          TEXT    NOT NULL,
    quantity          TEXT    NOT NULL DEFAULT '0',
    cost_basis        TEXT    NOT NULL DEFAULT '0',
    avg_open_price    TEXT,
    avg_close_price   TEXT,
    realized_pnl      TEXT    NOT NULL DEFAULT '0',
    realized_pnl_base TEXT,
    open_date         TEXT    NOT NULL,
    close_date        TEXT,
    is_closed         INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_positions_account_symbol ON positions(account_id, symbol);
CREATE INDEX idx_positions_is_closed ON positions(is_closed);

CREATE TABLE IF NOT EXISTS lots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    lot_id          TEXT    NOT NULL UNIQUE,
    account_id      INTEGER NOT NULL,
    symbol          TEXT    NOT NULL,
    lot_type        TEXT    NOT NULL,  -- 'buy' or 'sell'
    quantity        TEXT    NOT NULL,
    cost_basis      TEXT    NOT NULL DEFAULT '0',
    sell_price      TEXT,
    realized_pnl    TEXT    NOT NULL DEFAULT '0',
    open_date       TEXT    NOT NULL,
    close_date      TEXT,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_lots_account_symbol ON lots(account_id, symbol);
CREATE INDEX idx_lots_lot_id ON lots(lot_id);

CREATE TABLE IF NOT EXISTS lot_consumptions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    sell_lot_id         TEXT    NOT NULL,
    buy_lot_id          TEXT    NOT NULL,
    quantity_consumed   TEXT    NOT NULL,
    cost_basis_consumed TEXT    NOT NULL DEFAULT '0',
    realized_pnl        TEXT    NOT NULL DEFAULT '0',
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (sell_lot_id) REFERENCES lots(lot_id) ON DELETE CASCADE,
    FOREIGN KEY (buy_lot_id) REFERENCES lots(lot_id) ON DELETE CASCADE
);

CREATE INDEX idx_lot_consumptions_sell_lot ON lot_consumptions(sell_lot_id);
CREATE INDEX idx_lot_consumptions_buy_lot ON lot_consumptions(buy_lot_id);

ALTER TABLE transactions ADD COLUMN lot_id TEXT;

CREATE INDEX idx_transactions_lot_id ON transactions(lot_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_transactions_lot_id;
DROP TABLE IF EXISTS lot_consumptions;
DROP TABLE IF EXISTS lots;
DROP TABLE IF EXISTS positions;
-- Note: SQLite does not support DROP COLUMN in older versions;
-- the lot_id column on transactions will remain after rollback.
-- +goose StatementEnd
