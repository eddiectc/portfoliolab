-- name: CreateModelPortfolio :one
INSERT INTO model_portfolios (name, entries, created_at, updated_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetModelPortfolioByID :one
SELECT * FROM model_portfolios
WHERE id = ?;

-- name: GetModelPortfolioByName :one
SELECT * FROM model_portfolios
WHERE name = ?;

-- name: ListModelPortfolios :many
SELECT * FROM model_portfolios
ORDER BY name
LIMIT ? OFFSET ?;

-- name: UpdateModelPortfolio :one
UPDATE model_portfolios
SET name = ?, entries = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteModelPortfolio :exec
DELETE FROM model_portfolios
WHERE id = ?;

-- name: CountModelPortfolios :one
SELECT COUNT(*) FROM model_portfolios;
