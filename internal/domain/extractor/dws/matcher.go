package dws

import (
	"strings"
)

// URLMatcher checks if a URL belongs to a DWS fund.
type URLMatcher struct{}

// NewURLMatcher creates a new DWS URL matcher.
func NewURLMatcher() *URLMatcher {
	return &URLMatcher{}
}

// Match checks if the URL matches the DWS pattern.
// DWS slugs are used as identifiers. The matcher checks if the URL 
// contains the DWS API base or if it's a direct slug passed as a URL.
func (m *URLMatcher) Match(rawURL string) bool {
	if strings.Contains(rawURL, "etf.dws.com") {
		return true
	}
	
	// If the URL is just a slug (e.g. "IE00BGV..."), it's hard to match 
	// without a list of known slugs. However, the dispatcher 
	// usually handles the routing. For DWS, we can match by 
	// specific DWS slug patterns (e.g. starting with IE or LU ISINs).
	if (strings.HasPrefix(rawURL, "IE") || strings.HasPrefix(rawURL, "LU")) && 
	   len(rawURL) > 12 && !strings.Contains(rawURL, ".") {
		return true
	}

	return false
}
