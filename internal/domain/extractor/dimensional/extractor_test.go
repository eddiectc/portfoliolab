package dimensional

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockClient struct {
	fetchFunc func(url string, headers map[string]string) (string, error)
	postFunc  func(url string, body interface{}, headers map[string]string) (string, error)
}

func (m *mockClient) Fetch(url string, headers map[string]string) (string, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(url, headers)
	}
	return "", fmt.Errorf("fetch not mocked")
}

func (m *mockClient) Post(url string, body interface{}, headers map[string]string) (string, error) {
	if m.postFunc != nil {
		return m.postFunc(url, body, headers)
	}
	return "", fmt.Errorf("post not mocked")
}

func TestExtractor_Extract_Success(t *testing.T) {
	isin := "IE000EGGFVG6"
	sourceURL := fmt.Sprintf("https://www.dimensional.com/gb-en/funds/%s/global-core-equity-ucits-etf-acc", isin)
	asOfDate := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)

	// Registry API response
	registryJSON := fmt.Sprintf(`{
		"data": {
			"portfolios": [
				{
					"portfolioNumber": 1600,
					"meta": {
						"identifiers": [ { "slug": "isin", "value": "%s" } ]
					},
					"prices": [ 
						{ "date": { "value": "2026-05-28" }, "nav": { "value": 28.34 } },
						{ "date": { "value": "2026-05-27" }, "nav": { "value": 28.20 } }
					]
				}
			]
		}
	}`, isin)

	// Detail API response (direct subset of funddetail.json)
	detailJSON := `{"data":{"lensGroups":[{"data":{"lenses":[{"data":{"slug":"fundFacts","blends":[{"data":{"fundFacts":{"marketingName":"Global Core Equity UCITS ETF (Acc.)","fundAum":{"aum":{"value":1483669606.1}},"inceptionDate":{"value":"2025-11-12"}}}}]}},{"data":{"slug":"fundPrices","blends":[{"data":{"fundPrices":{"prices":[{"nav":{"value":28.3405}}]}}]}},{"data":{"slug":"charsEtfTopHoldingsDaily","blends":[{"data":{"fullHoldingsCsvUrl":"https://tools-blob.dimensional.com/etf/20260528/IE000EGGFVG6.csv"}}]}}]}}]}}`

	// CSV content
	csvContent := "date,etf_isin,ticker,description,weight,market_value\n2026-05-28,IE000EGGFVG6,AAPL,Apple Inc,0.05,5000000\n"

	extractor := NewExtractor()
	mock := &mockClient{
		fetchFunc: func(url string, headers map[string]string) (string, error) {
			if url == "https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true" {
				return registryJSON, nil
			}
			if url == "https://tools-blob.dimensional.com/etf/20260528/IE000EGGFVG6.csv" {
				return csvContent, nil
			}
			return "", fmt.Errorf("unexpected fetch URL: %s", url)
		},
		postFunc: func(url string, body interface{}, headers map[string]string) (string, error) {
			if url == "https://etf.dimensional.com/public/v2/fundcenter/funddetail" {
				return detailJSON, nil
			}
			return "", fmt.Errorf("unexpected post URL: %s", url)
		},
	}
	extractor.client = mock

	result, err := extractor.Extract(context.Background(), sourceURL)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, asOfDate, result.AsOfDate)
	assert.Equal(t, isin, result.FundInfo.Symbol)
	assert.Equal(t, "Global Core Equity UCITS ETF (Acc.)", result.FundInfo.Name)
	assert.Len(t, result.NavHistory, 2)
	assert.Equal(t, "2026-05-28", result.NavHistory[0].Date)
	assert.Len(t, result.Holdings, 1)
	assert.Equal(t, "AAPL", result.Holdings[0].Symbol)
}

func TestExtractor_Extract_RegistryError(t *testing.T) {
	isin := "IE000EGGFVG6"
	sourceURL := fmt.Sprintf("https://www.dimensional.com/gb-en/funds/%s/global-core-equity-ucits-etf-acc", isin)

	extractor := NewExtractor()
	extractor.client = &mockClient{
		fetchFunc: func(url string, headers map[string]string) (string, error) {
			return "", fmt.Errorf("registry api failure")
		},
	}

	_, err := extractor.Extract(context.Background(), sourceURL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map ISIN to portfolio number")
}

func TestExtractor_Match(t *testing.T) {
	extractor := NewExtractor()
	tests := []struct {
		url    string
		expect bool
	}{
		{"https://www.dimensional.com/gb-en/funds/ie000eggfvg6/global-core-equity-ucits-etf-acc", true},
		{"https://www.google.com", false},
	}

	for _, tc := range tests {
		if got := extractor.Match(tc.url); got != tc.expect {
			t.Errorf("Match(%q) = %v, want %v", tc.url, got, tc.expect)
		}
	}
}
