package efficientfrontier

import (
	"math"
	"sort"

	"github.com/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/eddiectc/portfoliolab/internal/market"
)

// ComputeCovarianceMatrix computes the sample covariance matrix from aligned
// daily returns for multiple symbols.
//
// The input is a map of symbol → daily returns. Returns must be aligned by
// date (same length, same trading days). The function first computes returns
// from the raw price data, then aligns them by date, and finally computes
// the sample covariance matrix.
//
// Returns the N×N covariance matrix (row-major) and the ordered list of
// symbol names. The matrix is symmetric with variances on the diagonal.
func ComputeCovarianceMatrix(pricesBySymbol map[string][]market.HistoricalPrice) ([][]float64, []string, error) {
	// Compute returns per symbol.
	returnsBySymbol := make(map[string][]symbolReturn, len(pricesBySymbol))
	symbols := make([]string, 0, len(pricesBySymbol))

	for sym, prices := range pricesBySymbol {
		rets := make([]symbolReturn, 0, len(prices)-1)
		// Copy and sort to avoid mutating caller's data.
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
			rets = append(rets, symbolReturn{
				date: sorted[i].Date.Unix(),
				ret:  (curr / prev) - 1.0,
			})
		}
		if len(rets) > 0 {
			returnsBySymbol[sym] = rets
			symbols = append(symbols, sym)
		}
	}

	sort.Strings(symbols)

	if len(symbols) < 2 {
		return nil, symbols, ErrInsufficientSymbols
	}

	// Align returns across all symbols by date.
	// Build a date → [symbol index → return] map.
	dateMap := make(map[int64][]float64)
	for _, sym := range symbols {
		for _, r := range returnsBySymbol[sym] {
			// Find or create entry for this date.
			entry, ok := dateMap[r.date]
			if !ok {
				entry = make([]float64, len(symbols))
				for i := range entry {
					entry[i] = math.NaN() // sentinel for missing
				}
			}
			idx := symbolIndex(symbols, sym)
			entry[idx] = r.ret
			dateMap[r.date] = entry
		}
	}

	// Collect only dates where all symbols have data.
	n := len(symbols)
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

	if len(aligned) < 2 {
		return nil, symbols, ErrInsufficientData
	}

	// Compute annualized covariance matrix using shared stats function.
	covMatrix := stats.AnnualizedCovarianceMatrix(aligned)

	return covMatrix, symbols, nil
}

// symbolReturn pairs a date (unix timestamp) with its daily return.
type symbolReturn struct {
	date int64
	ret  float64
}

// symbolIndex returns the index of sym in the sorted symbols slice.
func symbolIndex(symbols []string, sym string) int {
	for i, s := range symbols {
		if s == sym {
			return i
		}
	}
	return -1
}
