package blackrock

import (
	"net/url"
	"strings"
)

// URLMatcher matches iShares product pages by domain and path pattern.
type URLMatcher struct{}

// NewURLMatcher creates a new iShares URL matcher.
func NewURLMatcher() *URLMatcher {
	return &URLMatcher{}
}

// Match returns true if the URL belongs to an iShares product page.
// Matches *.ishares.com domains with /uk/ path segments.
func (m *URLMatcher) Match(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, ".ishares.com") && host != "ishares.com" {
		return false
	}

	return strings.Contains(parsed.Path, "/uk/")
}
