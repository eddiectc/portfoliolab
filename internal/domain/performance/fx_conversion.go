package performance

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// fxLookupFF holds a forward-fill lookup for FX rates: pair -> sorted dates + rates.
type fxLookupFF struct {
	dates []string
	rates map[string]decimal.Decimal
}

// collectFxPairs returns the unique FX pair symbols needed from the snapshots,
// excluding pairs where from == to (base currency).
func collectFxPairs(snapshots []dateSnapshot, baseCurrency string) []string {
	set := make(map[string]bool)
	for _, snap := range snapshots {
		for _, currency := range snap.positionCurrency {
			if currency != baseCurrency {
				set[market.FormatFxPair(currency, baseCurrency)] = true
			}
		}
		for currency := range snap.cashBalance {
			if currency != baseCurrency {
				set[market.FormatFxPair(currency, baseCurrency)] = true
			}
		}
		for currency := range snap.netDeposit {
			if currency != baseCurrency {
				set[market.FormatFxPair(currency, baseCurrency)] = true
			}
		}
	}
	pairs := make([]string, 0, len(set))
	for pair := range set {
		pairs = append(pairs, pair)
	}
	return pairs
}

// buildFxLookupFF fetches historical FX rates for the given pairs across the
// snapshot date range and returns a map of pair -> forward-fill lookup.
func buildFxLookupFF(
	ctx context.Context,
	marketProvider MarketDataProvider,
	fxPairs []string,
	snapshots []dateSnapshot,
	logger *slog.Logger,
) map[string]*fxLookupFF {
	lookup := make(map[string]*fxLookupFF)
	if marketProvider == nil || len(fxPairs) == 0 || len(snapshots) == 0 {
		return lookup
	}

	dateFrom := snapshots[0].date
	dateTo := snapshots[len(snapshots)-1].date

	for _, pair := range fxPairs {
		prices, err := marketProvider.GetHistoricalPrices(ctx, pair, dateFrom, dateTo)
		if err != nil {
			if logger != nil {
				logger.Warn("failed to read cached FX rates", "pair", pair, "error", err)
			}
			continue
		}
		if len(prices) == 0 {
			if logger != nil {
				logger.Debug("no cached FX rates", "pair", pair)
			}
			continue
		}

		dates := make([]string, len(prices))
		rates := make(map[string]decimal.Decimal, len(prices))
		for i, p := range prices {
			key := p.Date.Format("2006-01-02")
			dates[i] = key
			rates[key] = p.Close
		}
		sort.Strings(dates)
		lookup[pair] = &fxLookupFF{
			dates: dates,
			rates: rates,
		}
	}
	return lookup
}

// convertWithFxLookup converts a value using the FX rate lookup.
// Uses exact match when available, forward-fill (last known rate) for dates
// after the data, and backward-fill (first known rate) for dates before it.
// Returns (0, false) if no FX data exists — never returns the unconverted
// value, so callers who blindly add the result won't pollute totals with
// numerically wrong figures (e.g. USD treated as GBP).
func convertWithFxLookup(
	lookup map[string]*fxLookupFF,
	fromCurrency, toCurrency string,
	value decimal.Decimal,
	date time.Time,
) (decimal.Decimal, bool) {
	if fromCurrency == toCurrency {
		return value, true
	}
	pair := market.FormatFxPair(fromCurrency, toCurrency)
	ff, ok := lookup[pair]
	if !ok || len(ff.dates) == 0 {
		return decimal.Zero, false // no FX data — return 0, NOT unconverted value
	}
	dateKey := date.Format("2006-01-02")
	idx := sort.SearchStrings(ff.dates, dateKey)
	// Exact match.
	if idx < len(ff.dates) && ff.dates[idx] == dateKey {
		rate := ff.rates[dateKey]
		converted, _ := value.Mul(rate)
		return converted, true
	}
	// Forward-fill from previous date (target is after known data).
	if idx > 0 {
		rate := ff.rates[ff.dates[idx-1]]
		converted, _ := value.Mul(rate)
		return converted, true
	}
	// Backward-fill from first known date (target is before known data).
	// This handles dates before the first cached FX rate. Better than
	// using the raw unconverted value, which would be numerically wrong.
	rate := ff.rates[ff.dates[0]]
	converted, _ := value.Mul(rate)
	return converted, true
}

// convertToBase converts a value from one currency to another using the
// market data service. If currencies match, returns the value unchanged.
// If no FX rate is available, returns (0, false) — never the unconverted value.
func ConvertToBase(
	ctx context.Context,
	marketProvider MarketDataProvider,
	fromCurrency, toCurrency string,
	value decimal.Decimal,
	date time.Time,
) (decimal.Decimal, bool) {
	if fromCurrency == toCurrency {
		return value, true
	}

	if marketProvider == nil {
		return decimal.Zero, false
	}

	rate, _ := marketProvider.GetHistoricalFxRate(ctx, fromCurrency, toCurrency, date)
	if rate == nil {
		return decimal.Zero, false
	}

	converted, _ := value.Mul(rate.Rate)
	return converted, true
}

// buildFxLookupForInterpolation fetches FX rates for the interpolation date
// range (first point to dateTo) and builds a forward-fill lookup.
func buildFxLookupForInterpolation(
	ctx context.Context,
	marketProvider MarketDataProvider,
	fxPairs []string,
	points []EquityCurvePoint,
	dateTo time.Time,
) map[string]*fxLookupFF {
	lookup := make(map[string]*fxLookupFF)
	if marketProvider == nil || len(fxPairs) == 0 || len(points) == 0 {
		return lookup
	}

	dateFrom := points[0].Date

	for _, pair := range fxPairs {
		prices, err := marketProvider.GetHistoricalPrices(ctx, pair, dateFrom, dateTo)
		if err != nil || len(prices) == 0 {
			continue
		}
		dates := make([]string, len(prices))
		rates := make(map[string]decimal.Decimal, len(prices))
		for i, p := range prices {
			key := p.Date.Format("2006-01-02")
			dates[i] = key
			rates[key] = p.Close
		}
		sort.Strings(dates)
		lookup[pair] = &fxLookupFF{
			dates: dates,
			rates: rates,
		}
	}
	return lookup
}
