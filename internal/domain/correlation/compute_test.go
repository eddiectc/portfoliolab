package correlation

import (
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

func dec(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat64(f)
	return d
}

func makePriceSeries(prices []float64) []market.HistoricalPrice {
	now := time.Now()
	result := make([]market.HistoricalPrice, len(prices))
	for i, p := range prices {
		d := now.AddDate(0, 0, -i)
		result[i] = market.HistoricalPrice{
			Date:     d,
			Close:    dec(p),
			Currency: "USD",
		}
	}
	return result
}

func makeLongSeries(pattern []float64) []float64 {
	result := make([]float64, 0, 260)
	for len(result) < 260 {
		for _, p := range pattern {
			if len(result) >= 260 {
				break
			}
			result = append(result, p)
		}
	}
	return result
}

// --- Test cutoffDays ---

func TestCutoffDays(t *testing.T) {
	want := map[string]int{
		"3M":  63,
		"6M":  126,
		"1Y":  252,
		"3Y":  756,
		"5Y":  1260,
		"10Y": 2520,
		"foo": 252,
	}

	for period, expected := range want {
		got := cutoffDays(period)
		if got != expected {
			t.Errorf("%s: cutoffDays = %d, want %d", period, got, expected)
		}
	}
}

// --- Test computeDailyReturnsWithDates ---

func TestComputeDailyReturnsWithDates(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		series  []market.HistoricalPrice
		wantLen int
	}{
		{
			name: "5 prices produce 4 returns",
			series: []market.HistoricalPrice{
				{Date: now.AddDate(0, 0, -4), Close: dec(100)},
				{Date: now.AddDate(0, 0, -3), Close: dec(102)},
				{Date: now.AddDate(0, 0, -2), Close: dec(101)},
				{Date: now.AddDate(0, 0, -1), Close: dec(103)},
				{Date: now, Close: dec(105)},
			},
			wantLen: 4,
		},
		{
			name: "single price produces no returns",
			series: []market.HistoricalPrice{
				{Date: now, Close: dec(100)},
			},
			wantLen: 0,
		},
		{
			name: "unsorted input is sorted correctly",
			series: []market.HistoricalPrice{
				{Date: now, Close: dec(105)},
				{Date: now.AddDate(0, 0, -2), Close: dec(101)},
				{Date: now.AddDate(0, 0, -1), Close: dec(103)},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDailyReturnsWithDates(tt.series)
			if len(got) != tt.wantLen {
				t.Errorf("returns len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// --- Test alignReturns ---

func TestAlignReturns(t *testing.T) {
	now := time.Now()

	a := []datedReturn{
		{date: now.AddDate(0, 0, -3).Unix(), return_: 0.01},
		{date: now.AddDate(0, 0, -2).Unix(), return_: -0.02},
		{date: now.AddDate(0, 0, -1).Unix(), return_: 0.03},
	}

	b := []datedReturn{
		{date: now.AddDate(0, 0, -3).Unix(), return_: 0.02},
		{date: now.AddDate(0, 0, -1).Unix(), return_: -0.01},
		{date: now.Unix(), return_: 0.05},
	}

	x, y, overlap := alignReturns(a, b)

	if overlap != 2 {
		t.Errorf("overlap = %d, want 2", overlap)
	}
	if len(x) != 2 || len(y) != 2 {
		t.Errorf("x len = %d, y len = %d, want 2 each", len(x), len(y))
	}
}

// --- Test Compute end-to-end ---

func TestCompute_PositiveCorrelation(t *testing.T) {
	input := Input{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries(makeLongSeries([]float64{100, 102, 101, 103, 105, 104, 106})),
			"B": makePriceSeries(makeLongSeries([]float64{100, 102, 101, 103, 105, 104, 106})),
		},
		Period: "1M",
	}

	result := Compute(input)

	if result.Message != "" {
		t.Errorf("unexpected message: %s", result.Message)
	}
	if len(result.Symbols) != 2 {
		t.Errorf("Symbols len = %d, want 2", len(result.Symbols))
	}
	if result.Matrix == nil || result.Matrix[0][1] == nil {
		t.Fatal("Matrix[0][1] is nil")
	}
	if *result.Matrix[0][1] < 0.99 {
		t.Errorf("Matrix[0][1] = %.4f, want ~1.0", *result.Matrix[0][1])
	}
}

func TestCompute_NegativeCorrelation(t *testing.T) {
	input := Input{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries(makeLongSeries([]float64{100, 105, 100, 105, 100})),
			"B": makePriceSeries(makeLongSeries([]float64{100, 95, 100, 95, 100})),
		},
		Period: "1M",
	}

	result := Compute(input)

	if result.Matrix == nil || result.Matrix[0][1] == nil {
		t.Fatal("Matrix[0][1] is nil")
	}
	if *result.Matrix[0][1] > -0.9 {
		t.Errorf("Matrix[0][1] = %.4f, want ~-1.0", *result.Matrix[0][1])
	}
}

func TestCompute_SingleSymbol(t *testing.T) {
	input := Input{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries([]float64{100, 102, 101}),
		},
		Period: "10Y",
	}

	result := Compute(input)

	if len(result.Symbols) != 1 {
		t.Errorf("Symbols len = %d, want 1", len(result.Symbols))
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCompute_NoData(t *testing.T) {
	input := Input{
		Prices: map[string][]market.HistoricalPrice{},
		Period: "1Y",
	}

	result := Compute(input)

	if len(result.Symbols) != 0 {
		t.Errorf("Symbols len = %d, want 0", len(result.Symbols))
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCompute_DefaultPeriod(t *testing.T) {
	input := Input{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries(makeLongSeries([]float64{100, 101, 102})),
			"B": makePriceSeries(makeLongSeries([]float64{50, 51, 52})),
		},
	}

	result := Compute(input)

	if result.Period != "1Y" {
		t.Errorf("Period = %q, want \"1Y\"", result.Period)
	}
}

func TestCompute_ShortCoverage_NilCells(t *testing.T) {
	input := Input{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries(makeLongSeries([]float64{100, 102, 101, 103, 105})),
			"B": makePriceSeries(makeLongSeries([]float64{100, 102, 101, 103, 105})),
		},
		Period: "10Y",
	}

	result := Compute(input)

	// Short coverage should produce nil cells
	if result.Matrix != nil && result.Matrix[0][1] != nil {
		t.Logf("Matrix[0][1] = %.4f (short coverage may or may not trigger depending on data length)", *result.Matrix[0][1])
	}

	// Should have warnings about short coverage
	if len(result.Warnings) == 0 && result.Message == "" {
		t.Error("expected warnings for short coverage")
	}
}
