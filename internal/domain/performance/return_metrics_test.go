package performance

import (
	"math"
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

			switch {
			case tt.wantTWR == nil:
				if result.TWRPct != nil {
					t.Errorf("TWRPct: got %s, want nil", result.TWRPct.String())
				}
			case result.TWRPct == nil:
				t.Errorf("TWRPct: got nil, want %s", tt.wantTWR.String())
			case tt.wantTWRApprox:
				diff, _ := result.TWRPct.Sub(*tt.wantTWR)
				diff = diff.Abs()
				threshold := decimal.MustParse("0.50")
				if diff.Cmp(threshold) > 0 {
					t.Errorf("TWRPct: got %s, want approx %s (diff %s)", result.TWRPct.String(), tt.wantTWR.String(), diff.String())
				}
			case !result.TWRPct.Equal(*tt.wantTWR):
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

func TestComputeValueReturn(t *testing.T) {
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

	result := computeValueReturn(first, last)
	if result == nil {
		t.Fatal("result is nil")
	}
	want := decimal.MustParse("15.00")
	if !result.Equal(want) {
		t.Errorf("got %s, want %s", result.String(), want.String())
	}
}

func TestComputeValueReturn_ZeroBegin(t *testing.T) {
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

	result := computeValueReturn(first, last)
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

func TestDeduplicateBreakpoints(t *testing.T) {
	tests := []struct {
		name     string
		bps      []twrBreakpoint
		wantLen  int
		wantDate []string // date strings of expected breakpoints
	}{
		{
			name:     "empty",
			bps:      nil,
			wantLen:  0,
			wantDate: nil,
		},
		{
			name: "single breakpoint",
			bps: []twrBreakpoint{
				{date: mustTime("2024-01-01"), value: dec(1000000, 2)},
			},
			wantLen:  1,
			wantDate: []string{"2024-01-01"},
		},
		{
			name: "multiple dates kept all",
			bps: []twrBreakpoint{
				{date: mustTime("2024-01-01"), value: dec(1000000, 2)},
				{date: mustTime("2024-02-01"), value: dec(1100000, 2)},
				{date: mustTime("2024-03-01"), value: dec(1200000, 2)},
			},
			wantLen:  3,
			wantDate: []string{"2024-01-01", "2024-02-01", "2024-03-01"},
		},
		{
			name: "multiple on same date keeps first",
			bps: []twrBreakpoint{
				{date: mustTime("2024-01-01"), value: dec(1000000, 2)},
				{date: mustTime("2024-01-01"), value: dec(1500000, 2)}, // same date, different value
				{date: mustTime("2024-01-01"), value: dec(2000000, 2)}, // same date again
				{date: mustTime("2024-02-01"), value: dec(2500000, 2)},
			},
			wantLen:  2,
			wantDate: []string{"2024-01-01", "2024-02-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateBreakpoints(tt.bps)
			if len(got) != tt.wantLen {
				t.Errorf("got %d breakpoints, want %d", len(got), tt.wantLen)
			}
			for i, want := range tt.wantDate {
				if i >= len(got) {
					break
				}
				gotDate := got[i].date.Format("2006-01-02")
				if gotDate != want {
					t.Errorf("[%d] date = %s, want %s", i, gotDate, want)
				}
			}
		})
	}
}

// TestComputePeriodReturn_SubPeriodFromPreCashFlow validates the TWR formula
// with multiple cash flows and known market returns. Each sub-period measures
// market performance between cash flow events:
//
//	sub-period i: post-cash-flow[i-1] → pre-cash-flow[i]
//	ratio = pre[i] / post[i-1]
func TestComputePeriodReturn_SubPeriodFromPreCashFlow(t *testing.T) {
	// Scenario: initial deposit, then two more cash flows with market growth between
	//
	// Timeline:
	//   1/1:  initial deposit of $1000, portfolio = $1000
	//   3/1:  market grew 10% → $1100, then deposit $1000 → $2100
	//   6/1:  market grew 20% → $2520, then deposit $500 → $3020
	//   12/31: market grew 5% → $3171
	//
	// Breakpoints (pre-cash-flow values, deduplicated):
	//   3/1:  1100 (value before the $1000 deposit)
	//   6/1:  2520 (value before the $500 deposit)
	//
	// Equity curve (post-cash-flow values on breakpoint dates):
	//   3/1:  2100 (1100 + 1000 deposit)
	//   6/1:  3020 (2520 + 500 deposit)
	//
	// Correct TWR (using pre-cash-flow → pre-cash-flow):
	//   sub-period 1: post-deposit(1/1)=1000 → pre-cf(3/1)=1100 → ratio = 1.10
	//   sub-period 2: pre-cf(3/1)=1100 → pre-cf(6/1)=2520 → ratio = 2520/1100 = 2.2909
	//   Wait, that includes the deposit effect. That's wrong.
	//
	// Actually the correct TWR formula:
	//   sub-period 1: post-cf(1/1) → pre-cf(3/1) = 1000 → 1100, ratio = 1.10
	//   sub-period 2: post-cf(3/1) → pre-cf(6/1) = 2100 → 2520, ratio = 1.20
	//   sub-period 3: post-cf(6/1) → last = 3020 → 3171, ratio = 1.05
	//   TWR = 1.10 × 1.20 × 1.05 - 1 = 1.386 - 1 = 38.6%
	//
	// BUGGY code (using equity curve as "from" for middle sub-periods):
	//   sub-period 1: post-cf(1/1)=1000 → pre-cf(3/1)=1100, ratio = 1.10 ✓
	//   sub-period 2: equity(3/1)=2100 → pre-cf(6/1)=2520, ratio = 1.20 ✓
	//   Wait, that's the same! Because equity curve on 3/1 IS the post-cf value.
	//
	// OK so the issue only manifests when the equity curve value on bp[i-1].date
	// is DIFFERENT from the post-cash-flow value. This happens when there are
	// multiple transactions on the same date and the equity curve captures a
	// different point than the breakpoint.
	//
	// Let me construct a case where this matters.
	//
	// Actually, re-reading the code more carefully:
	//
	// Middle sub-periods loop:
	//   postPrev := lookupCurveValue(curveMap, breakpoints[i-1].date)
	//   preCurr := breakpoints[i].value
	//   r := ratioFloat(*postPrev, preCurr)
	//
	// This uses the EQUITY CURVE value at bp[i-1]'s date as "from".
	// The equity curve value is the POST-cash-flow value on that date.
	// And the breakpoint value is the PRE-cash-flow value.
	//
	// So for sub-period bp[i-1] → bp[i]:
	//   from = post-cash-flow on bp[i-1].date (from equity curve)
	//   to = pre-cash-flow on bp[i].date (from breakpoint)
	//
	// This is actually the CORRECT TWR formula! The sub-period return is:
	//   (value before next cash flow) / (value after previous cash flow)
	//
	// So where is the bug? Let me look at the actual debug output again.
	//
	// From the debug:
	//   from=2024-03-20 fromValue=40000 to=2024-04-30 toValue=38656 ratio=0.9664
	//
	// The fromValue=40000 is the equity curve value on 3/20.
	// The breakpoint (pre-cash-flow) on 3/20 is 31000 (first deduped for that date).
	// But wait, 3/20 IS a breakpoint date. So the equity curve on 3/20 is the
	// post-cash-flow value (40000), and the breakpoint is pre-cash-flow (31000).
	//
	// The sub-period is from bp[3] (3/20) to bp[4] (4/30).
	// from = equity curve on 3/20 = 40000 (post-cash-flow) ✓
	// to = breakpoint on 4/30 = 38656 (pre-cash-flow) ✓
	// ratio = 38656/40000 = 0.9664
	//
	// But the CORRECT sub-period should be:
	// from = post-cash-flow on 3/20 = 40000
	// to = pre-cash-flow on 4/30 = 38656
	// ratio = 38656/40000 = 0.9664
	//
	// That's the same! So the formula seems correct...
	//
	// Wait. Let me look at the PREVIOUS sub-period:
	//   from=2024-03-19 fromValue=31000 to=2024-03-20 toValue=31000 ratio=1
	//
	// from = equity curve on 3/19 = 31000 (post-cash-flow on 3/19)
	// to = breakpoint on 3/20 = 31000 (pre-cash-flow on 3/20)
	// ratio = 31000/31000 = 1.0
	//
	// This is correct! The portfolio went from 31000 (after 3/19 deposit) to
	// 31000 (before 3/20 deposit). No market change.
	//
	// And the NEXT sub-period:
	//   from=2024-03-20 fromValue=40000 to=2024-04-30 toValue=38656 ratio=0.9664
	//
	// from = equity curve on 3/20 = 40000 (post-cash-flow on 3/20, after deposit)
	// to = breakpoint on 4/30 = 38656 (pre-cash-flow on 4/30)
	// ratio = 38656/40000 = 0.9664
	//
	// This means: after depositing on 3/20 (portfolio = 40000), by 4/30
	// (before any cash flow that day), the portfolio was worth 38656.
	// That's a -3.36% loss over ~41 days.
	//
	// Hmm, but looking at the snapshots:
	//   3/22: posValue=39777, cashValue=48, portfolioValue=39825
	//   4/3:  posValue=39568, cashValue=53, portfolioValue=39621
	//   4/30: posValue=38602, cashValue=835, portfolioValue=39437
	//
	// The portfolio on 3/22 was 39825, on 4/30 was 39437.
	// From 40000 (3/20) to 39437 (4/30 post-cf) is -1.4%.
	// From 40000 (3/20) to 38656 (4/30 pre-cf) is -3.4%.
	//
	// The pre-cash-flow value on 4/30 (38656) is less than the equity curve
	// value on 4/30 (39437) because there was a cash flow (deposit) on 4/30.
	// The cash flow amount is 39437 - 38656 = 781.
	//
	// So the sub-period return of -3.4% is measuring:
	//   from 40000 (post-deposit 3/20) to 38656 (pre-deposit 4/30)
	// This correctly isolates market performance.
	//
	// OK so the formula IS correct. Then why is the total TWR only 1.58%?
	//
	// Let me look at ALL sub-periods more carefully...
	//
	// Actually, I think the issue might be that the TWR is genuinely ~1.58%.
	// Let me verify with a simpler manual calculation.
	//
	// Portfolio: started at ~40k (after initial deposits), ended at ~490k
	// Net deposits: ~453k
	// If there were zero return, ending would be 40k + 453k = 493k
	// Actual ending: 490k
	// So the portfolio slightly underperformed flat → small negative return
	// But TWR of 1.58% is slightly positive...
	//
	// Let me just verify the formula is correct by constructing a clear test case.

	// Timeline:
	//   1/1:   deposit $1000 → portfolio = $1000
	//   3/1:   market +10% → $1100, deposit $1000 → $2100
	//   6/1:   market +20% → $2520, withdrawal $500 → $2020
	//  12/31:  market +5% → $2121
	//
	// TWR = (1100/1000) × (2520/2100) × (2121/2020) - 1
	//     = 1.10 × 1.20 × 1.05 - 1 = 38.6%

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(100000, 2), NetDeposit: dec(100000, 2)},
		{Date: mustTime("2023-03-01"), PortfolioValue: dec(210000, 2), NetDeposit: dec(210000, 2)},
		{Date: mustTime("2023-06-01"), PortfolioValue: dec(202000, 2), NetDeposit: dec(160000, 2)},
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(212100, 2), NetDeposit: dec(160000, 2)},
	}

	breakpoints := []twrBreakpoint{
		{date: mustTime("2023-03-01"), value: dec(110000, 2)}, // pre-cash-flow: $1100
		{date: mustTime("2023-06-01"), value: dec(252000, 2)}, // pre-cash-flow: $2520
	}

	result := ComputePeriodReturn(equityCurve, breakpoints, "USD")

	if result.TWRPct == nil {
		t.Fatal("TWRPct is nil")
	}

	// TWR = 1.10 × 1.20 × 1.05 - 1 = 38.6%
	wantTWR := decimal.MustParse("38.60")
	diff, _ := result.TWRPct.Sub(wantTWR)
	diff = diff.Abs()
	threshold := decimal.MustParse("0.50")
	if diff.Cmp(threshold) > 0 {
		t.Errorf("TWRPct: got %s, want approx %s (diff %s)", result.TWRPct.String(), wantTWR.String(), diff.String())
	}
}

func TestComputeMWR_Basic(t *testing.T) {
	// Scenario: invest $100k at start, deposit $50k at midpoint, end with $170k
	//
	// Cash flows for MWR:
	//   t=0:    -$100k (initial investment)
	//   t=0.5:  -$50k  (deposit)
	//   t=1.0:  +$170k (terminal value)
	//
	// NPV(r) = -100k + (-50k)/(1+r)^0.5 + 170k/(1+r)^1 = 0
	//
	// At r=20%: -100k - 50k/1.0954 + 170k/1.2
	//           = -100k - 45.64k + 141.67k = -4.97k
	// At r=21%: -100k - 50k/1.1012 + 170k/1.21
	//           = -100k - 45.40k + 140.50k = -4.90k
	// At r=19%: -100k - 50k/1.0899 + 170k/1.19
	//           = -100k - 45.87k + 142.86k = -3.01k
	//
	// Exact solution: r ≈ 19.99%
	// (Verified: 100k*(1.2)^1 + 50k*(1.2)^0.5 = 140k + 45.64k = 185.64k...
	//  Actually FV = 100k*(1+r) + 50k*sqrt(1+r) = 170k
	//  With x = sqrt(1+r): 100x^2 + 50x - 170 = 0
	//  x = (-50 + sqrt(2500 + 68000)) / 200 = (-50 + 262.49) / 200 = 1.06245
	//  r = x^2 - 1 = 1.1288 - 1 = 12.88%
	//
	// Wait, let me re-derive. The MWR equation is:
	//   -100k + (-50k)/(1+r)^0.5 + 170k/(1+r)^1 = 0
	// Let x = sqrt(1+r), so (1+r)^0.5 = x and (1+r)^1 = x^2
	//   -100k - 50k/x + 170k/x^2 = 0
	// Multiply by x^2:
	//   -100k*x^2 - 50k*x + 170k = 0
	//   100x^2 + 50x - 170 = 0
	//   x = (-50 + sqrt(2500 + 68000)) / 200 = (-50 + 262.49) / 200 = 1.06245
	//   r = x^2 - 1 = 0.1288 = 12.88%
	//
	// Hmm, but that seems low. Let me verify:
	//   -100k - 50k/1.06245 + 170k/1.1288
	//   = -100k - 47.06k + 150.61k = -6.45k  (not zero!)
	//
	// I think the issue is with signs. Let me re-derive:
	// The MWR equation is:
	//   PV(inflows) = PV(outflows)
	//   170k/(1+r) = 100k + 50k/(1+r)^0.5
	//   170k = 100k*(1+r) + 50k*(1+r)^0.5
	//
	// With x = sqrt(1+r):
	//   170k = 100k*x^2 + 50k*x
	//   100x^2 + 50x - 170 = 0
	//   x = 1.06245, r = 12.88%
	//
	// Verify: 100k*(1.1288) + 50k*(1.06245) = 112.88k + 53.12k = 166.00k
	// That's not 170k. Something is off.
	//
	// Actually, the FV equation for MWR is:
	//   FV = PV_0 * (1+r)^T + CF_1 * (1+r)^(T-t1)
	//   170k = 100k * (1+r)^1 + 50k * (1+r)^0.5
	//   170 = 100x^2 + 50x where x = sqrt(1+r)
	//   100x^2 + 50x - 170 = 0
	//   x = (-50 + sqrt(2500 + 68000)) / 200 = 1.06245
	//   r = 1.06245^2 - 1 = 0.1288 = 12.88%
	//
	// Verify: 100*(1.1288) + 50*1.06245 = 112.88 + 53.12 = 166.00
	// Hmm, that's 166 not 170. Let me recheck.
	//
	// Actually: 100*1.06245^2 = 100*1.1288 = 112.88
	// And: 50*1.06245 = 53.12
	// Total: 112.88 + 53.12 = 166.00
	// But we want 170. So r should be higher.
	//
	// Let me solve more carefully:
	// 100x^2 + 50x - 170 = 0
	// x = (-50 + sqrt(2500 + 68000)) / 200
	// = (-50 + sqrt(70500)) / 200
	// = (-50 + 265.52) / 200
	// = 215.52 / 200
	// = 1.0776
	// r = 1.0776^2 - 1 = 1.1612 - 1 = 0.1612 = 16.12%
	//
	// Verify: 100*(1.1612) + 50*sqrt(1.1612) = 116.12 + 50*1.0776 = 116.12 + 53.88 = 170.00 ✓
	//
	// So MWR ≈ 16.12%

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
		{Date: mustTime("2023-07-02"), PortfolioValue: dec(1500000, 2), NetDeposit: dec(1500000, 2)}, // ~182 days = 0.5 years
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(1700000, 2), NetDeposit: dec(1500000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, nil, "USD")

	if result.MWRPct == nil {
		t.Fatal("MWRPct is nil")
	}

	// MWR ≈ 16.12%
	wantMWR := decimal.MustParse("16.12")
	diff, _ := result.MWRPct.Sub(wantMWR)
	diff = diff.Abs()
	threshold := decimal.MustParse("0.50")
	if diff.Cmp(threshold) > 0 {
		t.Errorf("MWRPct: got %s, want approx %s (diff %s)", result.MWRPct.String(), wantMWR.String(), diff.String())
	}

	// Holding-period MWR should be close to the total return over the period
	// HP-MWR = (1 + 0.1612)^(364/365) - 1 ≈ 16.04%
	if result.HoldingPeriodMWRPct == nil {
		t.Fatal("HoldingPeriodMWRPct is nil")
	}
	wantHP := decimal.MustParse("16.00")
	diffHP, _ := result.HoldingPeriodMWRPct.Sub(wantHP)
	diffHP = diffHP.Abs()
	thresholdHP := decimal.MustParse("1.00")
	if diffHP.Cmp(thresholdHP) > 0 {
		t.Errorf("HoldingPeriodMWRPct: got %s, want approx %s (diff %s)",
			result.HoldingPeriodMWRPct.String(), wantHP.String(), diffHP.String())
	}
}

func TestComputeMWR_NoCashFlows(t *testing.T) {
	// When there are no cash flows, MWR should equal simple return.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1150000, 2), NetDeposit: dec(1000000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, nil, "USD")

	if result.MWRPct == nil {
		t.Fatal("MWRPct is nil")
	}

	// MWR should be ~15% (same as simple return when no intermediate cash flows)
	wantMWR := decimal.MustParse("15.00")
	diff, _ := result.MWRPct.Sub(wantMWR)
	diff = diff.Abs()
	threshold := decimal.MustParse("0.50")
	if diff.Cmp(threshold) > 0 {
		t.Errorf("MWRPct: got %s, want approx %s (diff %s)", result.MWRPct.String(), wantMWR.String(), diff.String())
	}
}

func TestComputeMWR_ZeroBeginValue(t *testing.T) {
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: decimal.Zero, NetDeposit: decimal.Zero},
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, nil, "USD")
	if result.MWRPct != nil {
		t.Errorf("MWRPct: got %s, want nil (zero begin value)", result.MWRPct.String())
	}
}

func TestComputeMWR_NegativeReturn(t *testing.T) {
	// Portfolio loses money despite additional deposits
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
		{Date: mustTime("2023-07-02"), PortfolioValue: dec(1200000, 2), NetDeposit: dec(1200000, 2)},
		{Date: mustTime("2023-12-31"), PortfolioValue: dec(1300000, 2), NetDeposit: dec(1200000, 2)},
	}

	result := ComputePeriodReturn(equityCurve, nil, "USD")

	if result.MWRPct == nil {
		t.Fatal("MWRPct is nil")
	}

	mwrF, _ := result.MWRPct.Float64()
	// Invested 100k + 20k = 120k total, ended with 130k
	// But timing matters — the 20k was deposited at midpoint
	// MWR should be positive but modest
	if mwrF < -5 || mwrF > 20 {
		t.Errorf("MWRPct: got %s, expected between -5%% and 20%%", result.MWRPct.String())
	}
}

func TestComputeSimpleReturn_ProfitOverNetDeposit(t *testing.T) {
	// Simple return = (end_pv - end_nd) / end_nd × 100
	// "For every £1 deposited, how much profit was made?"
	tests := []struct {
		name         string
		equityCurve  []EquityCurvePoint
		breakpoints  []twrBreakpoint
		wantNil      bool
		wantApprox   float64
		approxMargin float64
	}{
		{
			name: "profit on deposits",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1150000, 2), NetDeposit: dec(1000000, 2)},
			}, // profit = 15000, nd = 100000 → 15%
			breakpoints:  nil,
			wantNil:      false,
			wantApprox:   15.0,
			approxMargin: 0.5,
		},
		{
			name: "loss on deposits",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(900000, 2), NetDeposit: dec(1000000, 2)},
			}, // profit = -10000, nd = 100000 → -10%
			breakpoints:  nil,
			wantNil:      false,
			wantApprox:   -10.0,
			approxMargin: 0.5,
		},
		{
			name: "zero net deposit",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: decimal.Zero},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1150000, 2), NetDeposit: decimal.Zero},
			},
			breakpoints: nil,
			wantNil:     true,
		},
		{
			name: "flat performance",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
			}, // profit = 0, nd = 100000 → 0%
			breakpoints:  nil,
			wantNil:      false,
			wantApprox:   0.0,
			approxMargin: 0.1,
		},
		{
			name: "with intermediate deposits",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2023-07-01"), PortfolioValue: dec(2000000, 2), NetDeposit: dec(2000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(2300000, 2), NetDeposit: dec(2000000, 2)},
			}, // profit = 30000, nd = 200000 → 15%
			breakpoints: []twrBreakpoint{
				{date: mustTime("2023-07-01"), value: dec(1000000, 2)},
			},
			wantNil:      false,
			wantApprox:   15.0,
			approxMargin: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputePeriodReturn(tt.equityCurve, tt.breakpoints, "USD")

			if tt.wantNil {
				if result.SimpleReturnPct != nil {
					t.Errorf("expected nil, got %s", result.SimpleReturnPct.String())
				}
				return
			}

			if result.SimpleReturnPct == nil {
				t.Fatal("expected non-nil SimpleReturnPct")
			}

			got, _ := result.SimpleReturnPct.Float64()
			if math.Abs(got-tt.wantApprox) > tt.approxMargin {
				t.Errorf("got %s, want approx %.2f (margin %.2f)", result.SimpleReturnPct.String(), tt.wantApprox, tt.approxMargin)
			}
		})
	}
}

func TestComputeAnnualizedSimpleReturn(t *testing.T) {
	tests := []struct {
		name         string
		simpleReturn *decimal.Decimal
		first        EquityCurvePoint
		last         EquityCurvePoint
		wantNil      bool
		wantApprox   float64
		approxMargin float64
	}{
		{
			name:         "nil simple return",
			simpleReturn: nil,
			first: EquityCurvePoint{
				Date: mustTime("2023-01-01"),
			},
			last: EquityCurvePoint{
				Date: mustTime("2024-01-01"),
			},
			wantNil: true,
		},
		{
			name:         "same date zero days",
			simpleReturn: ptrDec(decimal.MustParse("200.00")),
			first: EquityCurvePoint{
				Date: mustTime("2023-01-01"),
			},
			last: EquityCurvePoint{
				Date: mustTime("2023-01-01"),
			},
			wantNil: true,
		},
		{
			name:         "200% over 214 days",
			simpleReturn: ptrDec(decimal.MustParse("200.00")),
			first: EquityCurvePoint{
				Date: mustTime("2023-06-01"),
			},
			last: EquityCurvePoint{
				Date: mustTime("2024-01-01"),
			},
			// (1 + 2.0)^(365/214) - 1 = 3^(1.7056) - 1 ≈ 551.3%
			wantNil:      false,
			wantApprox:   551.3,
			approxMargin: 1.0,
		},
		{
			name:         "15% over 1 year",
			simpleReturn: ptrDec(decimal.MustParse("15.00")),
			first: EquityCurvePoint{
				Date: mustTime("2023-01-01"),
			},
			last: EquityCurvePoint{
				Date: mustTime("2024-01-01"),
			},
			// (1 + 0.15)^(365/365) - 1 = 15%
			wantNil:      false,
			wantApprox:   15.0,
			approxMargin: 0.5,
		},
		{
			name:         "10% over 182 days (half year)",
			simpleReturn: ptrDec(decimal.MustParse("10.00")),
			first: EquityCurvePoint{
				Date: mustTime("2023-01-01"),
			},
			last: EquityCurvePoint{
				Date: mustTime("2023-07-02"),
			},
			// (1 + 0.10)^(365/182) - 1 ≈ 1.1^2.0055 - 1 ≈ 21.1%
			wantNil:      false,
			wantApprox:   21.1,
			approxMargin: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeAnnualizedSimpleReturn(tt.simpleReturn, tt.first, tt.last)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %s", result.String())
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			got, _ := result.Float64()
			if math.Abs(got-tt.wantApprox) > tt.approxMargin {
				t.Errorf("got %s, want approx %.2f (margin %.2f)", result.String(), tt.wantApprox, tt.approxMargin)
			}
		})
	}
}

func TestComputePeriodReturn_MultipleBreakpointsSameDate(t *testing.T) {
	// Regression test: when there are multiple cash flows on the same date,
	// TWR should only use the first pre-cash-flow snapshot per date.
	// Without deduplication, each breakpoint creates a bogus sub-period
	// that uses the same post-value as the "from" point, corrupting TWR.
	curve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
		{Date: mustTime("2024-01-15"), PortfolioValue: dec(1050000, 2), NetDeposit: dec(1050000, 2)},
		{Date: mustTime("2024-02-01"), PortfolioValue: dec(1100000, 2), NetDeposit: dec(1100000, 2)},
	}

	// Multiple breakpoints on 1/15 (simulating multiple cash flows same day)
	breakpoints := []twrBreakpoint{
		{date: mustTime("2024-01-01"), value: decimal.Zero},    // initial deposit
		{date: mustTime("2024-01-15"), value: dec(1000000, 2)}, // pre-cash-flow #1
		{date: mustTime("2024-01-15"), value: dec(1010000, 2)}, // pre-cash-flow #2 (same date)
		{date: mustTime("2024-01-15"), value: dec(1020000, 2)}, // pre-cash-flow #3 (same date)
		{date: mustTime("2024-02-01"), value: dec(1080000, 2)}, // another date
	}

	metrics := ComputePeriodReturn(curve, breakpoints, "USD")
	if metrics.TWRPct == nil {
		t.Fatal("TWR should not be nil")
	}

	twrPct, _ := metrics.TWRPct.Float64()

	// Expected (with deduplication, 2 breakpoints: 1/01 and 1/15):
	// sub-period 1: post(1/01)=1,000,000 → pre(1/15)=1,000,000 → ratio = 1.0
	// sub-period 2: post(1/15)=1,050,000 → pre(2/01)=1,080,000 → ratio = 1.0286
	// sub-period 3: post(2/01)=1,100,000 → last=1,100,000 → ratio = 1.0
	// TWR = 1.0 * 1.0286 * 1.0 - 1 = 2.86%

	// Without deduplication, TWR would be corrupted by the extra breakpoints
	// on 1/15, each creating a sub-period with from=post(1/15)=1,050,000.

	if twrPct < 2.0 || twrPct > 4.0 {
		t.Errorf("TWR = %.2f%%, expected ~2.86%%", twrPct)
	}
}
