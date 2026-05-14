package position

import (
	"testing"

	"github.com/govalues/decimal"
)

func assertDrawdown(t *testing.T, got DrawdownAnalysis, wantMax, wantCurrent *decimal.Decimal, wantDuration *int) {
	t.Helper()

	if wantMax == nil {
		if got.MaxDrawdownPct != nil {
			t.Errorf("MaxDrawdownPct: got %s, want nil", got.MaxDrawdownPct.String())
		}
	} else if !got.MaxDrawdownPct.Equal(*wantMax) {
		t.Errorf("MaxDrawdownPct: got %s, want %s", got.MaxDrawdownPct.String(), wantMax.String())
	}

	if wantCurrent == nil {
		if got.CurrentDrawdownPct != nil {
			t.Errorf("CurrentDrawdownPct: got %s, want nil", got.CurrentDrawdownPct.String())
		}
	} else if !got.CurrentDrawdownPct.Equal(*wantCurrent) {
		t.Errorf("CurrentDrawdownPct: got %s, want %s", got.CurrentDrawdownPct.String(), wantCurrent.String())
	}

	if wantDuration == nil {
		if got.DrawdownDurationDays != nil {
			t.Errorf("DrawdownDurationDays: got %d, want nil", *got.DrawdownDurationDays)
		}
	} else if *got.DrawdownDurationDays != *wantDuration {
		t.Errorf("DrawdownDurationDays: got %d, want %d", *got.DrawdownDurationDays, *wantDuration)
	}
}

func TestComputeDrawdownAnalysis_NoDrawdown(t *testing.T) {
	// Flat portfolio — no drawdown at all.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1000000, 2)}, // $10,000
	}

	result := ComputeDrawdownAnalysis(points)

	zero := decimal.Zero
	dur := 0
	assertDrawdown(t, result, &zero, &zero, &dur)
}

func TestComputeDrawdownAnalysis_RisingPortfolio(t *testing.T) {
	// Continuously rising — peak keeps moving forward, drawdown always zero.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1100000, 2)}, // $11,000
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1200000, 2)}, // $12,000
	}

	result := ComputeDrawdownAnalysis(points)

	zero := decimal.Zero
	dur := 0
	assertDrawdown(t, result, &zero, &zero, &dur)
}

func TestComputeDrawdownAnalysis_FullDrawdown(t *testing.T) {
	// Peak at $12,000, then drops to $6,000 (50% drawdown).
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1200000, 2)}, // $12,000 (peak)
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(900000, 2)},  // $9,000 (25% DD)
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(600000, 2)},  // $6,000 (50% DD)
	}

	result := ComputeDrawdownAnalysis(points)

	wantMax := decimal.MustParse("50.00")
	wantCurrent := decimal.MustParse("50.00")
	dur := 2 // Jan 4 - Jan 2 = 2 days
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}

func TestComputeDrawdownAnalysis_RecoveringDrawdown(t *testing.T) {
	// Peak at $12,000, trough at $6,000 (50%), recovers to $9,000 (25% from peak).
	// Max drawdown stays 50%, current drawdown is 25%.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1200000, 2)}, // $12,000 (peak)
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(600000, 2)},  // $6,000 (50% DD)
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(900000, 2)},  // $9,000 (25% DD)
	}

	result := ComputeDrawdownAnalysis(points)

	wantMax := decimal.MustParse("50.00")
	wantCurrent := decimal.MustParse("25.00")
	dur := 2 // Jan 4 - Jan 2 = 2 days
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}

func TestComputeDrawdownAnalysis_CurrentDrawdownDuration(t *testing.T) {
	// Peak on Jan 1, then declining over a week.
	// Duration should be 7 calendar days.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000 (peak)
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(990000, 2)},  // $9,900
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(970000, 2)},  // $9,700
		{Date: mustTime("2024-01-06"), PortfolioValue: dec(950000, 2)},  // $9,500
		{Date: mustTime("2024-01-07"), PortfolioValue: dec(940000, 2)},  // $9,400
		{Date: mustTime("2024-01-08"), PortfolioValue: dec(930000, 2)},  // $9,300
	}

	result := ComputeDrawdownAnalysis(points)

	// Max drawdown = (10000 - 9300) / 10000 * 100 = 7.00%
	wantMax := decimal.MustParse("7.00")
	// Current drawdown = same as max (still declining)
	wantCurrent := decimal.MustParse("7.00")
	dur := 7 // Jan 8 - Jan 1 = 7 days
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}

func TestComputeDrawdownAnalysis_NewPeakResetsCurrent(t *testing.T) {
	// Drawdown then new peak — current drawdown resets to 0, duration = 0.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(800000, 2)},  // $8,000 (20% DD)
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1200000, 2)}, // $12,000 (new peak)
	}

	result := ComputeDrawdownAnalysis(points)

	// Max drawdown = 20% (from first peak)
	wantMax := decimal.MustParse("20.00")
	// Current drawdown = 0 (at new peak)
	wantCurrent := decimal.Zero
	dur := 0
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}

func TestComputeDrawdownAnalysis_Empty(t *testing.T) {
	result := ComputeDrawdownAnalysis([]NavPoint{})

	if result.MaxDrawdownPct != nil {
		t.Errorf("MaxDrawdownPct: got %s, want nil", result.MaxDrawdownPct.String())
	}
	if result.CurrentDrawdownPct != nil {
		t.Errorf("CurrentDrawdownPct: got %s, want nil", result.CurrentDrawdownPct.String())
	}
	if result.DrawdownDurationDays != nil {
		t.Errorf("DrawdownDurationDays: got %d, want nil", *result.DrawdownDurationDays)
	}
}

func TestComputeDrawdownAnalysis_SinglePoint(t *testing.T) {
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)},
	}

	result := ComputeDrawdownAnalysis(points)

	if result.MaxDrawdownPct != nil {
		t.Errorf("MaxDrawdownPct: got %s, want nil", result.MaxDrawdownPct.String())
	}
	if result.CurrentDrawdownPct != nil {
		t.Errorf("CurrentDrawdownPct: got %s, want nil", result.CurrentDrawdownPct.String())
	}
	if result.DrawdownDurationDays != nil {
		t.Errorf("DrawdownDurationDays: got %d, want nil", *result.DrawdownDurationDays)
	}
}

func TestComputeDrawdownAnalysis_AllZeroValues(t *testing.T) {
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: decimal.Zero},
		{Date: mustTime("2024-01-02"), PortfolioValue: decimal.Zero},
	}

	result := ComputeDrawdownAnalysis(points)

	if result.MaxDrawdownPct != nil {
		t.Errorf("MaxDrawdownPct: got %s, want nil", result.MaxDrawdownPct.String())
	}
}

func TestComputeDrawdownAnalysis_MultipleDrawdownPeriods(t *testing.T) {
	// Two drawdown periods: first 30%, second 15%. Max should be 30%.
	// Current should be 15% (from the second peak).
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1000000, 2)}, // $10,000 (peak 1)
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(700000, 2)},  // $7,000 (30% DD)
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(1100000, 2)}, // $11,000 (new peak)
		{Date: mustTime("2024-01-05"), PortfolioValue: dec(935000, 2)},  // $9,350 (15% DD from $11,000)
	}

	result := ComputeDrawdownAnalysis(points)

	wantMax := decimal.MustParse("30.00")
	wantCurrent := decimal.MustParse("15.00")
	dur := 1 // Jan 5 - Jan 4 = 1 day
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}
