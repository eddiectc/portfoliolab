-- name: CreatePortfolio :one
INSERT INTO portfolios (name, currency, created_at, updated_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPortfolio :one
SELECT * FROM portfolios WHERE id = $1;

-- name: ListPortfolios :many
SELECT * FROM portfolios
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdatePortfolio :one
UPDATE portfolios
SET name = $2, currency = $3, updated_at = $4
WHERE id = $1
RETURNING *;

-- name: DeletePortfolio :exec
DELETE FROM portfolios WHERE id = $1;

-- name: GetPortfolioByName :one
SELECT * FROM portfolios WHERE name = $1;
