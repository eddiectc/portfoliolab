package wisdomtree

import (
	"context"
	"errors"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the WisdomTree extractor.
const Name = "wisdomtree"

// Extractor extracts fund data from WisdomTree ETF pages.
// The full parsing implementation is in Task 3; this stub satisfies
// the interface for registration and routing.
type Extractor struct {
	matcher *URLMatcher
}

// NewExtractor creates a new WisdomTree extractor.
func NewExtractor() *Extractor {
	return &Extractor{
		matcher: NewURLMatcher(),
	}
}

// Name returns the extractor identifier.
func (e *Extractor) Name() string {
	return Name
}

// Match checks if a URL belongs to a WisdomTree domain.
func (e *Extractor) Match(rawURL string) bool {
	return e.matcher.Match(rawURL)
}

// Extract fetches and parses data from a WisdomTree ETF page.
// Returns ErrNotImplemented until the parsing logic is completed (Task 3).
var ErrNotImplemented = errors.New("WisdomTree extraction not yet implemented — parsers pending (Task 3)")

func (e *Extractor) Extract(_ context.Context, _ string) (*extractor.ExtractResult, error) {
	return nil, ErrNotImplemented
}
