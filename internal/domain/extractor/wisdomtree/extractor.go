package wisdomtree

import (
	"context"
	"fmt"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the WisdomTree extractor.
const Name = "wisdomtree"

// Extractor extracts fund data from WisdomTree ETF pages.
type Extractor struct {
	matcher *URLMatcher
	client  *Client
}

// NewExtractor creates a new WisdomTree extractor.
func NewExtractor() *Extractor {
	return &Extractor{
		matcher: NewURLMatcher(),
		client:  NewClient(),
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
// All sections are parsed atomically — if any fails, the entire extraction is rejected.
func (e *Extractor) Extract(ctx context.Context, sourceURL string) (*extractor.ExtractResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	html, err := e.client.Fetch(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}

	return extractFromHTML(html)
}

// SetClient sets the HTTP client for fetching pages.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

// extractFromHTML parses all sections from pre-fetched HTML.
// Used for testing and when renderer is not available.
func extractFromHTML(html string) (*extractor.ExtractResult, error) {
	fundInfo, err := ParseFundInfo(html)
	if err != nil {
		return nil, fmt.Errorf("parse fund info: %w", err)
	}

	fundProfile, err := ParseFundProfile(html)
	if err != nil {
		return nil, fmt.Errorf("parse fund profile: %w", err)
	}

	holdings, err := ParseHoldings(html)
	if err != nil {
		return nil, fmt.Errorf("parse holdings: %w", err)
	}

	navHistory, err := ParseNavHistory(html)
	if err != nil {
		return nil, fmt.Errorf("parse nav history: %w", err)
	}

	themes, err := ParseThemes(html)
	if err != nil {
		return nil, fmt.Errorf("parse themes: %w", err)
	}

	sectors, err := ParseSectors(html)
	if err != nil {
		return nil, fmt.Errorf("parse sectors: %w", err)
	}

	asOfDate, err := ParseAsOfDate(html)
	if err != nil {
		return nil, fmt.Errorf("parse as-of date: %w", err)
	}

	countryAllocation, err := ParseCountryAllocation(html)
	if err != nil {
		return nil, fmt.Errorf("parse country allocation: %w", err)
	}

	marketCap, err := ParseMarketCap(html)
	if err != nil {
		return nil, fmt.Errorf("parse market cap: %w", err)
	}

	characteristics, err := ParseFundCharacteristics(html)
	if err != nil {
		return nil, fmt.Errorf("parse fund characteristics: %w", err)
	}

	return &extractor.ExtractResult{
		AsOfDate:          asOfDate,
		FundInfo:          fundInfo,
		FundProfile:       fundProfile,
		Holdings:          holdings,
		NavHistory:        navHistory,
		Themes:            themes,
		Sectors:           sectors,
		CountryAllocation: countryAllocation,
		MarketCap:         marketCap,
		Characteristics:   characteristics,
	}, nil
}


