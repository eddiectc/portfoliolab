package hierarchicalriskparity

import (
	"math"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
)

// makePricesForHrp creates a price series with the given daily closes starting
// from baseDate. Used in hrp_test.go (returns_test.go has its own makePrices).
func makePricesForHrp(base time.Time, closes []float64) []market.HistoricalPrice {
	prices := make([]market.HistoricalPrice, len(closes))
	for i, c := range closes {
		d := base.AddDate(0, 0, i)
		prices[i] = market.HistoricalPrice{
			Date:  d,
			Close: dec(c),
		}
	}
	return prices
}

// hrpAlmostEqual compares two floats within tolerance.
func hrpAlmostEqual(t *testing.T, got, want, tol float64, msg string) {
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.10f, want %.10f (tol %.2e)", msg, got, want, tol)
	}
}

// --- Test computeCovarianceMatrix ---

func TestComputeCovarianceMatrix(t *testing.T) {
	tests := []struct {
		name     string
		aligned  [][]float64
		wantDiag []float64 // expected diagonal values (variances)
	}{
		{
			name: "two identical series — zero variance",
			aligned: [][]float64{
				{0.01, 0.01},
				{0.01, 0.01},
				{0.01, 0.01},
			},
			wantDiag: []float64{0, 0},
		},
		{
			name: "two independent series",
			aligned: [][]float64{
				{0.01, -0.01},
				{-0.01, 0.01},
				{0.02, -0.02},
				{-0.02, 0.02},
			},
			wantDiag: []float64{0.001 / 3, 0.001 / 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cov := computeCovarianceMatrix(tt.aligned)
			n := len(tt.wantDiag)
			if len(cov) != n {
				t.Fatalf("matrix rows = %d, want %d", len(cov), n)
			}
			for i, want := range tt.wantDiag {
				hrpAlmostEqual(t, cov[i][i], want, 1e-6, "cov["+string(rune('0'+i))+"]["+string(rune('0'+i))+"]")
			}
			// Symmetry check.
			for i := 0; i < n; i++ {
				for j := i + 1; j < n; j++ {
					hrpAlmostEqual(t, cov[i][j], cov[j][i], 1e-15, "symmetry")
				}
			}
		})
	}
}

// --- Test ComputeHrp ---

func TestComputeHrp(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		symbols    []string
		prices     map[string][]market.HistoricalPrice
		period     string
		wantErr    bool
		wantErrSub string
		wantAllocs int
	}{
		{
			name:       "single symbol — error",
			symbols:    []string{"AAPL"},
			prices:     map[string][]market.HistoricalPrice{"AAPL": makePricesForHrp(base, []float64{100, 101, 102})},
			wantErr:    true,
			wantErrSub: "insufficient_symbols",
		},
		{
			name:    "two symbols — basic",
			symbols: []string{"AAPL", "GOOGL"},
			prices: map[string][]market.HistoricalPrice{
				"AAPL":  makePricesForHrp(base, []float64{100, 101, 102, 101, 103, 102, 104}),
				"GOOGL": makePricesForHrp(base, []float64{200, 201, 199, 202, 200, 203, 201}),
			},
			period:     "3Y",
			wantAllocs: 4,
		},
		{
			name:    "three symbols — basic",
			symbols: []string{"A", "B", "C"},
			prices: map[string][]market.HistoricalPrice{
				"A": makePricesForHrp(base, []float64{100, 102, 101, 103, 102, 104, 103, 105, 104, 106}),
				"B": makePricesForHrp(base, []float64{50, 49, 50, 48, 51, 49, 50, 48, 51, 50}),
				"C": makePricesForHrp(base, []float64{80, 81, 80, 82, 81, 79, 82, 80, 81, 83}),
			},
			period:     "3Y",
			wantAllocs: 4,
		},
		{
			name:    "five symbols",
			symbols: []string{"S1", "S2", "S3", "S4", "S5"},
			prices: map[string][]market.HistoricalPrice{
				"S1": makePricesForHrp(base, generateDeterministicWalk(100, 100, 0.02)),
				"S2": makePricesForHrp(base, generateDeterministicWalk(50, 100, 0.03)),
				"S3": makePricesForHrp(base, generateDeterministicWalk(200, 100, 0.01)),
				"S4": makePricesForHrp(base, generateDeterministicWalk(75, 100, 0.04)),
				"S5": makePricesForHrp(base, generateDeterministicWalk(150, 100, 0.015)),
			},
			period:     "3Y",
			wantAllocs: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := HrpRequest{
				Symbols: tt.symbols,
				Prices:  tt.prices,
				Period:  tt.period,
			}

			result, err := ComputeHrp(request)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrSub != "" {
					got := err.Error()
					found := false
					for _, sub := range []string{tt.wantErrSub} {
						if len(got) >= len(sub) {
							for i := 0; i <= len(got)-len(sub); i++ {
								if got[i:i+len(sub)] == sub {
									found = true
									break
								}
							}
						}
					}
					if !found {
						t.Errorf("error = %q, want to contain %q", got, tt.wantErrSub)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Allocations) != tt.wantAllocs {
				t.Fatalf("allocations = %d, want %d", len(result.Allocations), tt.wantAllocs)
			}

			// Verify each allocation.
			for _, alloc := range result.Allocations {
				// Weights should cover all symbols.
				if len(alloc.Weights) != len(tt.symbols) {
					t.Errorf("allocation %q: weights = %d, want %d",
						alloc.Method, len(alloc.Weights), len(tt.symbols))
				}

				// Weights should sum to ~1.0.
				sum := 0.0
				for _, w := range alloc.Weights {
					sum += w
				}
				hrpAlmostEqual(t, sum, 1.0, 1e-6, "weight sum for "+alloc.Method)

				// All weights should be non-negative.
				for sym, w := range alloc.Weights {
					if w < -1e-10 {
						t.Errorf("allocation %q: weight[%q] = %.10f, want >= 0",
							alloc.Method, sym, w)
					}
				}

				// Dendrogram should exist and have correct leaf count.
				if alloc.Dendrogram == nil {
					t.Errorf("allocation %q: dendrogram is nil", alloc.Method)
				} else {
					leaves := countHrpLeaves(alloc.Dendrogram)
					if leaves != len(tt.symbols) {
						t.Errorf("allocation %q: dendrogram leaves = %d, want %d",
							alloc.Method, leaves, len(tt.symbols))
					}
				}
			}

			// Verify result metadata.
			if len(result.Symbols) != len(tt.symbols) {
				t.Errorf("symbols = %d, want %d", len(result.Symbols), len(tt.symbols))
			}
			if result.TradingDays <= 0 {
				t.Errorf("trading_days = %d, want > 0", result.TradingDays)
			}
		})
	}
}

// countHrpLeaves counts leaf nodes in a dendrogram tree.
func countHrpLeaves(node *DendrogramNode) int {
	if node == nil {
		return 0
	}
	if len(node.Children) == 0 {
		return 1
	}
	count := 0
	for _, child := range node.Children {
		count += countHrpLeaves(child)
	}
	return count
}

// --- Edge cases ---

func TestComputeHrpEdgeCases(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty symbols", func(t *testing.T) {
		_, err := ComputeHrp(HrpRequest{Symbols: []string{}, Prices: map[string][]market.HistoricalPrice{}})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("too many symbols", func(t *testing.T) {
		symbols := make([]string, 21)
		prices := make(map[string][]market.HistoricalPrice)
		for i := 0; i < 21; i++ {
			sym := string(rune('A' + i))
			symbols[i] = sym
			prices[sym] = makePricesForHrp(base, []float64{100, 101, 102, 103, 104})
		}
		_, err := ComputeHrp(HrpRequest{Symbols: symbols, Prices: prices})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("zero-return symbol (flat price)", func(t *testing.T) {
		_, err := ComputeHrp(HrpRequest{
			Symbols: []string{"FLAT", "MOVING"},
			Prices: map[string][]market.HistoricalPrice{
				"FLAT":   makePricesForHrp(base, []float64{100, 100, 100, 100, 100, 100}),
				"MOVING": makePricesForHrp(base, []float64{100, 102, 98, 103, 97, 104}),
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("insufficient data (only 2 prices per symbol = 1 return)", func(t *testing.T) {
		_, err := ComputeHrp(HrpRequest{
			Symbols: []string{"A", "B"},
			Prices: map[string][]market.HistoricalPrice{
				"A": makePricesForHrp(base, []float64{100, 101}),
				"B": makePricesForHrp(base, []float64{200, 201}),
			},
		})
		if err == nil {
			t.Fatal("expected error (insufficient data), got nil")
		}
	})

	t.Run("missing price data for a symbol", func(t *testing.T) {
		_, err := ComputeHrp(HrpRequest{
			Symbols: []string{"A", "B"},
			Prices: map[string][]market.HistoricalPrice{
				"A": makePricesForHrp(base, []float64{100, 101, 102, 103, 104}),
				// B is missing
			},
		})
		if err == nil {
			t.Fatal("expected error (insufficient data), got nil")
		}
	})
}

// --- Test replaceLeafNames ---

func TestReplaceLeafNames(t *testing.T) {
	symbols := []string{"AAPL", "GOOGL", "MSFT"}

	tree := &DendrogramNode{
		Distance: 1.5,
		Children: []*DendrogramNode{
			{Name: "0", Distance: 0},
			{
				Distance: 0.8,
				Children: []*DendrogramNode{
					{Name: "1", Distance: 0},
					{Name: "2", Distance: 0},
				},
			},
		},
	}

	replaceLeafNames(tree, symbols)

	// Check leaf names.
	leaves := collectLeafNames(tree)
	if len(leaves) != 3 {
		t.Fatalf("leaves = %d, want 3", len(leaves))
	}
	expected := map[string]bool{"AAPL": false, "GOOGL": false, "MSFT": false}
	for _, name := range leaves {
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("leaf %q not found in dendrogram", name)
		}
	}
}

func collectLeafNames(node *DendrogramNode) []string {
	if node == nil {
		return nil
	}
	if len(node.Children) == 0 {
		return []string{node.Name}
	}
	var names []string
	for _, child := range node.Children {
		names = append(names, collectLeafNames(child)...)
	}
	return names
}

// --- Test clusterByMethod ---

func TestClusterByMethod(t *testing.T) {
	dist := [][]float64{
		{0, 1.0},
		{1.0, 0},
	}

	for _, method := range AllLinkageMethods() {
		merges, tree := clusterByMethod(dist, method)
		if len(merges) != 1 {
			t.Errorf("method %q: merges = %d, want 1", method, len(merges))
		}
		if tree == nil {
			t.Errorf("method %q: tree is nil", method)
		}
	}
}

// --- generateDeterministicWalk for deterministic test data ---

// generateDeterministicWalk creates a deterministic price series starting at
// startPrice with the given volatility. Uses a sine-wave-based sequence
// (not random) for reproducible test data.
func generateDeterministicWalk(start float64, steps int, volatility float64) []float64 {
	prices := make([]float64, steps)
	prices[0] = start
	// Simple deterministic sequence based on index.
	for i := 1; i < steps; i++ {
		// Deterministic "random" return using sine wave + index.
		ret := volatility * math.Sin(float64(i)*1.3) * 0.5
		prices[i] = prices[i-1] * (1 + ret)
		if prices[i] < 0.01 {
			prices[i] = 0.01
		}
	}
	return prices
}
