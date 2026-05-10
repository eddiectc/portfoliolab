package position

import (
	"testing"

	"github.com/govalues/decimal"
)

func TestComputePeriodReturn(t *testing.T) {
	tests := []struct {
		name             string
		equityCurve      []EquityCurvePoint
		baseCurrency     string
		wantPeriodReturn  *decimal.Decimal
		wantAnnualized   *decimal.Decimal
		wantInsufficient bool
	}{
		{
			name: "standard positive return over 1 year",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1150000, 2), NetDeposit: dec(1000000, 2)},
			},
			baseCurrency: "USD",
			// Total return: (11500 - 10000) / 10000 * 100 = 15.00%
			wantPeriodReturn: ptrDec(dec(1500, 2)),
			// CAGR: (11500/10000)^(365/365) - 1 = 0.15 → 15.00%
			wantAnnualized: ptrDec(dec(1500, 2)),
			wantInsufficient: false,
		},
		{
			name: "positive return over 2 years",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2022-06-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-06-01"), PortfolioValue: dec(1300000, 2), NetDeposit: dec(1000000, 2)},
			},
			baseCurrency: "USD",
			// Total return: (13000 - 10000) / 10000 * 100 = 30.00%
			wantPeriodReturn: ptrDec(dec(3000, 2)),
			// CAGR: (1.3)^(365/731) - 1 ≈ 0.1400 → 14.00% (731 days: 2022-06-01 to 2024-06-01, includes leap day)
			wantAnnualized: ptrDec(dec(1400, 2)),
			wantInsufficient: false,
		},
		{
			name: "less than 1 year (still annualized)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-07-01"), PortfolioValue: dec(1100000, 2), NetDeposit: dec(1000000, 2)},
			},
			baseCurrency: "USD",
			// Total return: (11000 - 10000) / 10000 * 100 = 10.00%
			wantPeriodReturn: ptrDec(dec(1000, 2)),
			// CAGR: (1.1)^(365/182) - 1 ≈ 0.2106 → 21.06% (182 days: 2024-01-01 to 2024-07-01, leap year)
			wantAnnualized: ptrDec(decimal.MustParse("21.06")),
			wantInsufficient: false,
		},
		{
			name: "zero begin value (period return N/A)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(0, 2), NetDeposit: dec(0, 2)},
				{Date: mustTime("2024-06-01"), PortfolioValue: dec(500000, 2), NetDeposit: dec(0, 2)},
			},
			baseCurrency: "USD",
			wantPeriodReturn:  nil, // N/A — zero begin value
			wantAnnualized:   nil, // N/A — begin value is zero
			wantInsufficient: false,
		},
		{
			name: "negative net deposit (period return uses begin/end value)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(-500000, 2)},
				{Date: mustTime("2024-06-01"), PortfolioValue: dec(1200000, 2), NetDeposit: dec(-500000, 2)},
			},
			baseCurrency: "USD",
			// Period return: (12000 - 10000) / 10000 * 100 = 20.00%
			wantPeriodReturn:  ptrDec(dec(2000, 2)),
			// CAGR: (1.2)^(365/152) - 1 ≈ 0.5493 → 54.93% (152 days: 2024-01-01 to 2024-06-01, leap year)
			wantAnnualized:   ptrDec(decimal.MustParse("54.93")),
			wantInsufficient: false,
		},
		{
			name: "fewer than 2 data points (insufficient data)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
			},
			baseCurrency: "USD",
			wantPeriodReturn:  nil,
			wantAnnualized:   nil,
			wantInsufficient: true,
		},
		{
			name:             "empty equity curve",
			equityCurve:      []EquityCurvePoint{},
			baseCurrency:     "USD",
			wantPeriodReturn:  nil,
			wantAnnualized:   nil,
			wantInsufficient: true,
		},
		{
			name: "zero return (value equals deposit)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
			},
			baseCurrency: "USD",
			wantPeriodReturn:  ptrDec(decimal.Zero),
			wantAnnualized:   ptrDec(decimal.Zero),
			wantInsufficient: false,
		},
		{
			name: "negative return (value less than deposit)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(850000, 2), NetDeposit: dec(1000000, 2)},
			},
			baseCurrency: "USD",
			// Total return: (8500 - 10000) / 10000 * 100 = -15.00%
			wantPeriodReturn:  ptrDec(decimal.MustParse("-15.00")),
			// CAGR: (0.85)^(365/365) - 1 = -0.15 → -15.00%
			wantAnnualized:   ptrDec(decimal.MustParse("-15.00")),
			wantInsufficient: false,
		},
		{
			name: "multi-point curve (uses first and last)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2023-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2023-06-01"), PortfolioValue: dec(1200000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1100000, 2), NetDeposit: dec(1100000, 2)},
			},
			baseCurrency: "USD",
			// Period return: (11000 - 10000) / 10000 * 100 = 10.00%
			wantPeriodReturn:  ptrDec(dec(1000, 2)),
			// CAGR: (11000/10000)^(365/365) - 1 = 0.10 → 10.00%
			wantAnnualized:   ptrDec(dec(1000, 2)),
			wantInsufficient: false,
		},
		{
			name: "same date (no days elapsed, CAGR N/A)",
			equityCurve: []EquityCurvePoint{
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1000000, 2), NetDeposit: dec(1000000, 2)},
				{Date: mustTime("2024-01-01"), PortfolioValue: dec(1100000, 2), NetDeposit: dec(1000000, 2)},
			},
			baseCurrency: "USD",
			// Total return: (11000 - 10000) / 10000 * 100 = 10.00%
			wantPeriodReturn:  ptrDec(dec(1000, 2)),
			wantAnnualized:   nil, // N/A — zero days elapsed
			wantInsufficient: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputePeriodReturn(tt.equityCurve, tt.baseCurrency)

			if got.HasInsufficientData != tt.wantInsufficient {
				t.Errorf("has_insufficient_data: got %v, want %v", got.HasInsufficientData, tt.wantInsufficient)
			}

			checkDecimalPtr(t, "period_return_pct", got.PeriodReturnPct, tt.wantPeriodReturn)
			checkDecimalPtr(t, "annualized_return_pct", got.AnnualizedReturnPct, tt.wantAnnualized)
		})
	}
}

func checkDecimalPtr(t *testing.T, field string, got, want *decimal.Decimal) {
	t.Helper()

	if want == nil && got == nil {
		return
	}
	if want == nil && got != nil {
		t.Errorf("%s: got %s, want nil", field, got.String())
		return
	}
	if want != nil && got == nil {
		t.Errorf("%s: got nil, want %s", field, want.String())
		return
	}
	// Compare with tolerance of 0.01 for float64-based calculations.
	diff, _ := got.Sub(*want)
	if diff.IsNeg() {
		diff = diff.Abs()
	}
	tolerance := decimal.MustNew(1, 2) // 0.01
	if diff.Cmp(tolerance) > 0 {
		t.Errorf("%s: got %s, want %s (diff %s)", field, got.String(), want.String(), diff.String())
	}
}
