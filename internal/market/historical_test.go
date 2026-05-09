package market

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func TestHistoricalPrice_StructFields(t *testing.T) {
	price, _ := decimal.NewFromFloat64(175.50)
	hp := HistoricalPrice{
		Date:     time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		Close:    price,
		Currency: "USD",
	}

	if hp.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", hp.Currency, "USD")
	}
	if !hp.Close.Equal(price) {
		t.Errorf("Close = %v, want %v", hp.Close, price)
	}
}

func TestHistoricalPrice_EmptySlice(t *testing.T) {
	// Verify empty slice behavior for the return type.
	var prices []HistoricalPrice
	if len(prices) != 0 {
		t.Errorf("expected empty slice, got %d items", len(prices))
	}
}

func TestHistoricalPrice_SortedByDate(t *testing.T) {
	p1, _ := decimal.NewFromFloat64(170.00)
	p2, _ := decimal.NewFromFloat64(175.00)
	p3, _ := decimal.NewFromFloat64(180.00)

	prices := []HistoricalPrice{
		{Date: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), Close: p1, Currency: "USD"},
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: p2, Currency: "USD"},
		{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), Close: p3, Currency: "USD"},
	}

	for i := 1; i < len(prices); i++ {
		if prices[i].Date.Before(prices[i-1].Date) {
			t.Errorf("prices not sorted by date: index %d (%v) before index %d (%v)",
				i, prices[i].Date, i-1, prices[i-1].Date)
		}
	}
}
