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
			rm:   &RiskMeasures{},
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

func TestTopHolding_JSONSerialization(t *testing.T) {
	couponRate := 3.5
	finalMaturity := "2032-06-15"

	cases := []struct {
		name    string
		holding TopHolding
		check   func(t *testing.T, decoded TopHolding)
	}{
		{
			name: "equity holding with security type",
			holding: TopHolding{
				Symbol:       "AAPL",
				Name:         "Apple Inc.",
				Percent:      1.399,
				SecurityType: "Common Stock",
				AsOfDate:     "2026-03-31",
			},
			check: func(t *testing.T, decoded TopHolding) {
				if decoded.SecurityType != "Common Stock" {
					t.Errorf("SecurityType: got %q, want %q", decoded.SecurityType, "Common Stock")
				}
				if decoded.AsOfDate != "2026-03-31" {
					t.Errorf("AsOfDate: got %q, want %q", decoded.AsOfDate, "2026-03-31")
				}
				if decoded.CouponRate != nil {
					t.Errorf("CouponRate: got %v, want nil", decoded.CouponRate)
				}
				if decoded.FinalMaturity != nil {
					t.Errorf("FinalMaturity: got %v, want nil", decoded.FinalMaturity)
				}
			},
		},
		{
			name: "bond holding with coupon and maturity",
			holding: TopHolding{
				Symbol:        "US912828Z123",
				Name:          "US Treasury Note",
				Percent:       0.5,
				SecurityType:  "Government Bond",
				CouponRate:    &couponRate,
				FinalMaturity: &finalMaturity,
				AsOfDate:      "2026-03-31",
			},
			check: func(t *testing.T, decoded TopHolding) {
				if decoded.CouponRate == nil {
					t.Fatal("CouponRate is nil")
				} else if *decoded.CouponRate != 3.5 {
					t.Errorf("CouponRate: got %v, want 3.5", *decoded.CouponRate)
				}
				if decoded.FinalMaturity == nil {
					t.Fatal("FinalMaturity is nil")
				} else if *decoded.FinalMaturity != "2032-06-15" {
					t.Errorf("FinalMaturity: got %q, want %q", *decoded.FinalMaturity, "2032-06-15")
				}
			},
		},
		{
			name: "minimal holding (backward compat)",
			holding: TopHolding{
				Symbol:  "TEST",
				Name:    "Test",
				Percent: 0.1,
			},
			check: func(t *testing.T, decoded TopHolding) {
				if decoded.SecurityType != "" {
					t.Errorf("SecurityType: got %q, want empty", decoded.SecurityType)
				}
				if decoded.CouponRate != nil {
					t.Error("CouponRate: want nil")
				}
				if decoded.FinalMaturity != nil {
					t.Error("FinalMaturity: want nil")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.holding)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var decoded TopHolding
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			tc.check(t, decoded)
		})
	}
}

func TestSectorWeighting_JSONSerialization(t *testing.T) {
	cases := []struct {
		name  string
		sw    SectorWeighting
		check func(t *testing.T, decoded SectorWeighting)
	}{
		{
			name: "with date",
			sw: SectorWeighting{
				Sector:  "Technology",
				Percent: 25.5,
				Date:    "2026-03-31",
			},
			check: func(t *testing.T, decoded SectorWeighting) {
				if decoded.Date != "2026-03-31" {
					t.Errorf("Date: got %q, want %q", decoded.Date, "2026-03-31")
				}
			},
		},
		{
			name: "without date (backward compat)",
			sw: SectorWeighting{
				Sector:  "Financials",
				Percent: 15.0,
			},
			check: func(t *testing.T, decoded SectorWeighting) {
				if decoded.Date != "" {
					t.Errorf("Date: got %q, want empty", decoded.Date)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.sw)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var decoded SectorWeighting
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			tc.check(t, decoded)
		})
	}
}

func TestGeographicAllocation_JSONSerialization(t *testing.T) {
	cases := []struct {
		name  string
		ga    GeographicAllocation
		check func(t *testing.T, decoded GeographicAllocation)
	}{
		{
			name: "with region and date",
			ga: GeographicAllocation{
				Country:    "United States",
				Percent:    60.5,
				RegionName: "Developed Markets",
				RegionCode: "DM",
				Date:       "2026-03-31",
			},
			check: func(t *testing.T, decoded GeographicAllocation) {
				if decoded.RegionName != "Developed Markets" {
					t.Errorf("RegionName: got %q, want %q", decoded.RegionName, "Developed Markets")
				}
				if decoded.RegionCode != "DM" {
					t.Errorf("RegionCode: got %q, want %q", decoded.RegionCode, "DM")
				}
				if decoded.Date != "2026-03-31" {
					t.Errorf("Date: got %q, want %q", decoded.Date, "2026-03-31")
				}
			},
		},
		{
			name: "without region or date (backward compat)",
			ga: GeographicAllocation{
				Country: "Japan",
				Percent: 5.0,
			},
			check: func(t *testing.T, decoded GeographicAllocation) {
				if decoded.RegionName != "" {
					t.Errorf("RegionName: got %q, want empty", decoded.RegionName)
				}
				if decoded.RegionCode != "" {
					t.Errorf("RegionCode: got %q, want empty", decoded.RegionCode)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.ga)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var decoded GeographicAllocation
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			tc.check(t, decoded)
		})
	}
}

func TestEquityValuation_JSONSerialization(t *testing.T) {
	cases := []struct {
		name  string
		ev    EquityValuation
		check func(t *testing.T, decoded EquityValuation)
	}{
		{
			name: "with new fields",
			ev: EquityValuation{
				PriceToEarnings:  18.5,
				PriceToBook:      3.2,
				MedianMarketCap:  500.0,
				ForwardROE:       15.3,
				ForwardEPSGrowth: 8.7,
				RevenueRatio:     1.05,
			},
			check: func(t *testing.T, decoded EquityValuation) {
				if decoded.MedianMarketCap != 500.0 {
					t.Errorf("MedianMarketCap: got %f, want 500.0", decoded.MedianMarketCap)
				}
				if decoded.ForwardROE != 15.3 {
					t.Errorf("ForwardROE: got %f, want 15.3", decoded.ForwardROE)
				}
				if decoded.ForwardEPSGrowth != 8.7 {
					t.Errorf("ForwardEPSGrowth: got %f, want 8.7", decoded.ForwardEPSGrowth)
				}
				if decoded.RevenueRatio != 1.05 {
					t.Errorf("RevenueRatio: got %f, want 1.05", decoded.RevenueRatio)
				}
			},
		},
		{
			name: "without new fields (backward compat)",
			ev: EquityValuation{
				PriceToEarnings: 15.0,
				PriceToBook:     2.5,
			},
			check: func(t *testing.T, decoded EquityValuation) {
				if decoded.MedianMarketCap != 0 {
					t.Errorf("MedianMarketCap: got %f, want 0", decoded.MedianMarketCap)
				}
				if decoded.ForwardROE != 0 {
					t.Errorf("ForwardROE: got %f, want 0", decoded.ForwardROE)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.ev)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var decoded EquityValuation
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			tc.check(t, decoded)
		})
	}
}

func TestBondCharacteristics_JSONSerialization(t *testing.T) {
	cases := []struct {
		name  string
		bc    BondCharacteristics
		check func(t *testing.T, decoded BondCharacteristics)
	}{
		{
			name: "typical bond characteristics",
			bc: BondCharacteristics{
				AverageCoupon:   3.25,
				AverageMaturity: 7.5,
				AverageQuality:  7.8,
				AverageDuration: 6.2,
			},
			check: func(t *testing.T, decoded BondCharacteristics) {
				if decoded.AverageCoupon != 3.25 {
					t.Errorf("AverageCoupon: got %f, want 3.25", decoded.AverageCoupon)
				}
				if decoded.AverageMaturity != 7.5 {
					t.Errorf("AverageMaturity: got %f, want 7.5", decoded.AverageMaturity)
				}
				if decoded.AverageQuality != 7.8 {
					t.Errorf("AverageQuality: got %f, want 7.8", decoded.AverageQuality)
				}
				if decoded.AverageDuration != 6.2 {
					t.Errorf("AverageDuration: got %f, want 6.2", decoded.AverageDuration)
				}
			},
		},
		{
			name: "zero values",
			bc:   BondCharacteristics{},
			check: func(t *testing.T, decoded BondCharacteristics) {
				if decoded.AverageCoupon != 0 {
					t.Errorf("all fields should be zero")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.bc)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var decoded BondCharacteristics
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			tc.check(t, decoded)
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
