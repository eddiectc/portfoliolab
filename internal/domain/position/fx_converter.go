package position

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// FxRateProvider abstracts retrieval of FX rates, handling the
// historical → on-demand fetch → current spot fallback chain.
type FxRateProvider interface {
	// GetRateForDate returns the FX rate for converting baseCurrency to
	// quoteCurrency on or near the given date. It checks the database for
	// historical rates first, then falls back to fetching from the market
	// data provider and caching, and finally falls back to the current spot rate.
	// The second return value indicates whether a historical rate was found
	// (false means a fallback was used).
	GetRateForDate(ctx context.Context, baseCurrency, quoteCurrency string, date time.Time) (*market.FxRate, bool)

	// GetCurrentRate returns the current spot FX rate for converting
	// baseCurrency to quoteCurrency, checking the database cache first
	// before fetching. Returns nil if no rate is available.
	GetCurrentRate(ctx context.Context, baseCurrency, quoteCurrency string) (*market.FxRate, bool)
}

// FxConverter implements FxRateProvider by combining a market data repository
// (DB cache), an FX rate fetcher (on-demand fetch), and a logger.
type FxConverter struct {
	repo    MarketDataRepository
	fetcher market.FxRateFetcher
	logger  *slog.Logger
}

// MarketDataRepository defines the subset of market data operations
// needed by the FX converter and performance layer.
type MarketDataRepository interface {
	GetLatest(ctx context.Context, symbol string) (*market.MarketData, error)
	GetBySourceAndDate(ctx context.Context, symbol, source, date string) (*market.MarketData, error)
	Upsert(ctx context.Context, m *market.MarketData) error
	GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*market.MarketData, error)
	UpsertHistoricalPrices(ctx context.Context, symbol string, prices []market.HistoricalPrice) error
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
func (c *FxConverter) GetRateForDate(ctx context.Context, baseCurrency, quoteCurrency string, date time.Time) (*market.FxRate, bool) {
	pair := market.FormatFxPair(baseCurrency, quoteCurrency)
	dateStr := date.Format("2006-01-02")

	// Step 1: Check DB for historical rate on that date.
	md, err := c.repo.GetBySourceAndDate(ctx, pair, "yahoo", dateStr)
	if err != nil {
		c.logWarn("failed to get historical FX rate from DB", "base", baseCurrency, "quote", quoteCurrency, "date", dateStr, "error", err)
	}
	if md != nil {
		return c.toFxRate(baseCurrency, quoteCurrency, md), true
	}

	// Step 2: Try to fetch current rate from provider and cache it as historical.
	if rate, err := c.fetcher.FetchRate(ctx, baseCurrency, quoteCurrency); err == nil {
		// Cache the fetched rate as a historical snapshot for this date.
		cached := &market.MarketData{
			Symbol:    pair,
			Price:     rate.Rate,
			Currency:  quoteCurrency,
			DataType:  "fx",
			Source:    "yahoo",
			Date:      dateStr,
			FetchedAt: rate.FetchedAt,
		}
		if err := c.repo.Upsert(ctx, cached); err != nil {
			c.logWarn("failed to cache FX rate", "base", baseCurrency, "quote", quoteCurrency, "date", dateStr, "error", err)
		}
		return rate, false // fetched on-demand, not historical
	} else {
		c.logWarn("failed to fetch FX rate", "base", baseCurrency, "quote", quoteCurrency, "error", err)
	}

	// Step 3: Fall back to current spot rate from DB.
	spot, err := c.repo.GetCurrentFxRate(ctx, baseCurrency, quoteCurrency)
	if err != nil {
		c.logWarn("failed to get current FX rate from DB", "base", baseCurrency, "quote", quoteCurrency, "error", err)
	}
	if spot != nil {
		return c.toFxRate(baseCurrency, quoteCurrency, spot), false
	}

	// No rate available at all.
	c.logWarn("no FX rate available", "base", baseCurrency, "quote", quoteCurrency, "date", dateStr)
	return nil, false
}

// GetCurrentRate returns the current spot FX rate.
// Checks the DB cache first, then fetches from provider.
// The second return value is true if a rate was found.
func (c *FxConverter) GetCurrentRate(ctx context.Context, baseCurrency, quoteCurrency string) (*market.FxRate, bool) {
	// Check DB cache first.
	spot, err := c.repo.GetCurrentFxRate(ctx, baseCurrency, quoteCurrency)
	if err != nil {
		c.logWarn("failed to get current FX rate from DB", "base", baseCurrency, "quote", quoteCurrency, "error", err)
	}
	if spot != nil {
		return c.toFxRate(baseCurrency, quoteCurrency, spot), true
	}

	// Fetch from provider.
	if rate, err := c.fetcher.FetchRate(ctx, baseCurrency, quoteCurrency); err == nil {
		// Cache the fetched rate.
		pair := market.FormatFxPair(baseCurrency, quoteCurrency)
		cached := &market.MarketData{
			Symbol:    pair,
			Price:     rate.Rate,
			Currency:  quoteCurrency,
			DataType:  "fx",
			Source:    "yahoo",
			Date:      "", // current
			FetchedAt: rate.FetchedAt,
		}
		if err := c.repo.Upsert(ctx, cached); err != nil {
			c.logWarn("failed to cache current FX rate", "base", baseCurrency, "quote", quoteCurrency, "error", err)
		}
		return rate, true
	}

	c.logWarn("no current FX rate available", "base", baseCurrency, "quote", quoteCurrency)
	return nil, false
}

// logWarn logs a warning message if the logger is non-nil.
func (c *FxConverter) logWarn(msg string, args ...any) {
	if c.logger != nil {
		c.logger.Warn(msg, args...)
	}
}

// toFxRate converts a MarketData entry to an FxRate.
func (c *FxConverter) toFxRate(baseCurrency, quoteCurrency string, md *market.MarketData) *market.FxRate {
	return &market.FxRate{
		BaseCurrency:  baseCurrency,
		QuoteCurrency: quoteCurrency,
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

// FxRateDisplay holds the convention-rate pair label and value for display.
type FxRateDisplay struct {
	Pair string          // e.g. "GBP/USD"
	Rate decimal.Decimal // convention rate (e.g. 1.3 for GBP/USD)
}

// ConventionFxRate returns the FX rate displayed in standard market convention.
//
// Market convention: for pairs involving USD and a "major" currency
// (GBP, EUR, AUD, NZD, CAD), USD is the quote currency.
// E.g. position=USD, base=GBP → shows "GBP/USD 1.3000" (not "USD/GBP 0.7692").
//
// If the stored rate was fetched for the "inverted" pair (position/base),
// the rate is inverted (1/rate) to show the convention rate.
// If the stored rate is already in convention order, it is returned as-is.
func ConventionFxRate(positionCurrency, baseCurrency string, storedRate *decimal.Decimal) *FxRateDisplay {
	if positionCurrency == baseCurrency || storedRate == nil || storedRate.Equal(decimal.Zero) {
		return nil
	}

	conventionPair, inverted := conventionPairOrder(positionCurrency, baseCurrency)

	var rate decimal.Decimal
	if inverted {
		// storedRate is position/base, convention is base/position → invert
		rate, _ = decimal.One.Quo(*storedRate)
	} else {
		// storedRate is already in convention order
		rate = *storedRate
	}

	return &FxRateDisplay{
		Pair: conventionPair,
		Rate: rate,
	}
}

// conventionPairOrder returns the standard market-convention pair string and
// whether the position/base order is inverted relative to convention.
//
// Convention rules:
//   - USD vs GBP/EUR/AUD/NZD/CAD → USD is quote (e.g. GBP/USD, EUR/USD)
//   - EUR vs GBP → EUR is base (e.g. EUR/GBP)
//   - otherwise → first currency is base (position/base)
func conventionPairOrder(currencyA, currencyB string) (string, bool) {
	// Currencies where USD is conventionally the quote currency.
	usdMajors := map[string]bool{
		"GBP": true, "EUR": true, "AUD": true, "NZD": true, "CAD": true,
	}

	if currencyA == "USD" && usdMajors[currencyB] {
		// Convention: GBP/USD (USD is quote). Position/base was USD/GBP → inverted.
		return fmt.Sprintf("%s/%s", currencyB, currencyA), true
	}
	if currencyB == "USD" && usdMajors[currencyA] {
		// Convention: GBP/USD (USD is quote). Position/base was GBP/USD → not inverted.
		return fmt.Sprintf("%s/%s", currencyA, currencyB), false
	}

	// EUR/GBP convention: EUR is base.
	if currencyA == "EUR" && currencyB == "GBP" {
		return "EUR/GBP", false
	}
	if currencyA == "GBP" && currencyB == "EUR" {
		return "EUR/GBP", true
	}

	// Default: position/base order.
	return fmt.Sprintf("%s/%s", currencyA, currencyB), false
}
