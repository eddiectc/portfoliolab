-- Complete schema for sqlc (combines all migrations)
-- This file is used only by sqlc for type inference.
-- Actual database schema is managed by goose migrations.

CREATE TABLE portfolios (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    currency   TEXT    NOT NULL DEFAULT 'USD',
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE accounts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL,
    portfolio_id INTEGER NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (portfolio_id) REFERENCES portfolios(id) ON DELETE CASCADE
);

CREATE TABLE symbol_mappings (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    internal_symbol     TEXT    NOT NULL UNIQUE,
    market_data_symbol  TEXT    NOT NULL,
    is_benchmark        BOOLEAN NOT NULL DEFAULT 0,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE broker_symbol_mappings (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol_mapping_id   INTEGER NOT NULL,
    broker_name         TEXT    NOT NULL,
    broker_symbol       TEXT    NOT NULL,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (symbol_mapping_id) REFERENCES symbol_mappings(id) ON DELETE CASCADE,
    UNIQUE(broker_name, broker_symbol)
);

CREATE TABLE transactions (
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
    lot_id              TEXT,
    description         TEXT,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE positions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id        INTEGER NOT NULL,
    symbol            TEXT    NOT NULL,
    currency          TEXT    NOT NULL,
    quantity          TEXT    NOT NULL DEFAULT '0',
    cost_basis        TEXT    NOT NULL DEFAULT '0',
    avg_open_price    TEXT,
    avg_close_price   TEXT,
    realized_pnl      TEXT    NOT NULL DEFAULT '0',
    realized_pnl_pct  TEXT    DEFAULT NULL,
    realized_pnl_base TEXT,
    fx_rate_used      TEXT    DEFAULT '',
    fx_rate_fallback  BOOLEAN DEFAULT 0,
    open_date         TEXT    NOT NULL,
    close_date        TEXT,
    is_closed         INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE lots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    lot_id          TEXT    NOT NULL UNIQUE,
    account_id      INTEGER NOT NULL,
    symbol          TEXT    NOT NULL,
    lot_type        TEXT    NOT NULL,
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

CREATE TABLE lot_consumptions (
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
    geographic_allocations TEXT,
    fetched_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE market_data (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol      TEXT    NOT NULL,
    price       TEXT    NOT NULL,
    currency    TEXT    NOT NULL,
    data_type   TEXT    NOT NULL,
    source      TEXT    NOT NULL,
    date        TEXT    NOT NULL,
    fetched_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);
