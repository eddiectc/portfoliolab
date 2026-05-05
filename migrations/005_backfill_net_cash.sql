-- +goose Up
-- +goose StatementBegin
-- Backfill null net_cash values with quantity * price.
-- This ensures all existing transactions have a valid net_cash before it becomes required.
UPDATE transactions
SET net_cash = (CAST(quantity AS REAL) * CAST(price AS REAL))
WHERE net_cash IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No reversible operation — the original null values are lost after backfill.
-- +goose StatementEnd
