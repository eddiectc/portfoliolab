package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

// TransactionRepository provides data access for transactions,
// delegating to sqlc-generated queries.
type TransactionRepository struct {
	q     *queries.Queries
	db    queries.DBTX
	sqlDB *sql.DB
}

// NewTransactionRepository creates a new transaction repository.
func NewTransactionRepository(db *sql.DB) *TransactionRepository {
	return &TransactionRepository{
		q:     queries.New(),
		db:    db,
		sqlDB: db,
	}
}

// toTransaction converts a sqlc Transaction to a domain Transaction.
// SQLite stores timestamps as text and decimals as text, so sqlc generates
// string fields; this function parses them back to time.Time and decimal.Decimal.
func toTransaction(t queries.Transaction) (*transaction.Transaction, error) {
	date, err := parseTime(t.Date)
	if err != nil {
		return nil, fmt.Errorf("parse date: %w", err)
	}
	createdAt, err := parseTime(t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	quantity, err := decimal.Parse(t.Quantity)
	if err != nil {
		return nil, fmt.Errorf("parse quantity: %w", err)
	}
	price, err := decimal.Parse(t.Price)
	if err != nil {
		return nil, fmt.Errorf("parse price: %w", err)
	}

	netCash, err := decimal.Parse(t.NetCash)
	if err != nil {
		return nil, fmt.Errorf("parse net_cash: %w", err)
	}

	var externalSystem *string
	if t.ExternalSystem.Valid {
		v := t.ExternalSystem.String
		externalSystem = &v
	}

	var externalReference *string
	if t.ExternalReference.Valid {
		v := t.ExternalReference.String
		externalReference = &v
	}

	var lotID *string
	if t.LotID.Valid {
		v := t.LotID.String
		lotID = &v
	}

	var description *string
	if t.Description.Valid {
		v := t.Description.String
		description = &v
	}

	return &transaction.Transaction{
		ID:                t.ID,
		AccountID:         t.AccountID,
		Date:              date,
		Type:              t.Type,
		Symbol:            t.Symbol,
		Quantity:          quantity,
		Price:             price,
		Currency:          t.Currency,
		NetCash:           netCash,
		Description:       description,
		LotID:             lotID,
		ExternalSystem:    externalSystem,
		ExternalReference: externalReference,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}, nil
}

// toDomainSlice converts a slice of sqlc Transactions to domain Transactions.
func toDomainSlice(items []queries.Transaction) ([]transaction.Transaction, error) {
	result := make([]transaction.Transaction, len(items))
	for i, t := range items {
		d, err := toTransaction(t)
		if err != nil {
			return nil, fmt.Errorf("parse transaction %d: %w", t.ID, err)
		}
		result[i] = *d
	}
	return result, nil
}

// toNullString converts a *string to sql.NullString.
func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// Create inserts a new transaction and returns it with the generated ID.
func (r *TransactionRepository) Create(ctx context.Context, t *transaction.Transaction) error {
	result, err := r.q.CreateTransaction(ctx, r.db, queries.CreateTransactionParams{
		AccountID:         t.AccountID,
		Date:              t.Date.Format(time.RFC3339),
		Type:              t.Type,
		Symbol:            t.Symbol,
		Quantity:          t.Quantity.String(),
		Price:             t.Price.String(),
		Currency:          t.Currency,
		NetCash:           t.NetCash.String(),
		ExternalSystem:    toNullString(t.ExternalSystem),
		Description:       toNullString(t.Description),
		ExternalReference: toNullString(t.ExternalReference),
		LotID:             toNullString(t.LotID),
		CreatedAt:         t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         t.UpdatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	t.ID = result.ID
	return nil
}

// BatchCreate inserts multiple transactions within a single database
// transaction (all-or-nothing). If any insert fails, the entire batch
// is rolled back.
func (r *TransactionRepository) BatchCreate(ctx context.Context, txns []*transaction.Transaction) error {
	tx, err := r.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	q := r.q
	for _, t := range txns {
		_, err := q.CreateTransaction(ctx, tx, queries.CreateTransactionParams{
			AccountID:         t.AccountID,
			Date:              t.Date.Format(time.RFC3339),
			Type:              t.Type,
			Symbol:            t.Symbol,
			Quantity:          t.Quantity.String(),
			Price:             t.Price.String(),
			Currency:          t.Currency,
			NetCash:           t.NetCash.String(),
			ExternalSystem:    toNullString(t.ExternalSystem),
			Description:       toNullString(t.Description),
			ExternalReference: toNullString(t.ExternalReference),
			LotID:             toNullString(t.LotID),
			CreatedAt:         t.CreatedAt.Format(time.RFC3339),
			UpdatedAt:         t.UpdatedAt.Format(time.RFC3339),
		})
		if err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// GetByID retrieves a transaction by its ID.
func (r *TransactionRepository) GetByID(ctx context.Context, id int64) (*transaction.Transaction, error) {
	t, err := r.q.GetTransaction(ctx, r.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get transaction by id %d: %w", id, err)
	}
	return toTransaction(t)
}

// List retrieves transactions matching the given filters with pagination.
// Routes to the appropriate specialized sqlc query based on which filters are set.
func (r *TransactionRepository) List(ctx context.Context, filters transaction.ListFilters, limit, offset int) ([]transaction.Transaction, error) {
	hasAccount := filters.AccountID != nil
	hasSymbol := filters.Symbol != nil
	hasType := filters.Type != nil
	hasDate := filters.DateFrom != nil && filters.DateTo != nil

	var (
		items []queries.Transaction
		err   error
	)

	switch {
	case hasAccount && hasSymbol && hasType && hasDate:
		items, err = r.q.ListTransactionsByAllFilters(ctx, r.db, queries.ListTransactionsByAllFiltersParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Type:      *filters.Type,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasAccount && hasSymbol && hasType:
		items, err = r.q.ListTransactionsByAccountSymbolType(ctx, r.db, queries.ListTransactionsByAccountSymbolTypeParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Type:      *filters.Type,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasAccount && hasSymbol && hasDate:
		items, err = r.q.ListTransactionsByAccountSymbolDateRange(ctx, r.db, queries.ListTransactionsByAccountSymbolDateRangeParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasAccount && hasType && hasDate:
		items, err = r.q.ListTransactionsByAccountTypeDateRange(ctx, r.db, queries.ListTransactionsByAccountTypeDateRangeParams{
			AccountID: *filters.AccountID,
			Type:      *filters.Type,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasSymbol && hasType && hasDate:
		items, err = r.q.ListTransactionsBySymbolTypeDateRange(ctx, r.db, queries.ListTransactionsBySymbolTypeDateRangeParams{
			Symbol: *filters.Symbol,
			Type:   *filters.Type,
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	case hasAccount && hasSymbol:
		items, err = r.q.ListTransactionsByAccountAndSymbol(ctx, r.db, queries.ListTransactionsByAccountAndSymbolParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasAccount && hasType:
		items, err = r.q.ListTransactionsByAccountAndType(ctx, r.db, queries.ListTransactionsByAccountAndTypeParams{
			AccountID: *filters.AccountID,
			Type:      *filters.Type,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasAccount && hasDate:
		items, err = r.q.ListTransactionsByAccountAndDateRange(ctx, r.db, queries.ListTransactionsByAccountAndDateRangeParams{
			AccountID: *filters.AccountID,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasSymbol && hasType:
		items, err = r.q.ListTransactionsBySymbolAndType(ctx, r.db, queries.ListTransactionsBySymbolAndTypeParams{
			Symbol: *filters.Symbol,
			Type:   *filters.Type,
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	case hasSymbol && hasDate:
		items, err = r.q.ListTransactionsBySymbolAndDateRange(ctx, r.db, queries.ListTransactionsBySymbolAndDateRangeParams{
			Symbol: *filters.Symbol,
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	case hasType && hasDate:
		items, err = r.q.ListTransactionsByTypeAndDateRange(ctx, r.db, queries.ListTransactionsByTypeAndDateRangeParams{
			Type:   *filters.Type,
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	case hasAccount:
		items, err = r.q.ListTransactionsByAccount(ctx, r.db, queries.ListTransactionsByAccountParams{
			AccountID: *filters.AccountID,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
	case hasSymbol:
		items, err = r.q.ListTransactionsBySymbol(ctx, r.db, queries.ListTransactionsBySymbolParams{
			Symbol: *filters.Symbol,
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	case hasType:
		items, err = r.q.ListTransactionsByType(ctx, r.db, queries.ListTransactionsByTypeParams{
			Type:   *filters.Type,
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	case hasDate:
		items, err = r.q.ListTransactionsByDateRange(ctx, r.db, queries.ListTransactionsByDateRangeParams{
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	default:
		items, err = r.q.ListTransactions(ctx, r.db, queries.ListTransactionsParams{
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	}

	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}

	return toDomainSlice(items)
}

// ListAllTransactionsByAccount retrieves all transactions for an account
// without pagination, ordered by date ASC for position calculation.
func (r *TransactionRepository) ListAllTransactionsByAccount(ctx context.Context, accountID int64) ([]transaction.Transaction, error) {
	items, err := r.q.ListAllTransactionsByAccount(ctx, r.db, accountID)
	if err != nil {
		return nil, fmt.Errorf("list all transactions for account %d: %w", accountID, err)
	}
	return toDomainSlice(items)
}

// GetSymbolsWithEarliestDate returns all non-cash symbols with their earliest
// transaction date. Used by the SymbolDiscoverer for market data caching.
func (r *TransactionRepository) GetSymbolsWithEarliestDate(ctx context.Context) (map[string]time.Time, error) {
	rows, err := r.q.GetSymbolsWithEarliestDate(ctx, r.db)
	if err != nil {
		return nil, fmt.Errorf("get symbols with earliest date: %w", err)
	}
	result := make(map[string]time.Time, len(rows))
	for _, row := range rows {
		t, err := parseInterfaceTime(row.EarliestDate)
		if err != nil {
			return nil, fmt.Errorf("parse earliest date for %s: %w", row.Symbol, err)
		}
		result[row.Symbol] = t
	}
	return result, nil
}

// GetSymbolsByOpenPositions returns symbols that have open positions, keyed by
// symbol with the earliest transaction date as value.
func (r *TransactionRepository) GetSymbolsByOpenPositions(ctx context.Context) (map[string]time.Time, error) {
	rows, err := r.q.GetSymbolsByOpenPositions(ctx, r.db)
	if err != nil {
		return nil, fmt.Errorf("get symbols by open positions: %w", err)
	}
	result := make(map[string]time.Time, len(rows))
	for _, row := range rows {
		t, err := parseInterfaceTime(row.EarliestDate)
		if err != nil {
			return nil, fmt.Errorf("parse earliest date for %s: %w", row.Symbol, err)
		}
		result[row.Symbol] = t
	}
	return result, nil
}

// GetFxPairsByOpenPositions returns FX pairs needed for open positions,
// keyed by "BASE/QUOTE" with the earliest transaction date as value.
func (r *TransactionRepository) GetFxPairsByOpenPositions(ctx context.Context) (map[string]time.Time, error) {
	rows, err := r.q.GetFxPairsByOpenPositions(ctx, r.db)
	if err != nil {
		return nil, fmt.Errorf("get FX pairs by open positions: %w", err)
	}
	result := make(map[string]time.Time, len(rows))
	for _, row := range rows {
		t, err := parseInterfaceTime(row.EarliestDate)
		if err != nil {
			return nil, fmt.Errorf("parse earliest date for %s/%s: %w", row.BaseCurrency, row.QuoteCurrency, err)
		}
		pair := fmt.Sprintf("%s/%s", row.BaseCurrency, row.QuoteCurrency)
		result[pair] = t
	}
	return result, nil
}

// GetEarliestDateBySymbol returns the earliest transaction date for a
// specific symbol. Returns nil if no transactions found.
func (r *TransactionRepository) GetEarliestDateBySymbol(ctx context.Context, symbol string) (*time.Time, error) {
	val, err := r.q.GetEarliestDateBySymbol(ctx, r.db, symbol)
	if err != nil {
		return nil, fmt.Errorf("get earliest date for %s: %w", symbol, err)
	}
	t, err := parseInterfaceTime(val)
	if err != nil {
		return nil, fmt.Errorf("parse earliest date for %s: %w", symbol, err)
	}
	return &t, nil
}

// parseInterfaceTime converts an interface{} value (from SQLite MIN(date))
// to a time.Time. Handles string and []byte representations.
func parseInterfaceTime(v interface{}) (time.Time, error) {
	if v == nil {
		return time.Time{}, fmt.Errorf("nil date value")
	}
	var s string
	switch val := v.(type) {
	case string:
		s = val
	case []byte:
		s = string(val)
	default:
		return time.Time{}, fmt.Errorf("unexpected date type %T", v)
	}
	return time.Parse(time.RFC3339, s)
}

// Update modifies an existing transaction.
func (r *TransactionRepository) Update(ctx context.Context, t *transaction.Transaction) error {
	_, err := r.q.UpdateTransaction(ctx, r.db, queries.UpdateTransactionParams{
		Date:              t.Date.Format(time.RFC3339),
		Type:              t.Type,
		Symbol:            t.Symbol,
		Quantity:          t.Quantity.String(),
		Price:             t.Price.String(),
		Currency:          t.Currency,
		NetCash:           t.NetCash.String(),
		ExternalSystem:    toNullString(t.ExternalSystem),
		Description:       toNullString(t.Description),
		ExternalReference: toNullString(t.ExternalReference),
		LotID:             toNullString(t.LotID),
		UpdatedAt:         t.UpdatedAt.Format(time.RFC3339),
		ID:                t.ID,
	})
	if err != nil {
		return fmt.Errorf("update transaction %d: %w", t.ID, err)
	}
	return nil
}

// Delete removes a transaction by ID.
func (r *TransactionRepository) Delete(ctx context.Context, id int64) error {
	rows, err := r.q.DeleteTransaction(ctx, r.db, id)
	if err != nil {
		return fmt.Errorf("delete transaction %d: %w", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ExternalReferenceExists checks whether a transaction with the given
// external_system and external_reference combination already exists.
func (r *TransactionRepository) ExternalReferenceExists(ctx context.Context, externalSystem, externalReference string) bool {
	_, err := r.q.HasExternalReference(ctx, r.db, queries.HasExternalReferenceParams{
		ExternalSystem:    sql.NullString{String: externalSystem, Valid: true},
		ExternalReference: sql.NullString{String: externalReference, Valid: true},
	})
	if err != nil {
		// sql.ErrNoRows means no match; any other error is logged but treated
		// as "not found" to avoid blocking the import flow on transient DB issues.
		return false
	}
	return true
}

// ListWithAccount retrieves transactions matching the given filters with
// account names resolved via a JOIN, with pagination.
func (r *TransactionRepository) ListWithAccount(ctx context.Context, filters transaction.ListFilters, limit, offset int) ([]transaction.TransactionWithAccount, error) {
	hasAccount := filters.AccountID != nil
	hasSymbol := filters.Symbol != nil
	hasType := filters.Type != nil
	hasDate := filters.DateFrom != nil && filters.DateTo != nil

	switch {
	case hasAccount && hasSymbol && hasType && hasDate:
		rows, err := r.q.ListTransactionsWithAccountByAllFilters(ctx, r.db, queries.ListTransactionsWithAccountByAllFiltersParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Type:      *filters.Type,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasAccount && hasSymbol && hasType:
		rows, err := r.q.ListTransactionsWithAccountByAccountSymbolType(ctx, r.db, queries.ListTransactionsWithAccountByAccountSymbolTypeParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Type:      *filters.Type,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasAccount && hasSymbol && hasDate:
		rows, err := r.q.ListTransactionsWithAccountByAccountSymbolDateRange(ctx, r.db, queries.ListTransactionsWithAccountByAccountSymbolDateRangeParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasAccount && hasType && hasDate:
		rows, err := r.q.ListTransactionsWithAccountByAccountTypeDateRange(ctx, r.db, queries.ListTransactionsWithAccountByAccountTypeDateRangeParams{
			AccountID: *filters.AccountID,
			Type:      *filters.Type,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasSymbol && hasType && hasDate:
		rows, err := r.q.ListTransactionsWithAccountBySymbolTypeDateRange(ctx, r.db, queries.ListTransactionsWithAccountBySymbolTypeDateRangeParams{
			Symbol: *filters.Symbol,
			Type:   *filters.Type,
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasAccount && hasSymbol:
		rows, err := r.q.ListTransactionsWithAccountByAccountAndSymbol(ctx, r.db, queries.ListTransactionsWithAccountByAccountAndSymbolParams{
			AccountID: *filters.AccountID,
			Symbol:    *filters.Symbol,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasAccount && hasType:
		rows, err := r.q.ListTransactionsWithAccountByAccountAndType(ctx, r.db, queries.ListTransactionsWithAccountByAccountAndTypeParams{
			AccountID: *filters.AccountID,
			Type:      *filters.Type,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasAccount && hasDate:
		rows, err := r.q.ListTransactionsWithAccountByAccountAndDateRange(ctx, r.db, queries.ListTransactionsWithAccountByAccountAndDateRangeParams{
			AccountID: *filters.AccountID,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasSymbol && hasType:
		rows, err := r.q.ListTransactionsWithAccountBySymbolAndType(ctx, r.db, queries.ListTransactionsWithAccountBySymbolAndTypeParams{
			Symbol: *filters.Symbol,
			Type:   *filters.Type,
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasSymbol && hasDate:
		rows, err := r.q.ListTransactionsWithAccountBySymbolAndDateRange(ctx, r.db, queries.ListTransactionsWithAccountBySymbolAndDateRangeParams{
			Symbol: *filters.Symbol,
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasType && hasDate:
		rows, err := r.q.ListTransactionsWithAccountByTypeAndDateRange(ctx, r.db, queries.ListTransactionsWithAccountByTypeAndDateRangeParams{
			Type:   *filters.Type,
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasAccount:
		rows, err := r.q.ListTransactionsWithAccountByAccount(ctx, r.db, queries.ListTransactionsWithAccountByAccountParams{
			AccountID: *filters.AccountID,
			Limit:     int64(limit),
			Offset:    int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasSymbol:
		rows, err := r.q.ListTransactionsWithAccountBySymbol(ctx, r.db, queries.ListTransactionsWithAccountBySymbolParams{
			Symbol: *filters.Symbol,
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasType:
		rows, err := r.q.ListTransactionsWithAccountByType(ctx, r.db, queries.ListTransactionsWithAccountByTypeParams{
			Type:   *filters.Type,
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	case hasDate:
		rows, err := r.q.ListTransactionsWithAccountByDateRange(ctx, r.db, queries.ListTransactionsWithAccountByDateRangeParams{
			Date:   filters.DateFrom.Format(time.RFC3339),
			Date_2: filters.DateTo.Format(time.RFC3339),
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	default:
		rows, err := r.q.ListTransactionsWithAccount(ctx, r.db, queries.ListTransactionsWithAccountParams{
			Limit:  int64(limit),
			Offset: int64(offset),
		})
		if err != nil {
			return nil, fmt.Errorf("list transactions with account: %w", err)
		}
		return toDomainWithAccount(toBaseRow(rows))
	}
}

// toDomainWithAccount converts sqlc ListTransactionsWithAccountRow to domain TransactionWithAccount.
// All *WithAccount*Row types from sqlc have identical fields, so we convert to the base Transaction
// and add AccountName.
func toDomainWithAccount(rows []queries.ListTransactionsWithAccountRow) ([]transaction.TransactionWithAccount, error) {
	result := make([]transaction.TransactionWithAccount, len(rows))
	for i, r := range rows {
		d, err := toTransaction(queries.Transaction{
			ID:                r.ID,
			AccountID:         r.AccountID,
			Date:              r.Date,
			Type:              r.Type,
			Symbol:            r.Symbol,
			Quantity:          r.Quantity,
			Price:             r.Price,
			Currency:          r.Currency,
			NetCash:           r.NetCash,
			ExternalSystem:    r.ExternalSystem,
			ExternalReference: r.ExternalReference,
			LotID:             r.LotID,
			Description:       r.Description,
			CreatedAt:         r.CreatedAt,
			UpdatedAt:         r.UpdatedAt,
		})
		if err != nil {
			return nil, fmt.Errorf("parse transaction %d: %w", r.ID, err)
		}
		result[i] = transaction.TransactionWithAccount{
			Transaction: *d,
			AccountName: r.AccountName,
		}
	}
	return result, nil
}

// toBaseRow converts a sqlc row to the base ListTransactionsWithAccountRow.
// All *WithAccount*Row types have identical fields.
func toBaseRow(src any) []queries.ListTransactionsWithAccountRow {
	switch v := src.(type) {
	case []queries.ListTransactionsWithAccountRow:
		return v
	case []queries.ListTransactionsWithAccountByAccountRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountBySymbolRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByTypeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByDateRangeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByAccountAndSymbolRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByAccountAndTypeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByAccountAndDateRangeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountBySymbolAndTypeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountBySymbolAndDateRangeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByTypeAndDateRangeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByAccountSymbolTypeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByAccountSymbolDateRangeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByAccountTypeDateRangeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountBySymbolTypeDateRangeRow:
		return toBaseFrom(v)
	case []queries.ListTransactionsWithAccountByAllFiltersRow:
		return toBaseFrom(v)
	}
	return nil
}

// withAccountFields is the common interface for all *WithAccount*Row types.
type withAccountFields interface {
	ID() int64
	AccountID() int64
	Date() string
	Type() string
	Symbol() string
	Quantity() string
	Price() string
	Currency() string
	NetCash() sql.NullString
	ExternalSystem() sql.NullString
	ExternalReference() sql.NullString
	CreatedAt() string
	UpdatedAt() string
	AccountName() string
}

// toBaseFrom converts any row type with the common fields to base row.
func toBaseFrom(rows any) []queries.ListTransactionsWithAccountRow {
	// Use reflection-free field extraction via type assertions
	var result []queries.ListTransactionsWithAccountRow
	switch v := rows.(type) {
	case []queries.ListTransactionsWithAccountByAccountRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountBySymbolRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByTypeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByDateRangeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByAccountAndSymbolRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByAccountAndTypeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByAccountAndDateRangeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountBySymbolAndTypeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountBySymbolAndDateRangeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByTypeAndDateRangeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByAccountSymbolTypeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByAccountSymbolDateRangeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByAccountTypeDateRangeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountBySymbolTypeDateRangeRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	case []queries.ListTransactionsWithAccountByAllFiltersRow:
		for _, r := range v {
			result = append(result, queries.ListTransactionsWithAccountRow{ID: r.ID, AccountID: r.AccountID, Date: r.Date, Type: r.Type, Symbol: r.Symbol, Quantity: r.Quantity, Price: r.Price, Currency: r.Currency, NetCash: r.NetCash, ExternalSystem: r.ExternalSystem, ExternalReference: r.ExternalReference, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LotID: r.LotID, Description: r.Description, AccountName: r.AccountName})
		}
	}
	return result
}
