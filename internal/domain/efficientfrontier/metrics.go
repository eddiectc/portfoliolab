package efficientfrontier

import (
	"math"
	"sort"

	"github.com/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/eddiectc/portfoliolab/internal/market"
)

// alignReturnsBySymbol aligns daily returns across all symbols by date.
// Returns a slice of observations (each observation is one date), where each
// inner slice has one return per symbol in the order of the sorted symbol names.
// Only dates where ALL symbols have data are included.
func alignReturnsBySymbol(pricesBySymbol map[string][]market.HistoricalPrice, symbols []string) [][]float64 {
	// Build date → [symbolIndex → return] map.
	dateMap := make(map[int64][]float64)
	symbolIndex := make(map[string]int, len(symbols))
	for i, s := range symbols {
		symbolIndex[s] = i
	}
	n := len(symbols)

	for _, sym := range symbols {
		prices := pricesBySymbol[sym]
		if len(prices) < 2 {
			return nil
		}
		// Sort by date.
		sorted := make([]market.HistoricalPrice, len(prices))
		copy(sorted, prices)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Date.Before(sorted[j].Date)
		})
		for i := 1; i < len(sorted); i++ {
			prev, ok1 := sorted[i-1].Close.Float64()
			curr, ok2 := sorted[i].Close.Float64()
			if !ok1 || !ok2 || prev == 0 {
				continue
			}
			date := sorted[i].Date.Unix()
			ret := (curr / prev) - 1.0
			entry, ok := dateMap[date]
			if !ok {
				entry = make([]float64, n)
				for k := range entry {
					entry[k] = math.NaN()
				}
			}
			entry[symbolIndex[sym]] = ret
			dateMap[date] = entry
		}
	}

	// Collect only complete dates.
	var aligned [][]float64
	for _, entry := range dateMap {
		if len(entry) != n {
			continue
		}
		allPresent := true
		for _, v := range entry {
			if math.IsNaN(v) {
				allPresent = false
				break
			}
		}
		if allPresent {
			row := make([]float64, n)
			copy(row, entry)
			aligned = append(aligned, row)
		}
	}

	return aligned
}

// computePortfolioDailyReturns computes the daily returns of a portfolio
// given aligned daily returns (each row is one date, each column is one asset)
// and portfolio weights.
func computePortfolioDailyReturns(aligned [][]float64, weights []float64) []float64 {
	n := len(weights)
	rets := make([]float64, len(aligned))
	for i, row := range aligned {
		ret := 0.0
		for j := 0; j < n; j++ {
			ret += weights[j] * row[j]
		}
		rets[i] = ret
	}
	return rets
}

// ComputePortfolioSortino computes the Sortino ratio for a portfolio
// given its annualized return, aligned daily returns, weights, and
// annualized risk-free rate.
//
// The downside deviation is computed from the portfolio's daily returns
// (weighted combination) relative to the daily risk-free rate, then
// annualized via the shared stats.DownsideDeviation function.
func ComputePortfolioSortino(
	annualizedReturn float64,
	aligned [][]float64,
	weights []float64,
	annualizedRiskFree float64,
) float64 {
	dailyRiskFree := annualizedRiskFree / float64(stats.TradingDaysPerYear)
	portDailyRet := computePortfolioDailyReturns(aligned, weights)
	downsideDev := stats.DownsideDeviation(portDailyRet, dailyRiskFree)
	if downsideDev <= 0 {
		return 0
	}
	return stats.SortinoRatio(annualizedReturn, downsideDev, annualizedRiskFree)
}

// ComputeMaxDrawdown computes the maximum drawdown from a series of
// daily returns (ratios, e.g. 0.01 = 1%).
//
// It constructs an equity curve: equity[0] = 1.0, equity[t] = equity[t-1] * (1 + ret[t]).
// Then finds the largest peak-to-trough decline.
// Returns the max drawdown as a negative ratio (e.g. -0.15 = 15% decline).
// Returns 0 if the series has fewer than 2 observations.
func ComputeMaxDrawdown(dailyReturns []float64) float64 {
	n := len(dailyReturns)
	if n < 2 {
		return 0
	}

	// Build equity curve.
	equity := make([]float64, n+1)
	equity[0] = 1.0
	for i := 0; i < n; i++ {
		equity[i+1] = equity[i] * (1 + dailyReturns[i])
	}

	// Find max drawdown (largest peak-to-trough decline).
	peak := equity[0]
	maxDD := 0.0 // 0 means no drawdown
	for i := 1; i < len(equity); i++ {
		if equity[i] > peak {
			peak = equity[i]
		}
		if peak > 0 {
			drawdown := (equity[i] - peak) / peak
			if drawdown < maxDD {
				maxDD = drawdown
			}
		}
	}

	return maxDD
}

// ComputePortfolioMaxDrawdown computes the maximum drawdown for a portfolio
// given aligned daily returns and portfolio weights.
// Returns the max drawdown as a negative ratio (e.g. -0.15 = 15% decline).
func ComputePortfolioMaxDrawdown(aligned [][]float64, weights []float64) float64 {
	portDailyRet := computePortfolioDailyReturns(aligned, weights)
	return ComputeMaxDrawdown(portDailyRet)
}
