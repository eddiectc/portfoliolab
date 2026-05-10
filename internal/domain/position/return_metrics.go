package position

import (
	"math"

	"github.com/govalues/decimal"
)

// ComputePeriodReturn computes summary return metrics from an equity curve.
// It is a pure function with no external dependencies.
//
// PeriodReturnPct = (end_value - begin_value) / begin_value × 100
//   - nil when begin_value is zero or non-positive (N/A)
//
// AnnualizedReturnPct (CAGR) = (end_value / begin_value)^(365 / days) - 1
//   - nil when fewer than 2 data points (insufficient data)
//   - nil when begin_value is zero or non-positive
//   - uses math.Pow for the exponentiation step (float64) then converts back to decimal
//
// HasInsufficientData is true when fewer than 2 data points are available.
func ComputePeriodReturn(equityCurve []EquityCurvePoint, baseCurrency string) ReturnMetrics {
	if len(equityCurve) < 2 {
		return ReturnMetrics{
			PeriodReturnPct:     nil,
			AnnualizedReturnPct: nil,
			HasInsufficientData: true,
		}
	}

	var metrics ReturnMetrics

	// --- Period Return ---
	// (end_value - begin_value) / begin_value × 100
	first := equityCurve[0]
	last := equityCurve[len(equityCurve)-1]
	beginValue := first.PortfolioValue

	if beginValue.IsPos() {
		profit, _ := last.PortfolioValue.Sub(beginValue)
		hundred := decimal.MustNew(100, 0)
		ratio, _ := profit.Quo(beginValue)
		periodReturn, _ := ratio.Mul(hundred)
		// Round to 2 decimal places for display.
		metrics.PeriodReturnPct = ptrDec(periodReturn.Round(2))
	}
	// If begin_value is zero or non-positive, PeriodReturnPct stays nil (N/A).

	// --- CAGR ---
	endValue := last.PortfolioValue

	if beginValue.IsPos() {
		days := last.Date.Sub(first.Date).Hours() / 24.0
		if days > 0 {
			// CAGR = (end / begin)^(365 / days) - 1
			// Convert to float64 for math.Pow, then back to decimal.
			beginF, ok := beginValue.Float64()
			if !ok {
				beginF = 0
			}
			endF, ok := endValue.Float64()
			if !ok {
				endF = 0
			}
			if beginF > 0 {
				ratio := endF / beginF
				exponent := 365.0 / days
				cagr := math.Pow(ratio, exponent) - 1.0
				cagrPct, _ := decimal.NewFromFloat64(cagr * 100.0)
				metrics.AnnualizedReturnPct = ptrDec(cagrPct.Round(2))
			}
		}
	}
	// If begin_value is zero or same date, AnnualizedReturnPct stays nil.

	return metrics
}

// ptrDec returns a pointer to the given decimal.Decimal.
func ptrDec(d decimal.Decimal) *decimal.Decimal {
	return &d
}
