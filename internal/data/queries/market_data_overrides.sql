-- name: UpsertMarketDataOverride :one
INSERT INTO market_data_overrides (symbol, date, data_type, price, currency)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(symbol, date, data_type) DO UPDATE SET
    price = excluded.price,
    currency = excluded.currency,
    updated_at = datetime('now')
RETURNING *;

-- name: GetMarketDataOverride :one
SELECT * FROM market_data_overrides
WHERE symbol = ? AND date = ? AND data_type = ?;

-- name: ListMarketDataOverrides :many
SELECT * FROM market_data_overrides
ORDER BY symbol, date, data_type;

-- name: ListMarketDataOverridesBySymbol :many
SELECT * FROM market_data_overrides
WHERE symbol = ?
ORDER BY symbol, date, data_type;

-- name: DeleteMarketDataOverride :execrows
DELETE FROM market_data_overrides
WHERE symbol = ? AND date = ? AND data_type = ?;
