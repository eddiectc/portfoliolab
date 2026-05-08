-- name: GetLatestMarketData :one
SELECT * FROM market_data
WHERE symbol = ? AND date IS NULL
ORDER BY fetched_at DESC
LIMIT 1;

-- name: GetMarketDataBySymbolAndDate :many
SELECT * FROM market_data
WHERE symbol = ? AND date = ?
ORDER BY fetched_at DESC;

-- name: GetMarketDataBySymbolAndSourceAndDate :one
SELECT * FROM market_data
WHERE symbol = ? AND source = ? AND date = ?
ORDER BY fetched_at DESC
LIMIT 1;

-- name: GetCurrentFxRate :one
SELECT * FROM market_data
WHERE symbol = ? AND data_type = 'fx' AND date IS NULL
ORDER BY fetched_at DESC
LIMIT 1;

-- name: InsertMarketData :one
INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(symbol, source, date) DO UPDATE SET
    price = excluded.price,
    currency = excluded.currency,
    data_type = excluded.data_type,
    fetched_at = excluded.fetched_at
RETURNING *;

-- name: DeleteStaleMarketData :execrows
DELETE FROM market_data
WHERE date IS NULL
  AND symbol = ?
  AND fetched_at < ?;
