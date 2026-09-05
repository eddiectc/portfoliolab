-- +goose Up
-- +goose StatementBegin
CREATE TABLE target_allocations (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    portfolio_id INTEGER NOT NULL,
    symbol       TEXT    NOT NULL,
    target_pct   TEXT    NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (portfolio_id) REFERENCES portfolios(id) ON DELETE CASCADE,
    UNIQUE(portfolio_id, symbol)
);

CREATE INDEX idx_target_allocations_portfolio_id
    ON target_allocations(portfolio_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_target_allocations_portfolio_id;
DROP TABLE IF EXISTS target_allocations;
-- +goose StatementEnd
