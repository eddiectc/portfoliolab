package vanguard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the Vanguard extractor.
const Name = "vanguard"

// Extractor extracts fund data from Vanguard UK investor pages.
type Extractor struct {
	matcher *URLMatcher
	client  *Client
}

// NewExtractor creates a new Vanguard extractor.
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

// Match checks if a URL belongs to a Vanguard UK domain.
func (e *Extractor) Match(rawURL string) bool {
	return e.matcher.Match(rawURL)
}

// Extract fetches and parses data from a Vanguard fund page.
// Uses a two-phase approach:
//   - Phase 1: REST API for fund identity + portId resolution
//   - Phase 2: GraphQL API for deep data (holdings, sectors, countries, characteristics, NAV)
//
// All sections are parsed atomically — if any phase fails, the entire extraction is rejected.
func (e *Extractor) Extract(ctx context.Context, sourceURL string) (*extractor.ExtractResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Extract fund slug from URL
	slug, err := extractSlug(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("extract slug: %w", err)
	}

	// --- Phase 1: REST API — fund identity + profile ---
	restData, err := e.client.FetchREST(slug)
	if err != nil {
		return nil, fmt.Errorf("phase 1 (REST): %w", err)
	}

	fundInfo, portId, err := ParseFundIdentity(restData)
	if err != nil {
		return nil, fmt.Errorf("phase 1 (fund identity): %w", err)
	}

	fundProfile, err := ParseFundProfile(restData)
	if err != nil {
		return nil, fmt.Errorf("phase 1 (fund profile): %w", err)
	}

	// --- Phase 2: GraphQL — deep data ---
	portIds := []string{portId}

	// Holdings (with pagination)
	holdings, effectiveDate, err := e.fetchAllHoldings(ctx, portIds)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (holdings): %w", err)
	}

	// Sector allocation
	sectorData, err := e.client.FetchGraphQL("getSectorDiversification", map[string]interface{}{
		"portIds": portIds,
	}, sectorQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (sectors): %w", err)
	}
	sectors, _, err := ParseSectorAllocation(sectorData)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (sectors parse): %w", err)
	}

	// Country allocation
	countryData, err := e.client.FetchGraphQL("MarketAllocationGqlQuery", map[string]interface{}{
		"portIds": portIds,
	}, countryQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (countries): %w", err)
	}
	countries, _, err := ParseCountryAllocation(countryData)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (countries parse): %w", err)
	}

	// Fund characteristics
	charData, err := e.client.FetchGraphQL("FundCharacteristicsQuery", map[string]interface{}{
		"portIds": portIds,
	}, characteristicsQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (characteristics): %w", err)
	}
	characteristics, err := ParseFundCharacteristics(charData)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (characteristics parse): %w", err)
	}

	// NAV history
	navData, err := e.client.FetchGraphQL("PriceDetailsQuery", map[string]interface{}{
		"portIds":   portIds,
		"startDate": "2020-01-01",
		"endDate":   time.Now().Format("2006-01-02"),
		"limit":     float64(0),
	}, navQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (NAV): %w", err)
	}
	navHistory, err := ParseNavHistory(navData)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (NAV parse): %w", err)
	}

	return &extractor.ExtractResult{
		AsOfDate:          parseAsOfDate(effectiveDate),
		FundInfo:          fundInfo,
		FundProfile:       fundProfile,
		Holdings:          holdings,
		NavHistory:        navHistory,
		Sectors:           sectors,
		CountryAllocation: countries,
		Characteristics:   characteristics,
	}, nil
}

// fetchAllHoldings fetches all holdings pages with pagination.
func (e *Extractor) fetchAllHoldings(ctx context.Context, portIds []string) ([]extractor.Holding, string, error) {
	var allHoldings []extractor.Holding
	effectiveDate := ""
	lastItemKey := interface{}(nil)

	for {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		default:
		}

		variables := map[string]interface{}{
			"portIds":       portIds,
			"securityTypes": allSecurityTypes,
			"lastItemKey":   lastItemKey,
		}

		data, err := e.client.FetchGraphQLPage("HoldingDetailsQuery", variables, holdingsQuery)
		if err != nil {
			return nil, "", fmt.Errorf("fetch holdings page: %w", err)
		}

		holdings, date, err := ParseHoldings(data)
		if err != nil {
			return nil, "", fmt.Errorf("parse holdings page: %w", err)
		}

		if len(holdings) == 0 {
			break
		}

		if effectiveDate == "" {
			effectiveDate = date
		}

		allHoldings = append(allHoldings, holdings...)

		// Check if there are more pages
		// We need to re-parse to get lastItemKey
		hasMore, nextKey, err := checkHasMore(data)
		if err != nil {
			return nil, "", fmt.Errorf("check pagination: %w", err)
		}
		if !hasMore {
			break
		}
		lastItemKey = nextKey
	}

	if len(allHoldings) > 0 && effectiveDate == "" {
		return nil, "", fmt.Errorf("holdings missing effectiveDate")
	}

	return allHoldings, effectiveDate, nil
}

// checkHasMore checks if the holdings response has more pages and returns the next key.
func checkHasMore(data []byte) (bool, string, error) {
	var root graphqlRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return false, "", fmt.Errorf("unmarshal GraphQL root: %w", err)
	}

	var resp holdingsResponse
	if err := json.Unmarshal(root.Data, &resp); err != nil {
		return false, "", fmt.Errorf("unmarshal holdings: %w", err)
	}

	if len(resp.BorHoldings) == 0 || resp.BorHoldings[0].Holdings.LastItemKey == nil {
		return false, "", nil
	}
	return true, *resp.BorHoldings[0].Holdings.LastItemKey, nil
}

// SetClient sets the HTTP client for fetching data.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

// extractSlug extracts the fund slug from a Vanguard URL.
// URL pattern: https://www.vanguardinvestor.co.uk/investments/{fundSlug}
func extractSlug(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("no fund slug in URL path %q", parsed.Path)
	}

	// The slug is the last segment of the path
	slug := parts[len(parts)-1]
	if slug == "" {
		return "", fmt.Errorf("empty fund slug in URL %q", rawURL)
	}

	return slug, nil
}

// parseAsOfDate parses a date string into time.Time.
func parseAsOfDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
