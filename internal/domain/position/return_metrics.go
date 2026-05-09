package position

import (
	"math"

	"github.com/govalues/decimal"
)

// ComputeReturnMetrics computes summary return metrics from an equity curve.
// It is a pure function with no external dependencies.
//
// TotalReturnPct = (current_value - net_deposit) / net_deposit × 100
//   - nil when net_deposit is zero or negative (N/A)
//
// AnnualizedReturnPct (CAGR) = (end_value / begin_value)^(365 / days) - 1
//   - nil when fewer than 2 data points (insufficient data)
//   - nil when begin_value is zero or non-positive
//   - uses math.Pow for the exponentiation step (float64) then converts back to decimal
//
// HasInsufficientData is true when fewer than 2 data points are available.
func ComputeReturnMetrics(equityCurve []EquityCurvePoint, baseCurrency string) ReturnMetrics {
	if len(equityCurve) < 2 {
		return ReturnMetrics{
			TotalReturnPct:      nil,
			AnnualizedReturnPct: nil,
			HasInsufficientData: true,
		}
	}

	var metrics ReturnMetrics

	// --- Total Return ---
	// Use the last data point's portfolio value and net deposit.
	last := equityCurve[len(equityCurve)-1]
	netDeposit := last.NetDeposit

	if netDeposit.IsPos() {
		// (current_value - net_deposit) / net_deposit × 100
		profit, _ := last.PortfolioValue.Sub(netDeposit)
		hundred := decimal.MustNew(100, 0)
		ratio, _ := profit.Quo(netDeposit)
		totalReturn, _ := ratio.Mul(hundred)
		// Round to 2 decimal places for display.
		metrics.TotalReturnPct = ptrDec(totalReturn.Round(2))
	}
	// If net_deposit is zero or negative, TotalReturnPct stays nil (N/A).

	// --- CAGR ---
	first := equityCurve[0]
	beginValue := first.PortfolioValue
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
