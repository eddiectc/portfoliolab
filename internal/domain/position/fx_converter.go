package position

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// FxRateProvider abstracts retrieval of FX rates, handling the
// historical → on-demand fetch → current spot fallback chain.
type FxRateProvider interface {
	// GetRateForDate returns the FX rate for a currency pair on or near the
	// given date. It checks the database for historical rates first, then
	// falls back to fetching from the market data provider and caching,
	// and finally falls back to the current spot rate.
	// The second return value indicates whether a historical rate was found
	// (false means a fallback was used).
	GetRateForDate(ctx context.Context, pair string, date time.Time) (*market.FxRate, bool)

	// GetCurrentRate returns the current spot FX rate for a pair,
	// checking the database cache first before fetching.
	// Returns nil if no rate is available.
	GetCurrentRate(ctx context.Context, pair string) (*market.FxRate, bool)
}

// FxConverter implements FxRateProvider by combining a market data repository
// (DB cache), an FX rate fetcher (on-demand fetch), and a logger.
type FxConverter struct {
	repo   MarketDataRepository
	fetcher market.FxRateFetcher
	logger  *slog.Logger
}

// MarketDataRepository defines the subset of market data operations
// needed by the FX converter.
type MarketDataRepository interface {
	GetLatest(ctx context.Context, symbol string) (*market.MarketData, error)
	GetBySourceAndDate(ctx context.Context, symbol, source, date string) (*market.MarketData, error)
	Upsert(ctx context.Context, m *market.MarketData) error
	GetCurrentFxRate(ctx context.Context, pair string) (*market.MarketData, error)
}

// NewFxConverter creates a new FxConverter.
func NewFxConverter(repo MarketDataRepository, fetcher market.FxRateFetcher, logger *slog.Logger) *FxConverter {
	return &FxConverter{
		repo:    repo,
		fetcher: fetcher,
		logger:  logger,
	}
}

// GetRateForDate returns the FX rate for a currency pair on or near the given date.
// It checks the database for historical rates first, then falls back to fetching
// from the market data provider, and finally falls back to the current spot rate.
// The second return value is true if a historical (non-fallback) rate was found.
func (c *FxConverter) GetRateForDate(ctx context.Context, pair string, date time.Time) (*market.FxRate, bool) {
	dateStr := date.Format("2006-01-02")

	// Step 1: Check DB for historical rate on that date.
	md, err := c.repo.GetBySourceAndDate(ctx, pair, "yahoo", dateStr)
	if err != nil {
		c.logWarn("failed to get historical FX rate from DB", "pair", pair, "date", dateStr, "error", err)
	}
	if md != nil {
		return c.toFxRate(pair, md), true
	}

	// Step 2: Try to fetch current rate from provider and cache it as historical.
	if rate, err := c.fetcher.FetchRate(ctx, pair); err == nil {
		// Cache the fetched rate as a historical snapshot for this date.
		cached := &market.MarketData{
			Symbol:    pair,
			Price:     rate.Rate,
			Currency:  rate.QuoteCurrency,
			DataType:  "fx",
			Source:    "yahoo",
			Date:      dateStr,
			FetchedAt: rate.FetchedAt,
		}
		if err := c.repo.Upsert(ctx, cached); err != nil {
			c.logWarn("failed to cache FX rate", "pair", pair, "date", dateStr, "error", err)
		}
		return rate, false // fetched on-demand, not historical
	} else {
		c.logWarn("failed to fetch FX rate", "pair", pair, "error", err)
	}

	// Step 3: Fall back to current spot rate from DB.
	spot, err := c.repo.GetCurrentFxRate(ctx, pair)
	if err != nil {
		c.logWarn("failed to get current FX rate from DB", "pair", pair, "error", err)
	}
	if spot != nil {
		return c.toFxRate(pair, spot), false
	}

	// No rate available at all.
	c.logWarn("no FX rate available", "pair", pair, "date", dateStr)
	return nil, false
}

// GetCurrentRate returns the current spot FX rate for a pair.
// Checks the DB cache first, then fetches from provider.
// The second return value is true if a rate was found.
func (c *FxConverter) GetCurrentRate(ctx context.Context, pair string) (*market.FxRate, bool) {
	// Check DB cache first.
	spot, err := c.repo.GetCurrentFxRate(ctx, pair)
	if err != nil {
		c.logWarn("failed to get current FX rate from DB", "pair", pair, "error", err)
	}
	if spot != nil {
		return c.toFxRate(pair, spot), true
	}

	// Fetch from provider.
	if rate, err := c.fetcher.FetchRate(ctx, pair); err == nil {
		// Cache the fetched rate.
		cached := &market.MarketData{
			Symbol:    pair,
			Price:     rate.Rate,
			Currency:  rate.QuoteCurrency,
			DataType:  "fx",
			Source:    "yahoo",
			Date:      "", // current
			FetchedAt: rate.FetchedAt,
		}
		if err := c.repo.Upsert(ctx, cached); err != nil {
			c.logWarn("failed to cache current FX rate", "pair", pair, "error", err)
		}
		return rate, true
	}

	c.logWarn("no current FX rate available", "pair", pair)
	return nil, false
}

// logWarn logs a warning message if the logger is non-nil.
func (c *FxConverter) logWarn(msg string, args ...any) {
	if c.logger != nil {
		c.logger.Warn(msg, args...)
	}
}

// toFxRate converts a MarketData entry to an FxRate.
func (c *FxConverter) toFxRate(pair string, md *market.MarketData) *market.FxRate {
	base, quote, err := market.ParseFxPair(pair)
	if err != nil {
		// Fallback: use the symbol itself.
		return &market.FxRate{
			Pair:          pair,
			BaseCurrency:  pair,
			QuoteCurrency: md.Currency,
			Rate:          md.Price,
			FetchedAt:     md.FetchedAt,
		}
	}
	return &market.FxRate{
		Pair:          pair,
		BaseCurrency:  base,
		QuoteCurrency: quote,
		Rate:          md.Price,
		FetchedAt:     md.FetchedAt,
	}
}

// ConvertPnlToBase converts a realized P&L from the position currency to the
// portfolio base currency using the given FX rate. If the currencies match,
// the P&L is returned unchanged with no rate applied.
//
// Returns (convertedPnL, rateUsed, isFallback).
// - If position currency == base currency: returns (pnl, nil, false)
// - If FX rate available: returns (pnl * rate, rate, isFallback)
// - If no FX rate: returns (pnl, nil, true) — P&L unchanged, marked as fallback
func ConvertPnlToBase(pnl decimal.Decimal, positionCurrency, baseCurrency string,
	rate *market.FxRate, isFallback bool) (decimal.Decimal, *decimal.Decimal, bool) {

	// Same currency — no conversion needed.
	if positionCurrency == baseCurrency {
		return pnl, nil, false
	}

	// No rate available — return original P&L as fallback.
	if rate == nil {
		return pnl, nil, true
	}

	// Convert: pnl is in positionCurrency, rate is positionCurrency/baseCurrency.
	// pnl_in_base = pnl * rate.
	converted, err := pnl.Mul(rate.Rate)
	if err != nil {
		// On decimal error, return original as fallback.
		return pnl, &rate.Rate, true
	}

	return converted, &rate.Rate, isFallback
}

// BuildFxPair constructs the FX pair string from position currency and base
// currency. E.g., positionCurrency="GBP", baseCurrency="USD" → "GBP/USD".
func BuildFxPair(positionCurrency, baseCurrency string) string {
	if positionCurrency == baseCurrency {
		return ""
	}
	return fmt.Sprintf("%s/%s", positionCurrency, baseCurrency)
}
