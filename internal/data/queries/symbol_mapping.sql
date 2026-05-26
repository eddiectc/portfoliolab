-- name: CreateSymbolMapping :one
INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, is_benchmark, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSymbolMapping :one
SELECT * FROM symbol_mappings WHERE id = ?;

-- name: GetSymbolMappingByInternalSymbol :one
SELECT * FROM symbol_mappings WHERE internal_symbol = ?;

-- name: ListSymbolMappings :many
SELECT * FROM symbol_mappings
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListAllSymbolMappings :many
SELECT * FROM symbol_mappings
ORDER BY created_at DESC;

-- name: UpdateSymbolMapping :one
UPDATE symbol_mappings
SET internal_symbol = ?, market_data_symbol = ?, is_benchmark = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteSymbolMapping :execrows
DELETE FROM symbol_mappings WHERE id = ?;

-- name: AddBrokerSymbol :one
INSERT INTO broker_symbol_mappings (symbol_mapping_id, broker_name, broker_symbol, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetBrokerSymbolsByMappingID :many
SELECT * FROM broker_symbol_mappings
WHERE symbol_mapping_id = ?
ORDER BY broker_name, broker_symbol;

-- name: GetBrokerSymbolByBroker :one
SELECT bsm.* FROM broker_symbol_mappings bsm
JOIN symbol_mappings sm ON bsm.symbol_mapping_id = sm.id
WHERE bsm.broker_name = ? AND bsm.broker_symbol = ?;

-- name: DeleteBrokerSymbol :execrows
DELETE FROM broker_symbol_mappings WHERE id = ?;

-- name: DeleteBrokerSymbolsByMappingID :exec
DELETE FROM broker_symbol_mappings WHERE symbol_mapping_id = ?;

-- name: ListBenchmarkSymbols :many
SELECT * FROM symbol_mappings WHERE is_benchmark = 1 ORDER BY internal_symbol;

-- name: ListAllMarketDataSymbols :many
-- All symbols for market data fetching.
SELECT internal_symbol, market_data_symbol FROM symbol_mappings
ORDER BY internal_symbol;

-- name: UpdateSymbolMappingDataSourceURL :one
-- Update the data_source_url for a symbol mapping.
UPDATE symbol_mappings
SET data_source_url = ?, updated_at = ?
WHERE id = ?
RETURNING *;
