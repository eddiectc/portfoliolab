-- name: CreatePosition :one
INSERT INTO positions (account_id, symbol, currency, quantity, cost_basis, avg_open_price, avg_close_price, realized_pnl, realized_pnl_base, fx_rate_used, fx_rate_fallback, open_date, close_date, is_closed, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteAllPositionsForAccount :execrows
DELETE FROM positions WHERE account_id = ?;

-- name: GetOpenPositionsByAccount :many
SELECT * FROM positions
WHERE account_id = ? AND is_closed = 0
ORDER BY symbol ASC, open_date ASC
LIMIT ? OFFSET ?;

-- name: GetClosedPositionsByAccount :many
SELECT * FROM positions
WHERE account_id = ? AND is_closed = 1
ORDER BY symbol ASC, open_date ASC
LIMIT ? OFFSET ?;

-- name: GetPositionByID :one
SELECT * FROM positions WHERE id = ?;

-- name: CreateLot :one
INSERT INTO lots (lot_id, account_id, symbol, lot_type, quantity, cost_basis, sell_price, realized_pnl, open_date, close_date, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetLotByLotID :one
SELECT * FROM lots WHERE lot_id = ?;

-- name: GetLotsByAccountAndSymbol :many
SELECT * FROM lots
WHERE account_id = ? AND symbol = ?
ORDER BY open_date ASC;

-- name: DeleteAllLotsForAccount :execrows
DELETE FROM lots WHERE account_id = ?;

-- name: CreateLotConsumption :one
INSERT INTO lot_consumptions (sell_lot_id, buy_lot_id, quantity_consumed, cost_basis_consumed, realized_pnl, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetConsumptionsBySellLot :many
SELECT * FROM lot_consumptions WHERE sell_lot_id = ?;

-- name: GetConsumptionsByBuyLot :many
SELECT * FROM lot_consumptions WHERE buy_lot_id = ?;

-- name: DeleteAllLotConsumptionsForAccount :exec
DELETE FROM lot_consumptions
WHERE sell_lot_id IN (SELECT l.lot_id FROM lots l WHERE l.account_id = ?)
   OR buy_lot_id IN (SELECT l.lot_id FROM lots l WHERE l.account_id = ?);
