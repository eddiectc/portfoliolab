package symbol

import (
	"encoding/json"
	"testing"
)

func TestRiskMeasures_JSONSerialization(t *testing.T) {
	cases := []struct {
		name string
		rm   *RiskMeasures
	}{
		{
			name: "typical values",
			rm: &RiskMeasures{
				Volatility:    12.5,
				SharpeRatio:   0.85,
				InfoRatio:     0.42,
				Beta:          1.15,
				Correlation:   0.92,
				TrackingError: 3.7,
			},
		},
		{
			name: "zero values",
			rm: &RiskMeasures{},
		},
		{
			name: "negative ratios",
			rm: &RiskMeasures{
				Volatility:    25.0,
				SharpeRatio:   -0.5,
				InfoRatio:     -0.3,
				Beta:          0.8,
				Correlation:   -0.1,
				TrackingError: 8.5,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.rm)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded RiskMeasures
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if decoded.Volatility != tc.rm.Volatility {
				t.Errorf("Volatility: got %f, want %f", decoded.Volatility, tc.rm.Volatility)
			}
			if decoded.SharpeRatio != tc.rm.SharpeRatio {
				t.Errorf("SharpeRatio: got %f, want %f", decoded.SharpeRatio, tc.rm.SharpeRatio)
			}
			if decoded.InfoRatio != tc.rm.InfoRatio {
				t.Errorf("InfoRatio: got %f, want %f", decoded.InfoRatio, tc.rm.InfoRatio)
			}
			if decoded.Beta != tc.rm.Beta {
				t.Errorf("Beta: got %f, want %f", decoded.Beta, tc.rm.Beta)
			}
			if decoded.Correlation != tc.rm.Correlation {
				t.Errorf("Correlation: got %f, want %f", decoded.Correlation, tc.rm.Correlation)
			}
			if decoded.TrackingError != tc.rm.TrackingError {
				t.Errorf("TrackingError: got %f, want %f", decoded.TrackingError, tc.rm.TrackingError)
			}
		})
	}
}

func TestAssetClassEntry_JSONSerialization(t *testing.T) {
	cases := []struct {
		name    string
		entries []AssetClassEntry
	}{
		{
			name: "mixed positive and negative",
			entries: []AssetClassEntry{
				{AssetClass: "Equities", Percent: 20.3},
				{AssetClass: "Bonds", Percent: -25.7},
				{AssetClass: "Gold", Percent: 15.4},
			},
		},
		{
			name:    "empty slice",
			entries: []AssetClassEntry{},
		},
		{
			name: "zero percent",
			entries: []AssetClassEntry{
				{AssetClass: "Cash", Percent: 0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.entries)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded []AssetClassEntry
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if len(decoded) != len(tc.entries) {
				t.Fatalf("length: got %d, want %d", len(decoded), len(tc.entries))
			}

			for i, want := range tc.entries {
				if decoded[i].AssetClass != want.AssetClass {
					t.Errorf("[%d] AssetClass: got %q, want %q", i, decoded[i].AssetClass, want.AssetClass)
				}
				if decoded[i].Percent != want.Percent {
					t.Errorf("[%d] Percent: got %f, want %f", i, decoded[i].Percent, want.Percent)
				}
			}
		})
	}
}

func TestRegionDerivativeEntry_JSONSerialization(t *testing.T) {
	cases := []struct {
		name    string
		entries []RegionDerivativeEntry
	}{
		{
			name: "typical regional breakdown",
			entries: []RegionDerivativeEntry{
				{Region: "North America", Percent: 68.3},
				{Region: "Europe", Percent: -5.0},
			},
		},
		{
			name:    "empty slice",
			entries: []RegionDerivativeEntry{},
		},
		{
			name: "all negative",
			entries: []RegionDerivativeEntry{
				{Region: "North America", Percent: -40.0},
				{Region: "Europe", Percent: -35.0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.entries)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded []RegionDerivativeEntry
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if len(decoded) != len(tc.entries) {
				t.Fatalf("length: got %d, want %d", len(decoded), len(tc.entries))
			}

			for i, want := range tc.entries {
				if decoded[i].Region != want.Region {
					t.Errorf("[%d] Region: got %q, want %q", i, decoded[i].Region, want.Region)
				}
				if decoded[i].Percent != want.Percent {
					t.Errorf("[%d] Percent: got %f, want %f", i, decoded[i].Percent, want.Percent)
				}
			}
		})
	}
}

func TestCurrencyDerivativeEntry_JSONSerialization(t *testing.T) {
	cases := []struct {
		name    string
		entries []CurrencyDerivativeEntry
	}{
		{
			name: "typical currency breakdown",
			entries: []CurrencyDerivativeEntry{
				{Currency: "USD", Percent: 4.8},
				{Currency: "JPY", Percent: -89.4},
			},
		},
		{
			name:    "empty slice",
			entries: []CurrencyDerivativeEntry{},
		},
		{
			name: "single currency 100%",
			entries: []CurrencyDerivativeEntry{
				{Currency: "USD", Percent: 100.0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.entries)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded []CurrencyDerivativeEntry
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if len(decoded) != len(tc.entries) {
				t.Fatalf("length: got %d, want %d", len(decoded), len(tc.entries))
			}

			for i, want := range tc.entries {
				if decoded[i].Currency != want.Currency {
					t.Errorf("[%d] Currency: got %q, want %q", i, decoded[i].Currency, want.Currency)
				}
				if decoded[i].Percent != want.Percent {
					t.Errorf("[%d] Percent: got %f, want %f", i, decoded[i].Percent, want.Percent)
				}
			}
		})
	}
}

func TestSymbolDetails_NewFields_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		details *SymbolDetails
		check   func(t *testing.T, decoded SymbolDetails)
	}{
		{
			name: "all new fields populated",
			details: &SymbolDetails{
				InternalSymbol: "TEST",
				ShortName:      "Test Fund",
				RiskMeasures: &RiskMeasures{
					Volatility:    12.5,
					SharpeRatio:   0.85,
					InfoRatio:     0.42,
					Beta:          1.15,
					Correlation:   0.92,
					TrackingError: 3.7,
				},
				AssetClassAllocation: []AssetClassEntry{
					{AssetClass: "Equities", Percent: 20.3},
					{AssetClass: "Bonds", Percent: -25.7},
				},
				EquityDerivativesByRegion: []RegionDerivativeEntry{
					{Region: "North America", Percent: 68.3},
				},
				CurrencyDerivativesAllocation: []CurrencyDerivativeEntry{
					{Currency: "USD", Percent: 4.8},
				},
			},
			check: func(t *testing.T, decoded SymbolDetails) {
				if decoded.RiskMeasures == nil {
					t.Fatal("RiskMeasures is nil after round-trip")
				}
				if decoded.RiskMeasures.Volatility != 12.5 {
					t.Errorf("RiskMeasures.Volatility: got %f, want 12.5", decoded.RiskMeasures.Volatility)
				}
				if len(decoded.AssetClassAllocation) != 2 {
					t.Errorf("AssetClassAllocation length: got %d, want 2", len(decoded.AssetClassAllocation))
				}
				if len(decoded.EquityDerivativesByRegion) != 1 {
					t.Errorf("EquityDerivativesByRegion length: got %d, want 1", len(decoded.EquityDerivativesByRegion))
				}
				if len(decoded.CurrencyDerivativesAllocation) != 1 {
					t.Errorf("CurrencyDerivativesAllocation length: got %d, want 1", len(decoded.CurrencyDerivativesAllocation))
				}
			},
		},
		{
			name: "nil new fields",
			details: &SymbolDetails{
				InternalSymbol: "TEST",
				ShortName:      "Test Fund",
			},
			check: func(t *testing.T, decoded SymbolDetails) {
				if decoded.RiskMeasures != nil {
					t.Error("RiskMeasures should be nil")
				}
				if decoded.AssetClassAllocation != nil {
					t.Error("AssetClassAllocation should be nil")
				}
				if decoded.EquityDerivativesByRegion != nil {
					t.Error("EquityDerivativesByRegion should be nil")
				}
				if decoded.CurrencyDerivativesAllocation != nil {
					t.Error("CurrencyDerivativesAllocation should be nil")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.details)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded SymbolDetails
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			tc.check(t, decoded)
		})
	}
}
