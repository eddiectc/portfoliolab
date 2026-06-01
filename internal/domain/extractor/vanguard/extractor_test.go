package vanguard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	if e.Name() != Name {
		t.Errorf("expected name %q, got %q", Name, e.Name())
	}
}

func TestExtractor_NavHistoryDays_Default(t *testing.T) {
	e := NewExtractor()
	if e.navHistoryDays != DefaultNavHistoryDays {
		t.Errorf("expected default navHistoryDays %d, got %d", DefaultNavHistoryDays, e.navHistoryDays)
	}
}

func TestExtractor_NavHistoryDays_WithOptions(t *testing.T) {
	e := NewExtractor()
	WithNavHistoryDays(365)(e)
	if e.navHistoryDays != 365 {
		t.Errorf("expected navHistoryDays 365, got %d", e.navHistoryDays)
	}
}

func TestExtractor_Match(t *testing.T) {
	e := NewExtractor()

	if !e.Match("https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing") {
		t.Error("expected vanguardinvestor.co.uk URL to match")
	}
	if e.Match("https://www.wisdomtree.eu/en-gb/etfs/wmgt") {
		t.Error("expected wisdomtree.eu URL to not match")
	}
	if e.Match("https://www.vanguard.com/etfs/vo") {
		t.Error("expected vanguard.com (US) URL to not match")
	}
}

func TestExtractor_Extract(t *testing.T) {
	// REST response
	restResponse := map[string]interface{}{
		"name":                     "Vanguard FTSE All-World UCITS ETF",
		"ticker":                   "VWRL",
		"sedol":                    "BYVTR12",
		"portId":                   "9505",
		"inceptionDate":            "2019-09-24",
		"isin":                     "IE00BK5BQT80",
		"currencyCode":             "USD",
		"OCF":                      "0.40%",
		"benchmark":                "FTSE All-World Index",
		"managementType":           "Index",
		"assetClass":               "Equity",
		"fundType":                 "etf",
		"distributionStrategyType": "INCM",
		"region":                   "Global",
	}

	// GraphQL responses
	holdingsData := map[string]interface{}{
		"borHoldings": []map[string]interface{}{
			{
				"holdings": map[string]interface{}{
					"totalHoldings": 2,
					"lastItemKey":   nil,
					"items": []interface{}{
						map[string]interface{}{
							"effectiveDate":           "2026-04-30",
							"marketValuePercentage":   1.399,
							"issuerName":              "Apple Inc.",
							"securityLongDescription": "Apple Inc. Common Stock",
							"securityType":            "EQ.STOCK",
						},
						map[string]interface{}{
							"effectiveDate":           "2026-04-30",
							"marketValuePercentage":   0.5,
							"issuerName":              "Microsoft Corp.",
							"securityLongDescription": "Microsoft Corp. Common Stock",
							"securityType":            "EQ.STOCK",
						},
					},
				},
			},
		},
	}

	sectorData := map[string]interface{}{
		"funds": []interface{}{
			map[string]interface{}{
				"sectorDiversification": []interface{}{
					map[string]interface{}{
						"sectorCode":       "10",
						"date":             "2026-04-30",
						"sectorName":       "Technology",
						"fundPercent":      25.5,
						"benchmarkPercent": 28.0,
					},
				},
			},
		},
	}

	countryData := map[string]interface{}{
		"funds": []interface{}{
			map[string]interface{}{
				"marketAllocation": []interface{}{
					map[string]interface{}{
						"portId":              "9505",
						"date":                "2026-04-30",
						"countryCode":         "US",
						"countryName":         "United States",
						"fundMktPercent":      60.5,
						"benchmarkMktPercent": 58.0,
						"regionCode":          "NA",
						"regionName":          "North America",
						"holdingStatCode":     "FTCTYATPCS",
					},
				},
			},
		},
	}

	charData := map[string]interface{}{
		"polarisAnalyticsHistory": []interface{}{
			map[string]interface{}{
				"portId": "9505",
				"monthly": map[string]interface{}{
					"analytics": map[string]interface{}{
						"fund": map[string]interface{}{
							"items": []interface{}{
								map[string]interface{}{
									"codes": map[string]interface{}{
										"PERATIO":    map[string]interface{}{"analyticValue": "18.5"},
										"PBRATIO":    map[string]interface{}{"analyticValue": "3.2"},
										"MKTCAPMEDN": map[string]interface{}{"analyticValue": "500.0"},
										"FRC5YRROE":  map[string]interface{}{"analyticValue": "15.3"},
										"EPSFRC5YR":  map[string]interface{}{"analyticValue": "8.7"},
										"TRNVRRPTR":  map[string]interface{}{"analyticValue": "1.05"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	navData := map[string]interface{}{
		"funds": []interface{}{
			map[string]interface{}{
				"pricingDetails": map[string]interface{}{
					"navPrices": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"price":        10.5,
								"asOfDate":     "2026-05-30",
								"currencyCode": "USD",
							},
						},
					},
				},
			},
		},
	}

	restBytes, _ := json.Marshal(restResponse)
	holdingsBytes, _ := json.Marshal(map[string]interface{}{"data": holdingsData})
	sectorBytes, _ := json.Marshal(map[string]interface{}{"data": sectorData})
	countryBytes, _ := json.Marshal(map[string]interface{}{"data": countryData})
	charBytes, _ := json.Marshal(map[string]interface{}{"data": charData})
	navBytes, _ := json.Marshal(map[string]interface{}{"data": navData})

	e := NewExtractor()
	e.client = &Client{}

	// Mock REST fetch
	e.client.SetRestFetch(func(slug string) ([]byte, error) {
		return restBytes, nil
	})

	// Mock GraphQL fetch
	e.client.SetGraphqlFetch(func(operationName string, variables map[string]interface{}, query string) ([]byte, error) {
		switch operationName {
		case "HoldingDetailsQuery":
			return holdingsBytes, nil
		case "getSectorDiversification":
			return sectorBytes, nil
		case "MarketAllocationGqlQuery":
			return countryBytes, nil
		case "FundCharacteristicsQuery":
			return charBytes, nil
		case "PriceDetailsQuery":
			return navBytes, nil
		default:
			return nil, fmt.Errorf("unknown operation: %s", operationName)
		}
	})

	result, err := e.Extract(context.Background(), "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Verify FundInfo
	if result.FundInfo == nil {
		t.Fatal("FundInfo is nil")
	}
	if result.FundInfo.Symbol != "VWRL" {
		t.Errorf("FundInfo.Symbol: got %q, want %q", result.FundInfo.Symbol, "VWRL")
	}
	if result.FundInfo.Name != "Vanguard FTSE All-World UCITS ETF" {
		t.Errorf("FundInfo.Name: got %q, want %q", result.FundInfo.Name, "Vanguard FTSE All-World UCITS ETF")
	}

	// Verify FundProfile
	if result.FundProfile == nil {
		t.Fatal("FundProfile is nil")
	}
	if result.FundProfile.AnnualExpenseRatio != 0.004 {
		t.Errorf("AnnualExpenseRatio: got %f, want 0.004", result.FundProfile.AnnualExpenseRatio)
	}

	// Verify Holdings
	if len(result.Holdings) != 2 {
		t.Errorf("Holdings: got %d, want 2", len(result.Holdings))
	}
	if len(result.Holdings) > 0 {
		if result.Holdings[0].Name != "Apple Inc." {
			t.Errorf("Holdings[0].Name: got %q, want %q", result.Holdings[0].Name, "Apple Inc.")
		}
		if result.Holdings[0].Percent != 1.399 {
			t.Errorf("Holdings[0].Percent: got %f, want 1.399", result.Holdings[0].Percent)
		}
	}

	// Verify Sectors
	if len(result.Sectors) != 1 {
		t.Errorf("Sectors: got %d, want 1", len(result.Sectors))
	}
	if len(result.Sectors) > 0 {
		if result.Sectors[0].Sector != "Technology" {
			t.Errorf("Sectors[0].Sector: got %q, want %q", result.Sectors[0].Sector, "Technology")
		}
		if result.Sectors[0].Date != "2026-04-30" {
			t.Errorf("Sectors[0].Date: got %q, want %q", result.Sectors[0].Date, "2026-04-30")
		}
	}

	// Verify Countries
	if len(result.CountryAllocation) != 1 {
		t.Errorf("CountryAllocation: got %d, want 1", len(result.CountryAllocation))
	}
	if len(result.CountryAllocation) > 0 {
		if result.CountryAllocation[0].Country != "United States" {
			t.Errorf("CountryAllocation[0].Country: got %q, want %q", result.CountryAllocation[0].Country, "United States")
		}
		if result.CountryAllocation[0].RegionName != "North America" {
			t.Errorf("CountryAllocation[0].RegionName: got %q, want %q", result.CountryAllocation[0].RegionName, "North America")
		}
	}

	// Verify Characteristics
	if result.Characteristics == nil {
		t.Fatal("Characteristics is nil")
	}
	if result.Characteristics.PriceToEarnings != 18.5 {
		t.Errorf("PriceToEarnings: got %f, want 18.5", result.Characteristics.PriceToEarnings)
	}
	if result.Characteristics.MedianMarketCap != 500.0 {
		t.Errorf("MedianMarketCap: got %f, want 500.0", result.Characteristics.MedianMarketCap)
	}

	// Verify NAV History
	if len(result.NavHistory) != 1 {
		t.Errorf("NavHistory: got %d, want 1", len(result.NavHistory))
	}

	// Verify AsOfDate
	if result.AsOfDate.IsZero() {
		t.Error("AsOfDate is zero")
	}
}

func TestExtractor_Extract_Phase1Failure(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{}

	// Mock REST fetch to fail
	e.client.SetRestFetch(func(slug string) ([]byte, error) {
		return nil, fmt.Errorf("HTTP 404")
	})

	_, err := e.Extract(context.Background(), "https://www.vanguardinvestor.co.uk/investments/invalid-fund")
	if err == nil {
		t.Error("expected error for Phase 1 failure")
	}
}

func TestExtractor_Extract_Phase2Failure(t *testing.T) {
	restResponse := map[string]interface{}{
		"name":          "Test Fund",
		"ticker":        "TEST",
		"portId":        "9999",
		"inceptionDate": "2020-01-01",
		"isin":          "IE00TESTTEST",
	}
	restBytes, _ := json.Marshal(restResponse)

	e := NewExtractor()
	e.client = &Client{}
	e.client.SetRestFetch(func(slug string) ([]byte, error) {
		return restBytes, nil
	})

	// Mock GraphQL to fail on holdings
	e.client.SetGraphqlFetch(func(operationName string, variables map[string]interface{}, query string) ([]byte, error) {
		return nil, fmt.Errorf("GraphQL request failed")
	})

	_, err := e.Extract(context.Background(), "https://www.vanguardinvestor.co.uk/investments/test-fund")
	if err == nil {
		t.Error("expected error for Phase 2 failure")
	}
}

func TestExtractor_Extract_EmptyHoldings(t *testing.T) {
	restResponse := map[string]interface{}{
		"name":          "Test Fund",
		"ticker":        "TEST",
		"portId":        "9999",
		"inceptionDate": "2020-01-01",
		"isin":          "IE00TESTTEST",
	}
	restBytes, _ := json.Marshal(restResponse)

	// Empty holdings with no effectiveDate
	holdingsData := map[string]interface{}{
		"borHoldings": []map[string]interface{}{
			{
				"holdings": map[string]interface{}{
					"totalHoldings": 0,
					"lastItemKey":   nil,
					"items":         []interface{}{},
				},
			},
		},
	}
	holdingsBytes, _ := json.Marshal(map[string]interface{}{"data": holdingsData})

	e := NewExtractor()
	e.client = &Client{}
	e.client.SetRestFetch(func(slug string) ([]byte, error) {
		return restBytes, nil
	})

	e.client.SetGraphqlFetch(func(operationName string, variables map[string]interface{}, query string) ([]byte, error) {
		switch operationName {
		case "HoldingDetailsQuery":
			return holdingsBytes, nil
		default:
			// Return empty but valid responses for other queries
			data, _ := json.Marshal(map[string]interface{}{
				"data": map[string]interface{}{
					"funds": []interface{}{
						map[string]interface{}{
							"sectorDiversification": []interface{}{},
							"marketAllocation":      []interface{}{},
							"pricingDetails": map[string]interface{}{
								"navPrices": map[string]interface{}{"items": []interface{}{}},
							},
						},
					},
					"polarisAnalyticsHistory": []interface{}{
						map[string]interface{}{
							"portId": "9999",
							"monthly": map[string]interface{}{
								"analytics": map[string]interface{}{
									"fund": map[string]interface{}{
										"items": []interface{}{
											map[string]interface{}{
												"codes": map[string]interface{}{},
											},
										},
									},
								},
							},
						},
					},
				},
			})
			return data, nil
		}
	})

	result, err := e.Extract(context.Background(), "https://www.vanguardinvestor.co.uk/investments/test-fund")
	// Empty holdings should succeed (spec: stored as empty, not a failure)
	if err != nil {
		t.Fatalf("unexpected error for empty holdings: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Holdings) != 0 {
		t.Errorf("expected 0 holdings, got %d", len(result.Holdings))
	}
}

func TestExtractor_Extract_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	e := NewExtractor()
	_, err := e.Extract(ctx, "https://www.vanguardinvestor.co.uk/investments/test-fund")
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestExtractor_Extract_HTTPError(t *testing.T) {
	// Use a real httptest server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	e := NewExtractor()
	// Use real HTTP client pointing to our test server
	// We can't easily override the base URL, so we test through the client directly

	client := &Client{}
	client.SetRestFetch(func(slug string) ([]byte, error) {
		return nil, fmt.Errorf("HTTP 404")
	})

	e.SetClient(client)

	_, err := e.Extract(context.Background(), "https://www.vanguardinvestor.co.uk/investments/test-fund")
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}
