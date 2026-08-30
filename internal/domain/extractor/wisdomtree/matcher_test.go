package wisdomtree

import "testing"

func TestMatch(t *testing.T) {
	m := NewURLMatcher()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		// New format — US (ticker slug)
		{"us ticker", "https://www.wisdomtree.com/us/products/equity/ezm", true},
		{"us no www", "https://wisdomtree.com/us/products/equity/ezm", true},
		{"us trailing slash", "https://www.wisdomtree.com/us/products/equity/ezm/", true},
		{"us fixed-income asset class", "https://www.wisdomtree.com/us/products/fixed-income/abc", true},
		{"us capital-efficient asset class", "https://www.wisdomtree.com/us/products/capital-efficient/abc", true},
		{"http scheme", "http://www.wisdomtree.com/us/products/equity/ezm", true},
		// New format — EU/GB (name slug)
		{"gb name slug", "https://www.wisdomtree.com/gb/products/equities/wisdomtree-us-quality-growth-ucits-etf---usd-acc", true},
		{"gb name slug trailing slash", "https://www.wisdomtree.com/gb/products/equities/wisdomtree-us-quality-growth-ucits-etf---usd-acc/", true},
		{"de region", "https://wisdomtree.com/de/products/equities/xyz", true},
		{"fr region", "https://wisdomtree.com/fr/products/fixed-income/xyz", true},
		{"gb digital-assets asset class", "https://wisdomtree.com/gb/products/digital-assets/xyz", true},
		{"case-insensitive host", "https://WWW.WisdomTree.com/us/products/equity/ezm", true},
		// Rejected — old site URLs (no backward compatibility, user decision)
		{"old wisdomtree.eu path", "https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt---wisdomtree-exchange-mid-cap-growth", false},
		{"wisdomtree.eu root", "https://wisdomtree.eu/", false},
		// Rejected — legacy .com paths
		{"legacy .com/etfs path", "https://www.wisdomtree.com/us/en/etfs/wmgt", false},
		{"legacy .com root", "https://wisdomtree.com/", false},
		// Rejected — other domains
		{"vanguard domain", "https://www.vanguard.com/etfs/vo", false},
		{"blackrock domain", "https://www.blackrock.com/etfs/ishares", false},
		{"yahoo finance", "https://finance.yahoo.com/quote/VOO", false},
		// Rejected — malformed
		{"empty string", "", false},
		{"malformed URL", "://not-valid", false},
		{"missing slug", "https://wisdomtree.com/us/products/equity", false},
		{"extra segment", "https://wisdomtree.com/us/products/equity/ezm/extra", false},
		{"one-letter region", "https://wisdomtree.com/u/products/equity/ezm", false},
		{"three-letter region", "https://wisdomtree.com/usa/products/equity/ezm", false},
		{"missing products segment", "https://wisdomtree.com/us/equity/ezm", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
