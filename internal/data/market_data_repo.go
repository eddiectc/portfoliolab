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

// GetLatest retrieves the latest (current, date=”) market data entry for a
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

// GetHistoricalFxRateOnOrBefore retrieves the latest FX rate on or before the
// given date (forward-fill). Returns nil if no rate found.
func (r *MarketDataRepository) GetHistoricalFxRateOnOrBefore(ctx context.Context, symbol, source, date string) (*market.MarketData, error) {
	m, err := r.q.GetHistoricalFxRateOnOrBefore(ctx, r.db, queries.GetHistoricalFxRateOnOrBeforeParams{
		Symbol: symbol,
		Date:   date,
		Source: source,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get historical FX rate on or before %s/%s/%s: %w", symbol, source, date, err)
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

// GetCurrentFxRate retrieves the current (date=”) FX rate.
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

// DeleteStaleMarketData removes current (date=”) entries for a symbol that
// are older than the given fetched_at threshold.
func (r *MarketDataRepository) DeleteStaleMarketData(ctx context.Context, symbol string, olderThan time.Time) (int64, error) {
	return r.q.DeleteStaleMarketData(ctx, r.db, queries.DeleteStaleMarketDataParams{
		Symbol:    symbol,
		FetchedAt: olderThan.Format(time.RFC3339),
	})
}

// GetHistoricalPricesBySymbol reads cached historical prices for one symbol
// within [start, end], sorted by date ASC. Returns empty slice if no data found.
func (r *MarketDataRepository) GetHistoricalPricesBySymbol(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error) {
	rows, err := r.q.GetHistoricalPricesBySymbolAndRange(ctx, r.db, queries.GetHistoricalPricesBySymbolAndRangeParams{
		Symbol: symbol,
		Date:   start.Format("2006-01-02"),
		Date_2: end.Format("2006-01-02"),
	})
	if err != nil {
		return nil, fmt.Errorf("get historical prices for %s: %w", symbol, err)
	}

	var prices []market.HistoricalPrice
	for _, row := range rows {
		// Historical dates are stored as YYYY-MM-DD, not RFC3339.
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return nil, fmt.Errorf("parse date %q for %s: %w", row.Date, symbol, err)
		}
		price, err := decimal.Parse(row.Price)
		if err != nil {
			return nil, fmt.Errorf("parse price for %s on %s: %w", symbol, row.Date, err)
		}
		prices = append(prices, market.HistoricalPrice{
			Date:     date,
			Close:    price,
			Currency: row.Currency,
		})
	}

	return prices, nil
}

// GetLatestQuotesBatch reads the latest (date=”) quote for multiple symbols.
// Symbols with no cached quote are omitted from the result. This loops over
// the single-symbol sqlc query because sqlc doesn't support dynamic IN clauses
// for SQLite.
func (r *MarketDataRepository) GetLatestQuotesBatch(ctx context.Context, symbols []string) map[string]*market.MarketData {
	result := make(map[string]*market.MarketData)
	for _, symbol := range symbols {
		md, err := r.q.GetLatestQuote(ctx, r.db, symbol)
		if err != nil {
			// sql.ErrNoRows or other DB error — skip this symbol
			continue
		}
		m, err := toMarketDatum(md)
		if err != nil {
			continue
		}
		result[symbol] = m
	}
	return result
}

// GetLatestPriceDatePerSymbol reads MAX(date) for stock data per symbol,
// excluding current (date=”) entries. Symbols with no cached history are
// omitted from the result.
func (r *MarketDataRepository) GetLatestPriceDatePerSymbol(ctx context.Context, symbols []string) map[string]*time.Time {
	result := make(map[string]*time.Time)
	for _, symbol := range symbols {
		row, err := r.q.GetLatestPriceDatePerSymbol(ctx, r.db, symbol)
		if err != nil {
			// sql.ErrNoRows or other DB error — skip this symbol
			continue
		}
		// LatestDate is interface{} (can be nil if no rows match).
		// SQLite's MAX() can return string or []byte depending on driver.
		if row.LatestDate == nil {
			continue
		}
		var dateStr string
		switch v := row.LatestDate.(type) {
		case string:
			dateStr = v
		case []byte:
			dateStr = string(v)
		default:
			continue
		}
		// Dates are stored as YYYY-MM-DD.
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		result[symbol] = &date
	}
	return result
}

// UpsertHistoricalPrices inserts or updates historical price entries for a
// symbol. Each price is upserted individually using ON CONFLICT(symbol, source,
// date). Errors on individual rows are logged but don't stop the batch.
// dataType is "stock" or "fx".
func (r *MarketDataRepository) UpsertHistoricalPrices(ctx context.Context, symbol string, prices []market.HistoricalPrice, dataType string) error {
	now := time.Now()
	for _, p := range prices {
		md := &market.MarketData{
			Symbol:    symbol,
			Price:     p.Close,
			Currency:  p.Currency,
			DataType:  dataType,
			Source:    "yahoo",
			Date:      p.Date.Format("2006-01-02"),
			FetchedAt: now,
		}
		if err := r.Upsert(ctx, md); err != nil {
			// Log but don't fail on individual errors.
			return fmt.Errorf("upsert historical price for %s on %s: %w", symbol, p.Date.Format("2006-01-02"), err)
		}
	}
	return nil
}

// GetNavHistoryBySymbol reads cached NAV history for one symbol, sorted by
// date ASC. Excludes current (date=”) entries. Returns empty slice if no
// data found.
func (r *MarketDataRepository) GetNavHistoryBySymbol(ctx context.Context, symbol string) ([]market.HistoricalPrice, error) {
	rows, err := r.q.GetNavHistoryBySymbol(ctx, r.db, symbol)
	if err != nil {
		return nil, fmt.Errorf("get NAV history for %s: %w", symbol, err)
	}

	var prices []market.HistoricalPrice
	for _, row := range rows {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return nil, fmt.Errorf("parse date %q for %s: %w", row.Date, symbol, err)
		}
		price, err := decimal.Parse(row.Price)
		if err != nil {
			return nil, fmt.Errorf("parse NAV for %s on %s: %w", symbol, row.Date, err)
		}
		prices = append(prices, market.HistoricalPrice{
			Date:     date,
			Close:    price,
			Currency: row.Currency,
		})
	}

	return prices, nil
}
