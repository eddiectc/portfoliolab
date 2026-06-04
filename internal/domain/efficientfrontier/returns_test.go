package efficientfrontier

import (
	"math"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// dec converts a float64 to decimal.Decimal for test fixtures.
func dec(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat64(f)
	return d
}

// makePrices creates a price series for testing.
func makePrices(basePrice float64, changes []float64) []market.HistoricalPrice {
	now := time.Now()
	prices := make([]market.HistoricalPrice, 0, len(changes)+1)
	price := basePrice
	prices = append(prices, market.HistoricalPrice{
		Date:     now.AddDate(0, 0, -len(changes)),
		Close:    dec(price),
		Currency: "USD",
	})
	for i, change := range changes {
		price *= (1 + change)
		prices = append(prices, market.HistoricalPrice{
			Date:     now.AddDate(0, 0, -(len(changes) - i - 1)),
			Close:    dec(price),
			Currency: "USD",
		})
	}
	return prices
}

// --- Test ComputeReturns ---

func TestComputeReturns(t *testing.T) {
	tests := []struct {
		name      string
		prices    []market.HistoricalPrice
		wantLen   int
		wantErr   bool
		wantFirst float64 // first return value
	}{
		{
			name:      "5 prices produce 4 returns",
			prices:    makePrices(100, []float64{0.01, -0.02, 0.03, -0.01}),
			wantLen:   4,
			wantErr:   false,
			wantFirst: 0.01,
		},
		{
			name:    "single price returns error",
			prices:  []market.HistoricalPrice{{Date: time.Now(), Close: dec(100)}},
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "empty prices returns error",
			prices:  []market.HistoricalPrice{},
			wantLen: 0,
			wantErr: true,
		},
		{
			name:      "unsorted input is handled correctly",
			prices:    makePrices(100, []float64{0.02, 0.03}),
			wantLen:   2,
			wantErr:   false,
			wantFirst: 0.02,
		},
		{
			name:      "constant prices produce zero returns",
			prices:    makePrices(100, []float64{0, 0, 0}),
			wantLen:   3,
			wantErr:   false,
			wantFirst: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeReturns(tt.prices)
			if (err != nil) != tt.wantErr {
				t.Errorf("ComputeReturns() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && math.Abs(got[0]-tt.wantFirst) > 0.0001 {
				t.Errorf("first return = %.6f, want %.6f", got[0], tt.wantFirst)
			}
		})
	}
}

// --- Test ComputeAnnualizedReturn ---

func TestComputeAnnualizedReturn(t *testing.T) {
	tests := []struct {
		name        string
		returns     []float64
		tradingDays int
		wantApprox  float64
		wantEpsilon float64
	}{
		{
			name:        "positive daily returns annualize correctly",
			returns:     makePositiveReturns(252, 0.0004), // ~10% annual
			tradingDays: 252,
			wantApprox:  10.0,
			wantEpsilon: 2.0,
		},
		{
			name:        "zero returns annualize to zero",
			returns:     makeConstantReturns(252, 0),
			tradingDays: 252,
			wantApprox:  0,
			wantEpsilon: 0.01,
		},
		{
			name:        "negative returns annualize negative",
			returns:     makeConstantReturns(252, -0.0004),
			tradingDays: 252,
			wantApprox:  -10.0,
			wantEpsilon: 2.0,
		},
		{
			name:        "empty returns return zero",
			returns:     []float64{},
			tradingDays: 252,
			wantApprox:  0,
			wantEpsilon: 0.01,
		},
		{
			name:        "custom trading days adjusts annualization",
			returns:     makePositiveReturns(126, 0.0004), // half year
			tradingDays: 252,
			wantApprox:  10.0,
			wantEpsilon: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeAnnualizedReturn(tt.returns, tt.tradingDays)
			if math.Abs(got-tt.wantApprox) > tt.wantEpsilon {
				t.Errorf("got %.2f%%, want ~%.2f%% (±%.2f)", got, tt.wantApprox, tt.wantEpsilon)
			}
		})
	}
}

// --- Test ComputeAnnualizedVolatility ---

func TestComputeAnnualizedVolatility(t *testing.T) {
	tests := []struct {
		name        string
		returns     []float64
		tradingDays int
		wantMin     float64
		wantMax     float64
	}{
		{
			name:        "constant returns produce zero volatility",
			returns:     makeConstantReturns(252, 0.001),
			tradingDays: 252,
			wantMin:     0,
			wantMax:     0.01,
		},
		{
			name:        "alternating returns produce positive volatility",
			returns:     makeAlternatingReturns(252, 0.01, -0.01),
			tradingDays: 252,
			wantMin:     0.9, // daily stddev = 0.01, annualized = 0.01*sqrt(252/252) = 1%
			wantMax:     1.1,
		},
		{
			name:        "single return returns zero",
			returns:     []float64{0.01},
			tradingDays: 252,
			wantMin:     0,
			wantMax:     0,
		},
		{
			name:        "empty returns return zero",
			returns:     []float64{},
			tradingDays: 252,
			wantMin:     0,
			wantMax:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeAnnualizedVolatility(tt.returns, tt.tradingDays)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("got %.2f%%, want [%.2f, %.2f]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// --- helpers ---

func makePositiveReturns(n int, dailyReturn float64) []float64 {
	returns := make([]float64, n)
	for i := range returns {
		returns[i] = dailyReturn
	}
	return returns
}

func makeConstantReturns(n int, value float64) []float64 {
	returns := make([]float64, n)
	for i := range returns {
		returns[i] = value
	}
	return returns
}

func makeAlternatingReturns(n int, a, b float64) []float64 {
	returns := make([]float64, n)
	for i := range returns {
		if i%2 == 0 {
			returns[i] = a
		} else {
			returns[i] = b
		}
	}
	return returns
}
