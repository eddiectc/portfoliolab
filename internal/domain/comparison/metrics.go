package comparison

import (
	"math"
	"sort"
	"strconv"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/govalues/decimal"
)

// CAGRResult holds the compound annual growth rate computed from
// start/end portfolio values and the elapsed time.
type CAGRResult struct {
	// CAGRPct is the compound annual growth rate expressed as a percentage
	// (e.g. 12.50 = 12.50%). Nil when insufficient data.
	CAGRPct *decimal.Decimal `json:"cagr_pct,omitempty"`
	// DaysElapsed is the number of days between first and last equity curve point.
	DaysElapsed int `json:"days_elapsed"`
}

// ComputeCAGR computes the compound annual growth rate from an equity curve.
//
//	CAGR = (endValue / startValue) ^ (365 / days) - 1
//
// Returns nil CAGRPct when fewer than 2 points, start value is non-positive,
// or zero days elapsed.
func ComputeCAGR(points []EquityCurvePoint) CAGRResult {
	if len(points) < 2 {
		return CAGRResult{}
	}

	first := points[0]
	last := points[len(points)-1]

	startF, _ := first.PortfolioValue.Float64()
	endF, _ := last.PortfolioValue.Float64()

	if startF <= 0 {
		return CAGRResult{DaysElapsed: daysBetween(first.Date, last.Date)}
	}

	days := daysBetween(first.Date, last.Date)
	if days <= 0 {
		return CAGRResult{DaysElapsed: days}
	}

	ratio := endF / startF
	cagr := math.Pow(ratio, 365.0/float64(days)) - 1.0
	cagrPct, _ := decimal.NewFromFloat64(cagr * 100.0)
	cagrPct = cagrPct.Round(2)

	return CAGRResult{
		CAGRPct:   &cagrPct,
		DaysElapsed: days,
	}
}

// BetaAlphaResult holds the beta and alpha of portfolio A relative to
// portfolio B (the benchmark).
type BetaAlphaResult struct {
	// Beta is the systematic risk of portfolio A relative to B.
	// Beta = Cov(Ra, Rb) / Var(Rb). Nil when insufficient data or B has zero variance.
	Beta *decimal.Decimal `json:"beta,omitempty"`
	// Alpha is the excess return of A over what beta would predict.
	// Alpha = mean(Ra) - beta * mean(Rb), annualized as a percentage.
	// Nil when beta is nil.
	Alpha *decimal.Decimal `json:"alpha,omitempty"`
	// OverlapDays is the number of aligned daily return observations used.
	OverlapDays int `json:"overlap_days"`
}

// ComputeBetaAlpha computes the beta and alpha of portfolio A relative to
// portfolio B using aligned daily returns.
//
// Daily returns are derived from equity curve points: (value[t]/value[t-1]) - 1.
// Returns from both series are aligned by date (matching trading days only).
//
// Beta is computed via Pearson regression: Cov(Ra, Rb) / Var(Rb).
// Alpha is the annualized excess return: (mean(Ra) - beta * mean(Rb)) * 252 * 100.
//
// Returns nil Beta/Alpha when fewer than 2 aligned observations, or when
// portfolio B has zero variance (constant returns).
func ComputeBetaAlpha(aPoints, bPoints []EquityCurvePoint) BetaAlphaResult {
	aRet := equityCurveToDailyReturns(aPoints)
	bRet := equityCurveToDailyReturns(bPoints)

	if len(aRet) < 2 || len(bRet) < 2 {
		return BetaAlphaResult{}
	}

	x, y, overlap := alignDailyReturns(aRet, bRet)
	if overlap < 2 {
		return BetaAlphaResult{OverlapDays: overlap}
	}

	result := BetaAlphaResult{OverlapDays: overlap}

	// Compute means.
	sumX, sumY := 0.0, 0.0
	for i := 0; i < overlap; i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / float64(overlap)
	meanY := sumY / float64(overlap)

	// Covariance and variance.
	covXY := 0.0
	varY := 0.0
	for i := 0; i < overlap; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		covXY += dx * dy
		varY += dy * dy
	}

	if varY == 0 {
		// B has zero variance — beta is undefined.
		return result
	}

	beta := covXY / varY
	betaDec, _ := decimal.NewFromFloat64(beta)
	betaDec = betaDec.Round(4)
	result.Beta = &betaDec

	// Alpha = (mean(Ra) - beta * mean(Rb)) * 252, expressed as percentage.
	alpha := (meanX - beta*meanY) * 252.0
	alphaPct, _ := decimal.NewFromFloat64(alpha * 100.0)
	alphaPct = alphaPct.Round(2)
	result.Alpha = &alphaPct

	return result
}

// PortfolioCorrelationResult holds the overall correlation between two
// portfolio daily return series.
type PortfolioCorrelationResult struct {
	// Correlation is the Pearson correlation coefficient (-1.0 to 1.0).
	// Nil when insufficient data or zero variance in either series.
	Correlation *decimal.Decimal `json:"correlation,omitempty"`
	// OverlapDays is the number of aligned daily return observations used.
	OverlapDays int `json:"overlap_days"`
}

// ComputePortfolioCorrelation computes the Pearson correlation between two
// portfolio daily return series.
//
// Daily returns are derived from equity curve points: (value[t]/value[t-1]) - 1.
// Returns from both series are aligned by date (matching trading days only).
//
// Returns nil Correlation when fewer than 2 aligned observations, or when
// either series has zero variance.
func ComputePortfolioCorrelation(aPoints, bPoints []EquityCurvePoint) PortfolioCorrelationResult {
	aRet := equityCurveToDailyReturns(aPoints)
	bRet := equityCurveToDailyReturns(bPoints)

	if len(aRet) < 2 || len(bRet) < 2 {
		return PortfolioCorrelationResult{}
	}

	x, y, overlap := alignDailyReturns(aRet, bRet)
	if overlap < 2 {
		return PortfolioCorrelationResult{OverlapDays: overlap}
	}

	result := PortfolioCorrelationResult{OverlapDays: overlap}

	corr, _ := stats.PearsonCorrelation(x, y)
	if corr == 0 && overlap > 0 {
		// Distinguish between "zero correlation" and "undefined" (zero variance).
		// stats.PearsonCorrelation returns 0 for zero variance; check explicitly.
		if hasZeroVariance(x) || hasZeroVariance(y) {
			return result
		}
	}

	corrDec, _ := decimal.NewFromFloat64(corr)
	corrDec = corrDec.Round(4)
	result.Correlation = &corrDec

	return result
}

// PeriodExtremes holds the best/worst monthly and yearly returns, plus
// the win rate (percentage of positive-return months).
type PeriodExtremes struct {
	// BestMonth is the best monthly return as a percentage. Nil when no monthly data.
	BestMonth *decimal.Decimal `json:"best_month,omitempty"`
	// BestMonthLabel is the "YYYY-MM" label of the best month.
	BestMonthLabel string `json:"best_month_label,omitempty"`
	// WorstMonth is the worst monthly return as a percentage. Nil when no monthly data.
	WorstMonth *decimal.Decimal `json:"worst_month,omitempty"`
	// WorstMonthLabel is the "YYYY-MM" label of the worst month.
	WorstMonthLabel string `json:"worst_month_label,omitempty"`
	// BestYear is the best calendar-year return as a percentage. Nil when no yearly data.
	BestYear *decimal.Decimal `json:"best_year,omitempty"`
	// BestYearLabel is the "YYYY" label of the best year.
	BestYearLabel string `json:"best_year_label,omitempty"`
	// WorstYear is the worst calendar-year return as a percentage. Nil when no yearly data.
	WorstYear *decimal.Decimal `json:"worst_year,omitempty"`
	// WorstYearLabel is the "YYYY" label of the worst year.
	WorstYearLabel string `json:"worst_year_label,omitempty"`
	// WinRatePct is the percentage of months with positive returns.
	// Nil when no monthly data. Expressed as a percentage (e.g. 62.50).
	WinRatePct *decimal.Decimal `json:"win_rate_pct,omitempty"`
}

// ComputePeriodExtremes computes best/worst month, best/worst year, and
// win rate from an equity curve.
//
// Monthly returns are computed as (last_close / first_close - 1) * 100 per year-month.
// Yearly returns are computed as (last_close / first_close - 1) * 100 per calendar year.
// Win rate is the percentage of months with positive returns.
//
// Returns nil fields when insufficient data (fewer than 2 equity curve points).
func ComputePeriodExtremes(points []EquityCurvePoint) PeriodExtremes {
	if len(points) < 2 {
		return PeriodExtremes{}
	}

	// Group by year-month: key "YYYY-MM" -> []values.
	monthGroups := make(map[string][]decimal.Decimal)
	for _, p := range points {
		key := p.Date.Format("2006-01")
		monthGroups[key] = append(monthGroups[key], p.PortfolioValue)
	}

	// Group by year: key "YYYY" -> []values.
	yearGroups := make(map[string][]decimal.Decimal)
	for _, p := range points {
		key := p.Date.Format("2006")
		yearGroups[key] = append(yearGroups[key], p.PortfolioValue)
	}

	var result PeriodExtremes

	// Compute monthly returns.
	type labeledReturn struct {
		label string
		value decimal.Decimal
	}
	var monthlyReturns []labeledReturn
	for key, values := range monthGroups {
		if len(values) < 2 {
			continue
		}
		firstF, _ := values[0].Float64()
		lastF, _ := values[len(values)-1].Float64()
		if firstF <= 0 {
			continue
		}
		ret, _ := decimal.NewFromFloat64((lastF/firstF - 1.0) * 100.0)
		ret = ret.Round(2)
		monthlyReturns = append(monthlyReturns, labeledReturn{label: key, value: ret})
	}

	// Sort by label for deterministic results.
	sort.Slice(monthlyReturns, func(i, j int) bool {
		return monthlyReturns[i].label < monthlyReturns[j].label
	})

	if len(monthlyReturns) > 0 {
		best := monthlyReturns[0]
		worst := monthlyReturns[0]
		positiveCount := 0
		for _, m := range monthlyReturns {
			if m.value.Cmp(best.value) > 0 {
				best = m
			}
			if m.value.Cmp(worst.value) < 0 {
				worst = m
			}
			if m.value.IsPos() {
				positiveCount++
			}
		}
		result.BestMonth = &best.value
		result.BestMonthLabel = best.label
		result.WorstMonth = &worst.value
		result.WorstMonthLabel = worst.label

		winRate, _ := decimal.NewFromFloat64(float64(positiveCount) / float64(len(monthlyReturns)) * 100.0)
		winRate = winRate.Round(2)
		result.WinRatePct = &winRate
	}

	// Compute yearly returns.
	var yearlyReturns []labeledReturn
	for key, values := range yearGroups {
		if len(values) < 2 {
			continue
		}
		firstF, _ := values[0].Float64()
		lastF, _ := values[len(values)-1].Float64()
		if firstF <= 0 {
			continue
		}
		ret, _ := decimal.NewFromFloat64((lastF/firstF - 1.0) * 100.0)
		ret = ret.Round(2)
		yearlyReturns = append(yearlyReturns, labeledReturn{label: key, value: ret})
	}

	sort.Slice(yearlyReturns, func(i, j int) bool {
		return yearlyReturns[i].label < yearlyReturns[j].label
	})

	if len(yearlyReturns) > 0 {
		best := yearlyReturns[0]
		worst := yearlyReturns[0]
		for _, yr := range yearlyReturns {
			if yr.value.Cmp(best.value) > 0 {
				best = yr
			}
			if yr.value.Cmp(worst.value) < 0 {
				worst = yr
			}
		}
		result.BestYear = &best.value
		result.BestYearLabel = best.label
		result.WorstYear = &worst.value
		result.WorstYearLabel = worst.label
	}

	return result
}

// ReturnBucket holds a range label and the count of periods falling in that range.
type ReturnBucket struct {
	Label string `json:"label"` // e.g. "-10% to -5%", "5% to 10%"
	Count int    `json:"count"`
}

// ReturnDistribution holds annual and monthly return frequency histograms.
type ReturnDistribution struct {
	// Annual is the annual return list (one bucket per calendar year, sorted by year).
	// Each bucket has Count=1 and Label like "2024: +15.50%".
	Annual []ReturnBucket `json:"annual"`
	// AnnualBinned is the annual return frequency histogram — yearly returns
	// grouped into fixed-width bins (same 5% bin width as monthly).
	AnnualBinned []ReturnBucket `json:"annual_binned"`
	// Monthly is the monthly return histogram grouped into fixed-width bins.
	Monthly []ReturnBucket `json:"monthly"`
}

// ComputeReturnDistribution computes annual and monthly return frequency
// histograms from an equity curve.
//
// Annual: one entry per calendar year with the return percentage (label is
// "YYYY: +X.XX%" or "YYYY: -X.XX%").
//
// Monthly: fixed-width bins of 5% width, covering the range of observed
// monthly returns. Bin labels are "-10% to -5%", "-5% to 0%", "0% to 5%", etc.
//
// Returns empty slices when insufficient data.
func ComputeReturnDistribution(points []EquityCurvePoint) ReturnDistribution {
	if len(points) < 2 {
		return ReturnDistribution{}
	}

	// Group by year-month.
	monthGroups := make(map[string][]decimal.Decimal)
	for _, p := range points {
		key := p.Date.Format("2006-01")
		monthGroups[key] = append(monthGroups[key], p.PortfolioValue)
	}

	// Group by year.
	yearGroups := make(map[string][]decimal.Decimal)
	for _, p := range points {
		key := p.Date.Format("2006")
		yearGroups[key] = append(yearGroups[key], p.PortfolioValue)
	}

	// Compute annual returns.
	type yearReturn struct {
		year  string
		value float64
	}
	var annualReturns []yearReturn
	for key, values := range yearGroups {
		if len(values) < 2 {
			continue
		}
		firstF, _ := values[0].Float64()
		lastF, _ := values[len(values)-1].Float64()
		if firstF <= 0 {
			continue
		}
		ret := (lastF/firstF - 1.0) * 100.0
		annualReturns = append(annualReturns, yearReturn{year: key, value: ret})
	}

	sort.Slice(annualReturns, func(i, j int) bool {
		return annualReturns[i].year < annualReturns[j].year
	})

	annualBuckets := make([]ReturnBucket, 0, len(annualReturns))
	var annualVals []float64
	for _, yr := range annualReturns {
		label := formatAnnualBucket(yr.year, yr.value)
		annualBuckets = append(annualBuckets, ReturnBucket{Label: label, Count: 1})
		annualVals = append(annualVals, yr.value)
	}

	// Compute monthly returns.
	var monthlyVals []float64
	for _, values := range monthGroups {
		if len(values) < 2 {
			continue
		}
		firstF, _ := values[0].Float64()
		lastF, _ := values[len(values)-1].Float64()
		if firstF <= 0 {
			continue
		}
		ret := (lastF/firstF - 1.0) * 100.0
		monthlyVals = append(monthlyVals, ret)
	}

	monthlyBuckets := buildHistogramBins(monthlyVals)
	annualBinned := buildHistogramBins(annualVals)

	return ReturnDistribution{
		Annual:       annualBuckets,
		AnnualBinned: annualBinned,
		Monthly:      monthlyBuckets,
	}
}

// ComputeDrawdownSeries computes the drawdown-over-time series from an
// equity curve. It walks through the points tracking a running peak and
// computes the drawdown at each point as (peak - value) / peak × 100.
//
// Each point's Pct is expressed as a positive percentage (e.g. 15.50 = 15.50%
// below peak). Zero when the portfolio is at its peak.
//
// Returns empty slice when fewer than 2 points or all values are non-positive.
func ComputeDrawdownSeries(points []EquityCurvePoint) []DrawdownSeriesPoint {
	if len(points) < 2 {
		return nil
	}

	var peakVal decimal.Decimal
	var series []DrawdownSeriesPoint

	for _, p := range points {
		val := p.PortfolioValue
		if !val.IsPos() {
			continue
		}

		// Update running peak.
		if peakVal.IsZero() || peakVal.Less(val) {
			peakVal = val
		}

		// Compute drawdown from current peak.
		diff, _ := peakVal.Sub(val)
		drawdownPct, _ := diff.Quo(peakVal)
		drawdownPct, _ = drawdownPct.Mul(decimal.MustNew(100, 0))
		drawdownPct = drawdownPct.Round(2)

		series = append(series, DrawdownSeriesPoint{
			Date: p.Date,
			Pct:  drawdownPct,
		})
	}

	return series
}

// --- helpers ---

// equityCurveDailyReturn pairs a date with a daily return derived from
// consecutive equity curve points.
type equityCurveDailyReturn struct {
	date  time.Time
	return_ float64
}

// equityCurveToDailyReturns converts an equity curve to daily returns.
// N points produce N-1 returns.
func equityCurveToDailyReturns(points []EquityCurvePoint) []equityCurveDailyReturn {
	if len(points) < 2 {
		return nil
	}

	// Ensure sorted by date.
	sorted := make([]EquityCurvePoint, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	rets := make([]equityCurveDailyReturn, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		prevF, _ := sorted[i-1].PortfolioValue.Float64()
		currF, _ := sorted[i].PortfolioValue.Float64()
		if prevF <= 0 {
			continue
		}
		rets = append(rets, equityCurveDailyReturn{
			date:    sorted[i].Date,
			return_: currF / prevF - 1.0,
		})
	}
	return rets
}

// alignDailyReturns takes two sets of dated daily returns and produces
// aligned float64 slices (matching dates in the same order) plus the
// overlap count. x always corresponds to series a, y to series b.
// Delegates to stats.AlignSeries after converting to string-keyed maps.
func alignDailyReturns(a, b []equityCurveDailyReturn) ([]float64, []float64, int) {
	mapA := equityCurveReturnsToMap(a)
	mapB := equityCurveReturnsToMap(b)
	return stats.AlignSeries(mapA, mapB)
}

// equityCurveReturnsToMap converts a slice of dated daily returns to a
// string-keyed map for use with stats.AlignSeries.
func equityCurveReturnsToMap(rets []equityCurveDailyReturn) map[string]float64 {
	m := make(map[string]float64, len(rets))
	for _, r := range rets {
		m[r.date.Format("2006-01-02")] = r.return_
	}
	return m
}

// hasZeroVariance returns true if all values in the series are identical.
func hasZeroVariance(x []float64) bool {
	if len(x) < 2 {
		return true
	}
	for i := 1; i < len(x); i++ {
		if x[i] != x[0] {
			return false
		}
	}
	return true
}

// daysBetween returns the number of days between two dates.
func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24.0)
}

// formatAnnualBucket formats a single annual return as "YYYY: +X.XX%" or "YYYY: -X.XX%".
func formatAnnualBucket(year string, value float64) string {
	sign := "+"
	if value < 0 {
		sign = "-"
		value = -value
	}
	return year + ": " + sign + formatPct(value) + "%"
}

// formatPct formats a percentage value to 2 decimal places.
func formatPct(v float64) string {
	// Manual formatting to ensure consistent "X.XX" output.
	whole := int(v)
	frac := int(math.Round((v - float64(whole)) * 100))
	return strconv.Itoa(whole) + "." + padFrac(frac)
}

// padFrac ensures two-digit fractional part.
func padFrac(frac int) string {
	if frac < 10 {
		return "0" + strconv.Itoa(frac)
	}
	return strconv.Itoa(frac)
}

// buildHistogramBins builds fixed-width 5% bins from a set of monthly returns.
func buildHistogramBins(values []float64) []ReturnBucket {
	if len(values) == 0 {
		return []ReturnBucket{}
	}

	// Find min/max to determine bin range.
	minV := values[0]
	maxV := values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	// Bin width is 5%.
	binWidth := 5.0
	// Round min down and max up to nearest bin boundary.
	binStart := math.Floor(minV/binWidth) * binWidth
	binEnd := math.Ceil(maxV/binWidth) * binWidth

	// Initialize bins.
	bins := make(map[int]int) // bin index -> count
	for _, v := range values {
		idx := int(math.Floor((v - binStart) / binWidth))
		// Handle exact upper boundary.
		if float64(idx)*binWidth+binStart >= binEnd {
			idx--
		}
		bins[idx]++
	}

	// Build sorted bucket list.
	start := int(math.Floor(binStart / binWidth))
	end := int(math.Floor((binEnd - binWidth) / binWidth))

	buckets := make([]ReturnBucket, 0, end-start+1)
	for i := start; i <= end; i++ {
		lo := float64(i) * binWidth
		hi := lo + binWidth
		label := formatBinLabel(lo, hi)
		buckets = append(buckets, ReturnBucket{
			Label: label,
			Count: bins[i],
		})
	}

	return buckets
}

// formatBinLabel formats a bin range as "-10% to -5%", "0% to 5%", etc.
func formatBinLabel(lo, hi float64) string {
	return formatBinValue(lo) + " to " + formatBinValue(hi)
}

// formatBinValue formats a single bin boundary value.
func formatBinValue(v float64) string {
	// Round to avoid floating point artifacts.
	v = math.Round(v*100) / 100
	if v == math.Trunc(v) {
		return strconv.Itoa(int(v)) + "%"
	}
	// One decimal place for non-integer values.
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}


