package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/eddiectc/portfoliolab/internal/data/queries"
	"github.com/eddiectc/portfoliolab/internal/domain/marketdata"
	"github.com/govalues/decimal"
)

// MarketDataOverrideRepository provides data access for user-created market
// data overrides, delegating to sqlc-generated queries. It implements the
// marketdata.Store interface used by the domain service.
type MarketDataOverrideRepository struct {
	q  *queries.Queries
	db queries.DBTX
}

// Compile-time assertion: MarketDataOverrideRepository satisfies the domain
// Store interface.
var _ marketdata.Store = (*MarketDataOverrideRepository)(nil)

// NewMarketDataOverrideRepository creates a new market data override
// repository.
func NewMarketDataOverrideRepository(db *sql.DB) *MarketDataOverrideRepository {
	return &MarketDataOverrideRepository{
		q:  queries.New(),
		db: db,
	}
}

// toOverride converts a sqlc MarketDataOverride to a domain Override.
func toOverride(m queries.MarketDataOverride) (*marketdata.Override, error) {
	price, err := decimal.Parse(m.Price)
	if err != nil {
		return nil, fmt.Errorf("parse override price: %w", err)
	}
	createdAt, err := parseTime(m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse override created_at: %w", err)
	}
	updatedAt, err := parseTime(m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse override updated_at: %w", err)
	}
	return &marketdata.Override{
		Symbol:    m.Symbol,
		Date:      m.Date,
		DataType:  marketdata.DataType(m.DataType),
		Price:     price,
		Currency:  m.Currency,
		Source:    marketdata.Source,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// Upsert inserts or replaces the override for (Symbol, Date, DataType).
func (r *MarketDataOverrideRepository) Upsert(ctx context.Context, o *marketdata.Override) error {
	_, err := r.q.UpsertMarketDataOverride(ctx, r.db, queries.UpsertMarketDataOverrideParams{
		Symbol:   o.Symbol,
		Date:     o.Date,
		DataType: string(o.DataType),
		Price:    o.Price.String(),
		Currency: o.Currency,
	})
	if err != nil {
		return fmt.Errorf("upsert market data override for %s/%s/%s: %w", o.Symbol, o.Date, o.DataType, err)
	}
	return nil
}

// Get retrieves the override for (symbol, date, data_type). Returns nil if no
// override exists.
func (r *MarketDataOverrideRepository) Get(ctx context.Context, symbol, date string, dt marketdata.DataType) (*marketdata.Override, error) {
	m, err := r.q.GetMarketDataOverride(ctx, r.db, queries.GetMarketDataOverrideParams{
		Symbol:   symbol,
		Date:     date,
		DataType: string(dt),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get market data override for %s/%s/%s: %w", symbol, date, dt, err)
	}
	return toOverride(m)
}

// Delete removes the override for (symbol, date, data_type) and reports
// whether a row was actually deleted.
func (r *MarketDataOverrideRepository) Delete(ctx context.Context, symbol, date string, dt marketdata.DataType) (bool, error) {
	affected, err := r.q.DeleteMarketDataOverride(ctx, r.db, queries.DeleteMarketDataOverrideParams{
		Symbol:   symbol,
		Date:     date,
		DataType: string(dt),
	})
	if err != nil {
		return false, fmt.Errorf("delete market data override for %s/%s/%s: %w", symbol, date, dt, err)
	}
	return affected > 0, nil
}

// List returns all overrides, or only the symbol's when symbol is non-nil,
// ordered by symbol, date, data_type.
func (r *MarketDataOverrideRepository) List(ctx context.Context, symbol *string) ([]marketdata.Override, error) {
	var rows []queries.MarketDataOverride
	var err error
	if symbol == nil {
		rows, err = r.q.ListMarketDataOverrides(ctx, r.db)
	} else {
		rows, err = r.q.ListMarketDataOverridesBySymbol(ctx, r.db, *symbol)
	}
	if err != nil {
		return nil, fmt.Errorf("list market data overrides: %w", err)
	}

	overrides := make([]marketdata.Override, 0, len(rows))
	for _, row := range rows {
		o, err := toOverride(row)
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, *o)
	}
	return overrides, nil
}
