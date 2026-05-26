-- name: GetLatestMarketData :one
SELECT * FROM market_data
WHERE symbol = ? AND date = ''
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
WHERE symbol = ? AND data_type = 'fx' AND date = ''
ORDER BY fetched_at DESC
LIMIT 1;

-- name: InsertMarketData :one
INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(symbol, source, date) DO UPDATE SET
    price = excluded.price,
    currency = excluded.currency,
    data_type = excluded.data_type,
    fetched_at = excluded.fetched_at,
    updated_at = datetime('now')
RETURNING *;

-- name: DeleteStaleMarketData :execrows
DELETE FROM market_data
WHERE date = ''
  AND symbol = ?
  AND fetched_at < ?;

-- name: GetHistoricalPricesBySymbolAndRange :many
-- Historical prices for one symbol within [date_from, date_to], sorted by date ASC.
-- Excludes current (date='') entries. Returns 'stock' and 'fx' data_type.
SELECT id, symbol, price, currency, data_type, source, date, fetched_at, created_at, updated_at
FROM market_data
WHERE symbol = ?
  AND date >= ?
  AND date <= ?
  AND date != ''
  AND data_type IN ('stock', 'fx')
ORDER BY date ASC;

-- name: GetLatestQuote :one
-- Latest (date='') entry for a single symbol.
-- Returns the most recently fetched current quote.
SELECT id, symbol, price, currency, data_type, source, date, fetched_at, created_at, updated_at
FROM market_data
WHERE symbol = ?
  AND date = ''
  AND data_type = 'stock'
ORDER BY fetched_at DESC
LIMIT 1;

-- name: GetLatestPriceDatePerSymbol :one
-- MAX(date) for one symbol's stock data (to detect staleness).
-- Excludes current (date='') entries.
SELECT symbol, MAX(date) AS latest_date
FROM market_data
WHERE symbol = ?
  AND date != ''
  AND data_type = 'stock';

-- name: GetHistoricalFxRateOnOrBefore :one
-- Latest FX rate on or before the given date (forward-fill).
-- Used for position P&L conversion to match the equity curve's FX methodology.
SELECT id, symbol, price, currency, data_type, source, date, fetched_at, created_at, updated_at
FROM market_data
WHERE symbol = ?
  AND date <= ?
  AND date != ''
  AND data_type = 'fx'
  AND source = ?
ORDER BY date DESC
LIMIT 1;

-- name: GetDistinctCachedSymbols :many
-- Distinct symbols with cached data (stock or fx), with the latest cached date.
SELECT DISTINCT symbol, data_type, MAX(date) AS latest_date
FROM market_data
GROUP BY symbol, data_type;

-- name: GetNavHistoryBySymbol :many
-- NAV history for one symbol, sorted by date ASC.
-- Excludes current (date='') entries.
SELECT id, symbol, price, currency, data_type, source, date, fetched_at, created_at, updated_at
FROM market_data
WHERE symbol = ?
  AND data_type = 'nav'
  AND date != ''
ORDER BY date ASC;
