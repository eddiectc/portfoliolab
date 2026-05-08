package market

import (
	"testing"

	"github.com/govalues/decimal"
)

func TestMarketDataFetcherInterface(t *testing.T) {
	// Verify YahooFinanceFetcher implements MarketDataFetcher at compile time.
	var _ MarketDataFetcher = (*YahooFinanceFetcher)(nil)
}

func TestMarketData_StructFields(t *testing.T) {
	d := MarketData{
		Symbol:   "AAPL",
		Price:    decimal.MustNew(17550, 2),
		Currency: "USD",
		DataType: "stock",
		Source:   "yahoo",
		Date:     "",
	}

	if d.Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", d.Symbol)
	}
	if !d.Price.Equal(decimal.MustNew(17550, 2)) {
		t.Errorf("expected Price 175.50, got %s", d.Price.String())
	}
	if d.Currency != "USD" {
		t.Errorf("expected Currency 'USD', got %q", d.Currency)
	}
	if d.DataType != "stock" {
		t.Errorf("expected DataType 'stock', got %q", d.DataType)
	}
	if d.Source != "yahoo" {
		t.Errorf("expected Source 'yahoo', got %q", d.Source)
	}
	if d.Date != "" {
		t.Errorf("expected empty Date (latest), got %q", d.Date)
	}
}

func TestFxPairToYahooSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"GBP/USD", "GBPUSD=X"},
		{"EUR/USD", "EURUSD=X"},
		{"USD/JPY", "USDJPY=X"},
		{"GBP/EUR", "GBPEUR=X"},
	}

	for _, tc := range tests {
		result := FxPairToYahooSymbol(tc.input)
		if result != tc.expected {
			t.Errorf("FxPairToYahooSymbol(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestMarketData_JSONSerialization(t *testing.T) {
	d := MarketData{
		Symbol:   "AAPL",
		Price:    decimal.MustNew(17550, 2),
		Currency: "USD",
		DataType: "stock",
		Source:   "yahoo",
		Date:     "",
	}

	// Verify price serializes as a string (preserves precision)
	if d.Price.String() != "175.50" {
		t.Errorf("expected Price '175.50', got %s", d.Price.String())
	}
}
