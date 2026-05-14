package position

import (
	"time"

	"github.com/govalues/decimal"
)

// DailyReturn holds the date and percentage return for a single trading day,
// computed from consecutive equity curve points.
type DailyReturn struct {
	Date      time.Time       `json:"date"`
	ReturnPct decimal.Decimal `json:"return_pct"`
}

// ComputeDailyReturns computes the daily return percentage between consecutive
// equity curve points. Each return is (value[i] - value[i-1]) / value[i-1] × 100,
// expressed as a percentage (e.g. 1.50 = 1.50%).
//
// Returns nil for empty input. Returns a single-element slice with 0% return
// for single-point input (no prior day to compare against).
// Skips any point whose prior value is zero or negative (undefined return).
func ComputeDailyReturns(points []EquityCurvePoint) []DailyReturn {
	if len(points) == 0 {
		return nil
	}
	if len(points) == 1 {
		return []DailyReturn{
			{Date: points[0].Date, ReturnPct: decimal.Zero},
		}
	}

	var returns []DailyReturn
	for i := 1; i < len(points); i++ {
		prev := points[i-1].PortfolioValue
		curr := points[i].PortfolioValue

		if !prev.IsPos() {
			// Prior value is zero or negative — return is undefined.
			// Skip this point.
			continue
		}

		diff, _ := curr.Sub(prev)
		returnPct, _ := diff.Quo(prev)
		returnPct, _ = returnPct.Mul(decimal.MustNew(100, 0))
		returnPct = returnPct.Round(4)

		returns = append(returns, DailyReturn{
			Date:      points[i].Date,
			ReturnPct: returnPct,
		})
	}

	return returns
}
