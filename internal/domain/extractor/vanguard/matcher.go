package vanguard

import (
	"net/url"
	"strings"
)

// URLMatcher matches Vanguard UK investor pages by domain.
type URLMatcher struct{}

// NewURLMatcher creates a new Vanguard URL matcher.
func NewURLMatcher() *URLMatcher {
	return &URLMatcher{}
}

// Match returns true if the URL belongs to a Vanguard UK investor domain.
func (m *URLMatcher) Match(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	return strings.HasSuffix(host, ".vanguardinvestor.co.uk") ||
		host == "vanguardinvestor.co.uk"
}
