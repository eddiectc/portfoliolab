package comparison

import (
	"testing"

	"github.com/eddiectc/portfoliolab/internal/types/symbol"
)

// --- ComputeSectorAllocationForHoldings ---

func TestComputeSectorAllocationForHoldings_ETFWithSectorWeightings(t *testing.T) {
	// ETF at 60% weight with sectors: Technology 40%, Healthcare 30%, Financials 30%
	// Expected: Technology = 0.6*40/100 = 0.24, Healthcare = 0.18, Financials = 0.18
	holdings := []PortfolioHolding{
		getETFWithSectorWeightings("VWRP", 0.6, []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 40},
			{Sector: "Healthcare", Percent: 30},
			{Sector: "Financials", Percent: 30},
		}),
	}

	result := ComputeSectorAllocationForHoldings(holdings)

	if len(result.Breakdown) != 3 {
		t.Fatalf("Breakdown len = %d, want 3", len(result.Breakdown))
	}

	if !floatEq(result.Breakdown["Technology"], 0.24, 0.01) {
		t.Errorf("Technology = %.4f, want 0.24", result.Breakdown["Technology"])
	}
	if !floatEq(result.Breakdown["Healthcare"], 0.18, 0.01) {
		t.Errorf("Healthcare = %.4f, want 0.18", result.Breakdown["Healthcare"])
	}
	if !floatEq(result.Breakdown["Financials"], 0.18, 0.01) {
		t.Errorf("Financials = %.4f, want 0.18", result.Breakdown["Financials"])
	}

	if !floatEq(result.UnknownWeightPct, 0.0, 0.01) {
		t.Errorf("UnknownWeightPct = %.4f, want 0.0", result.UnknownWeightPct)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", result.Warnings)
	}
}

func TestComputeSectorAllocationForHoldings_StockWithPrimarySector(t *testing.T) {
	// Stock at 50% weight, sector = Technology
	// Expected: Technology = 0.50
	holdings := []PortfolioHolding{
		getStockWithSector("AAPL", 0.5, "Technology"),
	}

	result := ComputeSectorAllocationForHoldings(holdings)

	if len(result.Breakdown) != 1 {
		t.Fatalf("Breakdown len = %d, want 1", len(result.Breakdown))
	}

	if !floatEq(result.Breakdown["Technology"], 0.50, 0.01) {
		t.Errorf("Technology = %.4f, want 0.50", result.Breakdown["Technology"])
	}
}

func TestComputeSectorAllocationForHoldings_MixedETFAndStock(t *testing.T) {
	// ETF at 40%: Technology 50%, Healthcare 50%
	// Stock at 60%: Technology
	// Expected: Technology = 0.4*50/100 + 0.6 = 0.2+0.6 = 0.8, Healthcare = 0.4*50/100 = 0.2
	holdings := []PortfolioHolding{
		getETFWithSectorWeightings("VWRP", 0.4, []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 50},
			{Sector: "Healthcare", Percent: 50},
		}),
		getStockWithSector("AAPL", 0.6, "Technology"),
	}

	result := ComputeSectorAllocationForHoldings(holdings)

	if len(result.Breakdown) != 2 {
		t.Fatalf("Breakdown len = %d, want 2", len(result.Breakdown))
	}

	if !floatEq(result.Breakdown["Technology"], 0.80, 0.01) {
		t.Errorf("Technology = %.4f, want 0.80", result.Breakdown["Technology"])
	}
	if !floatEq(result.Breakdown["Healthcare"], 0.20, 0.01) {
		t.Errorf("Healthcare = %.4f, want 0.20", result.Breakdown["Healthcare"])
	}
}

func TestComputeSectorAllocationForHoldings_MissingSectorData(t *testing.T) {
	// ETF with no sector weightings + stock with no sector
	holdings := []PortfolioHolding{
		getETFWithSectorWeightings("UNKNOWN_ETF", 0.3, nil),
		getStockWithSector("NOSECTOR", 0.7, ""),
	}

	result := ComputeSectorAllocationForHoldings(holdings)

	// Both should go to unknown
	if !floatEq(result.UnknownWeightPct, 1.0, 0.01) {
		t.Errorf("UnknownWeightPct = %.4f, want 1.0", result.UnknownWeightPct)
	}

	if len(result.Warnings) != 2 {
		t.Fatalf("Warnings len = %d, want 2", len(result.Warnings))
	}

	if len(result.MissingSymbols) != 2 {
		t.Fatalf("MissingSymbols len = %d, want 2", len(result.MissingSymbols))
	}

	if result.MissingSymbols[0] != "UNKNOWN_ETF" {
		t.Errorf("MissingSymbols[0] = %q, want %q", result.MissingSymbols[0], "UNKNOWN_ETF")
	}
	if result.MissingSymbols[1] != "NOSECTOR" {
		t.Errorf("MissingSymbols[1] = %q, want %q", result.MissingSymbols[1], "NOSECTOR")
	}
}

func TestComputeSectorAllocationForHoldings_EmptyHoldings(t *testing.T) {
	result := ComputeSectorAllocationForHoldings(nil)

	if len(result.Breakdown) != 0 {
		t.Errorf("Breakdown len = %d, want 0", len(result.Breakdown))
	}
	if result.Message == "" {
		t.Error("Message should not be empty for empty holdings")
	}
}

// --- ComputeCountryAllocationForHoldings ---

func TestComputeCountryAllocationForHoldings_ETFWithGeographicAllocations(t *testing.T) {
	// ETF at 50% weight with countries: USA 60%, UK 20%, Germany 20%
	// Expected: USA = 0.5*60/100 = 0.30, UK = 0.10, Germany = 0.10
	holdings := []PortfolioHolding{
		getETFWithGeographicAllocations("VWRP", 0.5, []symbol.GeographicAllocation{
			{Country: "United States", Percent: 60},
			{Country: "United Kingdom", Percent: 20},
			{Country: "Germany", Percent: 20},
		}),
	}

	result := ComputeCountryAllocationForHoldings(holdings)

	if len(result.Breakdown) != 3 {
		t.Fatalf("Breakdown len = %d, want 3", len(result.Breakdown))
	}

	if !floatEq(result.Breakdown["United States"], 0.30, 0.01) {
		t.Errorf("United States = %.4f, want 0.30", result.Breakdown["United States"])
	}
	if !floatEq(result.Breakdown["United Kingdom"], 0.10, 0.01) {
		t.Errorf("United Kingdom = %.4f, want 0.10", result.Breakdown["United Kingdom"])
	}
	if !floatEq(result.Breakdown["Germany"], 0.10, 0.01) {
		t.Errorf("Germany = %.4f, want 0.10", result.Breakdown["Germany"])
	}
}

func TestComputeCountryAllocationForHoldings_StockWithCountry(t *testing.T) {
	// Stock at 50% weight, single 100% country allocation
	// Expected: USA = 0.5*100/100 = 0.50
	holdings := []PortfolioHolding{
		getStockWithCountry("AAPL", 0.5, "United States"),
	}

	result := ComputeCountryAllocationForHoldings(holdings)

	if len(result.Breakdown) != 1 {
		t.Fatalf("Breakdown len = %d, want 1", len(result.Breakdown))
	}

	if !floatEq(result.Breakdown["United States"], 0.50, 0.01) {
		t.Errorf("United States = %.4f, want 0.50", result.Breakdown["United States"])
	}
}

func TestComputeCountryAllocationForHoldings_MixedPortfolio(t *testing.T) {
	// ETF at 40%: USA 70%, Japan 30%
	// Stock at 60%: USA (100%)
	// Expected: USA = 0.4*70/100 + 0.6*100/100 = 0.28+0.6 = 0.88, Japan = 0.4*30/100 = 0.12
	holdings := []PortfolioHolding{
		getETFWithGeographicAllocations("VWRP", 0.4, []symbol.GeographicAllocation{
			{Country: "United States", Percent: 70},
			{Country: "Japan", Percent: 30},
		}),
		getStockWithCountry("AAPL", 0.6, "United States"),
	}

	result := ComputeCountryAllocationForHoldings(holdings)

	if !floatEq(result.Breakdown["United States"], 0.88, 0.01) {
		t.Errorf("United States = %.4f, want 0.88", result.Breakdown["United States"])
	}
	if !floatEq(result.Breakdown["Japan"], 0.12, 0.01) {
		t.Errorf("Japan = %.4f, want 0.12", result.Breakdown["Japan"])
	}
}

func TestComputeCountryAllocationForHoldings_MissingGeographicData(t *testing.T) {
	holdings := []PortfolioHolding{
		getETFWithGeographicAllocations("UNKNOWN_ETF", 0.5, nil),
		getStockWithCountry("NOGEO", 0.5, ""), // empty country name but has allocation entry
	}
	// Override: remove geographic allocations to simulate missing data
	holdings[1].GeographicAllocations = nil

	result := ComputeCountryAllocationForHoldings(holdings)

	if !floatEq(result.UnknownWeightPct, 1.0, 0.01) {
		t.Errorf("UnknownWeightPct = %.4f, want 1.0", result.UnknownWeightPct)
	}

	if len(result.MissingSymbols) != 2 {
		t.Fatalf("MissingSymbols len = %d, want 2", len(result.MissingSymbols))
	}
}

func TestComputeCountryAllocationForHoldings_EmptyHoldings(t *testing.T) {
	result := ComputeCountryAllocationForHoldings(nil)

	if len(result.Breakdown) != 0 {
		t.Errorf("Breakdown len = %d, want 0", len(result.Breakdown))
	}
	if result.Message == "" {
		t.Error("Message should not be empty for empty holdings")
	}
}

// --- SectorAllocationSorted ---

func TestSectorAllocationSorted(t *testing.T) {
	result := &SectorAllocationResult{
		Breakdown: map[string]float64{
			"Technology": 0.40,
			"Healthcare": 0.30,
			"Financials": 0.20,
			"Energy":     0.10,
		},
	}

	entries := SectorAllocationSorted(result)

	if len(entries) != 4 {
		t.Fatalf("len = %d, want 4", len(entries))
	}

	// Should be sorted by weight descending.
	if entries[0].Category != "Technology" || !floatEq(entries[0].Weight, 0.40, 0.01) {
		t.Errorf("[0] = %+v, want Technology 0.40", entries[0])
	}
	if entries[3].Category != "Energy" || !floatEq(entries[3].Weight, 0.10, 0.01) {
		t.Errorf("[3] = %+v, want Energy 0.10", entries[3])
	}
}

// --- normalizeSector ---

func TestNormalizeSector(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"technology", "Technology", "Technology"},
		{"healthcare", "Healthcare", "Healthcare"},
		{"financials", "Financials", "Financials"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSector(tt.in)
			if got != tt.want {
				t.Errorf("normalizeSector(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
