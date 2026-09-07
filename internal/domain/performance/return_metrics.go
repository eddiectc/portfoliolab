package performance

import (
	"math"
	"time"

	"github.com/govalues/decimal"
)

// ComputePeriodReturn computes summary return metrics from an equity curve
// using the Time-Weighted Return (TWR) and Money-Weighted Return (MWR) methods.
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
// MWR (Internal Rate of Return) finds the discount rate r that makes the
// net present value of all cash flows plus the terminal portfolio value
// equal to zero. Unlike TWR, MWR is affected by the timing and magnitude
// of deposits/withdrawals.
//
// TWRPct is nil when fewer than 2 data points or begin_value is non-positive.
// AnnualizedTWRPct is the annualized TWR: (1 + TWR)^(365/days) - 1.
// MWRPct is nil when fewer than 2 data points or begin_value is non-positive.
// HoldingPeriodMWRPct is the MWR expressed as a holding-period return.
// HasInsufficientData is true when fewer than 2 data points are available.
func ComputePeriodReturn(
	equityCurve []EquityCurvePoint,
	preCashFlowValues []twrBreakpoint,
	baseCurrency string, //nolint:revive // reserved for future multi-currency MWR
) ReturnMetrics {
	if len(equityCurve) < 2 {
		return ReturnMetrics{
			ProfitLoss:          nil,
			TWRPct:              nil,
			AnnualizedTWRPct:    nil,
			MWRPct:              nil,
			HoldingPeriodMWRPct: nil,
			HasInsufficientData: true,
		}
	}

	first := equityCurve[0]
	last := equityCurve[len(equityCurve)-1]
	beginValue := first.PortfolioValue

	if !beginValue.IsPos() {
		return ReturnMetrics{
			ProfitLoss:          nil,
			TWRPct:              nil,
			AnnualizedTWRPct:    nil,
			MWRPct:              nil,
			HoldingPeriodMWRPct: nil,
		}
	}

	// Build a date → portfolio value map from the equity curve for O(1) lookup.
	curveMap := buildCurveMap(equityCurve)

	// Deduplicate breakpoints: when there are multiple cash flows on the same
	// date, only the first pre-cash-flow snapshot (before any cash flow that
	// day) is needed for TWR. Subsequent snapshots on the same date would
	// create bogus sub-periods using the same post-value as the "from" point.
	preCashFlowValues = deduplicateBreakpoints(preCashFlowValues)

	// --- TWR ---
	twr := computeTWR(first, last, preCashFlowValues, curveMap)

	// --- MWR ---
	mwr := computeMWR(equityCurve)

	// --- Annualized ---
	days := last.Date.Sub(first.Date).Hours() / 24.0

	var metrics ReturnMetrics
	if twr != nil {
		metrics.TWRPct = twr
	}
	if days > 0 && twr != nil {
		twrF, _ := twr.Float64()
		twrF /= 100.0 // convert from percentage to ratio
		annualized := math.Pow(1.0+twrF, 365.0/days) - 1.0
		annPct, _ := decimal.NewFromFloat64(annualized * 100.0)
		metrics.AnnualizedTWRPct = ptrDec(annPct.Round(2))
	}
	if mwr != nil {
		metrics.MWRPct = mwr
		// Holding-period MWR: (1 + MWR)^(days/365) - 1
		if days >= 0 {
			mwrF, _ := mwr.Float64()
			mwrF /= 100.0 // convert from percentage to ratio
			hpr := math.Pow(1.0+mwrF, days/365.0) - 1.0
			hprPct, _ := decimal.NewFromFloat64(hpr * 100.0)
			metrics.HoldingPeriodMWRPct = ptrDec(hprPct.Round(2))
		}
	}

	// --- Profit/Loss ---
	// Absolute profit or loss: current portfolio value minus total net deposit.
	// Always available when there are at least 2 data points.
	profit, _ := last.PortfolioValue.Sub(last.NetDeposit)
	metrics.ProfitLoss = &profit

	// --- Simple return ---
	// Total profit (PortfolioValue - NetDeposit) as a percentage of
	// total NetDeposit. Answers "for every unit of currency deposited,
	// how much profit was made?" Always defined as long as net deposit > 0.
	if last.NetDeposit.IsPos() {
		profitF, _ := profit.Float64()
		ndF, _ := last.NetDeposit.Float64()
		pct, _ := decimal.NewFromFloat64((profitF / ndF) * 100.0)
		metrics.SimpleReturnPct = ptrDec(pct.Round(2))
		metrics.AnnualizedSimpleReturnPct = ComputeAnnualizedSimpleReturn(metrics.SimpleReturnPct, first, last)
	}

	return metrics
}

// curveDateMap maps date string (YYYY-MM-DD) → portfolio value.
type curveDateMap map[string]decimal.Decimal

// deduplicateBreakpoints keeps only the first breakpoint per date.
// When there are multiple cash flows on the same date, each creates a
// pre-cash-flow snapshot, but for TWR we only need the first one (before
// any cash flow that day). The post-cash-flow value on that date (from
// the equity curve) captures the total effect of all cash flows.
func deduplicateBreakpoints(bps []twrBreakpoint) []twrBreakpoint {
	if len(bps) <= 1 {
		return bps
	}
	result := make([]twrBreakpoint, 0, len(bps))
	seen := make(map[string]bool, len(bps))
	for _, bp := range bps {
		key := bp.date.Format("2006-01-02")
		if !seen[key] {
			seen[key] = true
			result = append(result, bp)
		}
	}
	return result
}

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
		return computeValueReturn(first, last)
	}

	// Geometrically link sub-period return ratios.
	// Each ratio is expressed as a decimal multiplier (e.g., 1.15 for 15% gain).
	// We use float64 for the product to avoid precision issues with many sub-periods.
	product := 1.0

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

// computeValueReturn computes the raw portfolio value return:
// (endValue - beginValue) / beginValue × 100.
// Used as the TWR fallback when there are no cash flows.
func computeValueReturn(first, last EquityCurvePoint) *decimal.Decimal {
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

// mwrCashFlow represents a single cash flow for MWR computation.
type mwrCashFlow struct {
	timeYears float64
	amount    float64
}

// computeMWR computes the Money-Weighted Return (Internal Rate of Return)
// from the equity curve. It finds the annualized discount rate r that makes
// the net present value of all cash flows plus the terminal portfolio value
// equal to zero.
//
// Cash flows are derived from the NetDeposit column of the equity curve:
//   - Initial portfolio value (negative, at t=0)
//   - Incremental net deposits between consecutive points (negative = deposit,
//     positive = withdrawal, at their respective times)
//   - Final portfolio value (positive, at t=T)
//
// The equation solved is:
//
//	-PV_0 + Σ(CF_i / (1+r)^t_i) + PV_T / (1+r)^T = 0
//
// where t_i is the time in years from the start date.
//
// Returns nil if computation is not possible (fewer than 2 points, zero
// begin value, or no solution found within bounds).
func computeMWR(curve []EquityCurvePoint) *decimal.Decimal {
	if len(curve) < 2 {
		return nil
	}

	first := curve[0]
	last := curve[len(curve)-1]

	beginF, _ := first.PortfolioValue.Float64()
	if beginF <= 0 {
		return nil
	}

	endF, _ := last.PortfolioValue.Float64()

	// Build cash flows: (time in years, amount)
	// CF_0 = -beginValue at t=0
	// CF_i = -(NetDeposit[i] - NetDeposit[i-1]) at t_i (negative = deposit)
	// CF_T = +endValue at t=T
	var cfs []mwrCashFlow
	cfs = append(cfs, mwrCashFlow{timeYears: 0, amount: -beginF})

	// Initialize prevNetDeposit to the first point's value so we only capture
	// incremental cash flows within the period, not the initial deposit.
	prevNetDeposit, _ := first.NetDeposit.Float64()
	for i := 1; i < len(curve); i++ {
		nd, _ := curve[i].NetDeposit.Float64()
		daysFromStart := curve[i].Date.Sub(first.Date).Hours() / 24.0
		timeYears := daysFromStart / 365.0

		// Incremental net deposit: positive = deposit (money in),
		// negative = withdrawal (money out).
		// For MWR: deposit is a cash outflow (negative), withdrawal is inflow (positive).
		incremental := nd - prevNetDeposit
		if incremental != 0 {
			cfs = append(cfs, mwrCashFlow{timeYears: timeYears, amount: -incremental})
		}
		prevNetDeposit = nd
	}

	// Terminal value (positive = money back).
	totalDays := last.Date.Sub(first.Date).Hours() / 24.0
	cfs = append(cfs, mwrCashFlow{timeYears: totalDays / 365.0, amount: endF})

	// Solve for r using bisection.
	r := solveIRR(cfs)
	if r == nil {
		return nil
	}

	mwrPct, _ := decimal.NewFromFloat64(*r * 100.0)
	return ptrDec(mwrPct.Round(2))
}

// solveIRR finds the internal rate of return using bisection.
// It searches for r in (-0.99, maxRate) such that NPV(r) ≈ 0.
// Returns nil if no solution is found within the search bounds.
func solveIRR(cfs []mwrCashFlow) *float64 {
	const maxRate = 10.0 // up to 1000%
	const iterations = 100

	lo, hi := -0.99, maxRate
	fLo := npv(cfs, lo)

	// Quick check: if NPV at max rate is negative, return is beyond our bounds.
	// This means the portfolio lost so much that no positive rate explains it.
	fHi := npv(cfs, hi)
	if fLo*fHi > 0 {
		// Both same sign — no root in range.
		return nil
	}

	for i := 0; i < iterations; i++ {
		mid := (lo + hi) / 2.0
		fMid := npv(cfs, mid)
		if math.Abs(fMid) < 1e-9 {
			return &mid
		}
		if fLo*fMid < 0 {
			hi = mid
		} else {
			lo = mid
			fLo = fMid
		}
	}

	mid := (lo + hi) / 2.0
	return &mid
}

// npv computes the net present value of cash flows at the given discount rate.
func npv(cfs []mwrCashFlow, r float64) float64 {
	var sum float64
	for _, cf := range cfs {
		sum += cf.amount / math.Pow(1.0+r, cf.timeYears)
	}
	return sum
}

// ComputeAnnualizedSimpleReturn computes the annualized simple return as a
// percentage. It takes the simple return (as a percentage) and annualizes it
// using the number of days between the first and last equity curve points:
//
//	Annualized = ((1 + simpleReturn)^(365/days) - 1) × 100
//
// Returns nil when the simple return is nil or when zero days elapsed.
func ComputeAnnualizedSimpleReturn(simpleReturn *decimal.Decimal, first, last EquityCurvePoint) *decimal.Decimal {
	if simpleReturn == nil {
		return nil
	}

	days := last.Date.Sub(first.Date).Hours() / 24.0
	if days <= 0 {
		return nil
	}

	simpleF, _ := simpleReturn.Float64()
	simpleF /= 100.0 // convert from percentage to ratio

	annualized := math.Pow(1.0+simpleF, 365.0/days) - 1.0
	pct, _ := decimal.NewFromFloat64(annualized * 100.0)
	return ptrDec(pct.Round(2))
}
