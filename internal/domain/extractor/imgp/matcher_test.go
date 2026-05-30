package imgp

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
		{"imgp.com www subdomain", "https://www.imgp.com/fund/LU2951555585", true},
		{"imgp.com root", "https://imgp.com/", true},
		{"imgp.com deep path", "https://www.imgp.com/fund/IE00B", true},
		{"imgp.com fund page", "https://www.imgp.com/fund/LU2951555585", true},
		{"wisdomtree domain", "https://www.wisdomtree.eu/en-gb/etfs/wmgt", false},
		{"vanguard domain", "https://www.vanguard.com/etfs/vo", false},
		{"yahoo finance", "https://finance.yahoo.com/quote/VOO", false},
		{"empty string", "", false},
		{"malformed URL", "://not-valid", false},
		{"case insensitive", "https://www.IMGp.CoM/fund/LU2951555585", true},
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
