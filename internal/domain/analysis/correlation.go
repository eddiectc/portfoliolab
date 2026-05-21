package analysis

import (
	"sort"
	"strconv"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
)

const (
	// minOverlapFraction is the fraction of the expected period that must
	// overlap between two symbols for a meaningful correlation. Matches the
	// short-coverage threshold so the bar is consistent.
	minOverlapFraction = 0.8
)

// dailyReturn pairs a trading date with its computed daily return.
type dailyReturn struct {
	date    int64     // Unix timestamp of the trading day
	return_ float64   // (close[t]/close[t-1]) - 1
}

// ComputeCorrelation computes the pairwise Pearson correlation matrix from
// historical price series.
//
// For each symbol the daily returns are derived from close prices:
//   return[t] = close[t] / close[t-1] - 1
//
// The lookback period is determined by period ("3M", "6M", "1Y", "3Y", "5Y", "10Y").
// Pairs with fewer than the adaptive overlap threshold produce a
// warning and a nil matrix cell (null in JSON, "-" in UI).
//
// Returns an empty-state message when fewer than 2 symbols have data.
func ComputeCorrelation(prices map[string][]market.HistoricalPrice, period string) *CorrelationResult {
	symbols := sortedSymbols(prices)

	var warnings []string

	// Determine cutoff date from period.
	cutoff, periodWarning := periodCutoff(period)
	if periodWarning != "" {
		warnings = append(warnings, periodWarning)
	}

	// Filter price series to the lookback period and compute dated returns.
	datedReturns := make(map[string][]dailyReturn, len(prices))
	var missingSymbols []string

	for _, sym := range symbols {
		series, ok := prices[sym]
		if !ok || len(series) == 0 {
			missingSymbols = append(missingSymbols, sym)
			continue
		}
		filtered := filterToPeriod(series, cutoff)
		if len(filtered) < 2 {
			missingSymbols = append(missingSymbols, sym)
			continue
		}
		rets := computeDailyReturnsWithDates(filtered)
		if len(rets) == 0 {
			missingSymbols = append(missingSymbols, sym)
			continue
		}
		datedReturns[sym] = rets
	}

	// Check if any symbol's data range is significantly shorter than the
	// requested period. Symbols with < 80% coverage get nil cells in the
	// matrix (UI renders as "-") so the user knows the data is incomplete.
	expectedDays := cutoffDays(period)
	shortCoverage := make(map[string]bool) // symbols with < 80% of requested period
	for _, sym := range symbols {
		rets, ok := datedReturns[sym]
		if !ok || expectedDays == 0 {
			continue
		}
		if len(rets) < int(float64(expectedDays)*0.8) {
			shortCoverage[sym] = true
			actualYears := float64(len(rets)) / 252.0 // ~trading days per year
			expectedYears := float64(expectedDays) / 252.0
			warnings = append(warnings,
				sym+": only "+strconv.FormatFloat(actualYears, 'f', 1, 64)+"Y of "+strconv.FormatFloat(expectedYears, 'f', 0, 64)+"Y price data (requested period)")
		}
	}

	// Symbols with valid return data, in sorted order.
	validSymbols := make([]string, 0, len(datedReturns))
	for _, sym := range symbols {
		if _, ok := datedReturns[sym]; ok {
			validSymbols = append(validSymbols, sym)
		}
	}

	// Add warnings for symbols excluded due to missing/insufficient data.
	for _, sym := range missingSymbols {
		warnings = append(warnings, sym+": insufficient price data for correlation")
	}

	if len(validSymbols) < 2 {
		msg := "Correlation requires at least 2 symbols with price data."
		if len(validSymbols) == 1 {
			msg = "Only 1 symbol has sufficient price data. Correlation requires at least 2."
		}
		return &CorrelationResult{
			Symbols:  validSymbols,
			Period:   period,
			Warnings: warnings,
			Message:  msg,
		}
	}

	n := len(validSymbols)
	matrix := make([][]*float64, n)
	for i := range matrix {
		matrix[i] = make([]*float64, n)
		// Self-correlation is always 1.0.
		one := 1.0
		matrix[i][i] = &one
	}

	// Compute pairwise correlations.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			symA := validSymbols[i]
			symB := validSymbols[j]

			// If either symbol doesn't cover the requested period, mark as nil.
			if shortCoverage[symA] || shortCoverage[symB] {
				matrix[i][j] = nil
				matrix[j][i] = nil
				continue
			}

			x, y, overlap := alignReturns(datedReturns[symA], datedReturns[symB])

			// Require 80% of the expected period to overlap, matching the
			// short-coverage threshold.
			minOverlap := int(float64(expectedDays) * minOverlapFraction)

			if overlap < minOverlap {
				// nil = insufficient data, UI renders as "-"
				matrix[i][j] = nil
				matrix[j][i] = nil
				warnings = append(warnings,
					symA+" ↔ "+symB+": only "+strconv.Itoa(overlap)+" overlapping days (minimum "+strconv.Itoa(minOverlap)+")")
			} else {
				corr, _ := stats.PearsonCorrelation(x, y)
				rounded := stats.RoundTo2(corr)
				matrix[i][j] = &rounded
				matrix[j][i] = &rounded
			}
		}
	}

	return &CorrelationResult{
		Matrix:   matrix,
		Symbols:  validSymbols,
		Period:   period,
		Warnings: warnings,
	}
}

// periodCutoff returns the start date for the given lookback period string
// and a warning if the period was unrecognized (defaults to 1Y).
func periodCutoff(period string) (time.Time, string) {
	now := time.Now()
	switch period {
	case "3M":
		return now.AddDate(0, -3, 0), ""
	case "6M":
		return now.AddDate(0, -6, 0), ""
	case "1Y":
		return now.AddDate(-1, 0, 0), ""
	case "3Y":
		return now.AddDate(-3, 0, 0), ""
	case "5Y":
		return now.AddDate(-5, 0, 0), ""
	case "10Y":
		return now.AddDate(-10, 0, 0), ""
	default:
		return now.AddDate(-1, 0, 0),
			"unrecognized period "+period+" — defaulting to 1Y"
	}
}

// cutoffDays returns the approximate number of trading days for the given
// period string (252 trading days per year). Returns 0 for unknown periods.
func cutoffDays(period string) int {
	switch period {
	case "3M":
		return 63  // 3 months × ~21 trading days
	case "6M":
		return 126 // 6 months × ~21 trading days
	case "1Y":
		return 252
	case "3Y":
		return 756
	case "5Y":
		return 1260
	case "10Y":
		return 2520
	default:
		return 252 // unrecognized periods default to 1Y, matching periodCutoff
	}
}

// filterToPeriod returns only price points on or after the cutoff date.
func filterToPeriod(series []market.HistoricalPrice, cutoff time.Time) []market.HistoricalPrice {
	var out []market.HistoricalPrice
	for _, p := range series {
		if !p.Date.Before(cutoff) {
			out = append(out, p)
		}
	}
	return out
}

// computeDailyReturnsWithDates computes daily returns preserving the date
// of each return for alignment across symbols.
// A series of N prices produces N-1 returns.
func computeDailyReturnsWithDates(series []market.HistoricalPrice) []dailyReturn {
	if len(series) < 2 {
		return nil
	}
	// Sort by date.
	sort.Slice(series, func(i, j int) bool {
		return series[i].Date.Before(series[j].Date)
	})

	rets := make([]dailyReturn, 0, len(series)-1)
	for i := 1; i < len(series); i++ {
		prev, ok1 := series[i-1].Close.Float64()
		curr, ok2 := series[i].Close.Float64()
		if !ok1 || !ok2 || prev == 0 {
			continue
		}
		rets = append(rets, dailyReturn{
			date:    series[i].Date.Unix(),
			return_: (curr / prev) - 1.0,
		})
	}
	return rets
}

// alignReturns takes two sets of dated daily returns and produces aligned
// float64 slices (matching dates in the same order) plus the overlap count.
// x always corresponds to series a, y to series b.
// Delegates to stats.AlignSeries after converting to string-keyed maps.
func alignReturns(a, b []dailyReturn) ([]float64, []float64, int) {
	mapA := dailyReturnsToMap(a)
	mapB := dailyReturnsToMap(b)
	return stats.AlignSeries(mapA, mapB)
}

// dailyReturnsToMap converts a slice of dated daily returns to a
// string-keyed map for use with stats.AlignSeries.
func dailyReturnsToMap(rets []dailyReturn) map[string]float64 {
	m := make(map[string]float64, len(rets))
	for _, r := range rets {
		// Use int64 unix timestamp formatted as string for consistent keys.
		key := strconv.FormatInt(r.date, 10)
		m[key] = r.return_
	}
	return m
}

// sortedSymbols returns the symbols from the prices map in sorted order.
func sortedSymbols(prices map[string][]market.HistoricalPrice) []string {
	syms := make([]string, 0, len(prices))
	for sym := range prices {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	return syms
}


