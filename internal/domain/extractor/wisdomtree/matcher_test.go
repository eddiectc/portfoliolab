package wisdomtree

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
		{"wisdomtree.eu www subdomain", "https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt---wisdomtree-exchange-mid-cap-growth", true},
		{"wisdomtree.eu root", "https://wisdomtree.eu/", true},
		{"wisdomtree.eu deep path", "https://www.wisdomtree.eu/en-gb/etfs/wmst", true},
		{"wisdomtree.com www", "https://www.wisdomtree.com/us/en/etfs/wmgt", true},
		{"wisdomtree.com root", "https://wisdomtree.com/", true},
		{"vanguard domain", "https://www.vanguard.com/etfs/vo", false},
		{"blackrock domain", "https://www.blackrock.com/etfs/ishares", false},
		{"yahoo finance", "https://finance.yahoo.com/quote/VOO", false},
		{"empty string", "", false},
		{"malformed URL", "://not-valid", false},
		{"case insensitive", "https://www.WisdomTree.EU/en-gb/etfs/wmgt", true},
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
