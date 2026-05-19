-- name: GetTargetAllocationsByPortfolio :many
SELECT * FROM target_allocations
WHERE portfolio_id = ?
ORDER BY symbol;

-- name: UpsertTargetAllocation :one
INSERT INTO target_allocations (portfolio_id, symbol, target_pct, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(portfolio_id, symbol)
DO UPDATE SET target_pct = excluded.target_pct, updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteTargetAllocationBySymbol :execrows
DELETE FROM target_allocations
WHERE portfolio_id = ? AND symbol = ?;

-- name: DeleteTargetAllocationsByPortfolio :execrows
DELETE FROM target_allocations
WHERE portfolio_id = ?;
