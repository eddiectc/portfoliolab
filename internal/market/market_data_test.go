package market

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func TestFormatFxPair(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		quote    string
		wantPair string
	}{
		{"GBP/USD", "GBP", "USD", "GBP/USD"},
		{"EUR/JPY", "EUR", "JPY", "EUR/JPY"},
		{"USD/CHF", "USD", "CHF", "USD/CHF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatFxPair(tt.base, tt.quote)
			if got != tt.wantPair {
				t.Errorf("FormatFxPair(%q, %q) = %q, want %q", tt.base, tt.quote, got, tt.wantPair)
			}
		})
	}
}

func TestFxRateFields(t *testing.T) {
	rate, _ := decimal.NewFromFloat64(1.2734)
	fx := &FxRate{
		BaseCurrency:  "GBP",
		QuoteCurrency: "USD",
		Rate:          rate,
		FetchedAt:     time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
	}

	if fx.BaseCurrency != "GBP" {
		t.Errorf("BaseCurrency = %q, want %q", fx.BaseCurrency, "GBP")
	}
	if fx.QuoteCurrency != "USD" {
		t.Errorf("QuoteCurrency = %q, want %q", fx.QuoteCurrency, "USD")
	}
}


