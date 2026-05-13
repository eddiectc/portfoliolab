package position

import (
	"time"

	"github.com/govalues/decimal"
)

// navBreakpoint holds the portfolio value just before a cash flow
// (deposit/withdrawal), used for unitization NAV computation.
type navBreakpoint struct {
	date  time.Time
	value decimal.Decimal // portfolio value in base currency, before the cash flow
}

// NavPoint is a single data point in the NAV history.
// Date is the trading day. NavPerUnit is the net asset value per unit on that
// date. Units is the total number of portfolio units outstanding. PortfolioValue
// is the total market value of the portfolio in base currency.
type NavPoint struct {
	Date           time.Time       `json:"date"`
	NavPerUnit     decimal.Decimal `json:"nav_per_unit"`
	Units          decimal.Decimal `json:"units"`
	PortfolioValue decimal.Decimal `json:"portfolio_value"`
}

// ComputeNavHistory computes the NAV (net asset value) history from an equity
// curve and cash flow breakpoints. It is a pure function with no external
// dependencies.
//
// Unitization works like a mutual fund: the portfolio is divided into units,
// and NAV per unit tracks cash-flow-independent performance. When the investor
// deposits money, new units are created at the current NAV. When the investor
// withdraws, units are redeemed at the current NAV. Between cash flows, NAV
// changes only due to market movements.
//
// The algorithm:
//   - Start with fixedUnits (10000) at the first equity curve point
//   - For each subsequent point, check for a cash flow breakpoint
//   - If a breakpoint exists: compute pre-cash-flow NAV from breakpoint value,
//     then compute new/redeemed units = cashFlow / preCashFlowNAV
//   - NAV after the cash flow equals the pre-cash-flow NAV (cash flows don't
//     change NAV; only market movements do)
//   - If no breakpoint: NAV = portfolioValue / units (market movement only)
//
// equityCurve is the full (unsliced) equity curve from inception.
// breakpoints are the pre-cash-flow portfolio values at each deposit/withdrawal
// date (excluding the initial deposit, which is handled by the fixed-units
// initialization). Breakpoints on the same date are deduplicated (first kept).
func ComputeNavHistory(equityCurve []EquityCurvePoint, breakpoints []navBreakpoint) []NavPoint {
	if len(equityCurve) == 0 {
		return nil
	}

	first := equityCurve[0]
	if !first.PortfolioValue.IsPos() {
		return nil
	}

	// Build a date → breakpoint map for O(1) lookup.
	// When multiple breakpoints exist on the same date, keep only the first
	// (before any cash flow that day), matching the TWR deduplication logic.
	bpMap := make(map[string]navBreakpoint)
	for _, bp := range breakpoints {
		key := bp.date.Format("2006-01-02")
		if _, exists := bpMap[key]; !exists {
			bpMap[key] = bp
		}
	}

	const fixedUnits int64 = 10000
	units := decimal.MustNew(fixedUnits, 0)

	var navHistory []NavPoint

	for i, point := range equityCurve {
		dateKey := point.Date.Format("2006-01-02")

		// Check if there's a cash flow (breakpoint) on this date.
		// Skip the first point — it's the initial deposit, handled by fixed units.
		if i > 0 {
			if bp, exists := bpMap[dateKey]; exists {
				// Pre-cash-flow NAV: value before the cash flow / current units.
				// The breakpoint captures market value just before the deposit/withdrawal.
				preCashFlowNAV, _ := bp.value.Quo(units)

				if preCashFlowNAV.IsPos() {
					cashFlow, _ := point.PortfolioValue.Sub(bp.value)
					if cashFlow.IsPos() {
						// Deposit: buy new units at pre-cash-flow NAV.
						newUnits, _ := cashFlow.Quo(preCashFlowNAV)
						units, _ = units.Add(newUnits)
					} else if cashFlow.IsNeg() {
						// Withdrawal: redeem units at pre-cash-flow NAV.
						withdrawalAmount := cashFlow.Abs()
						redeemedUnits, _ := withdrawalAmount.Quo(preCashFlowNAV)
						units, _ = units.Sub(redeemedUnits)
						if units.Less(decimal.Zero) {
							units = decimal.Zero
						}
					}
				}
			}
		}

		// NAV = portfolioValue / units (always recalculated from current state).
		var nav decimal.Decimal
		if units.IsPos() {
			nav, _ = point.PortfolioValue.Quo(units)
		}

		navHistory = append(navHistory, NavPoint{
			Date:           point.Date,
			NavPerUnit:     nav,
			Units:          units,
			PortfolioValue: point.PortfolioValue,
		})
	}

	return navHistory
}
