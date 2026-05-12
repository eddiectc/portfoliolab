package comparison

import (
	"testing"
)

func TestIsValidPredefined(t *testing.T) {
	tests := []struct {
		name   string
		ticker string
		want   bool
	}{
		{"S&P 500", "^GSPC", true},
		{"NASDAQ Composite", "^IXIC", true},
		{"Vanguard FTSE All-World", "VWRP.L", true},
		{"Vanguard S&P 500 UCITS", "VUSA.L", true},
		{"iShares NASDAQ 100", "XNAQ.L", true},
		{"random stock", "AAPL", false},
		{"empty ticker", "", false},
		{"lowercase", "^gspc", false},
		{"similar but wrong", "^GSPC ", false},
		{"fx pair", "GBPUSD=X", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidPredefined(tt.ticker)
			if got != tt.want {
				t.Errorf("IsValidPredefined(%q) = %v, want %v", tt.ticker, got, tt.want)
			}
		})
	}
}

func TestGetPredefined(t *testing.T) {
	got := GetPredefined()

	if len(got) != 5 {
		t.Errorf("GetPredefined() returned %d entries, want 5", len(got))
	}

	expected := map[string]string{
		"^GSPC":  "S&P 500",
		"^IXIC":  "NASDAQ Composite",
		"VWRP.L": "Vanguard FTSE All-World UCITS",
		"VUSA.L": "Vanguard S&P 500 UCITS",
		"XNAQ.L": "iShares NASDAQ 100 UCITS",
	}

	for ticker, wantName := range expected {
		gotName, ok := got[ticker]
		if !ok {
			t.Errorf("GetPredefined() missing ticker %q", ticker)
			continue
		}
		if gotName != wantName {
			t.Errorf("GetPredefined()[%q] = %q, want %q", ticker, gotName, wantName)
		}
	}

	// Verify it's a copy (mutating result doesn't affect original).
	got["FAKE"] = "Fake Benchmark"
	original := GetPredefined()
	if _, ok := original["FAKE"]; ok {
		t.Error("GetPredefined() returned a mutable reference instead of a copy")
	}
}

func TestPredefinedMap(t *testing.T) {
	// Verify the Predefined map has exactly 5 entries.
	if len(Predefined) != 5 {
		t.Errorf("Predefined map has %d entries, want 5", len(Predefined))
	}

	// Verify all tickers are valid.
	for ticker := range Predefined {
		if !IsValidPredefined(ticker) {
			t.Errorf("Predefined map contains ticker %q that IsValidPredefined rejects", ticker)
		}
	}
}
