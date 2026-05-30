package dimensional

import "testing"

func TestURLMatcher_Match(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "dimensional.com main domain",
			url:  "https://www.dimensional.com/gb-en/funds/ie000eggfvg6/global-core-equity-ucits-etf-acc",
			want: true,
		},
		{
			name: "etf.dimensional.com subdomain",
			url:  "https://etf.dimensional.com/public/v2/fundcenter",
			want: true,
		},
		{
			name: "tools-blob.dimensional.com subdomain",
			url:  "https://tools-blob.dimensional.com/etf/20260528/IE000EGGFVG6.csv",
			want: true,
		},
		{
			name: "google.com should not match",
			url:  "https://www.google.com/finance/quote/AAPL",
			want: false,
		},
		{
			name: "yahoo finance should not match",
			url:  "https://finance.yahoo.com/quote/SPY",
			want: false,
		},
		{
			name: "empty URL should not match",
			url:  "",
			want: false,
		},
	}

	matcher := NewURLMatcher()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.Match(tt.url)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestExtractor_Match(t *testing.T) {
	extractor := NewExtractor()
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "dimensional fund URL",
			url:  "https://www.dimensional.com/gb-en/funds/ie000eggfvg6/global-core-equity-ucits-etf-acc",
			want: true,
		},
		{
			name: "non-dimensional URL",
			url:  "https://www.google.com",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractor.Match(tt.url)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
