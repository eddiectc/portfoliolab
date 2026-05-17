package analysis

import (
	"math"
	"sort"
	"strconv"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

const (
	// minOverlapDays is the minimum number of overlapping daily returns
	// required to compute a meaningful correlation. Below this threshold
	// a warning is emitted and the cell is output as 0.
	minOverlapDays = 60
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
// The lookback period is determined by period ("1Y", "3Y", "5Y", "10Y").
// Pairs with fewer than minOverlapDays of overlapping returns produce a
// warning and a zero correlation coefficient.
//
// Returns an empty-state message when fewer than 2 symbols have data.
func ComputeCorrelation(prices map[string][]market.HistoricalPrice, period string) *CorrelationResult {
	symbols := sortedSymbols(prices)

	// Determine cutoff date from period.
	cutoff := periodCutoff(period)

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
		datedReturns[sym] = computeDailyReturnsWithDates(filtered)
	}

	// Symbols with valid return data, in sorted order.
	validSymbols := make([]string, 0, len(datedReturns))
	for _, sym := range symbols {
		if _, ok := datedReturns[sym]; ok {
			validSymbols = append(validSymbols, sym)
		}
	}

	// Add warnings for symbols excluded due to missing/insufficient data.
	var warnings []string
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
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
	}

	// Compute pairwise correlations.
	for i := 0; i < n; i++ {
		matrix[i][i] = 1.0 // self-correlation
		for j := i + 1; j < n; j++ {
			symA := validSymbols[i]
			symB := validSymbols[j]
			x, y, overlap := alignReturns(datedReturns[symA], datedReturns[symB])
			corr, _ := pearsonCorrelation(x, y)
			matrix[i][j] = roundTo2(corr)
			matrix[j][i] = roundTo2(corr)

			if overlap < minOverlapDays {
				warnings = append(warnings,
					symA+" ↔ "+symB+": only "+strconv.Itoa(overlap)+" overlapping days (minimum "+strconv.Itoa(minOverlapDays)+")")
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

// periodCutoff returns the start date for the given lookback period string.
func periodCutoff(period string) time.Time {
	now := time.Now()
	switch period {
	case "1Y":
		return now.AddDate(-1, 0, 0)
	case "3Y":
		return now.AddDate(-3, 0, 0)
	case "5Y":
		return now.AddDate(-5, 0, 0)
	case "10Y":
		return now.AddDate(-10, 0, 0)
	default:
		// Default to 1Y.
		return now.AddDate(-1, 0, 0)
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
func alignReturns(a, b []dailyReturn) ([]float64, []float64, int) {
	// Build date→return map for the smaller series for lookup efficiency.
	var mapFromA bool // true if the map is built from a
	var dateMap map[int64]float64
	var walk []dailyReturn // the series we iterate over

	if len(a) <= len(b) {
		mapFromA = true
		dateMap = make(map[int64]float64, len(a))
		for _, r := range a {
			dateMap[r.date] = r.return_
		}
		walk = b
	} else {
		mapFromA = false
		dateMap = make(map[int64]float64, len(b))
		for _, r := range b {
			dateMap[r.date] = r.return_
		}
		walk = a
	}

	x := make([]float64, 0, len(dateMap))
	y := make([]float64, 0, len(dateMap))

	for _, wr := range walk {
		mapped, ok := dateMap[wr.date]
		if !ok {
			continue
		}
		if mapFromA {
			// walk is b, map is a: mapped=a, wr=b
			x = append(x, mapped)
			y = append(y, wr.return_)
		} else {
			// walk is a, map is b: wr=a, mapped=b
			x = append(x, wr.return_)
			y = append(y, mapped)
		}
	}

	return x, y, len(x)
}

// pearsonCorrelation computes the Pearson correlation coefficient of two
// equally-lengthed float64 series. Returns the coefficient and sample count.
// If either series has zero variance, returns 0.
func pearsonCorrelation(x, y []float64) (float64, int) {
	n := len(x)
	if n == 0 || n != len(y) {
		return 0, 0
	}

	// Compute means.
	sumX, sumY := 0.0, 0.0
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	// Compute numerator and denominators.
	num := 0.0
	sumDx2 := 0.0
	sumDy2 := 0.0
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		num += dx * dy
		sumDx2 += dx * dx
		sumDy2 += dy * dy
	}

	if sumDx2 == 0 || sumDy2 == 0 {
		return 0, n
	}

	return num / math.Sqrt(sumDx2*sumDy2), n
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


