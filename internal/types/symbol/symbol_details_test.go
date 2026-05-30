package symbol

import (
	"encoding/json"
	"testing"
)

func TestRiskMeasures_JSONSerialization(t *testing.T) {
	rm := &RiskMeasures{
		Volatility:    12.5,
		SharpeRatio:   0.85,
		InfoRatio:     0.42,
		Beta:          1.15,
		Correlation:   0.92,
		TrackingError: 3.7,
	}

	data, err := json.Marshal(rm)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded RiskMeasures
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Volatility != rm.Volatility {
		t.Errorf("Volatility: got %f, want %f", decoded.Volatility, rm.Volatility)
	}
	if decoded.SharpeRatio != rm.SharpeRatio {
		t.Errorf("SharpeRatio: got %f, want %f", decoded.SharpeRatio, rm.SharpeRatio)
	}
	if decoded.InfoRatio != rm.InfoRatio {
		t.Errorf("InfoRatio: got %f, want %f", decoded.InfoRatio, rm.InfoRatio)
	}
	if decoded.Beta != rm.Beta {
		t.Errorf("Beta: got %f, want %f", decoded.Beta, rm.Beta)
	}
	if decoded.Correlation != rm.Correlation {
		t.Errorf("Correlation: got %f, want %f", decoded.Correlation, rm.Correlation)
	}
	if decoded.TrackingError != rm.TrackingError {
		t.Errorf("TrackingError: got %f, want %f", decoded.TrackingError, rm.TrackingError)
	}
}

func TestAssetClassEntry_JSONSerialization(t *testing.T) {
	entries := []AssetClassEntry{
		{AssetClass: "Equities", Percent: 20.3},
		{AssetClass: "Bonds", Percent: -25.7},
		{AssetClass: "Gold", Percent: 15.4},
	}

	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded []AssetClassEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded) != len(entries) {
		t.Fatalf("length: got %d, want %d", len(decoded), len(entries))
	}

	for i, want := range entries {
		if decoded[i].AssetClass != want.AssetClass {
			t.Errorf("[%d] AssetClass: got %q, want %q", i, decoded[i].AssetClass, want.AssetClass)
		}
		if decoded[i].Percent != want.Percent {
			t.Errorf("[%d] Percent: got %f, want %f", i, decoded[i].Percent, want.Percent)
		}
	}
}

func TestRegionDerivativeEntry_JSONSerialization(t *testing.T) {
	entries := []RegionDerivativeEntry{
		{Region: "North America", Percent: 68.3},
		{Region: "Europe", Percent: -5.0},
	}

	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded []RegionDerivativeEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded) != len(entries) {
		t.Fatalf("length: got %d, want %d", len(decoded), len(entries))
	}

	for i, want := range entries {
		if decoded[i].Region != want.Region {
			t.Errorf("[%d] Region: got %q, want %q", i, decoded[i].Region, want.Region)
		}
		if decoded[i].Percent != want.Percent {
			t.Errorf("[%d] Percent: got %f, want %f", i, decoded[i].Percent, want.Percent)
		}
	}
}

func TestCurrencyDerivativeEntry_JSONSerialization(t *testing.T) {
	entries := []CurrencyDerivativeEntry{
		{Currency: "USD", Percent: 4.8},
		{Currency: "JPY", Percent: -89.4},
	}

	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded []CurrencyDerivativeEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded) != len(entries) {
		t.Fatalf("length: got %d, want %d", len(decoded), len(entries))
	}

	for i, want := range entries {
		if decoded[i].Currency != want.Currency {
			t.Errorf("[%d] Currency: got %q, want %q", i, decoded[i].Currency, want.Currency)
		}
		if decoded[i].Percent != want.Percent {
			t.Errorf("[%d] Percent: got %f, want %f", i, decoded[i].Percent, want.Percent)
		}
	}
}

func TestSymbolDetails_NewFields_JSONRoundTrip(t *testing.T) {
	details := &SymbolDetails{
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
	}

	data, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SymbolDetails
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.RiskMeasures == nil {
		t.Fatal("RiskMeasures is nil after round-trip")
	}
	if decoded.RiskMeasures.Volatility != details.RiskMeasures.Volatility {
		t.Errorf("RiskMeasures.Volatility: got %f, want %f", decoded.RiskMeasures.Volatility, details.RiskMeasures.Volatility)
	}
	if len(decoded.AssetClassAllocation) != len(details.AssetClassAllocation) {
		t.Errorf("AssetClassAllocation length: got %d, want %d", len(decoded.AssetClassAllocation), len(details.AssetClassAllocation))
	}
	if len(decoded.EquityDerivativesByRegion) != len(details.EquityDerivativesByRegion) {
		t.Errorf("EquityDerivativesByRegion length: got %d, want %d", len(decoded.EquityDerivativesByRegion), len(details.EquityDerivativesByRegion))
	}
	if len(decoded.CurrencyDerivativesAllocation) != len(details.CurrencyDerivativesAllocation) {
		t.Errorf("CurrencyDerivativesAllocation length: got %d, want %d", len(decoded.CurrencyDerivativesAllocation), len(details.CurrencyDerivativesAllocation))
	}
}
