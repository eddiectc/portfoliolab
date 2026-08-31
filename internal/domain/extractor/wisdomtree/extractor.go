package wisdomtree

import (
	"context"
	"fmt"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the WisdomTree extractor.
const Name = "wisdomtree"

// Extractor extracts fund data from WisdomTree product pages (2026 site).
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

// Extract fetches and parses data from a WisdomTree product page.
//
// Flow (RESEARCH.md §10):
//  1. fetch the fund page (CycleTLS, browser fingerprint)
//  2. extract the wtClassID (fails explicitly when absent)
//  3. call the JSON API sequentially (fund-holdings, then fund-history;
//     the client enforces the rate limit between requests)
//  4. decode the React Flight payload from the page body
//  5. assemble the ExtractResult
//
// Error semantics: required data (fund info, holdings, the page "As of"
// date) is atomic — any failure rejects the whole extraction. Optional
// sections (profile, country, market cap, characteristics, sectors,
// themes) degrade to nil when absent from the page. JSON API failures are
// surfaced distinctly from page-parse failures (undocumented API,
// RESEARCH.md §9.1).
func (e *Extractor) Extract(ctx context.Context, sourceURL string) (*extractor.ExtractResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 1. Fund page.
	pageBody, err := e.client.Fetch(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}

	// 2. wtClassID keys both JSON API endpoints.
	wtClassID, err := ExtractWtClassID(pageBody)
	if err != nil {
		return nil, fmt.Errorf("wtClassID: %w", err)
	}

	// 3. JSON API (sequential; client rate-limits).
	holdingRecords, err := e.client.FundHoldings(ctx, wtClassID)
	if err != nil {
		return nil, fmt.Errorf("fund-holdings API: %w", err)
	}
	history, err := e.client.FundHistory(ctx, wtClassID)
	if err != nil {
		return nil, fmt.Errorf("fund-history API: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 4. Flight payload.
	flight := DecodeFlight(pageBody)

	// 5. Assemble.
	info, err := FundInfoFromHistory(history)
	if err != nil {
		return nil, fmt.Errorf("fund info: %w", err)
	}

	profile, err := ParseFundProfileFromFlight(flight)
	if err != nil {
		return nil, err
	}

	// As-of semantics (RESEARCH.md §9.8): the page "As of" table header is
	// the extraction as-of date; a missing as-of fails the extraction. The
	// NAV table's header carries the fund's reporting date.
	navTable := flight.Table("Net Asset Value")
	if navTable == nil {
		return nil, fmt.Errorf("page has no Net Asset Value table (as-of date unavailable)")
	}
	asOf, ok := navTable.AsOfDate()
	if !ok {
		return nil, fmt.Errorf("Net Asset Value table header %q carries no as-of date", navTable.AsOf)
	}

	navCurrency := "USD"
	if profile != nil && profile.BaseCurrency != "" {
		navCurrency = profile.BaseCurrency
	}

	navHistory, err := ParseNavHistoryFromAPI(history, navCurrency)
	if err != nil {
		return nil, fmt.Errorf("nav history: %w", err)
	}
	aum, err := LatestAUM(history)
	if err != nil {
		return nil, fmt.Errorf("aum: %w", err)
	}
	if profile != nil {
		profile.TotalNetAssets = aum
	}

	holdings := ParseHoldingsFromAPI(holdingRecords)
	if len(holdings) == 0 {
		return nil, fmt.Errorf("fund-holdings returned no tradeable rows")
	}

	// Optional sections — nil when absent on the page.
	countries, _ := ParseCountryAllocationFromFlight(flight)
	marketCap, _ := ParseMarketCapFromFlight(flight)
	characteristics, _ := ParseFundCharacteristicsFromFlight(flight)
	sectors, _ := ParseSectorsFromFlight(flight)
	themes, _ := ParseThemesFromFlight(flight)

	return &extractor.ExtractResult{
		Source:            Name,
		AsOfDate:          asOf,
		FundInfo:          info,
		FundProfile:       profile,
		Holdings:          holdings,
		NavHistory:        navHistory,
		CountryAllocation: countries,
		MarketCap:         marketCap,
		Characteristics:   characteristics,
		Sectors:           sectors,
		Themes:            themes,
	}, nil
}
