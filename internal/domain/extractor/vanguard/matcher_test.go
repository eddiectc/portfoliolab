package vanguard

import (
	"testing"
)

func TestURLMatcher_Match(t *testing.T) {
	matcher := NewURLMatcher()

	tests := []struct {
		name   string
		url    string
		expect bool
	}{
		{"vanguardinvestor.co.uk fund page", "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing", true},
		{"vanguardinvestor.co.uk root", "https://vanguardinvestor.co.uk/", true},
		{"vanguardinvestor.co.uk deep path", "https://www.vanguardinvestor.co.uk/investments/some-fund", true},
		{"vanguardinvestor.co.uk www", "https://www.vanguardinvestor.co.uk/", true},
		{"vanguard.com (US site)", "https://www.vanguard.com/etfs/vo", false},
		{"wisdomtree domain", "https://www.wisdomtree.eu/en-gb/etfs/wmgt", false},
		{"dws domain", "https://etf.dws.com/etfs/some-fund", false},
		{"yahoo finance", "https://finance.yahoo.com/quote/VWRL", false},
		{"empty string", "", false},
		{"malformed URL", "://not-valid", false},
		{"case insensitive", "https://www.VanguardInvestor.Co.Uk/investments/fund", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.Match(tt.url)
			if got != tt.expect {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.expect)
			}
		})
	}
}
