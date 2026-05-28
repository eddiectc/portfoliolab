package dws

import (
	"testing"
)

func TestParseHoldings_ActualAPIFormat(t *testing.T) {
	// This JSON matches the actual structure discovered via web_fetch
	data := `{
	  "tables": [
	    {
	      "values": [
	        {
	          "header": { "value": "US67066G1040" },
	          "column_0": { "value": "NVIDIA CORP" },
	          "column_1": { "value": "8.315%" },
	          "column_3": { "value": "United States" },
	          "column_4": { "value": "Technology" }
	        },
	        {
	          "header": { "value": "US0378331005" },
	          "column_0": { "value": "APPLE INC" },
	          "column_1": { "value": "7.345%" },
	          "column_3": { "value": "United States" },
	          "column_4": { "value": "Technology" }
	        }
	      ]
	    }
	  ]
	}`

	holdings, countries, sectors, err := ParseHoldings(data)
	if err != nil {
		t.Fatalf("ParseHoldings failed: %v", err)
	}

	if len(holdings) != 2 {
		t.Errorf("expected 2 holdings, got %d", len(holdings))
	}

	// Verify first holding
	if holdings[0].Symbol != "US67066G1040" || holdings[0].Name != "NVIDIA CORP" || holdings[0].Percent != 8.315 {
		t.Errorf("holding 0 mismatch: %+v", holdings[0])
	}

	// Verify aggregations
	var usaWeight float64
	for _, c := range countries {
		if c.Country == "United States" {
			usaWeight = c.Percent
		}
	}
	expectedUSA := 8.315 + 7.345
	if usaWeight != expectedUSA {
		t.Errorf("expected USA weight %f, got %f", expectedUSA, usaWeight)
	}

	var techWeight float64
	for _, s := range sectors {
		if s.Sector == "Technology" {
			techWeight = s.Percent
		}
	}
	if techWeight != expectedUSA {
		t.Errorf("expected Tech weight %f, got %f", expectedUSA, techWeight)
	}
}

func TestParseHoldings_EmptyTables(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"no tables", `{"tables": []}`},
		{"empty values", `{"tables": [{"values": []}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := ParseHoldings(tt.data)
			if err == nil {
				t.Error("expected error for empty holdings, got nil")
			}
		})
	}
}
