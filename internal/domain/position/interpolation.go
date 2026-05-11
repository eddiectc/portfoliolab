package position

import (
	"context"
	"sort"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
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

// interpolateDaily fills in non-transaction days by carrying forward the
// last known portfolio value and net deposit. Extends through dateTo using
// cached historical prices for the current positions, so the curve reflects
// actual price changes after the last transaction.
func interpolateDaily(
	points []EquityCurvePoint,
	dateTo time.Time,
	positions map[string]decimal.Decimal,
	positionCurrency map[string]string,
	cashBalance map[string]decimal.Decimal,
	pricesBySymbol map[string][]market.HistoricalPrice,
	baseCurrency string,
	marketService MarketDataService,
	ctx context.Context,
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

	// Build FX forward-fill lookup for the extended date range.
	fxPairs := collectFxPairsForInterpolation(positionCurrency, cashBalance, baseCurrency)
	fxLookup := buildFxLookupForInterpolation(ctx, marketService, fxPairs, points, dateTo)

	dateFrom := points[0].Date
	lastPointDate := points[len(points)-1].Date

	var result []EquityCurvePoint
	var lastPoint *EquityCurvePoint
	var lastNetDeposit decimal.Decimal

	for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if p, ok := pointMap[key]; ok {
			lastPoint = &p
			lastNetDeposit = p.NetDeposit
			result = append(result, p)
		} else if lastPoint != nil && !d.After(lastPointDate) {
			// Between transaction dates: carry forward last known value.
			result = append(result, EquityCurvePoint{
				Date:           d,
				PortfolioValue: lastPoint.PortfolioValue,
				NetDeposit:     lastNetDeposit,
			})
		} else if lastPoint != nil {
			// Beyond last transaction: compute portfolio value from
			// current positions + cash + cached historical prices.
			portfolioValue := computePortfolioValue(positions, positionCurrency, cashBalance, ff, fxLookup, baseCurrency, d)
			result = append(result, EquityCurvePoint{
				Date:           d,
				PortfolioValue: portfolioValue,
				NetDeposit:     lastNetDeposit,
			})
		}
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
		if isCashPosition(symbol) {
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
