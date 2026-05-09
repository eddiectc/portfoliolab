package position

import (
	"encoding/json"
	"testing"

	"github.com/govalues/decimal"
)

func TestEquityCurvePoint_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		point  EquityCurvePoint
		wantPV string // expected portfolio_value JSON string
		wantND string // expected net_deposit JSON string
	}{
		{
			name: "standard positive values",
			point: EquityCurvePoint{
				Date:           mustTime("2024-06-15"),
				PortfolioValue: decimal.MustParse("50000.00"),
				NetDeposit:     decimal.MustParse("45000.00"),
			},
			wantPV: "50000.00",
			wantND: "45000.00",
		},
		{
			name: "negative net deposit (withdrawals exceed deposits)",
			point: EquityCurvePoint{
				Date:           mustTime("2024-06-15"),
				PortfolioValue: decimal.MustParse("10000.00"),
				NetDeposit:     decimal.MustParse("-2000.00"),
			},
			wantPV: "10000.00",
			wantND: "-2000.00",
		},
		{
			name: "zero values",
			point: EquityCurvePoint{
				Date:           mustTime("2024-01-01"),
				PortfolioValue: decimal.Zero,
				NetDeposit:     decimal.Zero,
			},
			wantPV: "0",
			wantND: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.point)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got EquityCurvePoint
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if !got.PortfolioValue.Equal(tt.point.PortfolioValue) {
				t.Errorf("portfolio_value: got %s, want %s", got.PortfolioValue.String(), tt.wantPV)
			}
			if !got.NetDeposit.Equal(tt.point.NetDeposit) {
				t.Errorf("net_deposit: got %s, want %s", got.NetDeposit.String(), tt.wantND)
			}
			if !got.Date.Equal(tt.point.Date) {
				t.Errorf("date: got %v, want %v", got.Date, tt.point.Date)
			}
		})
	}
}

func TestReturnMetrics_JSONRoundTrip(t *testing.T) {
	totalReturn := decimal.MustParse("12.50")
	annualized := decimal.MustParse("8.33")

	tests := []struct {
		name  string
		metrics ReturnMetrics
		// What we expect to find after round-trip
		wantTotal      *decimal.Decimal
		wantAnnualized *decimal.Decimal
		wantInsufficient bool
	}{
		{
			name: "standard metrics",
			metrics: ReturnMetrics{
				TotalReturnPct:      &totalReturn,
				AnnualizedReturnPct: &annualized,
				HasInsufficientData: false,
			},
			wantTotal:      &totalReturn,
			wantAnnualized: &annualized,
			wantInsufficient: false,
		},
		{
			name: "nil total return (N/A case)",
			metrics: ReturnMetrics{
				TotalReturnPct:      nil,
				AnnualizedReturnPct: &annualized,
				HasInsufficientData: false,
			},
			wantTotal:      nil,
			wantAnnualized: &annualized,
			wantInsufficient: false,
		},
		{
			name: "insufficient data",
			metrics: ReturnMetrics{
				TotalReturnPct:      nil,
				AnnualizedReturnPct: nil,
				HasInsufficientData: true,
			},
			wantTotal:      nil,
			wantAnnualized: nil,
			wantInsufficient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.metrics)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got ReturnMetrics
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if tt.wantTotal == nil && got.TotalReturnPct != nil {
				t.Errorf("expected nil total_return_pct, got %s", got.TotalReturnPct.String())
			} else if tt.wantTotal != nil && (got.TotalReturnPct == nil || !got.TotalReturnPct.Equal(*tt.wantTotal)) {
				t.Errorf("total_return_pct: got %v, want %s", got.TotalReturnPct, tt.wantTotal.String())
			}

			if tt.wantAnnualized == nil && got.AnnualizedReturnPct != nil {
				t.Errorf("expected nil annualized_return_pct, got %s", got.AnnualizedReturnPct.String())
			} else if tt.wantAnnualized != nil && (got.AnnualizedReturnPct == nil || !got.AnnualizedReturnPct.Equal(*tt.wantAnnualized)) {
				t.Errorf("annualized_return_pct: got %v, want %s", got.AnnualizedReturnPct, tt.wantAnnualized.String())
			}

			if got.HasInsufficientData != tt.wantInsufficient {
				t.Errorf("has_insufficient_data: got %v, want %v", got.HasInsufficientData, tt.wantInsufficient)
			}
		})
	}
}

func TestPerformanceResult_JSONRoundTrip(t *testing.T) {
	totalReturn := decimal.MustParse("15.75")
	annualized := decimal.MustParse("10.25")

	point1 := EquityCurvePoint{
		Date:           mustTime("2024-01-01"),
		PortfolioValue: decimal.MustParse("100000.00"),
		NetDeposit:     decimal.MustParse("80000.00"),
	}
	point2 := EquityCurvePoint{
		Date:           mustTime("2024-06-01"),
		PortfolioValue: decimal.MustParse("120000.00"),
		NetDeposit:     decimal.MustParse("90000.00"),
	}

	tests := []struct {
		name   string
		result PerformanceResult
	}{
		{
			name: "full result with warnings",
			result: PerformanceResult{
				EquityCurve: []EquityCurvePoint{point1, point2},
				ReturnMetrics: ReturnMetrics{
					TotalReturnPct:      &totalReturn,
					AnnualizedReturnPct: &annualized,
					HasInsufficientData: false,
				},
				BaseCurrency: "USD",
				Warnings:     []string{"missing market data for XYZ"},
			},
		},
		{
			name: "empty result",
			result: PerformanceResult{
				EquityCurve: []EquityCurvePoint{},
				ReturnMetrics: ReturnMetrics{
					HasInsufficientData: true,
				},
				BaseCurrency: "GBP",
				Warnings:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got PerformanceResult
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.BaseCurrency != tt.result.BaseCurrency {
				t.Errorf("base_currency: got %q, want %q", got.BaseCurrency, tt.result.BaseCurrency)
			}
			if len(got.EquityCurve) != len(tt.result.EquityCurve) {
				t.Fatalf("equity_curve length: got %d, want %d", len(got.EquityCurve), len(tt.result.EquityCurve))
			}
			for i, want := range tt.result.EquityCurve {
				gotPt := got.EquityCurve[i]
				if !gotPt.PortfolioValue.Equal(want.PortfolioValue) {
					t.Errorf("curve[%d].portfolio_value: got %s, want %s", i, gotPt.PortfolioValue.String(), want.PortfolioValue.String())
				}
				if !gotPt.NetDeposit.Equal(want.NetDeposit) {
					t.Errorf("curve[%d].net_deposit: got %s, want %s", i, gotPt.NetDeposit.String(), want.NetDeposit.String())
				}
			}

			// Check warnings.
			if len(got.Warnings) != len(tt.result.Warnings) {
				t.Errorf("warnings length: got %d, want %d", len(got.Warnings), len(tt.result.Warnings))
			}
		})
	}
}

func TestRefreshResult_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		result RefreshResult
	}{
		{
			name: "successful refresh",
			result: RefreshResult{
				SymbolsRefreshed:  []string{"AAPL", "MSFT", "GOOGL"},
				FxPairsRefreshed:  []string{"GBP/USD", "EUR/USD"},
				FailedSymbols:     nil,
			},
		},
		{
			name: "partial failure",
			result: RefreshResult{
				SymbolsRefreshed:  []string{"AAPL", "MSFT"},
				FxPairsRefreshed:  []string{"GBP/USD"},
				FailedSymbols:     []string{"XYZ.DE"},
			},
		},
		{
			name: "empty refresh",
			result: RefreshResult{
				SymbolsRefreshed:  []string{},
				FxPairsRefreshed:  []string{},
				FailedSymbols:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got RefreshResult
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if !stringSliceEqual(got.SymbolsRefreshed, tt.result.SymbolsRefreshed) {
				t.Errorf("symbols_refreshed: got %v, want %v", got.SymbolsRefreshed, tt.result.SymbolsRefreshed)
			}
			if !stringSliceEqual(got.FxPairsRefreshed, tt.result.FxPairsRefreshed) {
				t.Errorf("fx_pairs_refreshed: got %v, want %v", got.FxPairsRefreshed, tt.result.FxPairsRefreshed)
			}
			if !stringSliceEqual(got.FailedSymbols, tt.result.FailedSymbols) {
				t.Errorf("failed_symbols: got %v, want %v", got.FailedSymbols, tt.result.FailedSymbols)
			}
		})
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
