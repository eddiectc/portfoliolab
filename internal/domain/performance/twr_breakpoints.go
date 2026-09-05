package performance

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// twrBreakpoint holds the portfolio value just before a cash flow,
// used as a breakpoint for TWR sub-period return computation.
type twrBreakpoint struct {
	date  time.Time
	value decimal.Decimal // portfolio value in base currency
}

// computePreCashFlowValues computes the portfolio value just before each
// cash flow (deposit/withdrawal), using the same price/FX logic as the
// equity curve. Returns a sorted slice of TWR breakpoints.
func computePreCashFlowValues(
	snapshots []dateSnapshot,
	pricesBySymbol map[string][]market.HistoricalPrice,
	baseCurrency string,
	marketProvider MarketDataProvider,
	logger *slog.Logger,
	ctx context.Context,
) []twrBreakpoint {
	if len(snapshots) == 0 {
		return nil
	}

	// Build price and FX lookups (same as equity curve).
	priceLookup := buildPriceLookup(pricesBySymbol)
	priceFF := buildPriceLookupFF(priceLookup)

	// Collect FX pairs from all pre-cash-flow snapshots.
	allPreSnaps := collectAllPreCashFlowSnaps(snapshots)
	fxPairs := collectFxPairsFromPreSnaps(allPreSnaps, baseCurrency)
	fxLookup := buildFxLookupFromPreSnaps(ctx, marketProvider, fxPairs, allPreSnaps, logger)

	var values []twrBreakpoint
	for _, snap := range snapshots {
		for _, pre := range snap.preCashFlowSnapshots {
			value := computePreCashFlowPortfolioValue(pre, priceFF, fxLookup, baseCurrency, logger)
			values = append(values, twrBreakpoint{
				date:  pre.date,
				value: value,
			})
		}
	}
	return values
}

// preCashFlowSnapWithDate wraps a pre-cash-flow snapshot with its date
// for FX lookup range determination.
type preCashFlowSnapWithDate struct {
	pre  preCashFlowSnapshot
	date time.Time // the snapshot date (for FX lookup range)
}

// collectAllPreCashFlowSnaps gathers all pre-cash-flow snapshots across dates.
func collectAllPreCashFlowSnaps(snapshots []dateSnapshot) []preCashFlowSnapWithDate {
	var all []preCashFlowSnapWithDate
	for _, snap := range snapshots {
		for _, pre := range snap.preCashFlowSnapshots {
			all = append(all, preCashFlowSnapWithDate{pre: pre, date: snap.date})
		}
	}
	return all
}

// collectFxPairsFromPreSnaps returns unique FX pairs needed for pre-cash-flow
// snapshots, excluding pairs where from == to (base currency).
func collectFxPairsFromPreSnaps(snaps []preCashFlowSnapWithDate, baseCurrency string) []string {
	set := make(map[string]bool)
	for _, sw := range snaps {
		for _, currency := range sw.pre.positionCurrency {
			if currency != baseCurrency {
				set[market.FormatFxPair(currency, baseCurrency)] = true
			}
		}
		for currency := range sw.pre.cashBalance {
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

// buildFxLookupFromPreSnaps fetches historical FX rates for the given pairs
// across the pre-cash-flow snapshot date range.
func buildFxLookupFromPreSnaps(
	ctx context.Context,
	marketProvider MarketDataProvider,
	fxPairs []string,
	snaps []preCashFlowSnapWithDate,
	logger *slog.Logger,
) map[string]*fxLookupFF {
	lookup := make(map[string]*fxLookupFF)
	if marketProvider == nil || len(fxPairs) == 0 || len(snaps) == 0 {
		return lookup
	}

	dateFrom := snaps[0].date
	dateTo := snaps[len(snaps)-1].date

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
		lookup[pair] = &fxLookupFF{dates: dates, rates: rates}
	}
	return lookup
}

// computePreCashFlowPortfolioValue computes the portfolio value (positions + cash)
// for a pre-cash-flow snapshot, converting to the base currency.
func computePreCashFlowPortfolioValue(
	pre preCashFlowSnapshot,
	priceFF map[string]*priceLookupFF,
	fxLookup map[string]*fxLookupFF,
	baseCurrency string,
	logger *slog.Logger,
) decimal.Decimal {
	var portfolioValue decimal.Decimal
	dateKey := pre.date.Format("2006-01-02")

	// Position values.
	for symbol, qty := range pre.positions {
		price, found := lookupPrice(priceFF, symbol, dateKey)
		if !found {
			if logger != nil {
				logger.Debug("performance: no price for pre-cash-flow position",
					"symbol", symbol, "qty", qty.String(), "date", dateKey)
			}
			continue
		}
		value, _ := qty.Mul(price.Close)
		if price.Currency != baseCurrency {
			var ok bool
			value, ok = convertWithFxLookup(fxLookup, price.Currency, baseCurrency, value, pre.date)
			if !ok {
				if logger != nil {
					logger.Warn("missing FX rate, skipping pre-cash-flow position",
						"pair", market.FormatFxPair(price.Currency, baseCurrency),
						"symbol", symbol, "date", dateKey)
				}
				continue // skip — can't include unconverted value
			}
		}
		portfolioValue, _ = portfolioValue.Add(value)
	}

	// Cash balances.
	for currency, balance := range pre.cashBalance {
		if currency != baseCurrency {
			var ok bool
			balance, ok = convertWithFxLookup(fxLookup, currency, baseCurrency, balance, pre.date)
			if !ok {
				if logger != nil {
					logger.Warn("missing FX rate, skipping pre-cash-flow cash",
						"pair", market.FormatFxPair(currency, baseCurrency),
						"date", pre.date.Format("2006-01-02"))
				}
				continue // skip — can't include unconverted value
			}
		}
		portfolioValue, _ = portfolioValue.Add(balance)
	}

	return portfolioValue
}
