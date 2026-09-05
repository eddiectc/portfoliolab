// Package correlation provides the shared pairwise Pearson correlation matrix
// computation used by both the portfolio analysis and portfolio comparison
// features.
//
// The algorithm:
//  1. Filter price series to the lookback period
//  2. Compute daily returns per symbol with date tracking
//  3. Check for short-coverage symbols (< 80% of expected period)
//  4. Compute pairwise Pearson correlation for all symbol pairs
//  5. Mark cells as nil when overlap is below threshold
package correlation

import (
	"sort"
	"strconv"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/eddiectc/portfoliolab/internal/util"
)

const (
	// minOverlapFraction is the fraction of the expected period that must
	// overlap between two symbols for a meaningful correlation.
	minOverlapFraction = 0.8
)

// Result holds the pairwise Pearson correlation matrix.
// Matrix is N×N where N = len(Symbols). Matrix[i][j] is a pointer to the
// correlation between Symbols[i] and Symbols[j], or nil when there is
// insufficient overlapping data.
type Result struct {
	Matrix   [][]*float64 `json:"matrix,omitempty"`
	Symbols  []string     `json:"symbols"`
	Period   string       `json:"period"`
	Warnings []string     `json:"warnings,omitempty"`
	Message  string       `json:"message,omitempty"`
}

// Input holds the data needed for correlation computation.
type Input struct {
	// Prices maps each symbol to its historical price series (sorted ASC).
	Prices map[string][]market.HistoricalPrice
	// Period is the lookback period for correlation (e.g. "1Y", "3Y", "5Y").
	// Supported: "3M", "6M", "1Y", "3Y", "5Y", "10Y". Defaults to "1Y".
	Period string
}

// Compute computes the pairwise Pearson correlation matrix from historical
// price series.
//
// For each symbol the daily returns are derived from close prices:
//
//	return[t] = close[t] / close[t-1] - 1
//
// Pairs with fewer than the adaptive overlap threshold (80% of expected period)
// produce a warning and a nil matrix cell (null in JSON, "-" in UI).
//
// Returns an empty-state message when fewer than 2 symbols have data.
func Compute(input Input) *Result {
	if input.Period == "" {
		input.Period = "1Y"
	}

	symbols := sortedSymbols(input.Prices)

	var warnings []string

	// Determine cutoff date from period.
	cutoff, periodWarning := util.PeriodCutoff(input.Period)
	if periodWarning != "" {
		warnings = append(warnings, periodWarning)
	}

	// Filter price series to the lookback period and compute dated returns.
	datedReturns := make(map[string][]datedReturn, len(input.Prices))
	var missingSymbols []string

	for _, sym := range symbols {
		series, ok := input.Prices[sym]
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

	// Check short coverage.
	expectedDays := cutoffDays(input.Period)
	shortCoverage := make(map[string]bool)
	for _, sym := range symbols {
		rets, ok := datedReturns[sym]
		if !ok || expectedDays == 0 {
			continue
		}
		if len(rets) < int(float64(expectedDays)*0.8) {
			shortCoverage[sym] = true
			actualYears := float64(len(rets)) / 252.0
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
		return &Result{
			Symbols:  validSymbols,
			Period:   input.Period,
			Warnings: warnings,
			Message:  msg,
		}
	}

	n := len(validSymbols)
	matrix := make([][]*float64, n)
	for i := range matrix {
		matrix[i] = make([]*float64, n)
		one := 1.0
		matrix[i][i] = &one
	}

	minOverlap := int(float64(expectedDays) * minOverlapFraction)

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			symA := validSymbols[i]
			symB := validSymbols[j]

			if shortCoverage[symA] || shortCoverage[symB] {
				matrix[i][j] = nil
				matrix[j][i] = nil
				continue
			}

			x, y, overlap := alignReturns(datedReturns[symA], datedReturns[symB])

			if overlap < minOverlap {
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

	return &Result{
		Matrix:   matrix,
		Symbols:  validSymbols,
		Period:   input.Period,
		Warnings: warnings,
	}
}

// --- Internal helpers ---

// datedReturn pairs a trading date with its computed daily return.
type datedReturn struct {
	date    int64   // Unix timestamp of the trading day
	return_ float64 // (close[t]/close[t-1]) - 1
}

func cutoffDays(period string) int {
	switch period {
	case "3M":
		return 63
	case "6M":
		return 126
	case "1Y":
		return 252
	case "3Y":
		return 756
	case "5Y":
		return 1260
	case "10Y":
		return 2520
	default:
		return 252
	}
}

func filterToPeriod(series []market.HistoricalPrice, cutoff time.Time) []market.HistoricalPrice {
	var out []market.HistoricalPrice
	for _, p := range series {
		if !p.Date.Before(cutoff) {
			out = append(out, p)
		}
	}
	return out
}

func computeDailyReturnsWithDates(series []market.HistoricalPrice) []datedReturn {
	if len(series) < 2 {
		return nil
	}
	sort.Slice(series, func(i, j int) bool {
		return series[i].Date.Before(series[j].Date)
	})

	rets := make([]datedReturn, 0, len(series)-1)
	for i := 1; i < len(series); i++ {
		prev, ok1 := series[i-1].Close.Float64()
		curr, ok2 := series[i].Close.Float64()
		if !ok1 || !ok2 || prev == 0 {
			continue
		}
		rets = append(rets, datedReturn{
			date:    series[i].Date.Unix(),
			return_: (curr / prev) - 1.0,
		})
	}
	return rets
}

func alignReturns(a, b []datedReturn) ([]float64, []float64, int) {
	mapA := returnsToMap(a)
	mapB := returnsToMap(b)
	return stats.AlignSeries(mapA, mapB)
}

func returnsToMap(rets []datedReturn) map[string]float64 {
	m := make(map[string]float64, len(rets))
	for _, r := range rets {
		key := strconv.FormatInt(r.date, 10)
		m[key] = r.return_
	}
	return m
}

func sortedSymbols(prices map[string][]market.HistoricalPrice) []string {
	syms := make([]string, 0, len(prices))
	for sym := range prices {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	return syms
}
