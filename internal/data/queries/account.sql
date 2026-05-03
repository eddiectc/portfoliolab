-- name: CreateAccount :one
INSERT INTO accounts (name, portfolio_id, created_at, updated_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = ?;

-- name: ListAccounts :many
SELECT * FROM accounts
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: GetAccountsByPortfolio :many
SELECT * FROM accounts
WHERE portfolio_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: GetAccountByName :one
SELECT * FROM accounts WHERE name = ?;

-- name: UpdateAccount :one
UPDATE accounts
SET name = ?, portfolio_id = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE id = ?;
