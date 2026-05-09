package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// MarketDataRepository provides data access for market data (stock quotes and
// FX rates), delegating to sqlc-generated queries.
type MarketDataRepository struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewMarketDataRepository creates a new market data repository.
func NewMarketDataRepository(db *sql.DB) *MarketDataRepository {
	return &MarketDataRepository{
		q:  queries.New(),
		db: db,
	}
}

// toMarketDatum converts a sqlc MarketDatum to a domain MarketData.
func toMarketDatum(m queries.MarketDatum) (*market.MarketData, error) {
	fetchedAt, err := parseTime(m.FetchedAt)
	if err != nil {
		return nil, fmt.Errorf("parse fetched_at: %w", err)
	}

	price, err := decimal.Parse(m.Price)
	if err != nil {
		return nil, fmt.Errorf("parse price: %w", err)
	}

	return &market.MarketData{
		Symbol:    m.Symbol,
		Price:     price,
		Currency:  m.Currency,
		DataType:  m.DataType,
		Source:    m.Source,
		Date:      m.Date,
		FetchedAt: fetchedAt,
	}, nil
}

// GetLatest retrieves the latest (current, date='') market data entry for a
// symbol, ordered by fetched_at DESC.
func (r *MarketDataRepository) GetLatest(ctx context.Context, symbol string) (*market.MarketData, error) {
	m, err := r.q.GetLatestMarketData(ctx, r.db, symbol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // no data found — not an error
		}
		return nil, fmt.Errorf("get latest market data for %s: %w", symbol, err)
	}
	return toMarketDatum(m)
}

// GetBySourceAndDate retrieves a specific market data entry by symbol, source,
// and date. Returns nil if no entry found.
func (r *MarketDataRepository) GetBySourceAndDate(ctx context.Context, symbol, source, date string) (*market.MarketData, error) {
	m, err := r.q.GetMarketDataBySymbolAndSourceAndDate(ctx, r.db, queries.GetMarketDataBySymbolAndSourceAndDateParams{
		Symbol: symbol,
		Source: source,
		Date:   date,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get market data for %s/%s/%s: %w", symbol, source, date, err)
	}
	return toMarketDatum(m)
}

// Upsert inserts or updates a market data entry. Uses ON CONFLICT(symbol,
// source, date) to update price/currency/data_type/fetched_at when a duplicate
// is found.
func (r *MarketDataRepository) Upsert(ctx context.Context, m *market.MarketData) error {
	_, err := r.q.InsertMarketData(ctx, r.db, queries.InsertMarketDataParams{
		Symbol:    m.Symbol,
		Price:     m.Price.String(),
		Currency:  m.Currency,
		DataType:  m.DataType,
		Source:    m.Source,
		Date:      m.Date,
		FetchedAt: m.FetchedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("upsert market data for %s: %w", m.Symbol, err)
	}
	return nil
}

// GetCurrentFxRate retrieves the current (date='') FX rate.
// baseCurrency is the source currency, quoteCurrency is the target.
// Returns nil if no rate found.
func (r *MarketDataRepository) GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*market.MarketData, error) {
	pair := market.FormatFxPair(baseCurrency, quoteCurrency)
	m, err := r.q.GetCurrentFxRate(ctx, r.db, pair)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get current FX rate for %s: %w", pair, err)
	}
	return toMarketDatum(m)
}

// DeleteStaleMarketData removes current (date='') entries for a symbol that
// are older than the given fetched_at threshold.
func (r *MarketDataRepository) DeleteStaleMarketData(ctx context.Context, symbol string, olderThan time.Time) (int64, error) {
	return r.q.DeleteStaleMarketData(ctx, r.db, queries.DeleteStaleMarketDataParams{
		Symbol:    symbol,
		FetchedAt: olderThan.Format(time.RFC3339),
	})
}
