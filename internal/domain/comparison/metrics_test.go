package comparison

import (
	"math"
	"testing"
	"time"

	"github.com/govalues/decimal"
)

// --- helpers ---

func eqPointsFromFloats(t *testing.T, base time.Time, values []float64) []EquityCurvePoint {
	t.Helper()
	points := make([]EquityCurvePoint, len(values))
	for i, v := range values {
		d, _ := decimal.NewFromFloat64(v)
		points[i] = EquityCurvePoint{
			Date:           base.AddDate(0, 0, i), // one day apart
			PortfolioValue: d,
		}
	}
	return points
}

func eqPointsFromDatesAndValues(t *testing.T, dates []time.Time, values []float64) []EquityCurvePoint {
	t.Helper()
	if len(dates) != len(values) {
		t.Fatal("dates and values length mismatch")
	}
	points := make([]EquityCurvePoint, len(dates))
	for i, v := range values {
		d, _ := decimal.NewFromFloat64(v)
		points[i] = EquityCurvePoint{
			Date:           dates[i],
			PortfolioValue: d,
		}
	}
	return points
}

func ptrDecF(t *testing.T, f float64) *decimal.Decimal {
	t.Helper()
	d, _ := decimal.NewFromFloat64(f)
	d = d.Round(2)
	return &d
}

func ptrDec4F(t *testing.T, f float64) *decimal.Decimal {
	t.Helper()
	d, _ := decimal.NewFromFloat64(f)
	d = d.Round(4)
	return &d
}

func approxEqual(t *testing.T, got, want *decimal.Decimal, epsilon float64) bool {
	t.Helper()
	if got == nil && want == nil {
		return true
	}
	if (got == nil) != (want == nil) {
		return false
	}
	gotF, _ := got.Float64()
	wantF, _ := want.Float64()
	return math.Abs(gotF-wantF) <= epsilon
}

// --- ComputeCAGR ---

func TestComputeCAGR(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		values      []float64
		wantCAGR    *decimal.Decimal
		wantDays    int
	}{
		{
			name:     "up 10% in 365 days",
			values:   makeUp366Days(10000, 11000),
			wantCAGR: ptrDecF(t, 10.0),
			wantDays: 365,
		},
		{
			name:     "up 50% over 364 days",
			values:   makeUp365Days(10000, 15000),
			wantCAGR: ptrDecF(t, 50.0),
			wantDays: 364,
		},
		{
			name:     "down 20% over 364 days",
			values:   makeUp365Days(10000, 8000),
			wantCAGR: ptrDecF(t, -20.0),
			wantDays: 364,
		},
		{
			name:     "flat over 364 days",
			values:   makeUp365Days(10000, 10000),
			wantCAGR: ptrDecF(t, 0.0),
			wantDays: 364,
		},
		{
			name:     "single point",
			values:   []float64{10000},
			wantCAGR: nil,
			wantDays: 0,
		},
		{
			name:     "empty",
			values:   []float64{},
			wantCAGR: nil,
			wantDays: 0,
		},
		{
			name:     "start is zero",
			values:   []float64{0, 10000},
			wantCAGR: nil,
			wantDays: 1,
		},
		{
			name:     "start is negative",
			values:   []float64{-100, 10000},
			wantCAGR: nil,
			wantDays: 1,
		},
		{
			name:     "4x over 364 days",
			values:   makeUp365Days(10000, 40000),
			wantCAGR: ptrDecF(t, 301.5), // 4^(365/364) - 1 ≈ 301.5%
			wantDays: 364,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := eqPointsFromFloats(t, base, tt.values)
			got := ComputeCAGR(points)

			if got.DaysElapsed != tt.wantDays {
				t.Errorf("DaysElapsed = %d, want %d", got.DaysElapsed, tt.wantDays)
			}

			if !approxEqual(t, got.CAGRPct, tt.wantCAGR, 0.5) {
				t.Errorf("CAGRPct = %v, want %v", got.CAGRPct, tt.wantCAGR)
			}
		})
	}
}

// makeUp365Days creates 365 equity curve points linearly interpolating from start to end.
// Points are one day apart, so days elapsed = 364.
func makeUp365Days(start, end float64) []float64 {
	days := 365
	values := make([]float64, days)
	for i := 0; i < days; i++ {
		t := float64(i) / float64(days-1)
		values[i] = start + (end-start)*t
	}
	return values
}

// makeUp366Days creates 366 equity curve points linearly interpolating from start to end.
// Points are one day apart, so days elapsed = 365.
func makeUp366Days(start, end float64) []float64 {
	days := 366
	values := make([]float64, days)
	for i := 0; i < days; i++ {
		t := float64(i) / float64(days-1)
		values[i] = start + (end-start)*t
	}
	return values
}

// --- ComputeBetaAlpha ---

func TestComputeBetaAlpha(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		aValues     []float64
		bValues     []float64
		wantBeta    *decimal.Decimal
		wantAlpha   *decimal.Decimal
		wantOverlap int
	}{
		{
			name:        "identical portfolios — beta=1.0, alpha=0",
			aValues:     []float64{100, 102, 105, 103, 108, 110},
			bValues:     []float64{100, 102, 105, 103, 108, 110},
			wantBeta:    ptrDec4F(t, 1.0),
			wantAlpha:   ptrDecF(t, 0.0),
			wantOverlap: 5,
		},
		{
			name:        "A returns exactly 2x B — beta=2.0",
			// A returns: [2%, 4%, 2%, 6%, 4%], B returns: [1%, 2%, 1%, 3%, 2%]
			// Price series constructed from cumulative returns.
			aValues:     []float64{100, 102, 106.08, 108.2016, 114.693648, 118.981379},
			bValues:     []float64{100, 101, 103.01, 104.0401, 107.161304, 109.295133},
			wantBeta:    ptrDec4F(t, 2.0),
			wantAlpha:   nil, // alpha ≈ 0 but floating point; skip exact check
			wantOverlap: 5,
		},
		{
			name:        "A is flat — beta=0",
			aValues:     []float64{100, 100, 100, 100, 100, 100},
			bValues:     []float64{100, 102, 105, 103, 108, 110},
			wantBeta:    ptrDec4F(t, 0.0),
			wantAlpha:   nil, // alpha depends on mean returns
			wantOverlap: 5,
		},
		{
			name:        "B is flat — beta undefined",
			aValues:     []float64{100, 102, 105, 103, 108, 110},
			bValues:     []float64{100, 100, 100, 100, 100, 100},
			wantBeta:    nil,
			wantAlpha:   nil,
			wantOverlap: 5,
		},
		{
			name:        "A single point",
			aValues:     []float64{100},
			bValues:     []float64{100, 102, 105},
			wantBeta:    nil,
			wantAlpha:   nil,
			wantOverlap: 0,
		},
		{
			name:        "both single point",
			aValues:     []float64{100},
			bValues:     []float64{100},
			wantBeta:    nil,
			wantAlpha:   nil,
			wantOverlap: 0,
		},
		{
			name:        "empty",
			aValues:     []float64{},
			bValues:     []float64{},
			wantBeta:    nil,
			wantAlpha:   nil,
			wantOverlap: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aPoints := eqPointsFromFloats(t, base, tt.aValues)
			bPoints := eqPointsFromFloats(t, base, tt.bValues)
			got := ComputeBetaAlpha(aPoints, bPoints)

			if got.OverlapDays != tt.wantOverlap {
				t.Errorf("OverlapDays = %d, want %d", got.OverlapDays, tt.wantOverlap)
			}

			if tt.wantBeta != nil {
				if got.Beta == nil {
					t.Errorf("Beta = nil, want %v", tt.wantBeta)
				} else if !approxEqual(t, got.Beta, tt.wantBeta, 0.05) {
					t.Errorf("Beta = %v, want ~%v", got.Beta, tt.wantBeta)
				}
			} else if got.Beta != nil {
				t.Errorf("Beta = %v, want nil", got.Beta)
			}

			if tt.wantAlpha != nil {
				if got.Alpha == nil {
					t.Errorf("Alpha = nil, want %v", tt.wantAlpha)
				} else if !approxEqual(t, got.Alpha, tt.wantAlpha, 0.5) {
					t.Errorf("Alpha = %v, want ~%v", got.Alpha, tt.wantAlpha)
				}
			}
		})
	}
}

// --- ComputePortfolioCorrelation ---

func TestComputePortfolioCorrelation(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		aValues       []float64
		bValues       []float64
		wantCorr      *decimal.Decimal
		wantOverlap   int
	}{
		{
			name:        "identical portfolios — correlation=1.0",
			aValues:     []float64{100, 102, 105, 103, 108, 110},
			bValues:     []float64{100, 102, 105, 103, 108, 110},
			wantCorr:    ptrDec4F(t, 1.0),
			wantOverlap: 5,
		},
		{
			name:        "perfectly negatively correlated",
			aValues:     []float64{100, 102, 105, 103, 108},
			bValues:     []float64{100, 98, 95, 97, 92}, // inverse movement
			wantCorr:    ptrDec4F(t, -1.0),
			wantOverlap: 4,
		},
		{
			name:        "A flat — zero variance",
			aValues:     []float64{100, 100, 100, 100, 100},
			bValues:     []float64{100, 102, 105, 103, 108},
			wantCorr:    nil, // zero variance in A
			wantOverlap: 4,
		},
		{
			name:        "B flat — zero variance",
			aValues:     []float64{100, 102, 105, 103, 108},
			bValues:     []float64{100, 100, 100, 100, 100},
			wantCorr:    nil, // zero variance in B
			wantOverlap: 4,
		},
		{
			name:        "single point",
			aValues:     []float64{100},
			bValues:     []float64{100, 102},
			wantCorr:    nil,
			wantOverlap: 0,
		},
		{
			name:        "empty",
			aValues:     []float64{},
			bValues:     []float64{},
			wantCorr:    nil,
			wantOverlap: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aPoints := eqPointsFromFloats(t, base, tt.aValues)
			bPoints := eqPointsFromFloats(t, base, tt.bValues)
			got := ComputePortfolioCorrelation(aPoints, bPoints)

			if got.OverlapDays != tt.wantOverlap {
				t.Errorf("OverlapDays = %d, want %d", got.OverlapDays, tt.wantOverlap)
			}

			if tt.wantCorr != nil {
				if got.Correlation == nil {
					t.Errorf("Correlation = nil, want %v", tt.wantCorr)
				} else if !approxEqual(t, got.Correlation, tt.wantCorr, 0.05) {
					t.Errorf("Correlation = %v, want ~%v", got.Correlation, tt.wantCorr)
				}
			} else if got.Correlation != nil {
				t.Errorf("Correlation = %v, want nil", got.Correlation)
			}
		})
	}
}

// --- ComputePeriodExtremes ---

func TestComputePeriodExtremes(t *testing.T) {
	tests := []struct {
		name         string
		dates        []time.Time
		values       []float64
		wantBestM    *decimal.Decimal
		wantWorstM   *decimal.Decimal
		wantBestY    *decimal.Decimal
		wantWorstY   *decimal.Decimal
		wantWinRate  *decimal.Decimal
	}{
		{
			name: "multi-month multi-year data",
			dates: []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			},
			values: []float64{
				100, 105, 102, // Jan 2024: +2%
				102, 98, 96,   // Feb 2024: -5.88%
				96, 100, 104,  // Mar 2024: +8.33%
				108, 108,      // Jan 2025: 0%
			},
			wantBestM:  ptrDecF(t, 8.33),  // Mar 2024
			wantWorstM: ptrDecF(t, -5.88), // Feb 2024
			wantBestY:  ptrDecF(t, 4.0),   // 2024: 100->104
			wantWorstY: ptrDecF(t, 0.0),   // 2025: 108->108
			wantWinRate: ptrDecF(t, 50.0), // 2 of 4 months positive (Jan 2025 is 0%, not positive)
		},
		{
			name:  "single month",
			dates: []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			},
			values:   []float64{100, 105},
			wantBestM: ptrDecF(t, 5.0),
			wantWorstM: ptrDecF(t, 5.0),
			wantBestY:  ptrDecF(t, 5.0),
			wantWorstY: ptrDecF(t, 5.0),
			wantWinRate: ptrDecF(t, 100.0),
		},
		{
			name:         "single point",
			dates:        []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			values:       []float64{100},
			wantBestM:    nil,
			wantWorstM:   nil,
			wantBestY:    nil,
			wantWorstY:   nil,
			wantWinRate:  nil,
		},
		{
			name:        "empty",
			dates:       []time.Time{},
			values:      []float64{},
			wantBestM:   nil,
			wantWorstM:  nil,
			wantBestY:   nil,
			wantWorstY:  nil,
			wantWinRate: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := eqPointsFromDatesAndValues(t, tt.dates, tt.values)
			got := ComputePeriodExtremes(points)

			if !approxEqual(t, got.BestMonth, tt.wantBestM, 0.05) {
				t.Errorf("BestMonth = %v, want %v", got.BestMonth, tt.wantBestM)
			}
			if !approxEqual(t, got.WorstMonth, tt.wantWorstM, 0.05) {
				t.Errorf("WorstMonth = %v, want %v", got.WorstMonth, tt.wantWorstM)
			}
			if !approxEqual(t, got.BestYear, tt.wantBestY, 0.05) {
				t.Errorf("BestYear = %v, want %v", got.BestYear, tt.wantBestY)
			}
			if !approxEqual(t, got.WorstYear, tt.wantWorstY, 0.05) {
				t.Errorf("WorstYear = %v, want %v", got.WorstYear, tt.wantWorstY)
			}
			if !approxEqual(t, got.WinRatePct, tt.wantWinRate, 0.5) {
				t.Errorf("WinRatePct = %v, want %v", got.WinRatePct, tt.wantWinRate)
			}
		})
	}
}

// --- ComputeReturnDistribution ---

func TestComputeReturnDistribution(t *testing.T) {
	tests := []struct {
		name           string
		dates          []time.Time
		values         []float64
		wantAnnualN    int
		wantAnnualBinN int // number of annual frequency bins (at least this many)
		wantMonthlyN   int // number of monthly bins (at least this many)
	}{
		{
			name: "multi-year data",
			dates: []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
			},
			values:       []float64{100, 105, 110, 108, 108, 115, 120},
			wantAnnualN:  2, // 2024, 2025
			wantAnnualBinN: 1, // at least 1 frequency bin
			wantMonthlyN: 1, // at least some monthly bins
		},
		{
			name:  "single point",
			dates: []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			values: []float64{100},
			wantAnnualN: 0,
			wantAnnualBinN: 0,
			wantMonthlyN: 0,
		},
		{
			name:        "empty",
			dates:       []time.Time{},
			values:      []float64{},
			wantAnnualN: 0,
			wantAnnualBinN: 0,
			wantMonthlyN: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := eqPointsFromDatesAndValues(t, tt.dates, tt.values)
			got := ComputeReturnDistribution(points)

			if len(got.Annual) != tt.wantAnnualN {
				t.Errorf("Annual buckets = %d, want %d", len(got.Annual), tt.wantAnnualN)
			}
			if len(got.AnnualBinned) < tt.wantAnnualBinN {
				t.Errorf("AnnualBinned buckets = %d, want >= %d", len(got.AnnualBinned), tt.wantAnnualBinN)
			}
			if len(got.Monthly) < tt.wantMonthlyN {
				t.Errorf("Monthly buckets = %d, want >= %d", len(got.Monthly), tt.wantMonthlyN)
			}

			// Check annual bucket labels contain year labels.
			for _, b := range got.Annual {
				if len(b.Label) < 4 {
					t.Errorf("Annual bucket label too short: %q", b.Label)
				}
				if b.Count != 1 {
					t.Errorf("Annual bucket count = %d, want 1", b.Count)
				}
			}
		})
	}
}

func TestComputeDrawdownSeries(t *testing.T) {
	tests := []struct {
		name      string
		dates     []time.Time
		values    []float64
		wantLen   int
		wantMax   float64 // max drawdown pct (approx)
	}{
		{
			name: "steady decline then recovery",
			dates: []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
			},
			values:  []float64{100, 95, 90, 95, 110},
			wantLen: 5,
			wantMax: 10.0, // peak=100, min=90 → 10%
		},
		{
			name:  "single point",
			dates: []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			values: []float64{100},
			wantLen: 0,
		},
		{
			name:    "empty",
			dates:   []time.Time{},
			values:  []float64{},
			wantLen: 0,
		},
		{
			name: "two points up",
			dates: []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			values:  []float64{100, 110},
			wantLen: 2,
			wantMax: 0, // always at or above peak
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := eqPointsFromDatesAndValues(t, tt.dates, tt.values)
			got := ComputeDrawdownSeries(points)

			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}

			if tt.wantLen > 0 && tt.wantMax > 0 {
				var maxF float64
				for _, p := range got {
					v, _ := p.Pct.Float64()
					if v > maxF {
						maxF = v
					}
				}
				if maxF < tt.wantMax-0.5 || maxF > tt.wantMax+0.5 {
					t.Errorf("max pct = %.2f, want approx %.2f", maxF, tt.wantMax)
				}
			}
		})
	}
}

// --- Edge cases: short data (< 30 days) returns N/A ---

func TestComputeBetaAlpha_ShortData(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Only 3 data points = 2 returns (< 30 days)
	aPoints := eqPointsFromFloats(t, base, []float64{100, 102, 101})
	bPoints := eqPointsFromFloats(t, base, []float64{100, 101, 102})

	got := ComputeBetaAlpha(aPoints, bPoints)

	// Should still compute (function doesn't enforce 30-day minimum —
	// that's a service-layer concern). Just verify it returns something.
	if got.OverlapDays != 2 {
		t.Errorf("OverlapDays = %d, want 2", got.OverlapDays)
	}
	// Beta and Alpha should be non-nil for 2 aligned observations.
	if got.Beta == nil {
		t.Error("Beta = nil, expected non-nil for 2 observations")
	}
}

func TestComputePortfolioCorrelation_ShortData(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	aPoints := eqPointsFromFloats(t, base, []float64{100, 102, 101})
	bPoints := eqPointsFromFloats(t, base, []float64{100, 101, 102})

	got := ComputePortfolioCorrelation(aPoints, bPoints)

	if got.OverlapDays != 2 {
		t.Errorf("OverlapDays = %d, want 2", got.OverlapDays)
	}
	// Correlation should be non-nil for 2 observations with non-zero variance.
	if got.Correlation == nil {
		t.Error("Correlation = nil, expected non-nil for 2 observations")
	}
}

// --- Zero volatility edge case ---

func TestComputeBetaAlpha_ZeroVolatility(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	aPoints := eqPointsFromFloats(t, base, []float64{100, 102, 105, 103})
	bPoints := eqPointsFromFloats(t, base, []float64{100, 100, 100, 100})

	got := ComputeBetaAlpha(aPoints, bPoints)

	if got.Beta != nil {
		t.Errorf("Beta = %v, want nil (B has zero variance)", got.Beta)
	}
	if got.Alpha != nil {
		t.Errorf("Alpha = %v, want nil (beta undefined)", got.Alpha)
	}
}

func TestComputePortfolioCorrelation_ZeroVolatility(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	aPoints := eqPointsFromFloats(t, base, []float64{100, 100, 100, 100})
	bPoints := eqPointsFromFloats(t, base, []float64{100, 102, 105, 103})

	got := ComputePortfolioCorrelation(aPoints, bPoints)

	if got.Correlation != nil {
		t.Errorf("Correlation = %v, want nil (A has zero variance)", got.Correlation)
	}
}

// --- Partially overlapping dates ---

func TestComputeBetaAlpha_PartialOverlap(t *testing.T) {
	// A has dates: Jan 1, 2, 3, 4, 5
	// B has dates: Jan 3, 4, 5, 6, 7
	// Returns on overlap dates (Jan 4, Jan 5) should match for beta=1.0.
	// A returns: Jan 4 = 105/100-1 = 0.05, Jan 5 = 102/105-1 = -0.0286
	// B returns: Jan 4 = 210/200-1 = 0.05, Jan 5 = 204/210-1 = -0.0286
	datesA := []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	datesB := []time.Time{
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC),
	}

	aPoints := eqPointsFromDatesAndValues(t, datesA, []float64{100, 102, 100, 105, 102})
	bPoints := eqPointsFromDatesAndValues(t, datesB, []float64{200, 210, 204, 210, 200})

	got := ComputeBetaAlpha(aPoints, bPoints)

	// Overlap: Jan 4, Jan 5 → 2 daily returns
	if got.OverlapDays != 2 {
		t.Errorf("OverlapDays = %d, want 2", got.OverlapDays)
	}
	// Same returns on overlap dates → beta should be ~1.0
	if got.Beta == nil {
		t.Error("Beta = nil, expected non-nil")
	} else {
		betaF, _ := got.Beta.Float64()
		if math.Abs(betaF-1.0) > 0.01 {
			t.Errorf("Beta = %v, want ~1.0", got.Beta)
		}
	}
}
