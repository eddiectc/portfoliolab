package allocation

import (
	"testing"

	"github.com/govalues/decimal"
)

func TestAllocationError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *AllocationError
		want string
	}{
		{
			name: "no portfolios",
			err:  ErrNoPortfolios,
			want: "no portfolios found",
		},
		{
			name: "invalid target pct",
			err:  ErrInvalidTargetPct,
			want: "target percentage must be between 0 and 100",
		},
		{
			name: "target sum not 100",
			err:  ErrTargetSumNot100,
			want: "target percentages must sum to exactly 100",
		},
		{
			name: "zero total value",
			err:  ErrZeroTotalValue,
			want: "portfolio has zero or negative total value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAllocationError_Code(t *testing.T) {
	tests := []struct {
		name string
		err  *AllocationError
		want string
	}{
		{"no portfolios", ErrNoPortfolios, "no_portfolios"},
		{"invalid target pct", ErrInvalidTargetPct, "invalid_target_pct"},
		{"target sum not 100", ErrTargetSumNot100, "target_sum_not_100"},
		{"zero total value", ErrZeroTotalValue, "zero_total_value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Code; got != tt.want {
				t.Errorf("Code = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAllocationRow_Struct(t *testing.T) {
	mvb := decimal.MustParse("1500.50")
	row := AllocationRow{
		Symbol:          "AAPL",
		MarketValue:     decimal.MustParse("1200.00"),
		MarketValueBase: &mvb,
		AllocationPct:   decimal.MustParse("30.0"),
		Currency:        "USD",
		HasMarketData:   true,
		AccountBreakdown: []AccountBreakdown{
			{
				AccountID:       1,
				AccountName:     "Broker A",
				Quantity:        decimal.MustParse("10"),
				MarketValue:     decimal.MustParse("1200.00"),
				MarketValueBase: &mvb,
				PctOfSymbol:     decimal.MustParse("100.0"),
			},
		},
	}

	if row.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want %q", row.Symbol, "AAPL")
	}
	if row.MarketValue.Equal(decimal.MustParse("1200.00")) != true {
		t.Errorf("MarketValue = %v, want 1200.00", row.MarketValue)
	}
	if row.AllocationPct.Equal(decimal.MustParse("30.0")) != true {
		t.Errorf("AllocationPct = %v, want 30.0", row.AllocationPct)
	}
	if len(row.AccountBreakdown) != 1 {
		t.Errorf("AccountBreakdown len = %d, want 1", len(row.AccountBreakdown))
	}
}

func TestDriftRow_Struct(t *testing.T) {
	row := DriftRow{
		Symbol:     "AAPL",
		ActualPct:  decimal.MustParse("35.0"),
		TargetPct:  decimal.MustParse("30.0"),
		DriftPct:   decimal.MustParse("5.0"),
		IsBalanced: true,
	}

	if row.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want %q", row.Symbol, "AAPL")
	}
	if !row.DriftPct.Equal(decimal.MustParse("5.0")) {
		t.Errorf("DriftPct = %v, want 5.0", row.DriftPct)
	}
	if !row.IsBalanced {
		t.Errorf("IsBalanced = false, want true")
	}
}

func TestRebalanceSuggestion_Struct(t *testing.T) {
	suggestion := RebalanceSuggestion{
		Symbol:         "MSFT",
		Direction:      "sell",
		Shares:         decimal.MustParse("5.5"),
		DollarValue:    decimal.MustParse("1500.00"),
		DriftReduction: decimal.MustParse("3.2"),
	}

	if suggestion.Direction != "sell" {
		t.Errorf("Direction = %q, want %q", suggestion.Direction, "sell")
	}
	if !suggestion.Shares.Equal(decimal.MustParse("5.5")) {
		t.Errorf("Shares = %v, want 5.5", suggestion.Shares)
	}
}

func TestAllocationFilter_Struct(t *testing.T) {
	filter := AllocationFilter{
		PortfolioIDs: []int64{1, 2},
	}

	if len(filter.PortfolioIDs) != 2 {
		t.Errorf("PortfolioIDs len = %d, want 2", len(filter.PortfolioIDs))
	}

	emptyFilter := AllocationFilter{}
	if len(emptyFilter.PortfolioIDs) != 0 {
		t.Errorf("empty filter PortfolioIDs len = %d, want 0", len(emptyFilter.PortfolioIDs))
	}
}

func TestTargetEntry_Struct(t *testing.T) {
	entry := TargetEntry{
		Symbol:    "AAPL",
		TargetPct: decimal.MustParse("25.0"),
	}

	if entry.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want %q", entry.Symbol, "AAPL")
	}
	if !entry.TargetPct.Equal(decimal.MustParse("25.0")) {
		t.Errorf("TargetPct = %v, want 25.0", entry.TargetPct)
	}
}
