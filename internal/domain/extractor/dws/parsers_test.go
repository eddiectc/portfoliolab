package dws

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func loadTestData(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read test data %s: %v", path, err)
	}
	return string(data)
}

func TestParseFundInfo(t *testing.T) {
	settingsData := loadTestData(t, "pdpSettings.json")
	metaData := loadTestData(t, "pdpMetaTags.json")
	symbol := "DE000A2X47S0"
	
	info, err := ParseFundInfo(settingsData, metaData, symbol)
	if err != nil {
		t.Fatalf("ParseFundInfo failed: %v", err)
	}

	if info.Symbol != symbol {
		t.Errorf("expected symbol %s, got %s", symbol, info.Symbol)
	}
	if info.Name != "Xtrackers Artificial Intelligence UCITS ETF" {
		t.Errorf("expected name 'Xtrackers Artificial Intelligence UCITS ETF', got %s", info.Name)
	}
}

func TestParseFundProfile(t *testing.T) {
	settingsData := loadTestData(t, "pdpSettings.json")
	metaData := loadTestData(t, "pdpMetaTags.json")
	
	profile, err := ParseFundProfile(settingsData, metaData)
	if err != nil {
		t.Fatalf("ParseFundProfile failed: %v", err)
	}

	if profile.Family != "Xtrackers" {
		t.Errorf("expected family Xtrackers, got %s", profile.Family)
	}
	if profile.AnnualExpenseRatio != 0.0020 {
		t.Errorf("expected TER 0.0020, got %f", profile.AnnualExpenseRatio)
	}
	if profile.BaseCurrency != "GBP" {
		t.Errorf("expected BaseCurrency GBP, got %s", profile.BaseCurrency)
	}
}

func TestParseHoldings(t *testing.T) {
	data := loadTestData(t, "holdings.json")
	
	holdings, countries, sectors, err := ParseHoldings(data)
	if err != nil {
		t.Fatalf("ParseHoldings failed: %v", err)
	}

	if len(holdings) != 5 {
		t.Errorf("expected 5 holdings, got %d", len(holdings))
	}

	// Verify one specific holding
	found := false
	for _, h := range holdings {
		if h.ISIN == "US0378331005" && h.Percent == 7.5 {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find expected holding US0378331005 with 7.5%")
	}

	// Verify country aggregation
	var usaWeight float64
	for _, c := range countries {
		if c.Country == "USA" {
			usaWeight = c.Percent
		}
	}
	expectedUSA := 7.5 + 6.8 + 3.2
	if usaWeight != expectedUSA {
		t.Errorf("expected USA weight %f, got %f", expectedUSA, usaWeight)
	}

	// Verify sector aggregation
	var techWeight float64
	for _, s := range sectors {
		if s.Sector == "Technology" {
			techWeight = s.Percent
		}
	}
	expectedTech := 7.5 + 6.8
	if techWeight != expectedTech {
		t.Errorf("expected Tech weight %f, got %f", expectedTech, techWeight)
	}
}

func TestParseAsOfDate(t *testing.T) {
	tests := []struct {
		name     string
		dateStr  string
		expected time.Time
		wantErr  bool
	}{
		{
			name:     "RFC3339",
			dateStr:  "2026-05-27T00:00:00Z",
			expected: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "Slash format",
			dateStr:  "27/05/2026",
			expected: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "Dash format",
			dateStr:  "27-05-2026",
			expected: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "ISO format",
			dateStr:  "2026-05-27",
			expected: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "Invalid format",
			dateStr:  "May 27, 2026",
			expected: time.Time{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := fmt.Sprintf(`{"asOfDate": "%s"}`, tt.dateStr)
			asOf, err := ParseAsOfDate(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAsOfDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !asOf.Equal(tt.expected) {
				t.Errorf("expected date %v, got %v", tt.expected, asOf)
			}
		})
	}
}

func TestParseHoldings_Empty(t *testing.T) {
	data := `{"holdings": []}`
	_, _, _, err := ParseHoldings(data)
	if err == nil {
		t.Error("expected error for empty holdings, got nil")
	}
}
