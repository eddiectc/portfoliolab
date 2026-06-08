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

	// Fetch all-holdings modal for ticker data
	modalURL := ExtractModalURL(html)
	var modalHTML string
	if modalURL != "" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		modalHTML, err = e.client.Fetch(modalURL)
		if err != nil {
			return nil, fmt.Errorf("fetch holdings modal: %w", err)
		}
	}

	// Fetch NAV history modal for funds where main page doesn't embed fundMarketData
	navModalURL := ExtractNavHistoryModalURL(html)
	var navModalHTML string
	if navModalURL != "" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		navModalHTML, err = e.client.Fetch(navModalURL)
		if err != nil {
			return nil, fmt.Errorf("fetch nav history modal: %w", err)
		}
	}

	return extractFromHTMLWithModals(html, modalHTML, navModalHTML)
}

// SetClient sets the HTTP client for fetching pages.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

// extractFromHTML parses all sections from pre-fetched HTML.
// Used for testing and when renderer is not available.
// Prefer extractFromHTMLWithModals for holdings with ticker data.
func extractFromHTML(html string) (*extractor.ExtractResult, error) {
	return extractFromHTMLWithModals(html, "", "")
}

// extractFromHTMLWithModals parses all sections from pre-fetched HTML,
// using the optional modalHTML for holdings with ticker/symbol data,
// and navModalHTML for NAV history when main page doesn't embed fundMarketData.
func extractFromHTMLWithModals(html, modalHTML, navModalHTML string) (*extractor.ExtractResult, error) {
	fundInfo, err := ParseFundInfo(html)
	if err != nil {
		return nil, fmt.Errorf("parse fund info: %w", err)
	}

	fundProfile, err := ParseFundProfile(html)
	if err != nil {
		return nil, fmt.Errorf("parse fund profile: %w", err)
	}

	// Use modal data for holdings (has tickers) if available, otherwise fall back to CSV
	var holdings []extractor.Holding
	if modalHTML != "" {
		holdings, err = ParseHoldingsFromModal(modalHTML)
		if err != nil {
			return nil, fmt.Errorf("parse holdings from modal: %w", err)
		}
	} else {
		holdings, err = ParseHoldings(html)
		if err != nil {
			return nil, fmt.Errorf("parse holdings: %w", err)
		}
	}

	navHistory, err := ParseNavHistory(html)
	if err != nil {
		return nil, fmt.Errorf("parse nav history: %w", err)
	}
	// Some funds don't embed fundMarketData on the main page —
	// fall back to the nav-history modal (HTML table format).
	if navHistory == nil && navModalHTML != "" {
		navHistory, err = ParseNavHistoryFromModal(navModalHTML)
		if err != nil {
			return nil, fmt.Errorf("parse nav history from modal: %w", err)
		}
	}
	// Set NAV currency from the fund's base currency.
	// The NAV values on WisdomTree pages are in the fund's base currency
	// (e.g. USD for WMGT), which can differ from the listing currency (e.g. GBP on LSE).
	if fundProfile != nil && fundProfile.BaseCurrency != "" {
		for i := range navHistory {
			navHistory[i].Currency = fundProfile.BaseCurrency
		}
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
