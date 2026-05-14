package performance

import (
	"math"

	"github.com/govalues/decimal"
)

// DrawdownAnalysis holds drawdown statistics derived from the NAV history.
// MaxDrawdownPct is the largest peak-to-trough decline observed, expressed as
// a positive percentage (e.g. 25.50 = 25.50%). Nil when fewer than 2 data
// points or all values are non-positive.
// CurrentDrawdownPct is the drawdown from the most recent peak to the last
// data point, expressed as a positive percentage. Zero when the portfolio is
// at an all-time high. Nil when fewer than 2 data points or all values are
// non-positive.
// DrawdownDurationDays is the number of calendar days since the most recent
// peak was set. Zero when the portfolio is currently at its peak. Nil when
// fewer than 2 data points or all values are non-positive.
type DrawdownAnalysis struct {
	MaxDrawdownPct      *decimal.Decimal `json:"max_drawdown_pct,omitempty"`
	CurrentDrawdownPct  *decimal.Decimal `json:"current_drawdown_pct,omitempty"`
	DrawdownDurationDays *int             `json:"drawdown_duration_days,omitempty"`
}

// ComputeDrawdownAnalysis computes drawdown statistics from a sequence of
// NavPoints. It walks through the points tracking a running peak and computes
// the drawdown at each point as (peak - value) / peak × 100.
//
// Uses NavPerUnit for the drawdown calculation, which isolates investment
// performance from cash flow effects. This means the drawdown reflects actual
// market losses, not the impact of deposits or withdrawals.
//
// The running peak is the maximum NAV per unit seen up to each point.
// When a new peak is reached, the drawdown resets to zero.
//
// Returns nil fields when fewer than 2 data points are available or all
// values are non-positive.
func ComputeDrawdownAnalysis(points []NavPoint) DrawdownAnalysis {
	if len(points) < 2 {
		return DrawdownAnalysis{}
	}

	var (
		peakVal          decimal.Decimal
		peakIdx          int
		maxDrawdownPct   decimal.Decimal
		currentDrawdownPct decimal.Decimal
		hasPositive      bool
	)

	for i := 0; i < len(points); i++ {
		val := points[i].NavPerUnit

		if !val.IsPos() {
			continue
		}
		hasPositive = true

		// Update running peak.
		if i == 0 || peakVal.Less(val) {
			peakVal = val
			peakIdx = i
		}

		// Compute drawdown from current peak.
		diff, _ := peakVal.Sub(val)
		drawdownPct, _ := diff.Quo(peakVal)
		drawdownPct, _ = drawdownPct.Mul(decimal.MustNew(100, 0))
		drawdownPct = drawdownPct.Round(4)

		if maxDrawdownPct.Less(drawdownPct) {
			maxDrawdownPct = drawdownPct
		}

		currentDrawdownPct = drawdownPct
	}

	if !hasPositive {
		return DrawdownAnalysis{}
	}

	maxDD := maxDrawdownPct
	currentDD := currentDrawdownPct

	// Compute duration from the most recent peak to the last point.
	// When current drawdown is zero (at peak), duration is also zero.
	var durationDays int
	if !currentDrawdownPct.Equal(decimal.Zero) {
		durationDays = int(math.Ceil(points[len(points)-1].Date.Sub(points[peakIdx].Date).Hours() / 24))
	}

	return DrawdownAnalysis{
		MaxDrawdownPct:       &maxDD,
		CurrentDrawdownPct:   &currentDD,
		DrawdownDurationDays: &durationDays,
	}
}
