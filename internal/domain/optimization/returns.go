package optimization

import (
	"errors"
	"sort"

	"github.com/eddiectc/portfoliolab/internal/market"
)

// ErrInsufficientData is returned when price data is too short for computation.
var ErrInsufficientData = errors.New("insufficient price data for computation")

// ComputeDailyReturns computes simple daily returns from a series of historical
// close prices. Returns are (close[t] / close[t-1]) - 1.
// The input series is sorted by date ascending.
// A series of N prices produces N-1 returns.
// Returns an error if fewer than 2 valid returns can be computed.
func ComputeDailyReturns(prices []market.HistoricalPrice) ([]float64, error) {
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

// ComputeDailyReturnsWithDates computes daily returns preserving the date
// of each return for alignment across symbols.
// A series of N prices produces N-1 returns.
type DatedReturn struct {
	Date   int64   // Unix timestamp of the trading day
	Return float64 // (close[t]/close[t-1]) - 1
}

func ComputeDailyReturnsWithDates(series []market.HistoricalPrice) []DatedReturn {
	if len(series) < 2 {
		return nil
	}
	// Sort by date.
	sort.Slice(series, func(i, j int) bool {
		return series[i].Date.Before(series[j].Date)
	})

	rets := make([]DatedReturn, 0, len(series)-1)
	for i := 1; i < len(series); i++ {
		prev, ok1 := series[i-1].Close.Float64()
		curr, ok2 := series[i].Close.Float64()
		if !ok1 || !ok2 || prev == 0 {
			continue
		}
		rets = append(rets, DatedReturn{
			Date:   series[i].Date.Unix(),
			Return: (curr / prev) - 1.0,
		})
	}
	return rets
}

// AlignReturns aligns daily returns across multiple symbols by trading date,
// returning only dates where ALL symbols have data.
//
// The symbols parameter defines the column order in the output matrix.
// Each row is one trading date; each column is one symbol (ordered by symbols).
// The returned count is the number of aligned trading days.
//
// Returns an error if fewer than 2 aligned observations can be produced.
func AlignReturns(pricesBySymbol map[string][]market.HistoricalPrice, symbols []string) ([][]float64, int, error) {
	nSymbols := len(symbols)
	if nSymbols == 0 {
		return nil, 0, ErrInsufficientData
	}

	// Build a lookup from symbol to its ordered index.
	symIndex := make(map[string]int, nSymbols)
	for i, s := range symbols {
		symIndex[s] = i
	}

	// Compute returns per symbol and index by date (unix epoch seconds).
	type priceWithDate struct {
		date int64
		ret  float64
	}
	retsBySymbol := make(map[string][]priceWithDate, nSymbols)

	for _, sym := range symbols {
		prices, ok := pricesBySymbol[sym]
		if !ok {
			continue
		}
		rets, err := ComputeDailyReturns(prices)
		if err != nil {
			continue // skip symbols with insufficient data
		}
		// Compute matching dates from the sorted prices.
		sorted := make([]market.HistoricalPrice, len(prices))
		copy(sorted, prices)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Date.Before(sorted[j].Date)
		})
		// Pair each return with the date of the "current" price (index i).
		var valid []priceWithDate
		retIdx := 0
		for i := 1; i < len(sorted); i++ {
			prev, ok1 := sorted[i-1].Close.Float64()
			_, ok2 := sorted[i].Close.Float64()
			if !ok1 || !ok2 || prev == 0 {
				continue
			}
			if retIdx < len(rets) {
				valid = append(valid, priceWithDate{
					date: sorted[i].Date.Unix(),
					ret:  rets[retIdx],
				})
				retIdx++
			}
		}
		retsBySymbol[sym] = valid
	}

	// Build date → [symbol → return] map.
	type rowEntry struct {
		values []float64
		count  int // how many symbols present
	}
	dateRows := make(map[int64]*rowEntry)

	for _, sym := range symbols {
		idx := symIndex[sym]
		for _, pr := range retsBySymbol[sym] {
			entry, ok := dateRows[pr.date]
			if !ok {
				entry = &rowEntry{
					values: make([]float64, nSymbols),
				}
				dateRows[pr.date] = entry
			}
			entry.values[idx] = pr.ret
			entry.count++
		}
	}

	// Collect only dates where all symbols have data, sorted by date.
	type datedRow struct {
		date int64
		row  []float64
	}
	var dated []datedRow
	for date, entry := range dateRows {
		if entry.count == nSymbols {
			row := make([]float64, nSymbols)
			copy(row, entry.values)
			dated = append(dated, datedRow{date: date, row: row})
		}
	}
	sort.Slice(dated, func(i, j int) bool {
		return dated[i].date < dated[j].date
	})

	aligned := make([][]float64, len(dated))
	for i, dr := range dated {
		aligned[i] = dr.row
	}

	if len(aligned) < 2 {
		return nil, 0, ErrInsufficientData
	}

	return aligned, len(aligned), nil
}
