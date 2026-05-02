-- name: CreatePortfolio :one
INSERT INTO portfolios (name, currency, created_at, updated_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetPortfolio :one
SELECT * FROM portfolios WHERE id = ?;

-- name: ListPortfolios :many
SELECT * FROM portfolios
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: UpdatePortfolio :one
UPDATE portfolios
SET name = ?, currency = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeletePortfolio :execrows
DELETE FROM portfolios WHERE id = ?;

-- name: GetPortfolioByName :one
SELECT * FROM portfolios WHERE name = ?;
