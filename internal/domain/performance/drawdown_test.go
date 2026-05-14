package performance

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
	// Flat NAV — no drawdown at all.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-03"), NavPerUnit: decimal.MustParse("1.00")},
	}

	result := ComputeDrawdownAnalysis(points)

	zero := decimal.Zero
	dur := 0
	assertDrawdown(t, result, &zero, &zero, &dur)
}

func TestComputeDrawdownAnalysis_RisingPortfolio(t *testing.T) {
	// Continuously rising NAV — peak keeps moving forward, drawdown always zero.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.MustParse("1.10")},
		{Date: mustTime("2024-01-03"), NavPerUnit: decimal.MustParse("1.20")},
	}

	result := ComputeDrawdownAnalysis(points)

	zero := decimal.Zero
	dur := 0
	assertDrawdown(t, result, &zero, &zero, &dur)
}

func TestComputeDrawdownAnalysis_FullDrawdown(t *testing.T) {
	// Peak NAV at 1.20, then drops to 0.60 (50% drawdown).
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.MustParse("1.20")}, // peak
		{Date: mustTime("2024-01-03"), NavPerUnit: decimal.MustParse("0.90")}, // 25% DD
		{Date: mustTime("2024-01-04"), NavPerUnit: decimal.MustParse("0.60")}, // 50% DD
	}

	result := ComputeDrawdownAnalysis(points)

	wantMax := decimal.MustParse("50.00")
	wantCurrent := decimal.MustParse("50.00")
	dur := 2 // Jan 4 - Jan 2 = 2 days
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}

func TestComputeDrawdownAnalysis_RecoveringDrawdown(t *testing.T) {
	// Peak NAV at 1.20, trough at 0.60 (50%), recovers to 0.90 (25% from peak).
	// Max drawdown stays 50%, current drawdown is 25%.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.MustParse("1.20")}, // peak
		{Date: mustTime("2024-01-03"), NavPerUnit: decimal.MustParse("0.60")}, // 50% DD
		{Date: mustTime("2024-01-04"), NavPerUnit: decimal.MustParse("0.90")}, // 25% DD
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
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.0000")}, // peak
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.MustParse("0.9900")},
		{Date: mustTime("2024-01-03"), NavPerUnit: decimal.MustParse("0.9700")},
		{Date: mustTime("2024-01-06"), NavPerUnit: decimal.MustParse("0.9500")},
		{Date: mustTime("2024-01-07"), NavPerUnit: decimal.MustParse("0.9400")},
		{Date: mustTime("2024-01-08"), NavPerUnit: decimal.MustParse("0.9300")},
	}

	result := ComputeDrawdownAnalysis(points)

	// Max drawdown = (1.00 - 0.93) / 1.00 * 100 = 7.00%
	wantMax := decimal.MustParse("7.00")
	// Current drawdown = same as max (still declining)
	wantCurrent := decimal.MustParse("7.00")
	dur := 7 // Jan 8 - Jan 1 = 7 days
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}

func TestComputeDrawdownAnalysis_NewPeakResetsCurrent(t *testing.T) {
	// Drawdown then new peak — current drawdown resets to 0, duration = 0.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.MustParse("0.80")}, // 20% DD
		{Date: mustTime("2024-01-03"), NavPerUnit: decimal.MustParse("1.20")}, // new peak
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
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.00")},
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
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.Zero},
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.Zero},
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
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.MustParse("1.0000")},
		{Date: mustTime("2024-01-02"), NavPerUnit: decimal.MustParse("1.0000")}, // peak 1
		{Date: mustTime("2024-01-03"), NavPerUnit: decimal.MustParse("0.7000")}, // 30% DD
		{Date: mustTime("2024-01-04"), NavPerUnit: decimal.MustParse("1.1000")}, // new peak
		{Date: mustTime("2024-01-05"), NavPerUnit: decimal.MustParse("0.9350")}, // 15% DD from 1.10
	}

	result := ComputeDrawdownAnalysis(points)

	wantMax := decimal.MustParse("30.00")
	wantCurrent := decimal.MustParse("15.00")
	dur := 1 // Jan 5 - Jan 4 = 1 day
	assertDrawdown(t, result, &wantMax, &wantCurrent, &dur)
}
