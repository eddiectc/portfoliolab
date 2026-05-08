package market

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func TestFxRateFetcherInterface(t *testing.T) {
	// Verify YahooFinanceFetcher implements FxRateFetcher at compile time.
	var _ FxRateFetcher = (*YahooFinanceFetcher)(nil)
}

func TestParseFxPair(t *testing.T) {
	tests := []struct {
		name      string
		pair      string
		wantBase  string
		wantQuote string
		wantErr   bool
	}{
		{"valid GBP/USD", "GBP/USD", "GBP", "USD", false},
		{"valid EUR/JPY", "EUR/JPY", "EUR", "JPY", false},
		{"empty base", "/USD", "", "", true},
		{"empty quote", "GBP/", "", "", true},
		{"no slash", "GBPUSD", "", "", true},
		{"empty string", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, quote, err := ParseFxPair(tt.pair)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseFxPair(%q) expected error, got nil", tt.pair)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseFxPair(%q) unexpected error: %v", tt.pair, err)
				return
			}
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
			if quote != tt.wantQuote {
				t.Errorf("quote = %q, want %q", quote, tt.wantQuote)
			}
		})
	}
}

func TestFxRateFields(t *testing.T) {
	rate, _ := decimal.NewFromFloat64(1.2734)
	fx := &FxRate{
		Pair:          "GBP/USD",
		BaseCurrency:  "GBP",
		QuoteCurrency: "USD",
		Rate:          rate,
		FetchedAt:     time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
	}

	if fx.Pair != "GBP/USD" {
		t.Errorf("Pair = %q, want %q", fx.Pair, "GBP/USD")
	}
	if fx.BaseCurrency != "GBP" {
		t.Errorf("BaseCurrency = %q, want %q", fx.BaseCurrency, "GBP")
	}
	if fx.QuoteCurrency != "USD" {
		t.Errorf("QuoteCurrency = %q, want %q", fx.QuoteCurrency, "USD")
	}
}

func TestFxError(t *testing.T) {
	err := &FxError{code: "invalid_fx_pair", message: "bad pair"}
	if err.Error() != "bad pair" {
		t.Errorf("FxError.Error() = %q, want %q", err.Error(), "bad pair")
	}
}
