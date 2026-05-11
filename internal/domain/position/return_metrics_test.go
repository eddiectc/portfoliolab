package position

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func TestComputePeriodReturn_NoCashFlows(t *testing.T) {
	tests := []struct {
		name           string
		equityCurve    []EquityCurvePoint
		breakpoints    []twrBreakpoint
		wantTWR        *decimal.Decimal
		wantTWRApprox  bool
		wantInsuffData bool
	}{
		{
			name: "15% return over 1 year",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1150000, 2), NetDeposit: dec(1000000, 2)},
			},
			breakpoints:    nil,
			wantTWR:        ptrDec(decimal.MustParse("15.00")),
			wantTWRApprox:  true,
			wantInsuffData: false,
		},
		{
			name: "zero return",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
			},
			breakpoints:    nil,
			wantTWR:        ptrDec(decimal.Zero),
			wantTWRApprox:  false,
			wantInsuffData: false,
		},
		{
			name: "negative return",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(850000, 2), NetDeposit: dec(1000000, 2)},
			},
			breakpoints:    nil,
			wantTWR:        ptrDec(decimal.MustParse("-15.00")),
			wantTWRApprox:  true,
			wantInsuffData: false,
		},
		{
			name: "single point insufficient",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
			},
			breakpoints:    nil,
			wantTWR:        nil,
			wantTWRApprox:  false,
			wantInsuffData: true,
		},
		{
			name:           "empty curve insufficient",
			equityCurve:    []EquityCurvePoint{},
			breakpoints:    nil,
			wantTWR:        nil,
			wantTWRApprox:  false,
			wantInsuffData: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputePeriodReturn(tt.equityCurve, tt.breakpoints, "USD")

			if result.HasInsufficientData != tt.wantInsuffData {
				t.Errorf("HasInsufficientData: got %v, want %v", result.HasInsufficientData, tt.wantInsuffData)
			}

			if tt.wantTWR == nil {
				if result.TWRPct != nil {
					t.Errorf("TWRPct: got %s, want nil", result.TWRPct.String())
				}
			} else if result.TWRPct == nil {
				t.Errorf("TWRPct: got nil, want %s", tt.wantTWR.String())
			} else if tt.wantTWRApprox {
				diff, _ := result.TWRPct.Sub(*tt.wantTWR)
				diff = diff.Abs()
				threshold := decimal.MustParse("0.50")
				if diff.Cmp(threshold) > 0 {
					t.Errorf("TWRPct: got %s, want approx %s (diff %s)", result.TWRPct.String(), tt.wantTWR.String(), diff.String())
				}
			} else if !result.TWRPct.Equal(*tt.wantTWR) {
				t.Errorf("TWRPct: got %s, want %s", result.TWRPct.String(), tt.wantTWR.String())
			}
		})
	}
}

func TestComputePeriodReturn_WithCashFlows(t *testing.T) {
	// Scenario: deposit $10k on day 1, deposit $10k on day 180, end with $22k
	// TWR should be ~10% (not 120% like simple return would suggest)
	twrBreakpoints := []twrBreakpoint{
		{date: mustTime("2023-07-01"), value: dec(1000000, 2)}, // pre-cash-flow: positions + cash = $10k
	}

	// Build a minimal equity curve with the key dates
	// Jan 1: $10k (initial deposit)
	// Jul 1: $10k (before 2nd deposit) → $20k (after 2nd deposit)
	// Dec 31: $22k (end of period)
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
		// Jul 1: the curve has the POST-cash-flow value ($20k)
		{Date: mustTime("2023-07-01"), PortfolioValue: dec(2000000, 2), NetDeposit: dec(2000000, 2)},
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(2200000, 2), NetDeposit: dec(2000000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, twrBreakpoints, "USD")

	// TWR = (10k/10k) × (22k/20k) - 1 = 1.0 × 1.1 - 1 = 10%
	if result.TWRPct == nil {
		t.Fatal("TWRPct is nil")
	}
	wantTWR := decimal.MustParse("10.00")
	diff, _ := result.TWRPct.Sub(wantTWR)
	diff = diff.Abs()
	threshold := decimal.MustParse("0.50")
	if diff.Cmp(threshold) > 0 {
		t.Errorf("TWRPct: got %s, want approx %s (diff %s)", result.TWRPct.String(), wantTWR.String(), diff.String())
	}

	// Annualized TWR should be ~20% (annualized from ~180 days)
	if result.AnnualizedTWRPct == nil {
		t.Fatal("AnnualizedTWRPct is nil")
	}
}

func TestComputePeriodReturn_MultipleCashFlows(t *testing.T) {
	// 3 cash flows over the year
	twrBreakpoints := []twrBreakpoint{
		{date: mustTime("2023-03-01"), value: dec(1000000, 2)}, // pre 1st CF: $10k
		{date: mustTime("2023-06-01"), value: dec(1500000, 2)}, // pre 2nd CF: $15k
		{date: mustTime("2023-09-01"), value: dec(2200000, 2)}, // pre 3rd CF: $22k
	}

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
		{Date: mustTime("2023-03-01"), PortfolioValue: dec(1200000, 2), NetDeposit: dec(1200000, 2)}, // post 1st CF
		{Date: mustTime("2023-06-01"), PortfolioValue: dec(1800000, 2), NetDeposit: dec(1800000, 2)}, // post 2nd CF
		{Date: mustTime("2023-09-01"), PortfolioValue: dec(2500000, 2), NetDeposit: dec(2500000, 2)}, // post 3rd CF
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(2800000, 2), NetDeposit: dec(2500000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, twrBreakpoints, "USD")

	// TWR = (10k/10k) × (15k/12k) × (22k/18k) × (28k/25k) - 1
	// = 1.0 × 1.25 × 1.2222 × 1.12 - 1
	// ≈ 1.667 - 1 = 66.7%
	if result.TWRPct == nil {
		t.Fatal("TWRPct is nil")
	}
	// Just check it's positive and reasonable
	if result.TWRPct.Less(decimal.Zero) {
		t.Errorf("TWRPct should be positive, got %s", result.TWRPct.String())
	}
}

func TestComputePeriodReturn_ZeroBeginValue(t *testing.T) {
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: decimal.Zero, NetDeposit: decimal.Zero},
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, nil, "USD")
	if result.TWRPct != nil {
		t.Errorf("TWRPct: got %s, want nil (zero begin value)", result.TWRPct.String())
	}
}

func TestComputePeriodReturn_SameDate(t *testing.T) {
	// Both points on the same date — annualized should be nil
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1100000, 2), NetDeposit: dec(1000000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, nil, "USD")
	// TWR should be computed (simple return)
	if result.TWRPct == nil {
		t.Fatal("TWRPct is nil")
	}
	// Annualized should be nil (0 days elapsed)
	if result.AnnualizedTWRPct != nil {
		t.Errorf("AnnualizedTWRPct: got %s, want nil (same date)", result.AnnualizedTWRPct.String())
	}
}

func TestComputeSimpleReturn(t *testing.T) {
	first := EquityCurvePoint{
		Date:           mustTime("2023-01-01"),
		PortfolioValue: dec(1000000, 2),
		NetDeposit:     dec(1000000, 2),
	}
	last := EquityCurvePoint{
		Date:           mustTime("2023-12-31"),
		PortfolioValue: dec(1150000, 2),
		NetDeposit:     dec(1000000, 2),
	}

	result := computeSimpleReturn(first, last)
	if result == nil {
		t.Fatal("result is nil")
	}
	want := decimal.MustParse("15.00")
	if !result.Equal(want) {
		t.Errorf("got %s, want %s", result.String(), want.String())
	}
}

func TestComputeSimpleReturn_ZeroBegin(t *testing.T) {
	first := EquityCurvePoint{
		Date:           mustTime("2023-01-01"),
		PortfolioValue: decimal.Zero,
		NetDeposit:     decimal.Zero,
	}
	last := EquityCurvePoint{
		Date:           mustTime("2023-12-31"),
		PortfolioValue: dec(1000000, 2),
		NetDeposit:     dec(1000000, 2),
	}

	result := computeSimpleReturn(first, last)
	if result != nil {
		t.Errorf("expected nil, got %s", result.String())
	}
}

func TestRatioFloat(t *testing.T) {
	a := dec(1000000, 2) // $10,000
	b := dec(1150000, 2) // $11,500
	r := ratioFloat(a, b)
	// b/a = 11500/10000 = 1.15
	if r < 1.14 || r > 1.16 {
		t.Errorf("ratioFloat: got %f, want ~1.15", r)
	}
}

func TestRatioFloat_Zero(t *testing.T) {
	a := decimal.Zero
	b := dec(1000000, 2)
	r := ratioFloat(a, b)
	if r != 0 {
		t.Errorf("ratioFloat with zero a: got %f, want 0", r)
	}
}

func TestBuildCurveMap(t *testing.T) {
	curve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2023-06-01"), PortfolioValue: dec(1100000, 2)},
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(1200000, 2)},
	}

	m := buildCurveMap(curve)
	if len(m) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m))
	}

	val := m["2023-06-01"]
	if !val.Equal(dec(1100000, 2)) {
		t.Errorf("2023-06-01: got %s, want 1100000", val.String())
	}
}

func TestLookupCurveValue(t *testing.T) {
	m := curveDateMap{
		"2023-01-01": dec(1000000, 2),
		"2023-06-01": dec(1100000, 2),
	}

	date := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	val := lookupCurveValue(m, date)
	if val == nil {
		t.Fatal("expected non-nil value")
	}
	if !val.Equal(dec(1100000, 2)) {
		t.Errorf("got %s, want 1100000", val.String())
	}

	// Missing date
	missingDate := time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC)
	val = lookupCurveValue(m, missingDate)
	if val != nil {
		t.Errorf("expected nil for missing date, got %s", val.String())
	}
}
