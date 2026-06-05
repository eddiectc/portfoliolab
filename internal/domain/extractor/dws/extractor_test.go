package dws

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExtractor_Extract_Success(t *testing.T) {
	ext := NewExtractor()
	ext.client.SetMinDelay(0) // disable rate limiting in tests

	// Mock responses
	responses := map[string]string{
		"test-slug/pdpSettings":      `{"productType": "ETF", "internalId": "ID123", "fundFamily": "Xtrackers", "costsAndFees": {"totalOngoingCosts": "0.20%"}}`,
		"test-slug/pdpMetaTagsTealium": `{"pdpResult": {"pageFrame": {"productHeader": {"texts": {"title": "Test ETF"}, "tableValues": [{"key": "isin", "value": "US123"}, {"key": "totalNetAssets", "value": "1000000000"}, {"key": "inceptionDate", "value": "2020-01-15"}]}}}}`,
		"test-slug/holdings":         `{"tables": [{"values": [{"header": {"value": "US123"}, "column_0": {"value": "Asset 1"}, "column_1": {"value": "10.0%"}, "column_3": {"value": "USA"}, "column_4": {"value": "Tech"}}]}]}`,
		"test-slug/performancechart": `{"asOfDate": "2026-05-27T00:00:00Z", "chartData": [{"timestamp": "2026-05-27T00:00:00Z", "value": 100.0}]}`,
	}

	ext.client.SetFetchFunc(func(url string) (string, error) {
		// Extract the slug/endpoint part from the URL
		// The client prepends baseURL, so we check for the suffix
		for k, v := range responses {
			if strings.HasSuffix(url, k) {
				return v, nil
			}
		}
		return "", errors.New("url not found in mock")
	})

	result, err := ext.Extract(context.Background(), "test-slug")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result.FundInfo.Symbol != "test-slug" {
		t.Errorf("expected symbol test-slug, got %s", result.FundInfo.Symbol)
	}
	if len(result.Holdings) != 1 {
		t.Errorf("expected 1 holding, got %d", len(result.Holdings))
	}
	if result.AsOfDate.Year() != 2026 {
		t.Errorf("expected year 2026, got %d", result.AsOfDate.Year())
	}
}

func TestExtractor_Extract_AtomicFailure(t *testing.T) {
	const metaTagsJSON = `{"pdpResult": {"pageFrame": {"productHeader": {"texts": {"title": "Test ETF"}, "tableValues": [{"key": "isin", "value": "US123"}, {"key": "totalNetAssets", "value": "1000000000"}, {"key": "inceptionDate", "value": "2020-01-15"}]}}}}`

	tests := []struct {
		name           string
		mockResponses  map[string]string
		mockError      string
		errorEndpoint  string
		wantErrContain string
	}{
		{
			name: "fail on settings",
			mockResponses: map[string]string{
				"test-slug/pdpSettings":      `invalid json`,
				"test-slug/pdpMetaTagsTealium": metaTagsJSON,
			},
			wantErrContain: "parse fund info",
		},
		{
			name: "fail on holdings",
			mockResponses: map[string]string{
				"test-slug/pdpSettings":      `{"productType": "ETF", "internalId": "ID123", "fundFamily": "Xtrackers", "costsAndFees": {"totalOngoingCosts": "0.20%"}}`,
				"test-slug/pdpMetaTagsTealium": metaTagsJSON,
				"test-slug/holdings":         `{"tables": []}`, // Empty holdings should fail
			},
			wantErrContain: "parse holdings",
		},
		{
			name: "fail on performance chart",
			mockResponses: map[string]string{
				"test-slug/pdpSettings":      `{"productType": "ETF", "internalId": "ID123", "fundFamily": "Xtrackers", "costsAndFees": {"totalOngoingCosts": "0.20%"}}`,
				"test-slug/pdpMetaTagsTealium": metaTagsJSON,
				"test-slug/holdings":         `{"tables": [{"values": [{"header": {"value": "US123"}, "column_0": {"value": "Asset 1"}, "column_1": {"value": "10.0%"}, "column_3": {"value": "USA"}, "column_4": {"value": "Tech"}}]}]}`,
				"test-slug/performancechart": `{"asOfDate": "invalid-date"}`,
			},
			wantErrContain: "parse as-of date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewExtractor()
			ext.client.SetMinDelay(0) // disable rate limiting in tests
			ext.client.SetFetchFunc(func(url string) (string, error) {
				for k, v := range tt.mockResponses {
					if strings.HasSuffix(url, k) {
						return v, nil
					}
				}
				return "", errors.New("url not found")
			})

			_, err := ext.Extract(context.Background(), "test-slug")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrContain)
			}
		})
	}
}
