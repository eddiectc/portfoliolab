package blackrock

import (
	"context"
	"fmt"
	"net/url"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the BlackRock/iShares extractor.
const Name = "blackrock"

// Extractor extracts fund data from iShares product pages.
type Extractor struct {
	matcher *URLMatcher
	client  *Client
}

// NewExtractor creates a new BlackRock/iShares extractor.
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

// Match checks if a URL belongs to an iShares domain.
func (e *Extractor) Match(rawURL string) bool {
	return e.matcher.Match(rawURL)
}

// Extract fetches and parses data from an iShares product page.
// Uses a two-phase approach:
//   - Phase 1: Fetch product page HTML for fund identity, profile, characteristics, component ID
//   - Phase 2: Fetch holdings CSV using component ID, derive sector/geography from holdings
//
// All sections are parsed atomically — if any phase fails, the entire extraction is rejected.
func (e *Extractor) Extract(ctx context.Context, sourceURL string) (*extractor.ExtractResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Ensure the URL has the switchLocale parameter
	sourceURL = ensureSwitchLocale(sourceURL)

	// --- Phase 1: Product page HTML ---
	html, err := e.client.Fetch(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — fetch product page: %w", err)
	}

	// Parse fund identity
	fundInfo, err := ParseFundIdentity(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse fund identity: %w", err)
	}

	// Parse fund profile
	fundProfile, err := ParseFundProfile(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse fund profile: %w", err)
	}

	// Parse fund characteristics (optional — may be nil for some fund types)
	characteristics, err := ParseFundCharacteristics(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse fund characteristics: %w", err)
	}

	// Parse component ID (required for Phase 2)
	componentID, err := ParseComponentID(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse component ID: %w", err)
	}

	// Parse as-of date from page
	asOfDate, err := ParseAsOfDate(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse as-of date: %w", err)
	}

	// --- Phase 2: Holdings CSV ---
	holdingsURL, err := buildHoldingsURL(sourceURL, componentID)
	if err != nil {
		return nil, fmt.Errorf("phase 2 — build holdings URL: %w", err)
	}

	holdingsCSV, err := e.client.Fetch(holdingsURL)
	if err != nil {
		return nil, fmt.Errorf("phase 2 — fetch holdings CSV: %w", err)
	}

	holdings, csvAsOfDate, err := ParseHoldings(holdingsCSV)
	if err != nil {
		return nil, fmt.Errorf("phase 2 — parse holdings: %w", err)
	}

	// Use CSV as-of date if available (more precise than page-level)
	if csvAsOfDate != "" {
		if parsed, err := parseIShareDate(csvAsOfDate); err == nil {
			asOfDate = parsed
		}
	}

	// Derive sector allocation from holdings
	sectors, err := DeriveSectorAllocation(holdings)
	if err != nil {
		return nil, fmt.Errorf("phase 2 — derive sector allocation: %w", err)
	}

	// Derive country allocation from holdings
	countries, err := DeriveCountryAllocation(holdings)
	if err != nil {
		return nil, fmt.Errorf("phase 2 — derive country allocation: %w", err)
	}

	// Update fund info symbol from Bloomberg ticker if available
	if fundProfile.BenchmarkTicker != "" {
		// Bloomberg ticker is "IWMO LN" — use the full value
		fundInfo.Symbol = fundProfile.BenchmarkTicker
	}

	return &extractor.ExtractResult{
		AsOfDate:          asOfDate,
		FundInfo:          fundInfo,
		FundProfile:       fundProfile,
		Holdings:          holdings,
		Sectors:           sectors,
		CountryAllocation: countries,
		Characteristics:   characteristics,
	}, nil
}

// SetClient sets the HTTP client for fetching pages.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

// ensureSwitchLocale adds the required query parameters to bypass investor type selection.
func ensureSwitchLocale(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Get("switchLocale") == "" {
		q.Set("switchLocale", "y")
		q.Set("siteEntryPassthrough", "true")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// buildHoldingsURL constructs the holdings CSV download URL from the product page URL.
// Pattern: /uk/individual/en/products/{portfolioId}/{seo-slug}/{componentId}.ajax?fileType=csv&fileName={ticker}_holdings&dataType=fund
func buildHoldingsURL(productURL string, componentID string) (string, error) {
	u, err := url.Parse(productURL)
	if err != nil {
		return "", fmt.Errorf("parse product URL: %w", err)
	}

	// Build the holdings path
	holdingsPath := u.Path + "/" + componentID + ".ajax"

	holdingsURL := &url.URL{
		Scheme:   u.Scheme,
		Host:     u.Host,
		Path:     holdingsPath,
		RawQuery: "fileType=csv&dataType=fund",
	}

	return holdingsURL.String(), nil
}
