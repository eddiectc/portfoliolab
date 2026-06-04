package efficientfrontier

import (
	"strings"
	"testing"
)

// --- Test FrontierError ---

func TestFrontierError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *FrontierError
		wantSub string
	}{
		{
			name:    "insufficient symbols",
			err:     ErrInsufficientSymbols,
			wantSub: "insufficient_symbols",
		},
		{
			name:    "too many symbols",
			err:     ErrTooManySymbols,
			wantSub: "too_many_symbols",
		},
		{
			name:    "insufficient data",
			err:     ErrInsufficientData,
			wantSub: "insufficient_data",
		},
		{
			name:    "singular matrix",
			err:     ErrSingularMatrix,
			wantSub: "singular_matrix",
		},
		{
			name:    "numerical failure",
			err:     ErrNumericalFailure,
			wantSub: "numerical_failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("Error() = %q, want to contain %q", got, tt.wantSub)
			}
			if !strings.Contains(got, "efficient frontier") {
				t.Errorf("Error() = %q, want to contain 'efficient frontier'", got)
			}
		})
	}
}

// --- Test FrontierError implements error ---

func TestFrontierErrorImplementsError(t *testing.T) {
	var err error = ErrInsufficientSymbols
	if err == nil {
		t.Error("FrontierError does not implement error interface")
	}
}

// --- Test FrontierPoint serialization fields ---

func TestFrontierPointFields(t *testing.T) {
	p := FrontierPoint{
		ReturnPct:     15.5,
		VolatilityPct: 12.3,
		SharpeRatio:   0.85,
		Weights:       []float64{0.3, 0.7},
	}

	if p.ReturnPct != 15.5 {
		t.Errorf("ReturnPct = %f, want 15.5", p.ReturnPct)
	}
	if p.VolatilityPct != 12.3 {
		t.Errorf("VolatilityPct = %f, want 12.3", p.VolatilityPct)
	}
	if p.SharpeRatio != 0.85 {
		t.Errorf("SharpeRatio = %f, want 0.85", p.SharpeRatio)
	}
	if len(p.Weights) != 2 {
		t.Errorf("Weights len = %d, want 2", len(p.Weights))
	}
}

// --- Test OptimizedPortfolio fields ---

func TestOptimizedPortfolioFields(t *testing.T) {
	p := OptimizedPortfolio{
		Name:          "Max Sharpe",
		ReturnPct:     18.0,
		VolatilityPct: 14.0,
		SharpeRatio:   1.0,
		Weights:       []float64{0.5, 0.5},
	}

	if p.Name != "Max Sharpe" {
		t.Errorf("Name = %q, want %q", p.Name, "Max Sharpe")
	}
	if len(p.Weights) != 2 {
		t.Errorf("Weights len = %d, want 2", len(p.Weights))
	}
}
