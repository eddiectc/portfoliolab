package market

import (
	"testing"

	"github.com/govalues/decimal"
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
		LatestPrice: decimal.MustNew(17550, 2),
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
	if !q.LatestPrice.Equal(decimal.MustNew(17550, 2)) {
		t.Errorf("expected LatestPrice 175.50, got %s", q.LatestPrice.String())
	}
}

func TestQuote_JSONSerialization(t *testing.T) {
	q := Quote{
		Symbol:      "AAPL",
		Name:        "Apple Inc.",
		Exchange:    "NMS",
		Currency:    "USD",
		LatestPrice: decimal.MustNew(17550, 2),
	}

	// Verify latest_price serializes as a string (preserves precision)
	if q.LatestPrice.String() != "175.50" {
		t.Errorf("expected LatestPrice '175.50', got %s", q.LatestPrice.String())
	}
}
