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
