package blackrock

import (
	"context"
	"fmt"
	"net/url"

	"github.com/eddiectc/portfoliolab/internal/domain/extractor"
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
// Uses a three-phase approach:
//   - Phase 1: Fetch product page HTML for fund identity, characteristics,
//     the product data API config, and the portfolio ID
//   - Phase 2: Fetch key fund facts JSON from the product data API
//   - Phase 3: Fetch holdings JSON from the product data API and derive
//     sector/geography allocations from the holdings
//
// All sections are parsed atomically — if any phase fails, the entire
// extraction is rejected.
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

	// Parse product data API config (profile + holdings come from the JSON API)
	cfg, err := ParseProductDataConfig(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse product data config: %w", err)
	}

	// Parse portfolio ID from the product URL
	portfolioID, err := ParsePortfolioID(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse portfolio ID: %w", err)
	}

	// Parse fund characteristics (optional — may be nil for some fund types)
	characteristics, err := ParseFundCharacteristics(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse fund characteristics: %w", err)
	}

	// Parse as-of date from page (fallback — the holdings snapshot date wins)
	pageAsOfDate, err := ParseAsOfDate(html)
	if err != nil {
		return nil, fmt.Errorf("phase 1 — parse as-of date: %w", err)
	}

	// --- Phase 2: Key fund facts (product data JSON API) ---
	kffURL, err := BuildProductDataURL(cfg, portfolioID, "keyFundFacts")
	if err != nil {
		return nil, fmt.Errorf("phase 2 — build key fund facts URL: %w", err)
	}

	kffBody, err := e.client.Fetch(kffURL)
	if err != nil {
		return nil, fmt.Errorf("phase 2 — fetch key fund facts: %w", err)
	}

	fundProfile, err := ParseFundProfileFromJSON(kffBody)
	if err != nil {
		return nil, fmt.Errorf("phase 2 — parse key fund facts: %w", err)
	}

	// Total Expense Ratio is not in the keyFundFacts payload — pick it up
	// from the page HTML when the JSON profile lacks it.
	if fundProfile.AnnualExpenseRatio == 0 {
		if htmlProfile, perr := ParseFundProfile(html); perr == nil {
			fundProfile.AnnualExpenseRatio = htmlProfile.AnnualExpenseRatio
		}
	}

	// --- Phase 3: Holdings (product data JSON API) ---
	holdingsURL, err := BuildProductDataURL(cfg, portfolioID, "holdings")
	if err != nil {
		return nil, fmt.Errorf("phase 3 — build holdings URL: %w", err)
	}

	holdingsBody, err := e.client.Fetch(holdingsURL)
	if err != nil {
		return nil, fmt.Errorf("phase 3 — fetch holdings: %w", err)
	}

	holdings, holdingsAsOfDate, err := ParseHoldingsFromJSON(holdingsBody)
	if err != nil {
		return nil, fmt.Errorf("phase 3 — parse holdings: %w", err)
	}

	// Use the holdings snapshot as-of date if available (more precise than page-level)
	asOfDate := pageAsOfDate
	if !holdingsAsOfDate.IsZero() {
		asOfDate = holdingsAsOfDate
	}

	// Derive sector allocation from holdings
	sectors, err := DeriveSectorAllocation(holdings)
	if err != nil {
		return nil, fmt.Errorf("phase 3 — derive sector allocation: %w", err)
	}

	// Derive country allocation from holdings
	countries, err := DeriveCountryAllocation(holdings)
	if err != nil {
		return nil, fmt.Errorf("phase 3 — derive country allocation: %w", err)
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
