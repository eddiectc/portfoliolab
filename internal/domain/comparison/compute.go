package comparison

import (
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// ComputeMWR computes the simple holding-period return for a benchmark
// from its historical prices: (end/start - 1) * 100.
//
// For an index or ETF with no cash flows, MWR equals the simple return.
// Returns nil if fewer than 2 prices or start price is non-positive.
func ComputeMWR(prices []market.HistoricalPrice) *decimal.Decimal {
	if len(prices) < 2 {
		return nil
	}

	start := prices[0].Close
	if !start.IsPos() {
		return nil
	}

	end := prices[len(prices)-1].Close

	startF, _ := start.Float64()
	endF, _ := end.Float64()

	ratio := endF / startF
	returnPct, _ := decimal.NewFromFloat64((ratio - 1.0) * 100.0)
	result := returnPct.Round(2)
	return &result
}

// ComputeMWRForPeriod filters prices to the [from, to] window and computes
// the holding-period return for that period.
// Returns nil if fewer than 2 prices fall within the window or start price
// is non-positive.
func ComputeMWRForPeriod(prices []market.HistoricalPrice, from, to time.Time) *decimal.Decimal {
	filtered := filterPrices(prices, from, to)
	return ComputeMWR(filtered)
}

// ComputeMonthlyReturns groups prices by year-month and computes the monthly
// return percentage for each month. Returns a map of "YYYY-MM" -> return %.
//
// For each month, the return is (last_close / first_close - 1) * 100.
// Months with fewer than 2 price points are omitted.
func ComputeMonthlyReturns(prices []market.HistoricalPrice) map[string]*decimal.Decimal {
	if len(prices) < 2 {
		return nil
	}

	// Group prices by year-month.
	groups := make(map[string][]decimal.Decimal)
	for _, p := range prices {
		key := p.Date.Format("2006-01")
		groups[key] = append(groups[key], p.Close)
	}

	result := make(map[string]*decimal.Decimal)
	for key, closes := range groups {
		if len(closes) < 2 {
			continue
		}
		first := closes[0]
		last := closes[len(closes)-1]
		if !first.IsPos() {
			continue
		}

		firstF, _ := first.Float64()
		lastF, _ := last.Float64()
		ratio := lastF / firstF
		returnPct, _ := decimal.NewFromFloat64((ratio - 1.0) * 100.0)
		val := returnPct.Round(2)
		result[key] = &val
	}

	return result
}

// filterPrices returns prices within [from, to] inclusive.
func filterPrices(prices []market.HistoricalPrice, from, to time.Time) []market.HistoricalPrice {
	var filtered []market.HistoricalPrice
	for _, p := range prices {
		if (p.Date.Equal(from) || p.Date.After(from)) && (p.Date.Equal(to) || p.Date.Before(to)) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
