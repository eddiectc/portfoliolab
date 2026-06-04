package efficientfrontier

import (
	"math"
	"sort"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

const (
	// tradingDaysPerYear is the standard number of trading days for annualization.
	tradingDaysPerYear = 252
)

// ComputeReturns computes daily returns from a series of historical close prices.
// Returns are computed as simple returns: (close[t] / close[t-1]) - 1.
// The input series is sorted by date ascending.
// A series of N prices produces N-1 returns.
func ComputeReturns(prices []market.HistoricalPrice) ([]float64, error) {
	if len(prices) < 2 {
		return nil, ErrInsufficientData
	}

	// Copy and sort to avoid mutating caller's slice.
	sorted := make([]market.HistoricalPrice, len(prices))
	copy(sorted, prices)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	rets := make([]float64, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		prev, ok1 := sorted[i-1].Close.Float64()
		curr, ok2 := sorted[i].Close.Float64()
		if !ok1 || !ok2 || prev == 0 {
			continue
		}
		rets = append(rets, (curr/prev)-1.0)
	}

	if len(rets) == 0 {
		return nil, ErrInsufficientData
	}
	return rets, nil
}

// ComputeAnnualizedReturn computes the annualized return from daily returns.
// Uses the compound annual growth rate (CAGR) formula:
//
//	(1 + r1) * (1 + r2) * ... * (1 + rn) ^ (252/n) - 1
func ComputeAnnualizedReturn(returns []float64, tradingDays int) float64 {
	if len(returns) == 0 {
		return 0
	}
	if tradingDays <= 0 {
		tradingDays = tradingDaysPerYear
	}

	// Compound all returns.
	compound := 1.0
	for _, r := range returns {
		compound *= (1 + r)
	}

	if compound <= 0 {
		return 0
	}

	// Annualize.
	annualized := math.Pow(compound, float64(tradingDays)/float64(len(returns))) - 1
	return annualized * 100.0 // return as percentage
}

// ComputeAnnualizedVolatility computes the annualized volatility from daily returns.
// Uses sample standard deviation of daily returns, annualized by sqrt(252/n).
func ComputeAnnualizedVolatility(returns []float64, tradingDays int) float64 {
	n := len(returns)
	if n < 2 {
		return 0
	}
	if tradingDays <= 0 {
		tradingDays = tradingDaysPerYear
	}

	// Compute mean daily return.
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(n)

	// Compute sample variance.
	variance := 0.0
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(n - 1) // sample variance (n-1)

	// Annualize: daily stddev * sqrt(252 / n_observed).
	dailyStddev := math.Sqrt(variance)
	annualized := dailyStddev * math.Sqrt(float64(tradingDays)/float64(n))
	return annualized * 100.0 // return as percentage
}
