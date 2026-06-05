package dws

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func TestParseNavHistory(t *testing.T) {
	jsonData := `{
		"asOfDate": "27/05/2026",
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

	navs, err := ParseNavHistory(jsonData)
	if err != nil {
		t.Fatalf("ParseNavHistory failed: %v", err)
	}

	if len(navs) != 2 {
		t.Fatalf("expected 2 nav points, got %d", len(navs))
	}

	expectedDate := time.UnixMilli(1611187200000)
	if !navs[0].Date.Equal(expectedDate) {
		t.Errorf("expected date %s, got %s", expectedDate, navs[0].Date)
	}

	expectedNav := decimal.MustNew(303211, 6) // 30.3211
	// Note: I used decimal.MustNew(int64(navVal*1000000), 6) in parsers.go
	// For 30.3211 * 1000000 = 30321100
	// Wait, 30.3211 * 1000000 is 30321100.
	// Let's check the actual implementation: decimal.MustNew(int64(navVal*1000000), 6)
	// 30.3211 * 1000000 = 30321100.
	// So expectedNav should be decimal.MustNew(30321100, 6)
	
	expectedNav = decimal.MustNew(30321100, 6)
	if !navs[0].NAV.Equal(expectedNav) {
		t.Errorf("expected NAV %s, got %s", expectedNav.String(), navs[0].NAV.String())
	}
}
