package comparison

import (
	"sort"
	"strconv"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// IntraPortfolioCorrelationInput holds the data needed to compute the
// correlation matrix for symbols within a single portfolio.
type IntraPortfolioCorrelationInput struct {
	// Prices maps each symbol to its historical price series (sorted ASC).
	Prices map[string][]market.HistoricalPrice
	// Period is the lookback period for correlation (e.g. "1Y", "3Y", "5Y").
	// Supported: "3M", "6M", "1Y", "3Y", "5Y", "10Y". Defaults to "1Y".
	Period string
}

// IntraPortfolioCorrelationResult holds the correlation matrix for symbols
// within a portfolio. Matrix is N×N where N = len(Symbols). Matrix[i][j]
// is a pointer to the correlation between Symbols[i] and Symbols[j], or nil
// when there is insufficient overlapping data.
type IntraPortfolioCorrelationResult struct {
	Matrix   [][]*float64 `json:"matrix,omitempty"`
	Symbols  []string     `json:"symbols"`
	Period   string       `json:"period"`
	Warnings []string     `json:"warnings,omitempty"`
	Message  string       `json:"message,omitempty"`
}

// ComputeIntraPortfolioCorrelation computes the pairwise Pearson correlation
// matrix from historical price series for the symbols in a portfolio.
//
// This is a thin wrapper around the analysis correlation logic, adapted for
// model portfolio inputs (symbol + weight pairs with historical prices) rather
// than position-based inputs.
//
// For each symbol, daily returns are derived from close prices:
//
//	return[t] = close[t] / close[t-1] - 1
//
// Pairs with fewer than the adaptive overlap threshold (80% of expected period)
// produce a warning and a nil matrix cell.
//
// Returns an empty-state message when fewer than 2 symbols have data.
func ComputeIntraPortfolioCorrelation(input IntraPortfolioCorrelationInput) *IntraPortfolioCorrelationResult {
	if input.Period == "" {
		input.Period = "1Y"
	}

	symbols := sortedSymbols(input.Prices)

	var warnings []string

	// Determine cutoff date from period.
	cutoff, periodWarning := correlationPeriodCutoff(input.Period)
	if periodWarning != "" {
		warnings = append(warnings, periodWarning)
	}

	// Filter price series to the lookback period and compute dated returns.
	datedReturns := make(map[string][]correlationDailyReturn, len(input.Prices))
	var missingSymbols []string

	for _, sym := range symbols {
		series, ok := input.Prices[sym]
		if !ok || len(series) == 0 {
			missingSymbols = append(missingSymbols, sym)
			continue
		}
		filtered := filterToCorrelationPeriod(series, cutoff)
		if len(filtered) < 2 {
			missingSymbols = append(missingSymbols, sym)
			continue
		}
		rets := computeCorrelationDailyReturnsWithDates(filtered)
		if len(rets) == 0 {
			missingSymbols = append(missingSymbols, sym)
			continue
		}
		datedReturns[sym] = rets
	}

	// Check short coverage.
	expectedDays := correlationCutoffDays(input.Period)
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

	for _, sym := range missingSymbols {
		warnings = append(warnings, sym+": insufficient price data for correlation")
	}

	if len(validSymbols) < 2 {
		msg := "Correlation requires at least 2 symbols with price data."
		if len(validSymbols) == 1 {
			msg = "Only 1 symbol has sufficient price data. Correlation requires at least 2."
		}
		return &IntraPortfolioCorrelationResult{
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

	minOverlap := int(float64(expectedDays) * 0.8)

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			symA := validSymbols[i]
			symB := validSymbols[j]

			if shortCoverage[symA] || shortCoverage[symB] {
				matrix[i][j] = nil
				matrix[j][i] = nil
				continue
			}

			x, y, overlap := correlationAlignReturns(
				datedReturns[symA], datedReturns[symB],
			)

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

	return &IntraPortfolioCorrelationResult{
		Matrix:   matrix,
		Symbols:  validSymbols,
		Period:   input.Period,
		Warnings: warnings,
	}
}

// --- helpers (mirrors analysis/correlation.go logic) ---

// correlationDailyReturn pairs a trading date with its computed daily return.
type correlationDailyReturn struct {
	date    int64   // Unix timestamp
	return_ float64 // (close[t]/close[t-1]) - 1
}

func correlationPeriodCutoff(period string) (time.Time, string) {
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
			"unrecognized period " + period + " — defaulting to 1Y"
	}
}

func correlationCutoffDays(period string) int {
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

func filterToCorrelationPeriod(series []market.HistoricalPrice, cutoff time.Time) []market.HistoricalPrice {
	var out []market.HistoricalPrice
	for _, p := range series {
		if !p.Date.Before(cutoff) {
			out = append(out, p)
		}
	}
	return out
}

func computeCorrelationDailyReturnsWithDates(series []market.HistoricalPrice) []correlationDailyReturn {
	if len(series) < 2 {
		return nil
	}
	sort.Slice(series, func(i, j int) bool {
		return series[i].Date.Before(series[j].Date)
	})

	rets := make([]correlationDailyReturn, 0, len(series)-1)
	for i := 1; i < len(series); i++ {
		prev, ok1 := series[i-1].Close.Float64()
		curr, ok2 := series[i].Close.Float64()
		if !ok1 || !ok2 || prev == 0 {
			continue
		}
		rets = append(rets, correlationDailyReturn{
			date:    series[i].Date.Unix(),
			return_: (curr / prev) - 1.0,
		})
	}
	return rets
}

func correlationAlignReturns(a, b []correlationDailyReturn) ([]float64, []float64, int) {
	mapA := correlationReturnsToMap(a)
	mapB := correlationReturnsToMap(b)
	return stats.AlignSeries(mapA, mapB)
}

func correlationReturnsToMap(rets []correlationDailyReturn) map[string]float64 {
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
