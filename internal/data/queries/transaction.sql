-- name: CreateTransaction :one
INSERT INTO transactions (account_id, date, type, symbol, quantity, price, currency, net_cash, external_system, external_reference, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTransaction :one
SELECT * FROM transactions WHERE id = ?;

-- name: UpdateTransaction :one
UPDATE transactions
SET date = ?, type = ?, symbol = ?, quantity = ?, price = ?, currency = ?, net_cash = ?, external_system = ?, external_reference = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteTransaction :execrows
DELETE FROM transactions WHERE id = ?;

-- name: ListTransactions :many
SELECT * FROM transactions
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAccount :many
SELECT * FROM transactions
WHERE account_id = ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsBySymbol :many
SELECT * FROM transactions
WHERE symbol = ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByType :many
SELECT * FROM transactions
WHERE type = ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByDateRange :many
SELECT * FROM transactions
WHERE date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAccountAndSymbol :many
SELECT * FROM transactions
WHERE account_id = ? AND symbol = ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAccountAndType :many
SELECT * FROM transactions
WHERE account_id = ? AND type = ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAccountAndDateRange :many
SELECT * FROM transactions
WHERE account_id = ? AND date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsBySymbolAndType :many
SELECT * FROM transactions
WHERE symbol = ? AND type = ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsBySymbolAndDateRange :many
SELECT * FROM transactions
WHERE symbol = ? AND date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByTypeAndDateRange :many
SELECT * FROM transactions
WHERE type = ? AND date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAccountSymbolType :many
SELECT * FROM transactions
WHERE account_id = ? AND symbol = ? AND type = ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAccountSymbolDateRange :many
SELECT * FROM transactions
WHERE account_id = ? AND symbol = ? AND date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAccountTypeDateRange :many
SELECT * FROM transactions
WHERE account_id = ? AND type = ? AND date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsBySymbolTypeDateRange :many
SELECT * FROM transactions
WHERE symbol = ? AND type = ? AND date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsByAllFilters :many
SELECT * FROM transactions
WHERE account_id = ? AND symbol = ? AND type = ? AND date >= ? AND date <= ?
ORDER BY date DESC, symbol ASC, type ASC, id ASC
LIMIT ? OFFSET ?;
