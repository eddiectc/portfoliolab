package dimensional

import (
	"strings"
)

// URLMatcher matches URLs from Dimensional Fund Advisors.
type URLMatcher struct{}

// NewURLMatcher creates a new URLMatcher.
func NewURLMatcher() *URLMatcher {
	return &URLMatcher{}
}

// Match checks if a URL belongs to the dimensional.com domain.
func (m *URLMatcher) Match(rawURL string) bool {
	return strings.Contains(rawURL, "dimensional.com")
}
