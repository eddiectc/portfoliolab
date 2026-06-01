package blackrock

import "testing"

func TestURLMatcher_Match(t *testing.T) {
	m := NewURLMatcher()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "ishares.com/uk product page matches",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf",
			want: true,
		},
		{
			name: "ishares.com/uk with query params matches",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/test?switchLocale=y",
			want: true,
		},
		{
			name: "ishares.com/us does not match",
			url:  "https://www.ishares.com/us/individual/en/products/123456/test",
			want: false,
		},
		{
			name: "ishares.com/de does not match",
			url:  "https://www.ishares.com/de/privat/de/produkte/123456/test",
			want: false,
		},
		{
			name: "other domain does not match",
			url:  "https://www.wisdomtree.eu/en-gb/etfs/wmgt",
			want: false,
		},
		{
			name: "vanguard does not match",
			url:  "https://fundresearch.vanguard.com/etfs/vo",
			want: false,
		},
		{
			name: "blackrock.com does not match",
			url:  "https://www.blackrock.com/uk/lf/ie/ishares-trust-ii/ishares-msci-world-ucits-etf-a",
			want: false,
		},
		{
			name: "invalid URL does not match",
			url:  "not a url",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.url)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
