package wisdomtree

import (
	"net/url"
	"regexp"
	"strings"
)

// wisdomtreeURLRe matches new-site product pages only:
//
//	https://[www.]wisdomtree.com/{region}/products/{asset-class}/{slug}/
//
// {region} is any 2-letter code, the asset class is lowercase with optional
// hyphens, and the slug is a ticker (US) or a lowercased name slug (EU).
// www and the trailing slash are both optional. Old wisdomtree.eu URLs and
// legacy wisdomtree.com/etfs/... paths are intentionally not matched (no
// backward compatibility — user decision, RESEARCH.md §1a).
var wisdomtreeURLRe = regexp.MustCompile(`^https?://(?:www\.)?wisdomtree\.com/[a-z]{2}/products/[a-z-]+/[a-z0-9-]+/?$`)

// URLMatcher matches WisdomTree ETF product pages by URL format.
type URLMatcher struct{}

// NewURLMatcher creates a new WisdomTree URL matcher.
func NewURLMatcher() *URLMatcher {
	return &URLMatcher{}
}

// Match returns true if the URL is a new-format WisdomTree product page.
// Scheme and host are compared case-insensitively.
func (m *URLMatcher) Match(rawURL string) bool {
	if _, err := url.Parse(rawURL); err != nil {
		return false
	}
	return wisdomtreeURLRe.MatchString(strings.ToLower(rawURL))
}
