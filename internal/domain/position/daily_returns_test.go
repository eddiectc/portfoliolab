package position

import (
	"testing"

	"github.com/govalues/decimal"
)

func TestComputeDailyReturns_Basic(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1010000, 2)}, // $10,100 (+1%)
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1020100, 2)}, // $10,201 (+1%)
		{Date: mustTime("2024-01-04"), PortfolioValue: dec(1010000, 2)}, // $10,100 (-1%)
	}

	result := ComputeDailyReturns(points)

	if len(result) != 3 {
		t.Fatalf("got %d returns, want 3", len(result))
	}

	// Day 2: (10100 - 10000) / 10000 × 100 = +1.00%
	wantPct := decimal.MustParse("1.00")
	if !result[0].ReturnPct.Equal(wantPct) {
		t.Errorf("day 2 return: got %s, want %s", result[0].ReturnPct.String(), wantPct.String())
	}
	if !result[0].Date.Equal(mustTime("2024-01-02")) {
		t.Errorf("day 2 date: got %v, want 2024-01-02", result[0].Date)
	}

	// Day 3: (10201 - 10100) / 10100 × 100 = +1.00%
	if !result[1].ReturnPct.Equal(wantPct) {
		t.Errorf("day 3 return: got %s, want %s", result[1].ReturnPct.String(), wantPct.String())
	}

	// Day 4: (10100 - 10201) / 10201 × 100 ≈ -0.9901%
	wantNeg := decimal.MustParse("-0.9901")
	if !result[2].ReturnPct.Equal(wantNeg) {
		t.Errorf("day 4 return: got %s, want %s", result[2].ReturnPct.String(), wantNeg.String())
	}
}

func TestComputeDailyReturns_ZeroValue(t *testing.T) {
	// When prior value is zero, return is undefined — that point is skipped.
	points := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: decimal.Zero},
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1010000, 2)}, // $10,100
	}

	result := ComputeDailyReturns(points)

	// Day 2 is skipped (prior value = 0), only day 3 remains
	if len(result) != 1 {
		t.Fatalf("got %d returns, want 1 (zero-value day skipped)", len(result))
	}

	// Day 3: (10100 - 10000) / 10000 × 100 = +1.00%
	wantPct := decimal.MustParse("1.00")
	if !result[0].ReturnPct.Equal(wantPct) {
		t.Errorf("day 3 return: got %s, want %s", result[0].ReturnPct.String(), wantPct.String())
	}
}

func TestComputeDailyReturns_SinglePoint(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)},
	}

	result := ComputeDailyReturns(points)

	if len(result) != 1 {
		t.Fatalf("got %d returns, want 1", len(result))
	}

	if !result[0].ReturnPct.Equal(decimal.Zero) {
		t.Errorf("single point return: got %s, want 0", result[0].ReturnPct.String())
	}
}

func TestComputeDailyReturns_Empty(t *testing.T) {
	result := ComputeDailyReturns([]EquityCurvePoint{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %d returns", len(result))
	}
}

func TestComputeDailyReturns_AllZeroValues(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: decimal.Zero},
		{Date: mustTime("2024-01-02"), PortfolioValue: decimal.Zero},
		{Date: mustTime("2024-01-03"), PortfolioValue: decimal.Zero},
	}

	result := ComputeDailyReturns(points)
	if len(result) != 0 {
		t.Errorf("expected 0 returns for all-zero values, got %d", len(result))
	}
}

func TestComputeDailyReturns_LargeSwing(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(2000000, 2)}, // $20,000 (+100%)
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(500000, 2)},  // $5,000 (-75%)
	}

	result := ComputeDailyReturns(points)

	if len(result) != 2 {
		t.Fatalf("got %d returns, want 2", len(result))
	}

	want100 := decimal.MustParse("100.00")
	if !result[0].ReturnPct.Equal(want100) {
		t.Errorf("day 2 return: got %s, want %s", result[0].ReturnPct.String(), want100.String())
	}

	wantNeg75 := decimal.MustParse("-75.00")
	if !result[1].ReturnPct.Equal(wantNeg75) {
		t.Errorf("day 3 return: got %s, want %s", result[1].ReturnPct.String(), wantNeg75.String())
	}
}

func TestComputeDailyReturns_NoChange(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2024-01-02"), PortfolioValue: dec(1000000, 2)},
		{Date: mustTime("2024-01-03"), PortfolioValue: dec(1000000, 2)},
	}

	result := ComputeDailyReturns(points)

	for i, ret := range result {
		if !ret.ReturnPct.Equal(decimal.Zero) {
			t.Errorf("day %d return: got %s, want 0", i+2, ret.ReturnPct.String())
		}
	}
}
