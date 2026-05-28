package dws

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the DWS extractor.
const Name = "dws"

// Extractor extracts fund data from the DWS JSON API.
type Extractor struct {
	client *Client
}

// NewExtractor creates a new DWS extractor.
func NewExtractor() *Extractor {
	return &Extractor{
		client: NewClient(),
	}
}

// Name returns the extractor identifier.
func (e *Extractor) Name() string {
	return Name
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

	fundProfile, err := ParseFundProfile(settingsData)
	if err != nil {
		return nil, fmt.Errorf("parse fund profile: %w", err)
	}

	// 2. Fetch and parse Holdings (Required)
	holdingsData, err := e.client.Fetch(slug, "holdings")
	if err != nil {
		return nil, fmt.Errorf("fetch holdings: %w", err)
	}

	holdings, countries, sectors, err := ParseHoldings(holdingsData)
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
	// The DWS source URL is expected to be the slug, 
	// or at least the last part of the path if it's a full URL.
	// For now, we assume the sourceURL passed to Extract is the slug.
	// If it's a full URL, we'll extract the last part.
	
	// Simplified: if it contains '://', take the last part of the path.
	// Otherwise, treat it as the slug.
	if containsProtocol(sourceURL) {
		return filepath.Base(sourceURL)
	}
	return sourceURL
}

func containsProtocol(s string) bool {
	return strings.Contains(s, "://")
}
