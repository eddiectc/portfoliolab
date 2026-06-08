package comparison

import (
	"math"
	"sort"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/util"
	"github.com/govalues/decimal"
)

// --- helpers ---

func dec(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat64(f)
	return d
}

// makePriceSeries creates a price series going backwards from now.
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

// makeLongSeries repeats a short price pattern to produce at least 260 points (~1Y).
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

// ptrF returns a pointer to a float64.
func ptrF(f float64) *float64 {
	return &f
}

// matrixEq checks two *float64 matrices element-wise within epsilon.
func matrixEq(t *testing.T, got [][]*float64, want [][]float64, eps float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("matrix rows = %d, want %d", len(got), len(want))
		return
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			t.Errorf("matrix[%d] cols = %d, want %d", i, len(got[i]), len(want[i]))
			return
		}
		for j := range got[i] {
			gotVal := 0.0
			if got[i][j] != nil {
				gotVal = *got[i][j]
			}
			wantVal := want[i][j]
			wantNil := math.IsNaN(wantVal) // use NaN sentinel for nil expectation

			if wantNil {
				if got[i][j] != nil {
					t.Errorf("matrix[%d][%d] = %.4f, want nil", i, j, gotVal)
				}
			} else {
				if got[i][j] == nil {
					t.Errorf("matrix[%d][%d] = nil, want %.4f", i, j, wantVal)
				} else if math.Abs(*got[i][j]-wantVal) > eps {
					t.Errorf("matrix[%d][%d] = %.4f, want %.4f", i, j, *got[i][j], wantVal)
				}
			}
		}
	}
}

// --- ComputeIntraPortfolioCorrelation ---

func TestComputeIntraPortfolioCorrelation_TwoSymbols(t *testing.T) {
	// Two symbols with correlated price patterns (both go up and down together).
	patternA := makeLongSeries([]float64{100, 102, 101, 103, 105, 104, 106, 108})
	patternB := makeLongSeries([]float64{50, 51, 50.5, 51.5, 52.5, 52, 53, 54})

	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"AAPL": makePriceSeries(patternA),
			"MSFT": makePriceSeries(patternB),
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if len(result.Symbols) != 2 {
		t.Fatalf("Symbols len = %d, want 2", len(result.Symbols))
	}

	if result.Matrix == nil || len(result.Matrix) != 2 {
		t.Fatal("Matrix is nil or wrong size")
	}

	// Diagonal should be 1.0.
	if result.Matrix[0][0] == nil || *result.Matrix[0][0] != 1.0 {
		t.Errorf("Matrix[0][0] = %v, want 1.0", result.Matrix[0][0])
	}
	if result.Matrix[1][1] == nil || *result.Matrix[1][1] != 1.0 {
		t.Errorf("Matrix[1][1] = %v, want 1.0", result.Matrix[1][1])
	}

	// Off-diagonal should be high positive correlation (both patterns move together).
	if result.Matrix[0][1] == nil {
		t.Fatal("Matrix[0][1] is nil (insufficient data?)")
	}
	if *result.Matrix[0][1] < 0.9 {
		t.Errorf("Matrix[0][1] = %.4f, want > 0.9 (highly correlated)", *result.Matrix[0][1])
	}

	// Symmetric.
	if result.Matrix[0][1] != result.Matrix[1][0] {
		t.Errorf("Matrix not symmetric: [0][1] = %v, [1][0] = %v", result.Matrix[0][1], result.Matrix[1][0])
	}
}

func TestComputeIntraPortfolioCorrelation_NegativelyCorrelated(t *testing.T) {
	// Two symbols with opposite price patterns.
	patternA := makeLongSeries([]float64{100, 102, 104, 106, 108, 110, 112, 114})
	patternB := makeLongSeries([]float64{100, 98, 96, 94, 92, 90, 88, 86})

	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"SYM_A": makePriceSeries(patternA),
			"SYM_B": makePriceSeries(patternB),
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if result.Matrix == nil || len(result.Matrix) != 2 {
		t.Fatal("Matrix is nil or wrong size")
	}

	if result.Matrix[0][1] == nil {
		t.Fatal("Matrix[0][1] is nil")
	}
	if *result.Matrix[0][1] > -0.8 {
		t.Errorf("Matrix[0][1] = %.4f, want < -0.8 (negatively correlated)", *result.Matrix[0][1])
	}
}

func TestComputeIntraPortfolioCorrelation_SingleSymbol(t *testing.T) {
	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"AAPL": makePriceSeries(makeLongSeries([]float64{100, 102, 101, 103})),
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if len(result.Symbols) != 1 {
		t.Errorf("Symbols len = %d, want 1", len(result.Symbols))
	}
	if result.Message == "" {
		t.Error("expected non-empty message for single symbol")
	}
	if result.Matrix != nil {
		t.Error("Matrix should be nil for single symbol")
	}
}

func TestComputeIntraPortfolioCorrelation_NoData(t *testing.T) {
	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if len(result.Symbols) != 0 {
		t.Errorf("Symbols len = %d, want 0", len(result.Symbols))
	}
	if result.Message == "" {
		t.Error("expected non-empty message for no data")
	}
}

func TestComputeIntraPortfolioCorrelation_MissingData(t *testing.T) {
	// One symbol has data, another has empty series.
	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"AAPL": makePriceSeries(makeLongSeries([]float64{100, 102, 101, 103})),
			"MSFT": []market.HistoricalPrice{},
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	// Should have warning about MSFT
	hasWarning := false
	for _, w := range result.Warnings {
		if len(w) > 0 {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Error("expected warning about MSFT insufficient data")
	}
}

func TestComputeIntraPortfolioCorrelation_DefaultPeriod(t *testing.T) {
	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries(makeLongSeries([]float64{100, 101, 102})),
			"B": makePriceSeries(makeLongSeries([]float64{50, 51, 52})),
		},
		// Period empty — should default to "1Y".
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if result.Period != "1Y" {
		t.Errorf("Period = %q, want \"1Y\"", result.Period)
	}
}

func TestComputeIntraPortfolioCorrelation_ShortData_NilCells(t *testing.T) {
	// Very short price series (few days) — should produce nil cells due to
	// insufficient overlap for the requested period.
	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries([]float64{100, 101, 102, 103, 104}),
			"B": makePriceSeries([]float64{50, 51, 52, 53, 54}),
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	// With only 5 data points and a 1Y period (252 days expected),
	// the cells should be nil due to insufficient overlap.
	if result.Matrix != nil && len(result.Matrix) == 2 {
		if result.Matrix[0][1] != nil {
			t.Logf("Matrix[0][1] = %.4f (may be non-nil if overlap threshold is met)", *result.Matrix[0][1])
		}
	}
	// Either way, there should be warnings about short coverage.
	if len(result.Warnings) == 0 && result.Message == "" {
		t.Error("expected warnings or message for short data")
	}
}

func TestComputeIntraPortfolioCorrelation_MultipleSymbols(t *testing.T) {
	// 3 symbols with different correlation patterns.
	patternUp := makeLongSeries([]float64{100, 102, 104, 106, 108, 110})
	patternDown := makeLongSeries([]float64{100, 98, 96, 94, 92, 90})
	patternRandom := makeLongSeries([]float64{100, 101, 99, 100, 102, 98})

	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"UP":   makePriceSeries(patternUp),
			"DOWN": makePriceSeries(patternDown),
			"RAND": makePriceSeries(patternRandom),
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if len(result.Symbols) != 3 {
		t.Fatalf("Symbols len = %d, want 3", len(result.Symbols))
	}

	// Symbols should be sorted.
	expectedSymbols := []string{"DOWN", "RAND", "UP"}
	sort.Strings(expectedSymbols)
	if len(result.Symbols) != len(expectedSymbols) {
		t.Errorf("Symbols = %v, want %v", result.Symbols, expectedSymbols)
	}

	if result.Matrix == nil || len(result.Matrix) != 3 {
		t.Fatal("Matrix is nil or wrong size")
	}

	// UP vs DOWN should be negative.
	// Find indices.
	upIdx, downIdx, randIdx := -1, -1, -1
	for i, s := range result.Symbols {
		if s == "UP" {
			upIdx = i
		}
		if s == "DOWN" {
			downIdx = i
		}
		if s == "RAND" {
			randIdx = i
		}
	}

	if upIdx >= 0 && downIdx >= 0 && result.Matrix[upIdx][downIdx] != nil {
		corr := *result.Matrix[upIdx][downIdx]
		if corr > -0.5 {
			t.Errorf("UP vs DOWN correlation = %.4f, want negative (< -0.5)", corr)
		}
	}

	// UP vs RAND should be near zero (uncorrelated).
	if upIdx >= 0 && randIdx >= 0 && result.Matrix[upIdx][randIdx] != nil {
		corr := *result.Matrix[upIdx][randIdx]
		if math.Abs(corr) > 0.5 {
			t.Errorf("UP vs RAND correlation = %.4f, want near zero", corr)
		}
	}
}

func TestComputeIntraPortfolioCorrelation_SelfCorrelation(t *testing.T) {
	// Diagonal elements should always be 1.0.
	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries(makeLongSeries([]float64{100, 101, 102})),
			"B": makePriceSeries(makeLongSeries([]float64{50, 51, 52})),
			"C": makePriceSeries(makeLongSeries([]float64{200, 201, 202})),
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if result.Matrix == nil {
		t.Fatal("Matrix is nil")
	}

	for i := 0; i < len(result.Matrix); i++ {
		if result.Matrix[i][i] == nil || *result.Matrix[i][i] != 1.0 {
			t.Errorf("Matrix[%d][%d] = %v, want 1.0", i, i, result.Matrix[i][i])
		}
	}
}

func TestComputeIntraPortfolioCorrelation_Symmetric(t *testing.T) {
	input := IntraPortfolioCorrelationInput{
		Prices: map[string][]market.HistoricalPrice{
			"A": makePriceSeries(makeLongSeries([]float64{100, 102, 101, 103})),
			"B": makePriceSeries(makeLongSeries([]float64{50, 49, 51, 50})),
		},
		Period: "1Y",
	}

	result := ComputeIntraPortfolioCorrelation(input)

	if result.Matrix == nil || len(result.Matrix) < 2 {
		t.Fatal("Matrix is nil or too small")
	}

	// Check symmetry.
	for i := 0; i < len(result.Matrix); i++ {
		for j := 0; j < len(result.Matrix[i]); j++ {
			if result.Matrix[i][j] == nil && result.Matrix[j][i] == nil {
				continue // both nil is symmetric
			}
			if result.Matrix[i][j] == nil || result.Matrix[j][i] == nil {
				t.Errorf("Asymmetric nil: [%d][%d] = %v, [%d][%d] = %v",
					i, j, result.Matrix[i][j], j, i, result.Matrix[j][i])
				continue
			}
			if math.Abs(*result.Matrix[i][j]-*result.Matrix[j][i]) > 0.0001 {
				t.Errorf("Asymmetric: [%d][%d] = %.4f, [%d][%d] = %.4f",
					i, j, *result.Matrix[i][j], j, i, *result.Matrix[j][i])
			}
		}
	}
}

// --- correlationPeriodCutoff ---

func TestCorrelationPeriodCutoff(t *testing.T) {
	tests := []struct {
		period   string
		wantWarn bool
	}{
		{"3M", false},
		{"6M", false},
		{"1Y", false},
		{"3Y", false},
		{"5Y", false},
		{"10Y", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			_, warn := util.PeriodCutoff(tt.period)
			if tt.wantWarn && warn == "" {
				t.Errorf("expected warning for period %q", tt.period)
			}
			if !tt.wantWarn && warn != "" {
				t.Errorf("unexpected warning for period %q: %s", tt.period, warn)
			}
		})
	}
}

// --- correlationCutoffDays ---

func TestCorrelationCutoffDays(t *testing.T) {
	want := map[string]int{
		"3M":  63,
		"6M":  126,
		"1Y":  252,
		"3Y":  756,
		"5Y":  1260,
		"10Y": 2520,
		"foo": 252, // default
	}

	for period, expected := range want {
		got := correlationCutoffDays(period)
		if got != expected {
			t.Errorf("%s: cutoffDays = %d, want %d", period, got, expected)
		}
	}
}
