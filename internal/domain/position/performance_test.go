package position

import (
	"encoding/json"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/performance"
	"github.com/govalues/decimal"
)

func TestEquityCurvePoint_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		point  performance.EquityCurvePoint
		wantPV string // expected portfolio_value JSON string
		wantND string // expected net_deposit JSON string
	}{
		{
			name: "standard positive values",
			point: performance.EquityCurvePoint{
				Date:           mustTime("2024-06-15"),
				PortfolioValue: decimal.MustParse("50000.00"),
				NetDeposit:     decimal.MustParse("45000.00"),
			},
			wantPV: "50000.00",
			wantND: "45000.00",
		},
		{
			name: "negative net deposit (withdrawals exceed deposits)",
			point: performance.EquityCurvePoint{
				Date:           mustTime("2024-06-15"),
				PortfolioValue: decimal.MustParse("10000.00"),
				NetDeposit:     decimal.MustParse("-2000.00"),
			},
			wantPV: "10000.00",
			wantND: "-2000.00",
		},
		{
			name: "zero values",
			point: performance.EquityCurvePoint{
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

			var got performance.EquityCurvePoint
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
	twr := decimal.MustParse("12.50")
	annualized := decimal.MustParse("8.33")

	tests := []struct {
		name             string
		metrics          performance.ReturnMetrics
		wantTWR          *decimal.Decimal
		wantAnnualized   *decimal.Decimal
		wantInsufficient bool
	}{
		{
			name: "standard metrics",
			metrics: performance.ReturnMetrics{
				TWRPct:              &twr,
				AnnualizedTWRPct:    &annualized,
				HasInsufficientData: false,
			},
			wantTWR:        &twr,
			wantAnnualized: &annualized,
		},
		{
			name: "nil TWR (N/A case)",
			metrics: performance.ReturnMetrics{
				TWRPct:              nil,
				AnnualizedTWRPct:    &annualized,
				HasInsufficientData: false,
			},
			wantTWR:        nil,
			wantAnnualized: &annualized,
		},
		{
			name: "insufficient data",
			metrics: performance.ReturnMetrics{
				TWRPct:              nil,
				AnnualizedTWRPct:    nil,
				HasInsufficientData: true,
			},
			wantTWR:          nil,
			wantAnnualized:   nil,
			wantInsufficient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.metrics)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got performance.ReturnMetrics
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if tt.wantTWR == nil && got.TWRPct != nil {
				t.Errorf("expected nil twr_pct, got %s", got.TWRPct.String())
			} else if tt.wantTWR != nil && (got.TWRPct == nil || !got.TWRPct.Equal(*tt.wantTWR)) {
				t.Errorf("twr_pct: got %v, want %s", got.TWRPct, tt.wantTWR.String())
			}

			if tt.wantAnnualized == nil && got.AnnualizedTWRPct != nil {
				t.Errorf("expected nil annualized_twr_pct, got %s", got.AnnualizedTWRPct.String())
			} else if tt.wantAnnualized != nil && (got.AnnualizedTWRPct == nil || !got.AnnualizedTWRPct.Equal(*tt.wantAnnualized)) {
				t.Errorf("annualized_twr_pct: got %v, want %s", got.AnnualizedTWRPct, tt.wantAnnualized.String())
			}

			if got.HasInsufficientData != tt.wantInsufficient {
				t.Errorf("has_insufficient_data: got %v, want %v", got.HasInsufficientData, tt.wantInsufficient)
			}
		})
	}
}

func TestPerformanceResult_JSONRoundTrip(t *testing.T) {
	twr := decimal.MustParse("15.75")
	annualized := decimal.MustParse("10.25")

	point1 := performance.EquityCurvePoint{
		Date:           mustTime("2024-01-01"),
		PortfolioValue: decimal.MustParse("100000.00"),
		NetDeposit:     decimal.MustParse("80000.00"),
	}
	point2 := performance.EquityCurvePoint{
		Date:           mustTime("2024-06-01"),
		PortfolioValue: decimal.MustParse("120000.00"),
		NetDeposit:     decimal.MustParse("90000.00"),
	}

	tests := []struct {
		name   string
		result performance.PerformanceResult
	}{
		{
			name: "full result with warnings",
			result: performance.PerformanceResult{
				EquityCurve: []performance.EquityCurvePoint{point1, point2},
				ReturnMetrics: performance.ReturnMetrics{
					TWRPct:              &twr,
					AnnualizedTWRPct:    &annualized,
					HasInsufficientData: false,
				},
				BaseCurrency: "USD",
				Warnings:     []string{"missing market data for XYZ"},
			},
		},
		{
			name: "empty result",
			result: performance.PerformanceResult{
				EquityCurve: []performance.EquityCurvePoint{},
				ReturnMetrics: performance.ReturnMetrics{
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

			var got performance.PerformanceResult
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
				SymbolsRefreshed: []string{"AAPL", "MSFT", "GOOGL"},
				FxPairsRefreshed: []string{"GBP/USD", "EUR/USD"},
				FailedSymbols:    nil,
			},
		},
		{
			name: "partial failure",
			result: RefreshResult{
				SymbolsRefreshed: []string{"AAPL", "MSFT"},
				FxPairsRefreshed: []string{"GBP/USD"},
				FailedSymbols:    []string{"XYZ.DE"},
			},
		},
		{
			name: "empty refresh",
			result: RefreshResult{
				SymbolsRefreshed: []string{},
				FxPairsRefreshed: []string{},
				FailedSymbols:    nil,
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
