-- name: CreateTransaction :one
INSERT INTO transactions (account_id, date, type, symbol, quantity, price, currency, net_cash, external_system, external_reference, lot_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTransaction :one
SELECT * FROM transactions WHERE id = ?;

-- name: UpdateTransaction :one
UPDATE transactions
SET date = ?, type = ?, symbol = ?, quantity = ?, price = ?, currency = ?, net_cash = ?, external_system = ?, external_reference = ?, lot_id = ?, updated_at = ?
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

-- name: ListAllTransactionsByAccount :many
SELECT * FROM transactions
WHERE account_id = ?
ORDER BY date ASC, id ASC;

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

-- name: ListTransactionsWithAccount :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByAccount :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountBySymbol :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.symbol = ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByType :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.type = ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByDateRange :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByAccountAndSymbol :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ? AND t.symbol = ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByAccountAndType :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ? AND t.type = ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByAccountAndDateRange :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ? AND t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountBySymbolAndType :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.symbol = ? AND t.type = ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountBySymbolAndDateRange :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.symbol = ? AND t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByTypeAndDateRange :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.type = ? AND t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByAccountSymbolType :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ? AND t.symbol = ? AND t.type = ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByAccountSymbolDateRange :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ? AND t.symbol = ? AND t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountByAccountTypeDateRange :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ? AND t.type = ? AND t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: ListTransactionsWithAccountBySymbolTypeDateRange :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.symbol = ? AND t.type = ? AND t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;

-- name: HasExternalReference :one
SELECT 1 FROM transactions
WHERE external_system = ? AND external_reference = ?
LIMIT 1;

-- name: ListTransactionsWithAccountByAllFilters :many
SELECT t.*, a.name AS account_name
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE t.account_id = ? AND t.symbol = ? AND t.type = ? AND t.date >= ? AND t.date <= ?
ORDER BY t.date DESC, t.symbol ASC, t.type ASC, t.id ASC
LIMIT ? OFFSET ?;
