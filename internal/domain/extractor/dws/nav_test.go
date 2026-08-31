package dws

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func TestParseNavCurrency(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected string
	}{
		{
			name: "NAV (USD)",
			data: `{
				"seriesConfiguration": [
					{"chartType": "Nav", "identifier": "NAV (USD)"},
					{"chartType": "Index", "identifier": "Some Index (USD)"}
				],
				"values": []
			}`,
			expected: "USD",
		},
		{
			name: "NAV (EUR)",
			data: `{
				"seriesConfiguration": [
					{"chartType": "Nav", "identifier": "NAV (EUR)"}
				],
				"values": []
			}`,
			expected: "EUR",
		},
		{
			name: "NAV (GBP)",
			data: `{
				"seriesConfiguration": [
					{"chartType": "Nav", "identifier": "NAV (GBP)"}
				],
				"values": []
			}`,
			expected: "GBP",
		},
		{
			name: "no series configuration",
			data: `{
				"seriesConfiguration": [],
				"values": []
			}`,
			expected: "",
		},
		{
			name: "no parentheses",
			data: `{
				"seriesConfiguration": [
					{"chartType": "Nav", "identifier": "NAV"}
				],
				"values": []
			}`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currency, err := ParseNavCurrency(tt.data)
			if err != nil {
				t.Fatalf("ParseNavCurrency failed: %v", err)
			}
			if currency != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, currency)
			}
		})
	}
}

func TestParseNavHistory(t *testing.T) {
	jsonData := `{
		"asOfDate": "27/05/2026",
		"seriesConfiguration": [
			{"chartType": "Nav", "identifier": "NAV (USD)"},
			{"chartType": "Index", "identifier": "Nasdaq Global Artificial Intelligence and Big Data Total Net Return Index (USD)"}
		],
		"values": [
			[
				1611187200000,
				[
					[30.3211, 30.3211, 0],
					[15160.57, 15160.57, 0]
				]
			],
			[
				1611273600000,
				[
					[30.2338, 30.2338, 0],
					[15116.92, 15116.92, 0]
				]
			]
		]
	}`

	currency, err := ParseNavCurrency(jsonData)
	if err != nil {
		t.Fatalf("ParseNavCurrency failed: %v", err)
	}

	navs, err := ParseNavHistory(jsonData, currency)
	if err != nil {
		t.Fatalf("ParseNavHistory failed: %v", err)
	}

	if len(navs) != 2 {
		t.Fatalf("expected 2 nav points, got %d", len(navs))
	}

	if navs[0].Currency != "USD" {
		t.Errorf("expected currency USD, got %s", navs[0].Currency)
	}

	expectedDate := time.UnixMilli(1611187200000)
	if !navs[0].Date.Equal(expectedDate) {
		t.Errorf("expected date %s, got %s", expectedDate, navs[0].Date)
	}

	// 30.3211 * 10^6 = 30321100 (matches parsers.go: MustNew(int64(navVal*1e6), 6))
	expectedNav := decimal.MustNew(30321100, 6)
	if !navs[0].NAV.Equal(expectedNav) {
		t.Errorf("expected NAV %s, got %s", expectedNav.String(), navs[0].NAV.String())
	}
}
