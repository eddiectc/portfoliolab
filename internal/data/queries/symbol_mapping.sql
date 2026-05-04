-- name: CreateSymbolMapping :one
INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, created_at, updated_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetSymbolMapping :one
SELECT * FROM symbol_mappings WHERE id = ?;

-- name: GetSymbolMappingByInternalSymbol :one
SELECT * FROM symbol_mappings WHERE internal_symbol = ?;

-- name: ListSymbolMappings :many
SELECT * FROM symbol_mappings
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: UpdateSymbolMapping :one
UPDATE symbol_mappings
SET internal_symbol = ?, market_data_symbol = ?, updated_at = ?
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
