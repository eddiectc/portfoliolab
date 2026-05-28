package dws

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the DWS extractor.
const Name = "dws"

// Extractor extracts fund data from the DWS JSON API.
type Extractor struct {
	matcher *URLMatcher
	client  *Client
}

// NewExtractor creates a new DWS extractor.
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

// Match checks if a URL belongs to a DWS fund.
func (e *Extractor) Match(rawURL string) bool {
	return e.matcher.Match(rawURL)
}

// Extract fetches and parses data from the DWS API.
// All sections are parsed atomically — if any required section fails, the entire extraction is rejected.
func (e *Extractor) Extract(ctx context.Context, sourceURL string) (*extractor.ExtractResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// The sourceURL for DWS is expected to be the slug.
	// In the current framework, the sourceURL is passed. 
	// For DWS, we expect the slug to be part of the URL or the URL itself.
	slug := extractSlug(sourceURL)
	if slug == "" {
		return nil, fmt.Errorf("could not extract slug from source URL %q", sourceURL)
	}

	// 1. Fetch and parse Fund Info & Profile (Settings)
	settingsData, err := e.client.Fetch(slug, "pdpSettings")
	if err != nil {
		return nil, fmt.Errorf("fetch settings: %w", err)
	}

	fundInfo, err := ParseFundInfo(settingsData, slug)
	if err != nil {
		return nil, fmt.Errorf("parse fund info: %w", err)
	}

	// 2. Fetch and parse Holdings (Required)
	holdingsData, err := e.client.Fetch(slug, "holdings")
	if err != nil {
		return nil, fmt.Errorf("fetch holdings: %w", err)
	}

	holdings, countries, sectors, aum, err := ParseHoldings(holdingsData)
	if err != nil {
		return nil, fmt.Errorf("parse holdings: %w", err)
	}

	// 3. Fetch and parse NAV History & As-Of Date (Required)
	chartData, err := e.client.Fetch(slug, "performancechart")
	if err != nil {
		return nil, fmt.Errorf("fetch performance chart: %w", err)
	}

	asOfDate, err := ParseAsOfDate(chartData)
	if err != nil {
		return nil, fmt.Errorf("parse as-of date: %w", err)
	}

	navHistory, err := ParseNavHistory(chartData)
	if err != nil {
		return nil, fmt.Errorf("parse nav history: %w", err)
	}

	// Now parse fund profile with the calculated AUM
	fundProfile, err := ParseFundProfile(settingsData, aum)
	if err != nil {
		return nil, fmt.Errorf("parse fund profile: %w", err)
	}

	return &extractor.ExtractResult{
		AsOfDate:          asOfDate,
		FundInfo:          fundInfo,
		FundProfile:       fundProfile,
		Holdings:          holdings,
		NavHistory:        navHistory,
		Sectors:           sectors,
		CountryAllocation: countries,
	}, nil
}

// SetClient sets the HTTP client for fetching data.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

func extractSlug(sourceURL string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return sourceURL
	}

	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return sourceURL
	}
	return parts[len(parts)-1]
}

// containsProtocol is no longer needed as we use url.Parse

