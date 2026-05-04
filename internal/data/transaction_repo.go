package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/data/queries"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

// TransactionRepository provides data access for transactions,
// delegating to sqlc-generated queries.
type TransactionRepository struct {
	q    *queries.Queries
	db   queries.DBTX
}

// NewTransactionRepository creates a new transaction repository.
func NewTransactionRepository(db *sql.DB) *TransactionRepository {
	return &TransactionRepository{
		q:  queries.New(),
		db: db,
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

	var netCash *decimal.Decimal
	if t.NetCash.Valid {
		v, err := decimal.Parse(t.NetCash.String)
		if err != nil {
			return nil, fmt.Errorf("parse net_cash: %w", err)
		}
		netCash = &v
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

// toNullDecimal converts a *decimal.Decimal to sql.NullString.
func toNullDecimal(d *decimal.Decimal) sql.NullString {
	if d == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: d.String(), Valid: true}
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
		NetCash:           toNullDecimal(t.NetCash),
		ExternalSystem:    toNullString(t.ExternalSystem),
		ExternalReference: toNullString(t.ExternalReference),
		CreatedAt:         t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         t.UpdatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	t.ID = result.ID
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
			Symbol:    *filters.Symbol,
			Type:      *filters.Type,
			Date:      filters.DateFrom.Format(time.RFC3339),
			Date_2:    filters.DateTo.Format(time.RFC3339),
			Limit:     int64(limit),
			Offset:    int64(offset),
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

// Update modifies an existing transaction.
func (r *TransactionRepository) Update(ctx context.Context, t *transaction.Transaction) error {
	_, err := r.q.UpdateTransaction(ctx, r.db, queries.UpdateTransactionParams{
		Date:              t.Date.Format(time.RFC3339),
		Type:              t.Type,
		Symbol:            t.Symbol,
		Quantity:          t.Quantity.String(),
		Price:             t.Price.String(),
		Currency:          t.Currency,
		NetCash:           toNullDecimal(t.NetCash),
		ExternalSystem:    toNullString(t.ExternalSystem),
		ExternalReference: toNullString(t.ExternalReference),
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
