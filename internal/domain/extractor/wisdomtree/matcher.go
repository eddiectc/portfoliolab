package wisdomtree

import (
	"net/url"
	"strings"
)

// URLMatcher matches WisdomTree ETF pages by domain.
type URLMatcher struct{}

// NewURLMatcher creates a new WisdomTree URL matcher.
func NewURLMatcher() *URLMatcher {
	return &URLMatcher{}
}

// Match returns true if the URL belongs to a WisdomTree domain.
// Matches *.wisdomtree.eu and wisdomtree.com patterns.
func (m *URLMatcher) Match(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	return strings.HasSuffix(host, ".wisdomtree.eu") ||
		strings.HasSuffix(host, ".wisdomtree.com") ||
		host == "wisdomtree.eu" ||
		host == "wisdomtree.com"
}
