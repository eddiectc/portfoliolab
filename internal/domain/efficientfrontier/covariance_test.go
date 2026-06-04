package efficientfrontier

import (
	"math"
	"sort"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// makePriceSeries creates a price series with daily changes.
func makePriceSeries(startPrice float64, changes []float64) []market.HistoricalPrice {
	now := time.Now()
	prices := make([]market.HistoricalPrice, 0, len(changes)+1)
	price := startPrice
	for i, change := range changes {
		if i > 0 {
			price *= (1 + change)
		}
		prices = append(prices, market.HistoricalPrice{
			Date:     now.AddDate(0, 0, -(len(changes) - i)),
			Close:    dec(price),
			Currency: "USD",
		})
	}
	return prices
}

// makeLongPriceSeries repeats a pattern of changes to produce enough data points.
func makeLongPriceSeries(startPrice float64, pattern []float64, targetLen int) []market.HistoricalPrice {
	changes := make([]float64, 0, targetLen)
	for len(changes) < targetLen {
		for _, c := range pattern {
			if len(changes) >= targetLen {
				break
			}
			changes = append(changes, c)
		}
	}
	return makePriceSeries(startPrice, changes[:targetLen])
}

// --- Test ComputeCovarianceMatrix ---

func TestComputeCovarianceMatrix(t *testing.T) {
	tests := []struct {
		name          string
		pricesBySym   map[string][]market.HistoricalPrice
		wantN         int
		wantErr       bool
		wantDiagPos   bool // diagonal should be positive (variance > 0)
		wantSymmetric bool
	}{
		{
			name: "two symbols with positive correlation",
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": makeLongPriceSeries(100, []float64{0.01, -0.005, 0.008, -0.003, 0.002}, 260),
				"MSFT": makeLongPriceSeries(200, []float64{0.01, -0.005, 0.008, -0.003, 0.002}, 260),
			},
			wantN:         2,
			wantErr:       false,
			wantDiagPos:   true,
			wantSymmetric: true,
		},
		{
			name: "two symbols with negative correlation",
			pricesBySym: map[string][]market.HistoricalPrice{
				"A": makeLongPriceSeries(100, []float64{0.01, -0.01, 0.01, -0.01}, 260),
				"B": makeLongPriceSeries(100, []float64{-0.01, 0.01, -0.01, 0.01}, 260),
			},
			wantN:         2,
			wantErr:       false,
			wantDiagPos:   true,
			wantSymmetric: true,
		},
		{
			name: "three symbols",
			pricesBySym: map[string][]market.HistoricalPrice{
				"A": makeLongPriceSeries(100, []float64{0.01, -0.005, 0.008}, 260),
				"B": makeLongPriceSeries(200, []float64{0.005, -0.01, 0.003}, 260),
				"C": makeLongPriceSeries(150, []float64{-0.003, 0.007, -0.005}, 260),
			},
			wantN:         3,
			wantErr:       false,
			wantDiagPos:   true,
			wantSymmetric: true,
		},
		{
			name: "single symbol returns error",
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": makeLongPriceSeries(100, []float64{0.01}, 260),
			},
			wantN:   1,
			wantErr: true,
		},
		{
			name:        "empty map returns error",
			pricesBySym: map[string][]market.HistoricalPrice{},
			wantN:       0,
			wantErr:     true,
		},
		{
			name: "insufficient data for one symbol",
			pricesBySym: map[string][]market.HistoricalPrice{
				"A": makeLongPriceSeries(100, []float64{0.01}, 260),
				"B": []market.HistoricalPrice{
					{Date: time.Now(), Close: dec(100)},
				},
			},
			wantN:   1, // only A has valid data
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matrix, symbols, err := ComputeCovarianceMatrix(tt.pricesBySym)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(symbols) != tt.wantN {
				t.Errorf("symbols len = %d, want %d", len(symbols), tt.wantN)
			}
			if len(matrix) != tt.wantN {
				t.Errorf("matrix rows = %d, want %d", len(matrix), tt.wantN)
			}

			// Check sorted order.
			sortedSyms := make([]string, len(symbols))
			copy(sortedSyms, symbols)
			sort.Strings(sortedSyms)
			for i := range symbols {
				if symbols[i] != sortedSyms[i] {
					t.Errorf("symbols not sorted: got %v, want %v", symbols, sortedSyms)
					break
				}
			}

			if tt.wantDiagPos {
				for i := 0; i < tt.wantN; i++ {
					if matrix[i][i] <= 0 {
						t.Errorf("diagonal[%d] = %.10f, want > 0", i, matrix[i][i])
					}
				}
			}

			if tt.wantSymmetric {
				for i := 0; i < tt.wantN; i++ {
					for j := i + 1; j < tt.wantN; j++ {
						if math.Abs(matrix[i][j]-matrix[j][i]) > 1e-10 {
							t.Errorf("matrix[%d][%d] = %.10f, matrix[%d][%d] = %.10f (not symmetric)",
								i, j, matrix[i][j], j, i, matrix[j][i])
						}
					}
				}
			}
		})
	}
}

// --- Test computeCovariance ---

func TestComputeCovariance(t *testing.T) {
	tests := []struct {
		name    string
		aligned [][]float64
		i, j    int
		want    float64
		epsilon float64
	}{
		{
			name: "positive covariance",
			aligned: [][]float64{
				{1, 2},
				{2, 3},
				{3, 4},
				{4, 5},
			},
			i:       0,
			j:       1,
			want:    1.6667, // sample covariance of (1,2,3,4) and (2,3,4,5)
			epsilon: 0.01,
		},
		{
			name: "negative covariance",
			aligned: [][]float64{
				{1, 5},
				{2, 4},
				{3, 3},
				{4, 2},
			},
			i:       0,
			j:       1,
			want:    -1.6667,
			epsilon: 0.01,
		},
		{
			name: "variance (covariance with self)",
			aligned: [][]float64{
				{1, 0},
				{2, 0},
				{3, 0},
				{4, 0},
			},
			i:       0,
			j:       0,
			want:    1.6667, // sample variance of (1,2,3,4)
			epsilon: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeCovariance(tt.aligned, tt.i, tt.j, len(tt.aligned))
			if math.Abs(got-tt.want) > tt.epsilon {
				t.Errorf("got %.6f, want %.6f (±%.6f)", got, tt.want, tt.epsilon)
			}
		})
	}
}
