//go:build integration

package dws

import (
	"context"
	"testing"
)

func TestRealExtractor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}
	ext := NewExtractor()

	// Use a set of known DWS slugs to verify live extraction.
	testCases := []struct {
		name string
		slug string
	}{
		{
			name: "Xtrackers S&P 500 ETF",
			slug: "LU0290358497-sp-500-ucits-etf",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ext.Extract(context.Background(), tc.slug)
			if err != nil {
				t.Fatalf("extraction failed for %s: %v", tc.slug, err)
			}

			if result.FundInfo == nil {
				t.Error("fund info is missing")
			}
			if len(result.Holdings) == 0 {
				t.Error("holdings list is empty")
			}
			if len(result.NavHistory) == 0 {
				t.Error("nav history is empty")
			}
			if result.AsOfDate.IsZero() {
				t.Error("as-of date is missing")
			}
		})
	}
}

func TestRealExtractor_InvalidSlug(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}
	ext := NewExtractor()
	
	slug := "non-existent-slug-12345"
	_, err := ext.Extract(context.Background(), slug)
	
	if err == nil {
		t.Error("expected error for invalid slug, got nil")
	}
}
