package comparison

import (
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- helpers ---

func pricesFromFloats(t *testing.T, base time.Time, values []float64) []market.HistoricalPrice {
	t.Helper()
	prices := make([]market.HistoricalPrice, len(values))
	for i, v := range values {
		d, _ := decimal.NewFromFloat64(v)
		prices[i] = market.HistoricalPrice{
			Date:     base.AddDate(0, 0, i), // one day apart
			Close:    d,
			Currency: "USD",
		}
	}
	return prices
}

func ptrDec(t *testing.T, f float64) *decimal.Decimal {
	t.Helper()
	d, _ := decimal.NewFromFloat64(f)
	d = d.Round(2)
	return &d
}

// --- ComputeMWR ---

func TestComputeMWR(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		values []float64
		want   *decimal.Decimal
	}{
		{
			name:   "normal uptrend",
			values: []float64{100, 105, 110, 108, 115},
			want:   ptrDec(t, 15.0), // (115/100 - 1) * 100
		},
		{
			name:   "declining",
			values: []float64{100, 95, 90, 85},
			want:   ptrDec(t, -15.0), // (85/100 - 1) * 100
		},
		{
			name:   "flat",
			values: []float64{100, 100, 100, 100},
			want:   ptrDec(t, 0.0),
		},
		{
			name:   "two prices only",
			values: []float64{200, 210},
			want:   ptrDec(t, 5.0),
		},
		{
			name:   "single price",
			values: []float64{100},
			want:   nil,
		},
		{
			name:   "empty",
			values: []float64{},
			want:   nil,
		},
		{
			name:   "start is zero",
			values: []float64{0, 100, 110},
			want:   nil,
		},
		{
			name:   "start is negative",
			values: []float64{-5, 100, 110},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prices := pricesFromFloats(t, base, tt.values)
			got := ComputeMWR(prices)

			if tt.want == nil {
				if got != nil {
					t.Errorf("ComputeMWR() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("ComputeMWR() = nil, want %v", tt.want)
				return
			}
			if !got.Equal(*tt.want) {
				t.Errorf("ComputeMWR() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- ComputeMWRForPeriod ---

func TestComputeMWRForPeriod(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := pricesFromFloats(t, base, []float64{100, 102, 105, 103, 110, 108, 112})
	// Dates: Jan 1=100, Jan 2=102, Jan 3=105, Jan 4=103, Jan 5=110, Jan 6=108, Jan 7=112

	tests := []struct {
		name   string
		from   time.Time
		to     time.Time
		values []float64 // expected filtered values for manual verification
		want   *decimal.Decimal
	}{
		{
			name:   "full range",
			from:   base,
			to:     base.AddDate(0, 0, 6),
			values: []float64{100, 102, 105, 103, 110, 108, 112},
			want:   ptrDec(t, 12.0), // (112/100 - 1) * 100
		},
		{
			name:   "middle window",
			from:   base.AddDate(0, 0, 2), // Jan 3
			to:     base.AddDate(0, 0, 5), // Jan 6
			values: []float64{105, 103, 110, 108},
			want:   ptrDec(t, 2.86), // (108/105 - 1) * 100 = 2.857...
		},
		{
			name:   "single day in range",
			from:   base.AddDate(0, 0, 3),
			to:     base.AddDate(0, 0, 3),
			values: []float64{103},
			want:   nil,
		},
		{
			name:   "no overlap",
			from:   base.AddDate(1, 0, 0), // far future
			to:     base.AddDate(1, 0, 10),
			values: []float64{},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeMWRForPeriod(prices, tt.from, tt.to)

			if tt.want == nil {
				if got != nil {
					t.Errorf("ComputeMWRForPeriod() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("ComputeMWRForPeriod() = nil, want %v", tt.want)
				return
			}
			if !got.Equal(*tt.want) {
				t.Errorf("ComputeMWRForPeriod() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- ComputeMonthlyReturns ---

func TestComputeMonthlyReturns(t *testing.T) {
	tests := []struct {
		name   string
		prices []market.HistoricalPrice
		want   map[string]*decimal.Decimal
	}{
		{
			name: "multiple months with varying returns",
			prices: []market.HistoricalPrice{
				{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10000, 2), Currency: "USD"},  // 100.00
				{Date: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10500, 2), Currency: "USD"}, // 105.00
				{Date: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10200, 2), Currency: "USD"}, // 102.00
				{Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10200, 2), Currency: "USD"},  // 102.00
				{Date: time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(9800, 2), Currency: "USD"},  // 98.00
				{Date: time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(9600, 2), Currency: "USD"},  // 96.00
			},
			want: map[string]*decimal.Decimal{
				"2025-01": ptrDec(t, 2.0),   // (102/100 - 1) * 100
				"2025-02": ptrDec(t, -5.88), // (96/102 - 1) * 100
			},
		},
		{
			name: "single month",
			prices: []market.HistoricalPrice{
				{Date: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(20000, 2), Currency: "USD"},
				{Date: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(21000, 2), Currency: "USD"},
			},
			want: map[string]*decimal.Decimal{
				"2025-03": ptrDec(t, 5.0),
			},
		},
		{
			name:   "empty prices",
			prices: []market.HistoricalPrice{},
			want:   nil,
		},
		{
			name: "single price",
			prices: []market.HistoricalPrice{
				{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10000, 2), Currency: "USD"},
			},
			want: nil,
		},
		{
			name: "partial month (single day in month omitted)",
			prices: []market.HistoricalPrice{
				{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10000, 2), Currency: "USD"},
				{Date: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10500, 2), Currency: "USD"},
				{Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10500, 2), Currency: "USD"}, // only one day in Feb
				{Date: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(11000, 2), Currency: "USD"},
				{Date: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(10800, 2), Currency: "USD"},
			},
			want: map[string]*decimal.Decimal{
				"2025-01": ptrDec(t, 5.0),
				"2025-03": ptrDec(t, -1.82), // (108/110 - 1) * 100
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeMonthlyReturns(tt.prices)

			if tt.want == nil {
				if got != nil {
					t.Errorf("ComputeMonthlyReturns() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("ComputeMonthlyReturns() = nil, want %v", tt.want)
				return
			}

			// Check all expected keys exist with correct values.
			for key, wantVal := range tt.want {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("ComputeMonthlyReturns() missing key %q", key)
					continue
				}
				if !gotVal.Equal(*wantVal) {
					t.Errorf("ComputeMonthlyReturns()[%q] = %v, want %v", key, gotVal, wantVal)
				}
			}

			// Check no extra keys.
			for key := range got {
				if _, ok := tt.want[key]; !ok {
					t.Errorf("ComputeMonthlyReturns() has unexpected key %q", key)
				}
			}
		})
	}
}
