package market

import (
	"testing"
)

func TestQuoteFetcherInterface(t *testing.T) {
	// Verify YahooFinanceFetcher implements QuoteFetcher at compile time.
	var _ QuoteFetcher = (*YahooFinanceFetcher)(nil)
}

func TestQuote_StructFields(t *testing.T) {
	q := Quote{
		Symbol:      "AAPL",
		Name:        "Apple Inc.",
		Exchange:    "NMS",
		Currency:    "USD",
		LatestPrice: 175.50,
	}

	if q.Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", q.Symbol)
	}
	if q.Name != "Apple Inc." {
		t.Errorf("expected Name 'Apple Inc.', got %q", q.Name)
	}
	if q.Exchange != "NMS" {
		t.Errorf("expected Exchange 'NMS', got %q", q.Exchange)
	}
	if q.Currency != "USD" {
		t.Errorf("expected Currency 'USD', got %q", q.Currency)
	}
	if q.LatestPrice != 175.50 {
		t.Errorf("expected LatestPrice 175.50, got %f", q.LatestPrice)
	}
}
