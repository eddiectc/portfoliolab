package imgp

import (
	"net/url"
	"strings"
)

// URLMatcher matches iM Global Partner (iMGP) fund pages by domain.
type URLMatcher struct{}

// NewURLMatcher creates a new iMGP URL matcher.
func NewURLMatcher() *URLMatcher {
	return &URLMatcher{}
}

// Match returns true if the URL belongs to an iMGP domain.
// Matches *.imgp.com patterns.
func (m *URLMatcher) Match(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	return strings.HasSuffix(host, ".imgp.com") ||
		host == "imgp.com"
}
