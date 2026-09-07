package performance

import (
	"context"
	"sort"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// priceLookupFF holds a forward-fill lookup for prices: symbol -> sorted dates + prices.
type priceLookupFF struct {
	dates  []string // sorted ascending
	prices map[string]market.HistoricalPrice
}

// buildPriceLookupFF builds a forward-fill price lookup from the price map.
func buildPriceLookupFF(lookup map[string]map[string]market.HistoricalPrice) map[string]*priceLookupFF {
	ff := make(map[string]*priceLookupFF)
	for symbol, dateMap := range lookup {
		dates := make([]string, 0, len(dateMap))
		for d := range dateMap {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		ff[symbol] = &priceLookupFF{dates: dates, prices: dateMap}
	}
	return ff
}

// lookupPrice retrieves a historical price for a symbol on or before a specific
// date. If the exact date is not found, it forward-fills from the nearest
// previous cached price. Returns false if no price exists before the given date.
func lookupPrice(ff map[string]*priceLookupFF, symbol, dateKey string) (market.HistoricalPrice, bool) {
	pf, ok := ff[symbol]
	if !ok {
		return market.HistoricalPrice{}, false
	}
	// Binary search for the largest date <= dateKey.
	idx := sort.SearchStrings(pf.dates, dateKey)
	// If exact match found.
	if idx < len(pf.dates) && pf.dates[idx] == dateKey {
		return pf.prices[dateKey], true
	}
	// Forward-fill from previous date.
	if idx > 0 {
		return pf.prices[pf.dates[idx-1]], true
	}
	return market.HistoricalPrice{}, false
}

// InterpolateDaily fills in non-transaction days by revaluing the
// point-in-time portfolio state (positions + cash from the last snapshot on
// or before each date) at that date's cached prices and FX rates, so the
// curve reflects daily market value movement between transactions. points
// must correspond 1:1 with snapshots (one point per snapshot date, in
// order). Extends through dateTo. Non-trading days fall out of the same
// mechanism: with no price that day, forward-fill carries the last trading
// day's value.
func InterpolateDaily(
	ctx context.Context,
	points []EquityCurvePoint,
	snapshots []dateSnapshot,
	dateTo time.Time,
	pricesBySymbol map[string][]market.HistoricalPrice,
	baseCurrency string,
	marketProvider MarketDataProvider,
) []EquityCurvePoint {
	if len(points) == 0 {
		return points
	}

	// Build a map of date -> point for quick lookup.
	pointMap := make(map[string]EquityCurvePoint, len(points))
	for _, p := range points {
		key := p.Date.Format("2006-01-02")
		pointMap[key] = p
	}

	// Build forward-fill lookups for price and FX lookups on extended dates.
	priceLookup := buildPriceLookup(pricesBySymbol)
	ff := buildPriceLookupFF(priceLookup)

	// Build FX forward-fill lookup for the extended date range. Pairs are
	// collected across all snapshots (not just the final state) because a
	// position or cash balance in a foreign currency may be gone by the end
	// but still needs conversion on intermediate dates.
	fxPairs := collectFxPairs(snapshots, baseCurrency)
	fxLookup := buildFxLookupForInterpolation(ctx, marketProvider, fxPairs, points, dateTo)

	dateFrom := points[0].Date

	var result []EquityCurvePoint
	var lastPoint *EquityCurvePoint
	snapIdx := -1

	for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if p, ok := pointMap[key]; ok {
			snapIdx++
			lastPoint = &p
			result = append(result, p)
			continue
		}
		if snapIdx < 0 || snapIdx >= len(snapshots) {
			continue
		}
		// Non-transaction day: revalue the point-in-time state (the last
		// snapshot on or before this date) at this date's prices and FX
		// rates. Forward-filled prices carry non-trading days forward from
		// the last trading day. Net deposit is reconverted at this date's
		// FX rate so that
		// profit = portfolio_value − net_deposit uses the same FX
		// convention for both sides (valuation-date FX).
		snap := snapshots[snapIdx] //nolint:gosec // snapIdx bounds-checked above
		portfolioValue := computePortfolioValue(snap.positions, snap.positionCurrency, snap.cashBalance, ff, fxLookup, baseCurrency, d)
		netDepBase := computeNetDeposit(snap.netDeposit, fxLookup, baseCurrency, d)
		result = append(result, EquityCurvePoint{
			Date:           d,
			PortfolioValue: portfolioValue,
			NetDeposit:     netDepBase,
			NavPerUnit:     lastPoint.NavPerUnit,
			Units:          lastPoint.Units,
		})
	}

	return result
}

// computePortfolioValue calculates the portfolio value for a given date using
// the current positions, cash balance, cached historical prices, and FX rates.
func computePortfolioValue(
	positions map[string]decimal.Decimal,
	positionCurrency map[string]string,
	cashBalance map[string]decimal.Decimal,
	ff map[string]*priceLookupFF,
	fxLookup map[string]*fxLookupFF,
	baseCurrency string,
	date time.Time,
) decimal.Decimal {
	var portfolioValue decimal.Decimal
	dateKey := date.Format("2006-01-02")

	// Cash balance.
	for currency, val := range cashBalance {
		if currency != baseCurrency {
			converted, ok := convertWithFxLookup(fxLookup, currency, baseCurrency, val, date)
			if !ok {
				continue // skip — can't include unconverted value
			}
			val = converted
		}
		portfolioValue, _ = portfolioValue.Add(val)
	}

	// Position values.
	for symbol, qty := range positions {
		if isCashSymbol(symbol) {
			continue
		}
		price, ok := lookupPrice(ff, symbol, dateKey)
		if !ok {
			continue
		}
		value, _ := qty.Mul(price.Close)
		if positionCurrency[symbol] != baseCurrency {
			converted, ok := convertWithFxLookup(fxLookup, positionCurrency[symbol], baseCurrency, value, date)
			if !ok {
				continue // skip — can't include unconverted value
			}
			value = converted
		}
		portfolioValue, _ = portfolioValue.Add(value)
	}

	return portfolioValue
}

// computeNetDeposit converts the cumulative net deposit (by currency) to
// base currency using the given date's FX rate. This ensures the equity
// curve's net deposit uses valuation-date FX, consistent with portfolio value.
func computeNetDeposit(
	netDeposit map[string]decimal.Decimal,
	fxLookup map[string]*fxLookupFF,
	baseCurrency string,
	date time.Time,
) decimal.Decimal {
	var netDepBase decimal.Decimal
	for currency, deposit := range netDeposit {
		if currency != baseCurrency {
			converted, ok := convertWithFxLookup(fxLookup, currency, baseCurrency, deposit, date)
			if !ok {
				continue // skip — can't include unconverted value
			}
			deposit = converted
		}
		netDepBase, _ = netDepBase.Add(deposit)
	}
	return netDepBase
}
