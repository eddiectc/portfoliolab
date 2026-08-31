package dimensional

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"github.com/govalues/decimal"
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

	// 2. Map ISIN to portfolioNumber and fetch NAV history using the Fund Center Registry API
	portfolioNumber, navHistory, err := e.getPortfolioNumberAndNavHistory(ctx, isin)
	if err != nil {
		return nil, fmt.Errorf("map ISIN to portfolio number: %w", err)
	}

	// The reference date is the date of the most recent NAV point.
	if len(navHistory) == 0 {
		return nil, fmt.Errorf("no NAV history found for ISIN %s", isin)
	}
	asOfDate := navHistory[0].Date

	// 3. Fetch detailed fund data via POST request
	headers := map[string]string{"x-selected-country": "GB"}
	body := map[string]int{"portfolioNumber": portfolioNumber}
	detailJSON, err := e.client.Post("https://etf.dimensional.com/public/v2/fundcenter/funddetail", body, headers)
	if err != nil {
		return nil, fmt.Errorf("fetch fund detail: %w", err)
	}

	// 4. Parse the detailed data
	fundInfo, fundProfile, sectors, countries, _, csvURL, err := ParseFundDetail(detailJSON)
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

	if len(holdings) == 0 {
		return nil, fmt.Errorf("holdings list is empty for ISIN %s — extraction failed (atomic)", isin)
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

func (e *Extractor) getPortfolioNumberAndNavHistory(ctx context.Context, isin string) (int, []extractor.NavPoint, error) {
	url := "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true"
	headers := map[string]string{"x-selected-country": "GB"}

	resp, err := e.client.Fetch(url, headers)
	if err != nil {
		return 0, nil, err
	}

	var data struct {
		Data struct {
			Portfolios []struct {
				PortfolioNumber int `json:"portfolioNumber"`
				Meta            struct {
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
		return 0, nil, fmt.Errorf("decode fund center: %w", err)
	}

	for _, p := range data.Data.Portfolios {
		for _, id := range p.Meta.Identifiers {
			if id.Slug == "isin" && strings.ToUpper(id.Value) == isin {
				var navHistory []extractor.NavPoint
				for _, price := range p.Prices {
					if price.Nav.Value != nil {
						// Convert interface{} NAV to float64 safely
						var navVal float64
						switch v := price.Nav.Value.(type) {
						case float64:
							navVal = v
						case float32:
							navVal = float64(v)
						case int:
							navVal = float64(v)
						default:
							continue
						}

						date, parseErr := time.Parse("2006-01-02", price.Date.Value)
						if parseErr != nil {
							continue
						}
						navHistory = append(navHistory, extractor.NavPoint{
							Date: date,
							NAV:  decimal.MustParse(fmt.Sprintf("%.4f", navVal)),
						})
					}
				}

				if len(navHistory) == 0 {
					return 0, nil, fmt.Errorf("no price entries with valid NAV found for ISIN %s", isin)
				}

				return p.PortfolioNumber, navHistory, nil
			}
		}
	}

	return 0, nil, fmt.Errorf("ISIN %s not found in fund center", isin)
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
