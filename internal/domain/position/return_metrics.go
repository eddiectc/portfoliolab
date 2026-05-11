package position

import (
	"math"
	"time"

	"github.com/govalues/decimal"
)

// ComputePeriodReturn computes summary return metrics from an equity curve
// using the Time-Weighted Return (TWR) method.
// It is a pure function with no external dependencies.
//
// TWR isolates investment performance from the timing of deposits/withdrawals
// by breaking the period into sub-periods between cash flow events and
// geometrically linking the sub-period returns:
//
// TWR = (V_pre[0] / V_first) × (V_pre[1] / V_post[0]) × ... × (V_last / V_post[n-1]) - 1
//
// where V_pre[i] is the portfolio value just before cash flow i,
// V_post[i] is the portfolio value on the same date (from the equity curve),
// and V_first/V_last are the first/last equity curve points.
//
// When there are no cash flows, TWR degenerates to the simple return:
// (V_last / V_first) - 1.
//
// TWRPct is nil when fewer than 2 data points or begin_value is non-positive.
// AnnualizedTWRPct is the annualized TWR: (1 + TWR)^(365/days) - 1.
// HasInsufficientData is true when fewer than 2 data points are available.
func ComputePeriodReturn(
	equityCurve []EquityCurvePoint,
	preCashFlowValues []twrBreakpoint,
	baseCurrency string,
) ReturnMetrics {
	if len(equityCurve) < 2 {
		return ReturnMetrics{
			TWRPct:              nil,
			AnnualizedTWRPct:    nil,
			HasInsufficientData: true,
		}
	}

	first := equityCurve[0]
	last := equityCurve[len(equityCurve)-1]
	beginValue := first.PortfolioValue

	if !beginValue.IsPos() {
		return ReturnMetrics{
			TWRPct:           nil,
			AnnualizedTWRPct: nil,
		}
	}

	// Build a date → portfolio value map from the equity curve for O(1) lookup.
	curveMap := buildCurveMap(equityCurve)

	// --- TWR ---
	twr := computeTWR(first, last, preCashFlowValues, curveMap)

	var metrics ReturnMetrics
	if twr != nil {
		metrics.TWRPct = twr
	}

	// --- Annualized TWR ---
	days := last.Date.Sub(first.Date).Hours() / 24.0
	if days > 0 && twr != nil {
		twrF, _ := twr.Float64()
		twrF /= 100.0 // convert from percentage to ratio
		annualized := math.Pow(1.0+twrF, 365.0/days) - 1.0
		annPct, _ := decimal.NewFromFloat64(annualized * 100.0)
		metrics.AnnualizedTWRPct = ptrDec(annPct.Round(2))
	}

	return metrics
}

// curveDateMap maps date string (YYYY-MM-DD) → portfolio value.
type curveDateMap map[string]decimal.Decimal

// buildCurveMap builds a date → portfolio value map from the equity curve.
func buildCurveMap(curve []EquityCurvePoint) curveDateMap {
	m := make(curveDateMap, len(curve))
	for _, p := range curve {
		key := p.Date.Format("2006-01-02")
		m[key] = p.PortfolioValue
	}
	return m
}

// computeTWR computes the Time-Weighted Return as a percentage.
// Returns nil if computation is not possible.
//
// TWR breaks the period at each cash flow and geometrically links
// sub-period returns:
//
// TWR = ∏(V_post[i] / V_pre[i]) - 1
//
// where V_pre[i] is the portfolio value just before cash flow i,
// V_post[i] is the value on the same date after the cash flow,
// V_pre[0] = first equity curve point, V_post[n] = last equity curve point.
//
// When the first breakpoint has value=0 (initial deposit, no prior
// portfolio), the pre-deposit sub-period is skipped and the first
// sub-period starts from the post-deposit value.
func computeTWR(
	first, last EquityCurvePoint,
	breakpoints []twrBreakpoint,
	curveMap curveDateMap,
) *decimal.Decimal {
	endValue := last.PortfolioValue

	// No cash flows: simple return.
	if len(breakpoints) == 0 {
		return computeSimpleReturn(first, last)
	}

	// Geometrically link sub-period return ratios.
	// Each ratio is expressed as a decimal multiplier (e.g., 1.15 for 15% gain).
	// We use float64 for the product to avoid precision issues with many sub-periods.
	var product float64 = 1.0

	// --- First sub-period ---
	// If the first breakpoint has value=0 (initial deposit with no prior
	// portfolio), skip the pre-deposit sub-period and start from the
	// post-cash-flow value on that date.
	firstPre := breakpoints[0]
	handledUpTo := 0 // index of the last breakpoint handled in the first sub-period
	if !firstPre.value.IsPos() {
		// Use post-cash-flow value on the first breakpoint's date as the
		// starting point for the first measurable sub-period.
		postFirst := lookupCurveValue(curveMap, firstPre.date)
		if postFirst == nil || !postFirst.IsPos() {
			return nil
		}

		if len(breakpoints) == 1 {
			// Single breakpoint (initial deposit only): return from post-deposit to end.
			r := ratioFloat(*postFirst, endValue)
			if r <= 0 {
				return nil
			}
			product *= r
			handledUpTo = 1 // past the last breakpoint, so skip final sub-period
		} else {
			// Return from post-deposit to next pre-cash-flow.
			nextPre := breakpoints[1].value
			r := ratioFloat(*postFirst, nextPre)
			if r <= 0 {
				return nil
			}
			product *= r
				handledUpTo = 1
		}
	} else {
		// Normal case: return from first equity curve point to first pre-cash-flow.
		r := ratioFloat(first.PortfolioValue, firstPre.value)
		if r <= 0 {
			return nil
		}
		product *= r
	}

	// --- Middle sub-periods ---
	// post-cash-flow[i-1] → pre-cash-flow[i]
	startIdx := handledUpTo
	if startIdx == 0 && !firstPre.value.IsPos() {
		startIdx = 1 // skip the first breakpoint (already handled above)
	}
	for i := startIdx + 1; i < len(breakpoints); i++ {
		postPrev := lookupCurveValue(curveMap, breakpoints[i-1].date)
		if postPrev == nil || !postPrev.IsPos() {
			return nil
		}
		preCurr := breakpoints[i].value
		r := ratioFloat(*postPrev, preCurr)
		if r <= 0 {
			return nil
		}
		product *= r
	}

	// --- Final sub-period ---
	// post-cash-flow[n-1] → last equity curve point
	// Skip if the first sub-period already covered to the end (single breakpoint case).
	if handledUpTo < len(breakpoints) {
		lastPre := breakpoints[len(breakpoints)-1]
		postLast := lookupCurveValue(curveMap, lastPre.date)
		if postLast == nil || !postLast.IsPos() {
			return nil
		}
		ratioLast := ratioFloat(*postLast, endValue)
		if ratioLast <= 0 {
			return nil
		}
		product *= ratioLast
	}

	// TWR = product - 1, expressed as percentage.
	twr := product - 1.0
	twrPct, _ := decimal.NewFromFloat64(twr * 100.0)
	return ptrDec(twrPct.Round(2))
}

// computeSimpleReturn computes (end - begin) / begin × 100.
func computeSimpleReturn(first, last EquityCurvePoint) *decimal.Decimal {
	beginValue := first.PortfolioValue
	if !beginValue.IsPos() {
		return nil
	}
	beginF, _ := beginValue.Float64()
	endF, _ := last.PortfolioValue.Float64()
	if beginF <= 0 {
		return nil
	}
	ratio := endF / beginF
	twrPct, _ := decimal.NewFromFloat64((ratio - 1.0) * 100.0)
	return ptrDec(twrPct.Round(2))
}

// lookupCurveValue finds the portfolio value for a given date in the curve map.
func lookupCurveValue(m curveDateMap, date time.Time) *decimal.Decimal {
	key := date.Format("2006-01-02")
	val, ok := m[key]
	if !ok {
		return nil
	}
	return &val
}

// ratioFloat computes b/a as float64. Returns 0 if a or b is non-positive.
func ratioFloat(a, b decimal.Decimal) float64 {
	aF, okA := a.Float64()
	bF, okB := b.Float64()
	if !okA || !okB || aF <= 0 || bF <= 0 {
		return 0
	}
	return bF / aF
}

// ptrDec returns a pointer to the given decimal.Decimal.
func ptrDec(d decimal.Decimal) *decimal.Decimal {
	return &d
}
