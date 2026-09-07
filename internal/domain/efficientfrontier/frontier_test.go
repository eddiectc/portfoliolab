package efficientfrontier

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/eddiectc/portfoliolab/internal/market"
)

// --- Test ComputeFrontier ---

func TestComputeFrontier(t *testing.T) {
	tests := []struct {
		name          string
		request       FrontierRequest
		wantErr       bool
		wantErrCode   string
		wantMessage   bool // expect non-empty message (empty state)
		wantPoints    bool // expect frontier points
		wantMaxSharpe bool
		wantMinVar    bool
	}{
		{
			name: "two symbols — happy path",
			request: FrontierRequest{
				Symbols:      []string{"AAPL", "MSFT"},
				Prices:       makeTestPrices(2),
				Period:       "1Y",
				RiskFreeRate: 0.045,
			},
			wantErr:       false,
			wantPoints:    true,
			wantMaxSharpe: true,
			wantMinVar:    true,
		},
		{
			name: "three symbols",
			request: FrontierRequest{
				Symbols:      []string{"AAPL", "MSFT", "GOOGL"},
				Prices:       makeTestPrices(3),
				Period:       "1Y",
				RiskFreeRate: 0.045,
			},
			wantErr:       false,
			wantPoints:    true,
			wantMaxSharpe: true,
			wantMinVar:    true,
		},
		{
			name: "single symbol — empty state message",
			request: FrontierRequest{
				Symbols:      []string{"AAPL"},
				Prices:       makeTestPrices(1),
				Period:       "1Y",
				RiskFreeRate: 0.045,
			},
			wantErr:     false,
			wantMessage: true,
		},
		{
			name: "no symbols — empty state",
			request: FrontierRequest{
				Symbols:      []string{},
				Prices:       map[string][]market.HistoricalPrice{},
				Period:       "1Y",
				RiskFreeRate: 0.045,
			},
			wantErr:     false,
			wantMessage: true,
		},
		{
			name: "too many symbols — error",
			request: FrontierRequest{
				Symbols:      []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K"},
				Prices:       map[string][]market.HistoricalPrice{},
				Period:       "1Y",
				RiskFreeRate: 0.045,
			},
			wantErr:     true,
			wantErrCode: "too_many_symbols",
		},
		{
			name: "insufficient price data — empty state",
			request: FrontierRequest{
				Symbols: []string{"AAPL", "MSFT"},
				Prices: map[string][]marketHistoricalPrice{
					"AAPL": {{Date: time.Time{}, Close: dec(100)}},
					"MSFT": {{Date: time.Time{}, Close: dec(200)}},
				},
				Period:       "1Y",
				RiskFreeRate: 0.045,
			},
			wantErr:     false,
			wantMessage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ComputeFrontier(tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrCode != "" {
				if fe, ok := err.(*FrontierError); ok {
					if fe.Code != tt.wantErrCode {
						t.Errorf("error code = %q, want %q", fe.Code, tt.wantErrCode)
					}
				} else {
					t.Errorf("expected *FrontierError, got %T", err)
				}
				return
			}
			if tt.wantErr {
				return
			}

			if tt.wantMessage && result.Message == "" {
				t.Errorf("expected non-empty message")
			}
			if !tt.wantMessage && result.Message != "" {
				t.Errorf("unexpected message: %s", result.Message)
			}

			if tt.wantPoints {
				if len(result.FrontierPoints) == 0 {
					t.Errorf("expected frontier points, got none")
				} else {
					// Check points are sorted by volatility ascending.
					for i := 1; i < len(result.FrontierPoints); i++ {
						if result.FrontierPoints[i].VolatilityPct < result.FrontierPoints[i-1].VolatilityPct {
							t.Errorf("point[%d] vol %.2f < point[%d] vol %.2f (not sorted)",
								i, result.FrontierPoints[i].VolatilityPct,
								i-1, result.FrontierPoints[i-1].VolatilityPct)
							break
						}
					}
					// Check weights sum to ~1.0.
					for i, p := range result.FrontierPoints {
						sum := 0.0
						for _, w := range p.Weights {
							sum += w
						}
						if math.Abs(sum-1.0) > 0.01 {
							t.Errorf("point[%d] weights sum = %.6f, want ~1.0", i, sum)
							break
						}
					}
				}
			}

			if tt.wantMaxSharpe {
				if result.MaxSharpe == nil {
					t.Errorf("expected MaxSharpe portfolio")
				} else {
					sum := 0.0
					for _, w := range result.MaxSharpe.Weights {
						sum += w
					}
					if math.Abs(sum-1.0) > 0.01 {
						t.Errorf("MaxSharpe weights sum = %.6f, want ~1.0", sum)
					}
				}
			}

			if tt.wantMinVar {
				if result.MinVariance == nil {
					t.Errorf("expected MinVariance portfolio")
				} else {
					sum := 0.0
					for _, w := range result.MinVariance.Weights {
						sum += w
					}
					if math.Abs(sum-1.0) > 0.01 {
						t.Errorf("MinVariance weights sum = %.6f, want ~1.0", sum)
					}
				}
			}
		})
	}
}

// --- Test portfolioReturn ---

func TestPortfolioReturn(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		weights []float64
		want    float64
		epsilon float64
	}{
		{
			name:    "equal weights average return",
			returns: []float64{10.0, 20.0},
			weights: []float64{0.5, 0.5},
			want:    15.0,
			epsilon: 0.01,
		},
		{
			name:    "all in one asset",
			returns: []float64{10.0, 20.0},
			weights: []float64{1.0, 0.0},
			want:    10.0,
			epsilon: 0.01,
		},
		{
			name:    "three assets",
			returns: []float64{10.0, 20.0, 30.0},
			weights: []float64{0.2, 0.3, 0.5},
			want:    23.0, // 0.2*10 + 0.3*20 + 0.5*30 = 2+6+15
			epsilon: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stats.PortfolioReturn(tt.returns, tt.weights)
			if math.Abs(got-tt.want) > tt.epsilon {
				t.Errorf("got %.4f, want %.4f", got, tt.want)
			}
		})
	}
}

// --- Test portfolioVolatility ---

func TestPortfolioVolatility(t *testing.T) {
	tests := []struct {
		name      string
		covMatrix [][]float64
		weights   []float64
		wantMin   float64
		wantMax   float64
	}{
		{
			name: "equal weights on uncorrelated assets",
			covMatrix: [][]float64{
				{0.04, 0.0},
				{0.0, 0.09},
			},
			weights: []float64{0.5, 0.5},
			wantMin: 0.15, // sqrt(0.5^2*0.04 + 0.5^2*0.09) = sqrt(0.0325) = 0.1803
			wantMax: 0.20,
		},
		{
			name: "all in one asset",
			covMatrix: [][]float64{
				{0.04, 0.01},
				{0.01, 0.09},
			},
			weights: []float64{1.0, 0.0},
			wantMin: 0.19, // sqrt(0.04) = 0.2
			wantMax: 0.21,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stats.PortfolioVolatility(tt.covMatrix, tt.weights)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("got %.4f, want [%.4f, %.4f]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// --- Test filterParetoFrontier ---

func TestFilterParetoFrontier(t *testing.T) {
	// Points: (return, volatility)
	// Efficient frontier: higher return requires higher volatility.
	// A point is dominated if another has both higher return AND lower volatility.
	evaluated := []portfolioEval{
		{weights: nil, ret: 8, volatility: 10, sharpe: 0.3},  // efficient (lowest vol)
		{weights: nil, ret: 12, volatility: 15, sharpe: 0.5}, // efficient
		{weights: nil, ret: 18, volatility: 22, sharpe: 0.4}, // efficient
		{weights: nil, ret: 25, volatility: 30, sharpe: 0.6}, // efficient (highest return)
		{weights: nil, ret: 10, volatility: 20, sharpe: 0.2}, // dominated by (12, 15)
		{weights: nil, ret: 5, volatility: 12, sharpe: 0.1},  // dominated by (8, 10)
		{weights: nil, ret: 15, volatility: 25, sharpe: 0.3}, // dominated by (18, 22)
	}

	efficient := filterParetoFrontier(evaluated)

	// Should have 4 efficient points.
	if len(efficient) < 4 {
		t.Errorf("efficient points = %d, want >= 4", len(efficient))
	}

	// Check sorted by volatility ascending.
	for i := 1; i < len(efficient); i++ {
		if efficient[i].volatility < efficient[i-1].volatility {
			t.Errorf("not sorted by volatility at index %d", i)
			break
		}
	}
}

// --- Test sampleFrontierPoints ---

func TestSampleFrontierPoints(t *testing.T) {
	// Create 100 efficient points.
	efficient := make([]portfolioEval, 100)
	for i := range efficient {
		efficient[i] = portfolioEval{
			weights:    nil,
			ret:        float64(i) + 5,
			volatility: float64(i) + 10,
			sharpe:     0.5,
		}
	}

	sampled := sampleFrontierPoints(efficient, 25)
	if len(sampled) != 25 {
		t.Errorf("sampled = %d, want 25", len(sampled))
	}

	// Check sorted by volatility.
	for i := 1; i < len(sampled); i++ {
		if sampled[i].volatility < sampled[i-1].volatility {
			t.Errorf("not sorted at index %d", i)
			break
		}
	}

	// If input <= target, return all.
	small := efficient[:5]
	sampled2 := sampleFrontierPoints(small, 25)
	if len(sampled2) != 5 {
		t.Errorf("sampled small = %d, want 5", len(sampled2))
	}
}

// --- Test roundWeights ---

func TestRoundWeights(t *testing.T) {
	weights := []float64{0.333333333, 0.333333333, 0.333333334}
	rounded := roundWeights(weights)

	sum := 0.0
	for _, w := range rounded {
		sum += w
	}
	if math.Abs(sum-1.0) > 0.0001 {
		t.Errorf("rounded weights sum = %.10f, want 1.0", sum)
	}
}

// --- Test samplePortfolios ---

func TestSamplePortfolios(t *testing.T) {
	portfolios := samplePortfolios(3, 100)
	if len(portfolios) != 100 {
		t.Errorf("portfolios = %d, want 100", len(portfolios))
	}

	for i, p := range portfolios {
		if len(p) != 3 {
			t.Errorf("portfolio[%d] dims = %d, want 3", i, len(p))
		}
		sum := 0.0
		for _, w := range p {
			sum += w
		}
		if math.Abs(sum-1.0) > 0.0001 {
			t.Errorf("portfolio[%d] weights sum = %.10f, want 1.0", i, sum)
			break
		}
		for j, w := range p {
			if w < 0 {
				t.Errorf("portfolio[%d] weight[%d] = %.10f < 0", i, j, w)
			}
		}
	}
}

// --- Test ComputeFrontier — sorted symbols ---

func TestComputeFrontier_SortedSymbols(t *testing.T) {
	request := FrontierRequest{
		Symbols:      []string{"ZZZ", "AAA", "MMM"},
		Prices:       makeTestPrices(3),
		Period:       "1Y",
		RiskFreeRate: 0.045,
	}

	result, err := ComputeFrontier(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Symbols) != 3 {
		t.Fatalf("symbols = %d, want 3", len(result.Symbols))
	}
	sorted := make([]string, len(result.Symbols))
	copy(sorted, result.Symbols)
	sort.Strings(sorted)
	for i := range result.Symbols {
		if result.Symbols[i] != sorted[i] {
			t.Errorf("symbols not sorted: got %v, want %v", result.Symbols, sorted)
			break
		}
	}
}
