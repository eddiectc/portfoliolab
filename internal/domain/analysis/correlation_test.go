package analysis

import (
	"math"
	"sort"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- helpers ---

// dec converts a float64 to a decimal.Decimal (for test data).
func dec(f float64) decimal.Decimal {
	// NewFromFloat64 is lossy but fine for test fixtures.
	d, _ := decimal.NewFromFloat64(f)
	return d
}

// makeSeries creates a price series for one symbol with the given daily prices.
// Prices are assigned to consecutive trading days going backwards from now.
func makeSeries(sym string, prices []float64) map[string][]market.HistoricalPrice {
	now := time.Now()
	result := make(map[string][]market.HistoricalPrice)
	for i, p := range prices {
		d := now.AddDate(0, 0, -i)
		result[sym] = append(result[sym], market.HistoricalPrice{
			Date:     d,
			Close:    dec(p),
			Currency: "USD",
		})
	}
	return result
}

// makeMultiSeries creates price series for multiple symbols.
// Each entry is (symbol, []prices).
func makeMultiSeries(entries []struct {
	sym    string
	prices []float64
}) map[string][]market.HistoricalPrice {
	now := time.Now()
	result := make(map[string][]market.HistoricalPrice)
	for _, e := range entries {
		for i, p := range e.prices {
			d := now.AddDate(0, 0, -i)
			result[e.sym] = append(result[e.sym], market.HistoricalPrice{
				Date:     d,
				Close:    dec(p),
				Currency: "USD",
			})
		}
	}
	return result
}

// matrixEq checks two float64 matrices are element-wise equal within epsilon.
func matrixEq(t *testing.T, got, want [][]float64, eps float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("matrix rows = %d, want %d", len(got), len(want))
		return
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			t.Errorf("matrix row %d cols = %d, want %d", i, len(got[i]), len(want[i]))
			return
		}
		for j := range got[i] {
			if math.Abs(got[i][j]-want[i][j]) > eps {
				t.Errorf("matrix[%d][%d] = %.6f, want %.6f", i, j, got[i][j], want[i][j])
			}
		}
	}
}

// sliceEq checks two string slices are equal.
func sliceEq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("slice len = %d, want %d; got %v, want %v", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("slice[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- Test ComputeDailyReturnsWithDates ---

func TestComputeDailyReturnsWithDates(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		series []market.HistoricalPrice
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

// --- Test pearsonCorrelation ---

func TestPearsonCorrelation(t *testing.T) {
	tests := []struct {
		name     string
		x        []float64
		y        []float64
		wantR    float64
		wantN    int
	}{
		{
			name:    "perfect positive correlation",
			x:       []float64{1, 2, 3, 4, 5},
			y:       []float64{2, 4, 6, 8, 10},
			wantR:   1.0,
			wantN:   5,
		},
		{
			name:    "perfect negative correlation",
			x:       []float64{1, 2, 3, 4, 5},
			y:       []float64{10, 8, 6, 4, 2},
			wantR:   -1.0,
			wantN:   5,
		},
		{
			name:    "zero correlation (symmetric parabola)",
			x:       []float64{-3, -2, -1, 0, 1, 2, 3},
			y:       []float64{1, 4, 1, 0, 1, 4, 1},
			wantR:   0.0,
			wantN:   7,
		},
		{
			name:    "identical series",
			x:       []float64{0.01, -0.02, 0.03, -0.01, 0.02},
			y:       []float64{0.01, -0.02, 0.03, -0.01, 0.02},
			wantR:   1.0,
			wantN:   5,
		},
		{
			name:    "zero variance in x",
			x:       []float64{5, 5, 5, 5},
			y:       []float64{1, 2, 3, 4},
			wantR:   0,
			wantN:   4,
		},
		{
			name:    "zero variance in y",
			x:       []float64{1, 2, 3, 4},
			y:       []float64{5, 5, 5, 5},
			wantR:   0,
			wantN:   4,
		},
		{
			name:    "empty series",
			x:       []float64{},
			y:       []float64{},
			wantR:   0,
			wantN:   0,
		},
		{
			name:    "unequal length",
			x:       []float64{1, 2, 3},
			y:       []float64{1, 2},
			wantR:   0,
			wantN:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, n := pearsonCorrelation(tt.x, tt.y)
			if n != tt.wantN {
				t.Errorf("n = %d, want %d", n, tt.wantN)
			}
			if tt.wantN > 0 && math.Abs(r-tt.wantR) > 0.0001 {
				t.Errorf("r = %.6f, want %.6f", r, tt.wantR)
			}
		})
	}
}

// --- Test alignReturns ---

func TestAlignReturns(t *testing.T) {
	now := time.Now()

	a := []dailyReturn{
		{date: now.AddDate(0, 0, -3).Unix(), return_: 0.01},
		{date: now.AddDate(0, 0, -2).Unix(), return_: -0.02},
		{date: now.AddDate(0, 0, -1).Unix(), return_: 0.03},
	}

	// b shares 2 dates with a (day -3 and -1), misses day -2, has extra day -0.
	b := []dailyReturn{
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
	// Day -3: a=0.01, b=0.02
	if math.Abs(x[0]-0.01) > 0.0001 || math.Abs(y[0]-0.02) > 0.0001 {
		t.Errorf("first pair: x=%.4f, y=%.4f, want 0.01, 0.02", x[0], y[0])
	}
	// Day -1: a=0.03, b=-0.01
	if math.Abs(x[1]-0.03) > 0.0001 || math.Abs(y[1]-(-0.01)) > 0.0001 {
		t.Errorf("second pair: x=%.4f, y=%.4f, want 0.03, -0.01", x[1], y[1])
	}
}

// --- Test ComputeCorrelation ---

func TestComputeCorrelation(t *testing.T) {
	tests := []struct {
		name          string
		prices        map[string][]market.HistoricalPrice
		period        string
		wantMatrix    [][]float64
		wantSymbols   []string
		wantMessage   string
		wantWarningRe string // substring to match in warnings
	}{
		{
			name: "perfect positive correlation (identical series)",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				{"AAPL", []float64{100, 102, 101, 103, 105, 104, 106}},
				{"MSFT", []float64{100, 102, 101, 103, 105, 104, 106}},
			}),
			period: "10Y",
			wantMatrix: [][]float64{
				{1.0, 1.0},
				{1.0, 1.0},
			},
			wantSymbols: []string{"AAPL", "MSFT"},
			wantMessage: "",
		},

		{
			name: "strong negative correlation (opposite oscillations)",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				// A: up, down, up, down pattern
				{"SYM_A", []float64{100, 105, 100, 105, 100}},
				// B: down, up, down, up pattern (opposite)
				{"SYM_B", []float64{100, 95, 100, 95, 100}},
			}),
			period: "10Y",
			wantMatrix: [][]float64{
				{1.0, -1.0},
				{-1.0, 1.0},
			},
			wantSymbols: []string{"SYM_A", "SYM_B"},
			wantMessage: "",
		},

		{
			name: "3 symbols with mixed correlations",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				{"A", []float64{100, 105, 100, 105, 100}},
				{"B", []float64{100, 105, 100, 105, 100}}, // identical to A → r=1
				{"C", []float64{100, 95, 100, 95, 100}},   // opposite oscillation → r≈-1
			}),
			period: "10Y",
			wantMatrix: [][]float64{
				{1.0, 1.0, -1.0},
				{1.0, 1.0, -1.0},
				{-1.0, -1.0, 1.0},
			},
			wantSymbols: []string{"A", "B", "C"},
			wantMessage: "",
		},

		{
			name: "single symbol returns message",
			prices: makeSeries("AAPL", []float64{100, 102, 101}),
			period: "10Y",
			wantMatrix: nil,
			wantSymbols: []string{"AAPL"},
			wantMessage: "Only 1 symbol has sufficient price data. Correlation requires at least 2.",
		},

		{
			name:   "no symbols returns message",
			prices: map[string][]market.HistoricalPrice{},
			period: "10Y",
			wantMatrix: nil,
			wantSymbols: []string{},
			wantMessage: "Correlation requires at least 2 symbols with price data.",
		},

		{
			name: "missing prices for a symbol generates warning",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				{"AAPL", []float64{100, 102, 101, 103, 105}},
				{"MSFT", []float64{100, 102, 101, 103, 105}},
				{"GOOGL", []float64{100}}, // only 1 price → no returns
			}),
			period: "10Y",
			wantMatrix: [][]float64{
				{1.0, 1.0},
				{1.0, 1.0},
			},
			wantSymbols:   []string{"AAPL", "MSFT"},
			wantMessage:   "",
			wantWarningRe: "GOOGL",
		},

		{
			name: "insufficient overlap generates warning",
			prices: func() map[string][]market.HistoricalPrice {
				now := time.Now()
				result := make(map[string][]market.HistoricalPrice)
				// A has 5 prices (4 returns)
				for i := 0; i < 5; i++ {
					d := now.AddDate(0, 0, -i)
					result["A"] = append(result["A"], market.HistoricalPrice{
						Date: d, Close: dec(100+float64(i)),
					})
				}
				// B has 5 prices but on completely different dates (100 days earlier)
				for i := 0; i < 5; i++ {
					d := now.AddDate(0, 0, -(100+i))
					result["B"] = append(result["B"], market.HistoricalPrice{
						Date: d, Close: dec(100+float64(i)),
					})
				}
				return result
			}(),
			period: "10Y",
			wantMatrix: [][]float64{
				{1.0, 0},
				{0, 1.0},
			},
			wantSymbols:   []string{"A", "B"},
			wantMessage:   "",
			wantWarningRe: "overlapping days",
		},

		{
			name: "mismatched date ranges uses available overlap",
			prices: func() map[string][]market.HistoricalPrice {
				now := time.Now()
				result := make(map[string][]market.HistoricalPrice)
				// A: days 0-9 (10 prices, 9 returns)
				for i := 0; i < 10; i++ {
					d := now.AddDate(0, 0, -i)
					result["A"] = append(result["A"], market.HistoricalPrice{
						Date: d, Close: dec(100+float64(i)*0.5),
					})
				}
				// B: days 3-12 (10 prices, 9 returns), overlapping days 3-9 (7 returns)
				for i := 3; i < 13; i++ {
					d := now.AddDate(0, 0, -i)
					result["B"] = append(result["B"], market.HistoricalPrice{
						Date: d, Close: dec(100+float64(i-3)*0.5),
					})
				}
				return result
			}(),
			period: "10Y",
			wantMatrix: nil, // just check it doesn't crash and produces valid output
			wantSymbols: []string{"A", "B"},
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeCorrelation(tt.prices, tt.period)

			// Check message.
			if got.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMessage)
			}

			// Check symbols (sorted).
			gotSyms := make([]string, len(got.Symbols))
			copy(gotSyms, got.Symbols)
			sort.Strings(gotSyms)
			sliceEq(t, gotSyms, tt.wantSymbols)

			// Check matrix if expected.
			if tt.wantMatrix != nil {
				if got.Matrix == nil {
					t.Errorf("Matrix is nil, want non-nil")
				} else {
					matrixEq(t, got.Matrix, tt.wantMatrix, 0.01)
				}
			}

			// Check warning substring if expected.
			if tt.wantWarningRe != "" {
				found := false
				for _, w := range got.Warnings {
					if contains(w, tt.wantWarningRe) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q, got %v", tt.wantWarningRe, got.Warnings)
				}
			}
		})
	}
}

// --- Test period filtering ---

func TestPeriodCutoff(t *testing.T) {
	now := time.Now()
	tests := []struct {
		period     string
		wantYears  int
	}{
		{"1Y", 1},
		{"3Y", 3},
		{"5Y", 5},
		{"10Y", 10},
		{"invalid", 1}, // defaults to 1Y
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			cutoff := periodCutoff(tt.period)
			expected := now.AddDate(-tt.wantYears, 0, 0)
			diff := cutoff.Sub(expected).Hours()
			if math.Abs(diff) > 1 {
				t.Errorf("cutoff = %v, want ~%v (diff %.0f hours)", cutoff, expected, diff)
			}
		})
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
