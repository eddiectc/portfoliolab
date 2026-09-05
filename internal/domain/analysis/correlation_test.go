package analysis

import (
	"sort"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- helpers ---

func dec(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat64(f)
	return d
}

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

func matrixEq(t *testing.T, got [][]*float64, want [][]float64, eps float64) {
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
			wantVal := want[i][j]
			gotPtr := got[i][j]
			if wantVal == -999 {
				if gotPtr != nil {
					t.Errorf("matrix[%d][%d] = %v, want nil", i, j, *gotPtr)
				}
			} else if gotPtr == nil {
				t.Errorf("matrix[%d][%d] = nil, want %.6f", i, j, wantVal)
			} else if mathAbs(*gotPtr-wantVal) > eps {
				t.Errorf("matrix[%d][%d] = %.6f, want %.6f", i, j, *gotPtr, wantVal)
			}
		}
	}
}

func mathAbs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

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

// --- Test ComputeCorrelation ---

func TestComputeCorrelation(t *testing.T) {
	tests := []struct {
		name          string
		prices        map[string][]market.HistoricalPrice
		period        string
		wantMatrix    [][]float64
		wantSymbols   []string
		wantMessage   string
		wantWarningRe string
	}{
		{
			name: "perfect positive correlation (identical series)",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				{"AAPL", makeLongSeries([]float64{100, 102, 101, 103, 105, 104, 106})},
				{"MSFT", makeLongSeries([]float64{100, 102, 101, 103, 105, 104, 106})},
			}),
			period: "1M",
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
				{"SYM_A", makeLongSeries([]float64{100, 105, 100, 105, 100})},
				{"SYM_B", makeLongSeries([]float64{100, 95, 100, 95, 100})},
			}),
			period: "1M",
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
				{"A", makeLongSeries([]float64{100, 105, 100, 105, 100})},
				{"B", makeLongSeries([]float64{100, 105, 100, 105, 100})},
				{"C", makeLongSeries([]float64{100, 95, 100, 95, 100})},
			}),
			period: "1M",
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
			prices: map[string][]market.HistoricalPrice{
				"AAPL": makePriceSeries([]float64{100, 102, 101}),
			},
			period:      "10Y",
			wantMatrix:  nil,
			wantSymbols: []string{"AAPL"},
			wantMessage: "Only 1 symbol has sufficient price data. Correlation requires at least 2.",
		},

		{
			name:        "no symbols returns message",
			prices:      map[string][]market.HistoricalPrice{},
			period:      "10Y",
			wantMatrix:  nil,
			wantSymbols: []string{},
			wantMessage: "Correlation requires at least 2 symbols with price data.",
		},

		{
			name: "missing prices for a symbol generates warning",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				{"AAPL", makeLongSeries([]float64{100, 102, 101, 103, 105})},
				{"MSFT", makeLongSeries([]float64{100, 102, 101, 103, 105})},
				{"GOOGL", []float64{100}},
			}),
			period: "1M",
			wantMatrix: [][]float64{
				{1.0, 1.0},
				{1.0, 1.0},
			},
			wantSymbols:   []string{"AAPL", "MSFT"},
			wantMessage:   "",
			wantWarningRe: "GOOGL",
		},

		{
			name: "short coverage marks cells as nil",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				{"A", makeLongSeries([]float64{100, 102, 101, 103, 105})},
				{"B", makeLongSeries([]float64{100, 102, 101, 103, 105})},
			}),
			period: "10Y",
			wantMatrix: [][]float64{
				{1.0, -999},
				{-999, 1.0},
			},
			wantSymbols:   []string{"A", "B"},
			wantMessage:   "",
			wantWarningRe: "price data (requested period)",
		},

		{
			name: "invalid period generates warning",
			prices: makeMultiSeries([]struct {
				sym    string
				prices []float64
			}{
				{"A", makeLongSeries([]float64{100, 102, 101, 103, 105})},
				{"B", makeLongSeries([]float64{100, 102, 101, 103, 105})},
			}),
			period: "invalid",
			wantMatrix: [][]float64{
				{1.0, 1.0},
				{1.0, 1.0},
			},
			wantSymbols:   []string{"A", "B"},
			wantMessage:   "",
			wantWarningRe: "unrecognized period",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeCorrelation(tt.prices, tt.period)

			if got.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMessage)
			}

			gotSyms := make([]string, len(got.Symbols))
			copy(gotSyms, got.Symbols)
			sort.Strings(gotSyms)
			sliceEq(t, gotSyms, tt.wantSymbols)

			if tt.wantMatrix != nil {
				if got.Matrix == nil {
					t.Errorf("Matrix is nil, want non-nil")
				} else {
					matrixEq(t, got.Matrix, tt.wantMatrix, 0.01)
				}
			}

			if tt.wantWarningRe != "" {
				found := false
				for _, w := range got.Warnings {
					if stringContains(w, tt.wantWarningRe) {
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

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
