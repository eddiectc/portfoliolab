package dimensional

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/govalues/decimal"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the Dimensional extractor.
const Name = "dimensional"

// Extractor extracts fund data from Dimensional Fund Advisors APIs.
type Extractor struct {
	matcher *URLMatcher
	client  ClientAPI
}

// NewExtractor creates a new Dimensional extractor.
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

// Match checks if a URL belongs to a Dimensional domain.
func (e *Extractor) Match(rawURL string) bool {
	return e.matcher.Match(rawURL)
}

// Extract fetches and parses data from Dimensional APIs.
func (e *Extractor) Extract(ctx context.Context, sourceURL string) (*extractor.ExtractResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 1. Identify ISIN from URL
	isin, err := extractISIN(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("extract ISIN: %w", err)
	}
	isin = strings.ToUpper(isin)

	// 2. Map ISIN to portfolioNumber using the Fund Center Registry API
	portfolioNumber, asOfDate, err := e.getPortfolioNumberAndDate(ctx, isin)
	if err != nil {
		return nil, fmt.Errorf("map ISIN to portfolio number: %w", err)
	}

	// 3. Fetch detailed fund data via POST request
	headers := map[string]string{"x-selected-country": "GB"}
	body := map[string]int{"portfolioNumber": portfolioNumber}
	detailJSON, err := e.client.Post("https://etf.dimensional.com/public/v2/fundcenter/funddetail", body, headers)
	if err != nil {
		return nil, fmt.Errorf("fetch fund detail: %w", err)
	}

	// 4. Parse the detailed data
	fundInfo, fundProfile, sectors, countries, nav, csvURL, err := ParseFundDetail(detailJSON)
	if err != nil {
		return nil, fmt.Errorf("parse fund detail: %w", err)
	}

	// Ensure fundInfo has the correct symbol
	fundInfo.Symbol = isin

	// 5. Fetch and parse holdings from CSV
	if csvURL == "" {
		return nil, fmt.Errorf("full holdings CSV URL not found in response")
	}
	csvContent, err := e.client.Fetch(csvURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch holdings CSV: %w", err)
	}

	holdings, err := ParseHoldingsCSV(csvContent)
	if err != nil {
		return nil, fmt.Errorf("parse holdings: %w", err)
	}

	// 6. Construct NAV history (using the single NAV point for now, or could be extended)
	navHistory := []extractor.NavPoint{
		{
			Date: asOfDate.Format("2006-01-02"),
			NAV:  decimal.MustParse(fmt.Sprintf("%.4f", nav)),
		},
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

func (e *Extractor) getPortfolioNumberAndDate(ctx context.Context, isin string) (int, time.Time, error) {
	url := "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true"
	headers := map[string]string{"x-selected-country": "GB"}

	resp, err := e.client.Fetch(url, headers)
	if err != nil {
		return 0, time.Time{}, err
	}

	var data struct {
		Data struct {
			Portfolios []struct {
				PortfolioNumber int `json:"portfolioNumber"`
				Meta struct {
					Identifiers []struct {
						Value string `json:"value"`
						Slug  string `json:"slug"`
					} `json:"identifiers"`
				} `json:"meta"`
				Prices []struct {
					Date struct {
						Value string `json:"value"`
					} `json:"date"`
					Nav struct {
						Value interface{} `json:"value"`
					} `json:"nav"`
				} `json:"prices"`
			} `json:"portfolios"`
		} `json:"data"`
	}

	if err := json.NewDecoder(strings.NewReader(resp)).Decode(&data); err != nil {
		return 0, time.Time{}, fmt.Errorf("decode fund center: %w", err)
	}

	for _, p := range data.Data.Portfolios {
		for _, id := range p.Meta.Identifiers {
			if id.Slug == "isin" && strings.ToUpper(id.Value) == isin {
				// Find the most recent date that has a non-null NAV.
				for _, price := range p.Prices {
					if price.Nav.Value != nil {
						t, err := time.Parse("2006-01-02", price.Date.Value)
						if err != nil {
							return 0, time.Time{}, fmt.Errorf("parse price date %q: %w", price.Date.Value, err)
						}
						return p.PortfolioNumber, t, nil
					}
				}
				return 0, time.Time{}, fmt.Errorf("no price entries with valid NAV found for ISIN %s", isin)
			}
		}
	}

	return 0, time.Time{}, fmt.Errorf("ISIN %s not found in fund center", isin)
}

// SetClient sets the HTTP client for fetching pages.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

func extractISIN(url string) (string, error) {
	re := regexp.MustCompile(`/funds/([a-z0-9]{12})/`)
	match := re.FindStringSubmatch(strings.ToLower(url))
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("ISIN not found in URL")
	}
	return match[1], nil
}
