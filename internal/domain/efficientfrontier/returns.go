package efficientfrontier

import (
	"sort"

	"github.com/eddiectc/portfoliolab/internal/market"
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
