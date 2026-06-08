package hierarchicalriskparity

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
// basePrice is the starting price; changes are daily return ratios.
func makePrices(basePrice float64, changes []float64) []market.HistoricalPrice {
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
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

// --- Test ComputeDailyReturns ---

func TestComputeDailyReturns(t *testing.T) {
	tests := []struct {
		name      string
		prices    []market.HistoricalPrice
		wantLen   int
		wantErr   bool
		wantFirst float64
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
			name:      "constant prices produce zero returns",
			prices:    makePrices(100, []float64{0, 0, 0}),
			wantLen:   3,
			wantErr:   false,
			wantFirst: 0,
		},
		{
			name:      "unsorted input is handled correctly",
			prices:    makePrices(100, []float64{0.02, 0.03}),
			wantLen:   2,
			wantErr:   false,
			wantFirst: 0.02,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeDailyReturns(tt.prices)
			if (err != nil) != tt.wantErr {
				t.Errorf("ComputeDailyReturns() error = %v, wantErr %v", err, tt.wantErr)
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

// --- Test AlignReturns ---

func TestAlignReturns(t *testing.T) {
	// Create overlapping price series for two symbols.
	// Symbol A: 4 prices (3 returns), dates: day 0, 1, 2, 3
	// Symbol B: 4 prices (3 returns), dates: day 0, 1, 2, 3
	// Full overlap → 3 aligned rows.
	pricesA := makePrices(100, []float64{0.01, -0.02, 0.03})
	pricesB := makePrices(50, []float64{-0.01, 0.02, -0.015})

	tests := []struct {
		name           string
		pricesBySymbol map[string][]market.HistoricalPrice
		symbols        []string
		wantRows       int
		wantCols       int
		wantErr        bool
	}{
		{
			name: "full overlap",
			pricesBySymbol: map[string][]market.HistoricalPrice{
				"A": pricesA,
				"B": pricesB,
			},
			symbols:  []string{"A", "B"},
			wantRows: 3,
			wantCols: 2,
			wantErr:  false,
		},
		{
			name: "single symbol",
			pricesBySymbol: map[string][]market.HistoricalPrice{
				"A": pricesA,
			},
			symbols:  []string{"A"},
			wantRows: 3,
			wantCols: 1,
			wantErr:  false,
		},
		{
			name:           "empty symbols",
			pricesBySymbol: map[string][]market.HistoricalPrice{},
			symbols:        []string{},
			wantRows:       0,
			wantCols:       0,
			wantErr:        true,
		},
		{
			name: "symbol with no data is excluded",
			pricesBySymbol: map[string][]market.HistoricalPrice{
				"A": pricesA,
				"B": []market.HistoricalPrice{}, // no data
			},
			symbols:  []string{"A", "B"},
			wantRows: 0,
			wantErr:  true, // B has no data, so no fully-aligned dates
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aligned, count, err := AlignReturns(tt.pricesBySymbol, tt.symbols)
			if (err != nil) != tt.wantErr {
				t.Errorf("AlignReturns() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if count != tt.wantRows {
				t.Errorf("count = %d, want %d", count, tt.wantRows)
			}
			if len(aligned) != tt.wantRows {
				t.Errorf("len(aligned) = %d, want %d", len(aligned), tt.wantRows)
			}
			if len(aligned) > 0 && len(aligned[0]) != tt.wantCols {
				t.Errorf("len(aligned[0]) = %d, want %d", len(aligned[0]), tt.wantCols)
			}
		})
	}
}

func TestAlignReturnsSortedByDate(t *testing.T) {
	pricesA := makePrices(100, []float64{0.01, -0.02, 0.03})
	pricesB := makePrices(50, []float64{-0.01, 0.02, -0.015})

	aligned, _, err := AlignReturns(
		map[string][]market.HistoricalPrice{"A": pricesA, "B": pricesB},
		[]string{"A", "B"},
	)
	if err != nil {
		t.Fatalf("AlignReturns() error = %v", err)
	}

	// Verify column A matches the expected returns in order.
	wantA := []float64{0.01, -0.02, 0.03}
	for i, want := range wantA {
		if math.Abs(aligned[i][0]-want) > 0.0001 {
			t.Errorf("aligned[%d][0] = %.6f, want %.6f", i, aligned[i][0], want)
		}
	}

	// Verify column B matches the expected returns in order.
	wantB := []float64{-0.01, 0.02, -0.015}
	for i, want := range wantB {
		if math.Abs(aligned[i][1]-want) > 0.0001 {
			t.Errorf("aligned[%d][1] = %.6f, want %.6f", i, aligned[i][1], want)
		}
	}
}
