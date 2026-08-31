package efficientfrontier

import (
	"math"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
)

func TestComputeMaxDrawdown(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		wantMin float64 // expected max drawdown range (negative)
		wantMax float64
		epsilon float64
	}{
		{
			name:    "no drawdown — monotonically increasing",
			returns: []float64{0.01, 0.02, 0.01, 0.03},
			wantMin: 0,
			wantMax: 0,
			epsilon: 0.0001,
		},
		{
			name:    "single drawdown",
			returns: []float64{0.01, 0.02, -0.05, 0.01},
			// equity: 1.0, 1.01, 1.0302, 0.97869, 0.98848
			// peak = 1.0302, trough = 0.97869, dd = (0.97869-1.0302)/1.0302 = -0.05
			wantMin: -0.051,
			wantMax: -0.049,
			epsilon: 0.001,
		},
		{
			name:    "large drawdown",
			returns: []float64{0.05, 0.05, -0.2, -0.1, 0.02},
			// equity: 1.0, 1.05, 1.1025, 0.882, 0.7938, 0.8097
			// peak = 1.1025, trough = 0.7938, dd = (0.7938-1.1025)/1.1025 = -0.28
			wantMin: -0.285,
			wantMax: -0.275,
			epsilon: 0.001,
		},
		{
			name:    "recovery after drawdown",
			returns: []float64{0.01, -0.1, 0.15, 0.05},
			// equity: 1.0, 1.01, 0.909, 1.0454, 1.0977
			// peak = 1.01, trough = 0.909, dd = (0.909-1.01)/1.01 = -0.1
			wantMin: -0.101,
			wantMax: -0.099,
			epsilon: 0.001,
		},
		{
			name:    "multiple drawdowns — worst wins",
			returns: []float64{0.01, -0.03, 0.02, -0.1, 0.05},
			// equity: 1.0, 1.01, 0.9797, 0.9993, 0.8994, 0.9443
			// peak = 1.01, trough = 0.8994, dd = (0.8994-1.01)/1.01 = -0.1096
			wantMin: -0.111,
			wantMax: -0.108,
			epsilon: 0.001,
		},
		{
			name:    "too few observations",
			returns: []float64{0.01},
			wantMin: 0,
			wantMax: 0,
			epsilon: 0.0001,
		},
		{
			name:    "empty returns",
			returns: []float64{},
			wantMin: 0,
			wantMax: 0,
			epsilon: 0.0001,
		},
		{
			name:    "constant returns (zero)",
			returns: []float64{0, 0, 0, 0},
			wantMin: 0,
			wantMax: 0,
			epsilon: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeMaxDrawdown(tt.returns)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("got %.4f, want [%.4f, %.4f]", got, tt.wantMin, tt.wantMax)
			}
			// Max drawdown should never be positive.
			if got > 0 {
				t.Errorf("max drawdown = %.4f, should be <= 0", got)
			}
		})
	}
}

func TestComputePortfolioMaxDrawdown(t *testing.T) {
	// Aligned returns: 4 dates, 2 assets.
	// Asset A: +1%, +2%, -5%, +1%
	// Asset B: +0.5%, -1%, +3%, -2%
	aligned := [][]float64{
		{0.01, 0.005},
		{0.02, -0.01},
		{-0.05, 0.03},
		{0.01, -0.02},
	}

	tests := []struct {
		name    string
		weights []float64
		wantMin float64
		wantMax float64
	}{
		{
			name:    "100% in asset A",
			weights: []float64{1.0, 0.0},
			// Same as asset A returns: 0.01, 0.02, -0.05, 0.01
			// equity: 1.0, 1.01, 1.0302, 0.97869, 0.98848
			// dd = (0.97869-1.0302)/1.0302 = -0.05
			wantMin: -0.051,
			wantMax: -0.049,
		},
		{
			name:    "100% in asset B",
			weights: []float64{0.0, 1.0},
			// Returns: 0.005, -0.01, 0.03, -0.02
			// equity: 1.0, 1.005, 0.99495, 1.0248, 1.0043
			// peak = 1.0248, trough = 1.0043, dd = -0.02
			wantMin: -0.021,
			wantMax: -0.019,
		},
		{
			name:    "50/50 portfolio",
			weights: []float64{0.5, 0.5},
			// Port returns: 0.0075, 0.005, -0.01, -0.005
			// equity: 1.0, 1.0075, 1.01256, 1.00243, 0.99741
			// peak = 1.01256, trough = 0.99741, dd = -0.01496
			wantMin: -0.016,
			wantMax: -0.013,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputePortfolioMaxDrawdown(aligned, tt.weights)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("got %.4f, want [%.4f, %.4f]", got, tt.wantMin, tt.wantMax)
			}
			if got > 0 {
				t.Errorf("max drawdown = %.4f, should be <= 0", got)
			}
		})
	}
}

func TestComputePortfolioSortino(t *testing.T) {
	// Aligned returns: 100 dates, 2 assets.
	// Asset A: moderate positive returns with occasional large drops
	// Asset B: variable positive returns (some below rf daily rate)
	aligned := make([][]float64, 100)
	for i := 0; i < 100; i++ {
		aRet := 0.003
		bRet := 0.0015 // lower, some days below daily rf (0.045/252 ≈ 0.00018)
		if i%20 == 19 {
			aRet = -0.04 // occasional crash for asset A
		}
		if i%7 == 6 {
			bRet = -0.002 // occasional small dip for asset B
		}
		aligned[i] = []float64{aRet, bRet}
	}

	tests := []struct {
		name    string
		weights []float64
		rf      float64
		check   func(t *testing.T, got float64)
	}{
		{
			name:    "50/50 portfolio with moderate rf",
			weights: []float64{0.5, 0.5},
			rf:      0.045,
			check: func(t *testing.T, got float64) {
				// Sortino should be positive (annualized return well above rf)
				if got <= 0 {
					t.Errorf("expected positive Sortino, got %.4f", got)
				}
			},
		},
		{
			name:    "all in asset B (stable, small dips)",
			weights: []float64{0.0, 1.0},
			rf:      0.045,
			check: func(t *testing.T, got float64) {
				// Some returns below rf, so downside > 0, Sortino should be positive
				if got <= 0 {
					t.Errorf("expected positive Sortino, got %.4f", got)
				}
			},
		},
		{
			name:    "all in asset A (with crashes)",
			weights: []float64{1.0, 0.0},
			rf:      0.045,
			check: func(t *testing.T, got float64) {
				// Sortino can be positive or negative depending on crash frequency
				// Just check it's a finite number
				if math.IsNaN(got) || math.IsInf(got, 0) {
					t.Errorf("expected finite Sortino, got %.4f", got)
				}
			},
		},
		{
			name:    "high risk-free rate (above return)",
			weights: []float64{0.5, 0.5},
			rf:      0.50,
			check: func(t *testing.T, got float64) {
				// Sortino should be negative (return < rf)
				if got >= 0 {
					t.Errorf("expected negative Sortino, got %.4f", got)
				}
			},
		},
		// (zero-downside case tested separately below)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portAnnRet := stats.AnnualizedReturn(computePortfolioDailyReturns(aligned, tt.weights))
			got := ComputePortfolioSortino(portAnnRet, aligned, tt.weights, tt.rf)
			tt.check(t, got)
		})
	}

	// Zero downside: all returns above rf → Sortino = 0.
	t.Run("zero downside returns Sortino of 0", func(t *testing.T) {
		// All returns are positive, rf is very low.
		aligned := [][]float64{
			{0.005}, {0.003}, {0.002}, {0.004}, {0.001},
		}
		annRet := stats.AnnualizedReturn(computePortfolioDailyReturns(aligned, []float64{1.0}))
		got := ComputePortfolioSortino(annRet, aligned, []float64{1.0}, 0.001)
		if got != 0 {
			t.Errorf("expected Sortino = 0 (no downside), got %.4f", got)
		}
	})

	// Relative comparison: Sortino of asset with less downside should be higher.
	t.Run("sortino penalizes downside more than total vol", func(t *testing.T) {
		retA := stats.AnnualizedReturn(computePortfolioDailyReturns(aligned, []float64{1.0, 0.0}))
		retB := stats.AnnualizedReturn(computePortfolioDailyReturns(aligned, []float64{0.0, 1.0}))
		sortinoA := ComputePortfolioSortino(retA, aligned, []float64{1.0, 0.0}, 0.045)
		sortinoB := ComputePortfolioSortino(retB, aligned, []float64{0.0, 1.0}, 0.045)

		// Asset B has smaller downside (small dips vs crashes), so higher Sortino
		if sortinoB <= sortinoA {
			t.Errorf("less-downside asset Sortino (%.4f) should be > crash-prone asset Sortino (%.4f)",
				sortinoB, sortinoA)
		}
	})
}

func TestComputePortfolioDailyReturns(t *testing.T) {
	aligned := [][]float64{
		{0.01, 0.02},
		{-0.01, 0.005},
		{0.005, -0.005},
	}
	weights := []float64{0.6, 0.4}

	got := computePortfolioDailyReturns(aligned, weights)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	// Day 0: 0.6*0.01 + 0.4*0.02 = 0.006 + 0.008 = 0.014
	if math.Abs(got[0]-0.014) > 0.0001 {
		t.Errorf("got[0] = %.4f, want 0.014", got[0])
	}
	// Day 1: 0.6*(-0.01) + 0.4*0.005 = -0.006 + 0.002 = -0.004
	if math.Abs(got[1]-(-0.004)) > 0.0001 {
		t.Errorf("got[1] = %.4f, want -0.004", got[1])
	}
	// Day 2: 0.6*0.005 + 0.4*(-0.005) = 0.003 - 0.002 = 0.001
	if math.Abs(got[2]-0.001) > 0.0001 {
		t.Errorf("got[2] = %.4f, want 0.001", got[2])
	}
}
