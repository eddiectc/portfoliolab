package performance

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

// approxEqual checks if two decimals are within the given tolerance.
func approxEqual(got, want, tolerance decimal.Decimal) bool {
	diff, _ := got.Sub(want)
	diff = diff.Abs()
	return diff.Cmp(tolerance) <= 0
}

func TestComputeNavHistory_InitialDeposit(t *testing.T) {
	// Single deposit of $10,000, no subsequent cash flows.
	// Fixed units = 10000, NAV = 10000/10000 = $1.00.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1010000, 2)}, // $10,100 (+1%)
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(999900, 2)},  // $9,999 (-1.1%)
	}

	result := ComputeNavHistory(equityCurve, nil, mustTime("2024-01-01"))

	if len(result) != 3 {
		t.Fatalf("got %d points, want 3", len(result))
	}

	// Day 1: NAV = 10000/10000 = $1.00
	wantNAV := decimal.MustParse("1.00")
	if !result[0].NavPerUnit.Equal(wantNAV) {
		t.Errorf("day 1 NAV: got %s, want %s", result[0].NavPerUnit.String(), wantNAV.String())
	}
	if !result[0].Units.Equal(decimal.MustNew(10000, 0)) {
		t.Errorf("day 1 units: got %s, want 10000", result[0].Units.String())
	}

	// Day 2: NAV = 10100/10000 = $1.01
	wantNAV2 := decimal.MustParse("1.01")
	if !approxEqual(result[1].NavPerUnit, wantNAV2, decimal.MustParse("0.001")) {
		t.Errorf("day 2 NAV: got %s, want approx %s", result[1].NavPerUnit.String(), wantNAV2.String())
	}

	// Day 3: NAV = 9999/10000 = $0.9999
	wantNAV3 := decimal.MustParse("0.9999")
	if !approxEqual(result[2].NavPerUnit, wantNAV3, decimal.MustParse("0.001")) {
		t.Errorf("day 3 NAV: got %s, want approx %s", result[2].NavPerUnit.String(), wantNAV3.String())
	}
}

func TestComputeNavHistory_SubsequentDeposit(t *testing.T) {
	// Day 1: deposit $10,000 → 10000 units, NAV = $1.00
	// Day 2: market grows to $10,500, NAV = $1.05
	// Day 3: pre-cash-flow = $10,500, deposit $5,000 → portfolio = $15,500
	//   newUnits = 5000 / 1.05 = 4761.9048, totalUnits = 14761.9048
	//   NAV stays $1.05 (cash flow doesn't change NAV)
	// Day 4: market grows to $16,000, NAV = 16000/14761.9048 = $1.0839

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1050000, 2)}, // $10,500
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1550000, 2)}, // $15,500 (after deposit)
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(1600000, 2)}, // $16,000
	}

	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(1050000, 2)}, // pre-cash-flow: $10,500
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 4 {
		t.Fatalf("got %d points, want 4", len(result))
	}

	// Day 1: NAV = $1.00
	if !result[0].NavPerUnit.Equal(decimal.MustParse("1.00")) {
		t.Errorf("day 1 NAV: got %s, want 1.00", result[0].NavPerUnit.String())
	}

	// Day 2: NAV = $1.05
	wantNAV2 := decimal.MustParse("1.05")
	if !approxEqual(result[1].NavPerUnit, wantNAV2, decimal.MustParse("0.001")) {
		t.Errorf("day 2 NAV: got %s, want approx %s", result[1].NavPerUnit.String(), wantNAV2.String())
	}

	// Day 3: after deposit, units = 10000 + 5000/1.05 = 14761.9048
	// NAV should still be $1.05 (unchanged by deposit)
	if !approxEqual(result[2].NavPerUnit, wantNAV2, decimal.MustParse("0.001")) {
		t.Errorf("day 3 NAV: got %s, want approx %s (unchanged by deposit)", result[2].NavPerUnit.String(), wantNAV2.String())
	}
	wantUnits := decimal.MustParse("14761.90")
	if !approxEqual(result[2].Units, wantUnits, decimal.MustParse("1.00")) {
		t.Errorf("day 3 units: got %s, want approx %s", result[2].Units.String(), wantUnits.String())
	}

	// Day 4: NAV = 16000 / 14761.9048 ≈ $1.0839
	wantNAV4 := decimal.MustParse("1.08")
	if result[3].NavPerUnit.Less(wantNAV4) {
		t.Errorf("day 4 NAV: got %s, want at least %s", result[3].NavPerUnit.String(), wantNAV4.String())
	}
}

func TestComputeNavHistory_Withdrawal(t *testing.T) {
	// Day 1: deposit $10,000 → 10000 units, NAV = $1.00
	// Day 2: market grows to $12,000, NAV = $1.20
	// Day 3: pre-cash-flow = $12,000, withdrawal $3,000 → portfolio = $9,000
	//   redeemedUnits = 3000 / 1.20 = 2500, totalUnits = 7500
	//   NAV stays $1.20
	// Day 4: market to $9,375, NAV = 9375/7500 = $1.25

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1200000, 2)}, // $12,000
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(900000, 2)},  // $9,000 (after withdrawal)
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(937500, 2)},  // $9,375
	}

	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(1200000, 2)}, // pre-cash-flow: $12,000
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 4 {
		t.Fatalf("got %d points, want 4", len(result))
	}

	// Day 3: after withdrawal, NAV should still be $1.20
	wantNAV := decimal.MustParse("1.20")
	if !approxEqual(result[2].NavPerUnit, wantNAV, decimal.MustParse("0.001")) {
		t.Errorf("day 3 NAV: got %s, want approx %s", result[2].NavPerUnit.String(), wantNAV.String())
	}

	// Units should be 7500 (10000 - 2500)
	wantUnits := decimal.MustNew(7500, 0)
	if !result[2].Units.Equal(wantUnits) {
		t.Errorf("day 3 units: got %s, want %s", result[2].Units.String(), wantUnits.String())
	}

	// Day 4: NAV = 9375/7500 = $1.25
	wantNAV4 := decimal.MustParse("1.25")
	if !approxEqual(result[3].NavPerUnit, wantNAV4, decimal.MustParse("0.001")) {
		t.Errorf("day 4 NAV: got %s, want approx %s", result[3].NavPerUnit.String(), wantNAV4.String())
	}
}

func TestComputeNavHistory_ZeroValuePortfolio(t *testing.T) {
	// Zero-value portfolio with a valid inception date still gets unitized:
	// 10000 units, NAV = 0/10000 = $0.00.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: decimal.Zero},
	}

	result := ComputeNavHistory(equityCurve, nil, mustTime("2024-01-01"))
	if len(result) != 1 {
		t.Fatalf("got %d points, want 1", len(result))
	}
	if !result[0].NavPerUnit.Equal(decimal.Zero) {
		t.Errorf("NAV: got %s, want 0", result[0].NavPerUnit.String())
	}
	if !result[0].Units.Equal(decimal.MustNew(10000, 0)) {
		t.Errorf("units: got %s, want 10000", result[0].Units.String())
	}
}

func TestComputeNavHistory_EmptyCurve(t *testing.T) {
	result := ComputeNavHistory([]EquityCurvePoint{}, nil, mustTime("2024-01-01"))
	if result != nil {
		t.Errorf("expected nil for empty curve, got %d points", len(result))
	}
}

func TestComputeNavHistory_WithdrawalCap(t *testing.T) {
	// Day 1: deposit $10,000 → 10000 units, NAV = $1.00
	// Day 2: market to $10,000, NAV = $1.00
	// Day 3: pre-cash-flow = $10,000, withdrawal $15,000 (exceeds portfolio)
	//   redeemedUnits = 15000 / 1.00 = 15000, totalUnits = 10000 - 15000 = -5000 → capped at 0

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(-500000, 2)}, // -$5,000 (after excessive withdrawal)
	}

	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(1000000, 2)}, // pre-cash-flow: $10,000
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 3 {
		t.Fatalf("got %d points, want 3", len(result))
	}

	// Units should be capped at 0, not negative
	if result[2].Units.Less(decimal.Zero) {
		t.Errorf("units: got %s (negative), should be capped at 0", result[2].Units.String())
	}
	if !result[2].Units.Equal(decimal.Zero) {
		t.Errorf("units: got %s, want 0", result[2].Units.String())
	}
}

func TestComputeNavHistory_FractionalUnits(t *testing.T) {
	// Deposit $10,000 → 10000 units, NAV = $1.00
	// Market to $10,300, NAV = $1.03
	// Deposit $1,001 → newUnits = 1001/1.03 = 971.8447 (fractional)

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1030000, 2)}, // $10,300
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1130100, 2)}, // $11,301 (after deposit)
	}

	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(1030000, 2)}, // pre-cash-flow: $10,300
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 3 {
		t.Fatalf("got %d points, want 3", len(result))
	}

	// Units should be 10000 + 1001/1.03 ≈ 10971.84
	wantUnits := decimal.MustParse("10971.84")
	if !approxEqual(result[2].Units, wantUnits, decimal.MustParse("1.00")) {
		t.Errorf("units: got %s, want approx %s", result[2].Units.String(), wantUnits.String())
	}

	// NAV should still be $1.03
	wantNAV := decimal.MustParse("1.03")
	if !approxEqual(result[2].NavPerUnit, wantNAV, decimal.MustParse("0.001")) {
		t.Errorf("NAV: got %s, want approx %s", result[2].NavPerUnit.String(), wantNAV.String())
	}
}

func TestComputeNavHistory_MultipleTransactionsSameDay(t *testing.T) {
	// Day 1: deposit $10,000 → 10000 units, NAV = $1.00
	// Day 2: market to $10,000, NAV = $1.00
	// Day 3: two deposits on same day
	//   pre-cash-flow = $10,000, total deposit = $3,000 (both combined)
	//   portfolio after both = $13,000
	//   newUnits = 3000 / 1.00 = 3000, totalUnits = 13000
	//   NAV = $1.00 (unchanged)

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1300000, 2)}, // $13,000 (after both deposits)
	}

	// Multiple breakpoints on same day — only first is used (deduplication)
	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(1000000, 2)}, // pre first deposit
		{Date: mustTime("2024-01-03"), Value: dec(1100000, 2)}, // pre second deposit (ignored)
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 3 {
		t.Fatalf("got %d points, want 3", len(result))
	}

	// Units = 10000 + 3000 = 13000 (cash flow = 13000 - 10000 = 3000)
	wantUnits := decimal.MustNew(13000, 0)
	if !result[2].Units.Equal(wantUnits) {
		t.Errorf("units: got %s, want %s", result[2].Units.String(), wantUnits.String())
	}

	// NAV = $1.00
	wantNAV := decimal.MustParse("1.00")
	if !result[2].NavPerUnit.Equal(wantNAV) {
		t.Errorf("NAV: got %s, want %s", result[2].NavPerUnit.String(), wantNAV.String())
	}
}

func TestComputeNavHistory_MultipleDepositsDifferentDays(t *testing.T) {
	// Day 1: deposit $10,000 → 10000 units, NAV = $1.00
	// Day 2: market to $10,500, NAV = $1.05
	// Day 3: deposit $5,000, pre-cash-flow = $10,500
	//   newUnits = 5000/1.05 = 4761.90, totalUnits = 14761.90
	//   portfolio = $15,500, NAV = $1.05
	// Day 4: market to $16,000, NAV = 16000/14761.90 = $1.0839
	// Day 5: deposit $2,000, pre-cash-flow = $16,000
	//   newUnits = 2000/1.0839 = 1845.17, totalUnits = 16607.07
	//   portfolio = $18,000, NAV = $1.0839

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1050000, 2)}, // $10,500
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1550000, 2)}, // $15,500
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(1600000, 2)}, // $16,000
		{Date: mustTime("2024-01-05"), PortfolioValue: dec(1800000, 2)}, // $18,000
	}

	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(1050000, 2)}, // pre-cash-flow: $10,500
		{Date: mustTime("2024-01-05"), Value: dec(1600000, 2)}, // pre-cash-flow: $16,000
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 5 {
		t.Fatalf("got %d points, want 5", len(result))
	}

	// Day 3: NAV = $1.05 (unchanged by deposit)
	wantNAV3 := decimal.MustParse("1.05")
	if !approxEqual(result[2].NavPerUnit, wantNAV3, decimal.MustParse("0.001")) {
		t.Errorf("day 3 NAV: got %s, want approx %s", result[2].NavPerUnit.String(), wantNAV3.String())
	}

	// Day 5: NAV should equal day 4 NAV (unchanged by deposit)
	if !approxEqual(result[4].NavPerUnit, result[3].NavPerUnit, decimal.MustParse("0.001")) {
		t.Errorf("day 5 NAV: got %s, day 4 NAV: %s (should be equal)",
			result[4].NavPerUnit.String(), result[3].NavPerUnit.String())
	}

	// Day 5: units should be ~16607
	wantUnits5 := decimal.MustParse("16607")
	if !approxEqual(result[4].Units, wantUnits5, decimal.MustParse("10.00")) {
		t.Errorf("day 5 units: got %s, want approx %s", result[4].Units.String(), wantUnits5.String())
	}
}

func TestComputeNavHistory_SinglePoint(t *testing.T) {
	// Just the initial deposit, no subsequent points.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
	}

	result := ComputeNavHistory(equityCurve, nil, mustTime("2024-01-01"))

	if len(result) != 1 {
		t.Fatalf("got %d points, want 1", len(result))
	}

	if !result[0].NavPerUnit.Equal(decimal.MustParse("1.00")) {
		t.Errorf("NAV: got %s, want 1.00", result[0].NavPerUnit.String())
	}
	if !result[0].Units.Equal(decimal.MustNew(10000, 0)) {
		t.Errorf("units: got %s, want 10000", result[0].Units.String())
	}
}

func TestComputeNavHistory_NoMarketChange(t *testing.T) {
	// Portfolio value stays constant, no cash flows.
	// NAV should remain $1.00 throughout.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1000000, 2)},
	}

	result := ComputeNavHistory(equityCurve, nil, mustTime("2024-01-01"))

	for i, point := range result {
		if !point.NavPerUnit.Equal(decimal.MustParse("1.00")) {
			t.Errorf("day %d NAV: got %s, want 1.00", i+1, point.NavPerUnit.String())
		}
	}
}

func TestComputeNavHistory_DatesMatch(t *testing.T) {
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1050000, 2)},
	}

	result := ComputeNavHistory(equityCurve, nil, mustTime("2024-01-01"))

	if !result[0].Date.Equal(mustTime("2024-01-01")) {
		t.Errorf("point 0 date: got %v, want 2024-01-01", result[0].Date)
	}
	if !result[1].Date.Equal(mustTime("2024-01-02")) {
		t.Errorf("point 1 date: got %v, want 2024-01-02", result[1].Date)
	}
}

func TestComputeNavHistory_PortfolioValueMatches(t *testing.T) {
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1050000, 2)},
	}

	result := ComputeNavHistory(equityCurve, nil, mustTime("2024-01-01"))

	for i, point := range result {
		if !point.PortfolioValue.Equal(equityCurve[i].PortfolioValue) {
			t.Errorf("point %d PortfolioValue: got %s, want %s",
				i, point.PortfolioValue.String(), equityCurve[i].PortfolioValue.String())
		}
	}
}

func TestComputeNavHistory_NonDepositFirstTransaction(t *testing.T) {
	// First transaction is a buy (not a deposit). Without an inception date
	// (no deposit occurred), the portfolio remains un-unitized. Points before
	// the inception date show 0 units and 0 NAV.
	//
	// In the real flow, ComputeEquityCurve only sets inceptionDate when a
	// deposit is found. Without a deposit, inceptionDate is zero and
	// ComputeNavHistory returns nil. This test simulates the case where
	// buy/sell transactions exist but no deposit has occurred yet.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1050000, 2)}, // $10,500
	}

	// No deposit → inceptionDate is zero → nil result.
	result := ComputeNavHistory(equityCurve, nil, time.Time{})

	if result != nil {
		t.Errorf("expected nil for no-deposit portfolio, got %d points", len(result))
	}
}

func TestComputeNavHistory_NoDepositEver(t *testing.T) {
	// Only buy/sell transactions, no deposit ever. The system remains
	// un-unitized throughout.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1050000, 2)},
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(999900, 2)},
	}

	// No deposit → inceptionDate is zero → nil result.
	result := ComputeNavHistory(equityCurve, nil, time.Time{})

	if result != nil {
		t.Errorf("expected nil for no-deposit portfolio, got %d points", len(result))
	}
}

func TestComputeNavHistory_BuyBeforeDeposit(t *testing.T) {
	// Buy on Jan 1, deposit on Jan 5. Unitization starts on Jan 5.
	// Points before Jan 5 show 0 units and 0 NAV.
	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000 (buy)
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1010000, 2)}, // $10,100
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(999900, 2)},  // $9,999
		{Date: mustTime("2024-01-05"), PortfolioValue: dec(1500000, 2)}, // $15,000 (after deposit)
		{Date: mustTime("2024-01-06"), PortfolioValue: dec(1515000, 2)}, // $15,150
	}

	// Inception date is the deposit date (Jan 5).
	result := ComputeNavHistory(equityCurve, nil, mustTime("2024-01-05"))

	if len(result) != 5 {
		t.Fatalf("got %d points, want 5", len(result))
	}

	// Points before deposit: un-unitized (0 units, 0 NAV).
	for i := 0; i < 3; i++ {
		if !result[i].Units.Equal(decimal.Zero) {
			t.Errorf("day %d units: got %s, want 0 (before deposit)", i+1, result[i].Units.String())
		}
		if !result[i].NavPerUnit.Equal(decimal.Zero) {
			t.Errorf("day %d NAV: got %s, want 0 (before deposit)", i+1, result[i].NavPerUnit.String())
		}
	}

	// Point on deposit date: unitized with 10000 units.
	// NAV = 15000/10000 = $1.50
	if !result[3].Units.Equal(decimal.MustNew(10000, 0)) {
		t.Errorf("day 4 units: got %s, want 10000", result[3].Units.String())
	}
	wantNAV := decimal.MustParse("1.50")
	if !approxEqual(result[3].NavPerUnit, wantNAV, decimal.MustParse("0.001")) {
		t.Errorf("day 4 NAV: got %s, want approx %s", result[3].NavPerUnit.String(), wantNAV.String())
	}

	// Point after deposit: NAV changes with market.
	// NAV = 15150/10000 = $1.515
	wantNAV2 := decimal.MustParse("1.515")
	if !approxEqual(result[4].NavPerUnit, wantNAV2, decimal.MustParse("0.001")) {
		t.Errorf("day 5 NAV: got %s, want approx %s", result[4].NavPerUnit.String(), wantNAV2.String())
	}
}

func TestComputeNavHistory_LargeDeposit(t *testing.T) {
	// Day 1: deposit $1,000 → 10000 units, NAV = $0.10
	// Day 2: market to $1,100, NAV = $0.11
	// Day 3: deposit $100,000 (100x portfolio), pre-cash-flow = $1,100
	//   newUnits = 100000/0.11 = 909090.91, totalUnits = 919090.91
	//   portfolio = $101,100, NAV = $0.11

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(100000, 2)},   // $1,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(110000, 2)},   // $1,100
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(10110000, 2)}, // $101,100
	}

	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(110000, 2)}, // pre-cash-flow: $1,100
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 3 {
		t.Fatalf("got %d points, want 3", len(result))
	}

	// Day 3: NAV should still be $0.11
	wantNAV := decimal.MustParse("0.11")
	if !approxEqual(result[2].NavPerUnit, wantNAV, decimal.MustParse("0.001")) {
		t.Errorf("day 3 NAV: got %s, want approx %s", result[2].NavPerUnit.String(), wantNAV.String())
	}

	// Units should be ~919091
	wantUnits := decimal.MustParse("919091")
	if !approxEqual(result[2].Units, wantUnits, decimal.MustParse("100.00")) {
		t.Errorf("day 3 units: got %s, want approx %s", result[2].Units.String(), wantUnits.String())
	}
}

func TestComputeNavHistory_WithdrawalThenDeposit(t *testing.T) {
	// Day 1: deposit $10,000 → 10000 units, NAV = $1.00
	// Day 2: market to $12,000, NAV = $1.20
	// Day 3: withdrawal $2,000, pre-cash-flow = $12,000
	//   redeemedUnits = 2000/1.20 = 1666.67, totalUnits = 8333.33
	//   portfolio = $10,000, NAV = $1.20
	// Day 4: market to $10,500, NAV = 10500/8333.33 = $1.26
	// Day 5: deposit $3,000, pre-cash-flow = $10,500
	//   newUnits = 3000/1.26 = 2380.95, totalUnits = 10714.29
	//   portfolio = $13,500, NAV = $1.26

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1200000, 2)}, // $12,000
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1000000, 2)}, // $10,000 (after withdrawal)
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(1050000, 2)}, // $10,500
		{Date: mustTime("2024-01-05"), PortfolioValue: dec(1350000, 2)}, // $13,500 (after deposit)
	}

	breakpoints := []NavBreakpoint{
		{Date: mustTime("2024-01-03"), Value: dec(1200000, 2)}, // pre-cash-flow: $12,000
		{Date: mustTime("2024-01-05"), Value: dec(1050000, 2)}, // pre-cash-flow: $10,500
	}

	result := ComputeNavHistory(equityCurve, breakpoints, mustTime("2024-01-01"))

	if len(result) != 5 {
		t.Fatalf("got %d points, want 5", len(result))
	}

	// Day 3: after withdrawal, NAV = $1.20
	wantNAV3 := decimal.MustParse("1.20")
	if !approxEqual(result[2].NavPerUnit, wantNAV3, decimal.MustParse("0.001")) {
		t.Errorf("day 3 NAV: got %s, want approx %s", result[2].NavPerUnit.String(), wantNAV3.String())
	}

	// Day 5: after deposit, NAV should equal day 4 NAV
	if !approxEqual(result[4].NavPerUnit, result[3].NavPerUnit, decimal.MustParse("0.001")) {
		t.Errorf("day 5 NAV: got %s, day 4 NAV: %s (should be equal after deposit)",
			result[4].NavPerUnit.String(), result[3].NavPerUnit.String())
	}
}
