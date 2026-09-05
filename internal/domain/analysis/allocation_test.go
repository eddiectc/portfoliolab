package analysis

import (
	"testing"

	"github.com/eddiectc/portfoliolab/internal/types/symbol"
)

// --- ComputeSectorAllocation tests ---

func TestComputeSectorAllocation_HappyPath(t *testing.T) {
	positions := []PositionWithDetails{
		// ETF: 50% of portfolio, 40% tech + 30% finance + 30% other
		getETFWithSectorWeightings("VEA", 50, []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 40},
			{Sector: "Financial Services", Percent: 30},
			{Sector: "Healthcare", Percent: 30},
		}),
		// Stock: 30% of portfolio, Technology sector
		getStockWithSector("AAPL", 30, "Technology"),
		// Stock: 20% of portfolio, Healthcare sector
		getStockWithSector("JNJ", 20, "Healthcare"),
	}

	result := ComputeSectorAllocation(positions)

	// VEA contributes: 50 * 40/100 = 20% to Technology
	// VEA contributes: 50 * 30/100 = 15% to Financial Services
	// VEA contributes: 50 * 30/100 = 15% to Healthcare
	// AAPL contributes: 30% to Technology
	// JNJ contributes: 20% to Healthcare
	// Expected: Technology=50, Financial Services=15, Healthcare=35

	want := map[string]float64{
		"Technology":         50.0,
		"Financial Services": 15.0,
		"Healthcare":         35.0,
	}

	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if result.UnknownWeightPct != 0 {
		t.Errorf("unknown weight = %f, want 0", result.UnknownWeightPct)
	}
	if result.Message != "" {
		t.Errorf("unexpected message: %q", result.Message)
	}

	for sector, wantWeight := range want {
		got, ok := result.Breakdown[sector]
		if !ok {
			t.Errorf("missing sector %q in breakdown", sector)
			continue
		}
		if got != wantWeight {
			t.Errorf("%s weight = %f, want %f", sector, got, wantWeight)
		}
	}

	if len(result.Breakdown) != len(want) {
		t.Errorf("breakdown has %d entries, want %d: %v", len(result.Breakdown), len(want), result.Breakdown)
	}
}

func TestComputeSectorAllocation_ETFOnly(t *testing.T) {
	positions := []PositionWithDetails{
		getETFWithSectorWeightings("VTI", 60, []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 30},
			{Sector: "Healthcare", Percent: 20},
			{Sector: "Financial Services", Percent: 50},
		}),
		getETFWithSectorWeightings("VXUS", 40, []symbol.SectorWeighting{
			{Sector: "Financial Services", Percent: 25},
			{Sector: "Consumer Cyclical", Percent: 75},
		}),
	}

	result := ComputeSectorAllocation(positions)

	// VTI: 60*30/100=18 tech, 60*20/100=12 healthcare, 60*50/100=30 finance
	// VXUS: 40*25/100=10 finance, 40*75/100=30 consumer cyclical
	// Expected: Technology=18, Healthcare=12, Financial Services=40, Consumer Cyclical=30

	want := map[string]float64{
		"Technology":         18.0,
		"Healthcare":         12.0,
		"Financial Services": 40.0,
		"Consumer Cyclical":  30.0,
	}

	for sector, wantWeight := range want {
		got := result.Breakdown[sector]
		if got != wantWeight {
			t.Errorf("%s weight = %f, want %f", sector, got, wantWeight)
		}
	}
	if result.UnknownWeightPct != 0 {
		t.Errorf("unknown weight = %f, want 0", result.UnknownWeightPct)
	}
}

func TestComputeSectorAllocation_StockOnly(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithSector("AAPL", 40, "Technology"),
		getStockWithSector("JPM", 35, "Financial Services"),
		getStockWithSector("XOM", 25, "Energy"),
	}

	result := ComputeSectorAllocation(positions)

	want := map[string]float64{
		"Technology":         40.0,
		"Financial Services": 35.0,
		"Energy":             25.0,
	}

	for sector, wantWeight := range want {
		got := result.Breakdown[sector]
		if got != wantWeight {
			t.Errorf("%s weight = %f, want %f", sector, got, wantWeight)
		}
	}
	if result.UnknownWeightPct != 0 {
		t.Errorf("unknown weight = %f, want 0", result.UnknownWeightPct)
	}
}

func TestComputeSectorAllocation_PartialData(t *testing.T) {
	positions := []PositionWithDetails{
		// ETF with no sector data
		PositionWithDetails{
			Symbol:          "OBSCURE",
			PortfolioWeight: 30,
			SymbolDetails: &symbol.SymbolDetails{
				QuoteType: "ETF",
			},
		},
		// Stock with no sector
		PositionWithDetails{
			Symbol:          "UNKNOWN",
			PortfolioWeight: 20,
			SymbolDetails: &symbol.SymbolDetails{
				QuoteType: "EQUITY",
			},
		},
		// Stock with no details at all
		PositionWithDetails{
			Symbol:          "NODETAILS",
			PortfolioWeight: 10,
		},
		// Stock with sector
		getStockWithSector("AAPL", 40, "Technology"),
	}

	result := ComputeSectorAllocation(positions)

	if result.Breakdown["Technology"] != 40.0 {
		t.Errorf("Technology weight = %f, want 40", result.Breakdown["Technology"])
	}
	if result.UnknownWeightPct != 60.0 {
		t.Errorf("unknown weight = %f, want 60", result.UnknownWeightPct)
	}
	if len(result.Warnings) != 3 {
		t.Errorf("warnings = %d, want 3: %v", len(result.Warnings), result.Warnings)
	}
}

func TestComputeSectorAllocation_AllMissingData(t *testing.T) {
	positions := []PositionWithDetails{
		PositionWithDetails{
			Symbol:          "X",
			PortfolioWeight: 50,
			SymbolDetails: &symbol.SymbolDetails{
				QuoteType: "ETF",
			},
		},
		PositionWithDetails{
			Symbol:          "Y",
			PortfolioWeight: 50,
		},
	}

	result := ComputeSectorAllocation(positions)

	if result.UnknownWeightPct != 100.0 {
		t.Errorf("unknown weight = %f, want 100", result.UnknownWeightPct)
	}
	if len(result.Breakdown) != 0 {
		t.Errorf("breakdown should be empty, got: %v", result.Breakdown)
	}
}

func TestComputeSectorAllocation_SinglePosition(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithSector("AAPL", 100, "Technology"),
	}

	result := ComputeSectorAllocation(positions)

	if result.Breakdown["Technology"] != 100.0 {
		t.Errorf("Technology weight = %f, want 100", result.Breakdown["Technology"])
	}
	if result.UnknownWeightPct != 0 {
		t.Errorf("unknown weight = %f, want 0", result.UnknownWeightPct)
	}
}

func TestComputeSectorAllocation_Empty(t *testing.T) {
	result := ComputeSectorAllocation([]PositionWithDetails{})

	if result.Breakdown == nil {
		t.Error("breakdown should be empty map, not nil")
	}
	if len(result.Breakdown) != 0 {
		t.Errorf("breakdown should be empty, got: %v", result.Breakdown)
	}
	if result.Message == "" {
		t.Error("expected empty-state message")
	}
}

func TestComputeSectorAllocation_WeightedAggregation(t *testing.T) {
	// Verify that ETF weight × sector % is computed correctly.
	// ETF at 25% portfolio weight, 60% in one sector → 15% contribution.
	positions := []PositionWithDetails{
		getETFWithSectorWeightings("ETF1", 25, []symbol.SectorWeighting{
			{Sector: "Energy", Percent: 60},
			{Sector: "Utilities", Percent: 40},
		}),
		getETFWithSectorWeightings("ETF2", 75, []symbol.SectorWeighting{
			{Sector: "Energy", Percent: 20},
			{Sector: "Utilities", Percent: 80},
		}),
	}

	result := ComputeSectorAllocation(positions)

	// Energy: 25*60/100 + 75*20/100 = 15 + 15 = 30
	// Utilities: 25*40/100 + 75*80/100 = 10 + 60 = 70
	if result.Breakdown["Energy"] != 30.0 {
		t.Errorf("Energy = %f, want 30", result.Breakdown["Energy"])
	}
	if result.Breakdown["Utilities"] != 70.0 {
		t.Errorf("Utilities = %f, want 70", result.Breakdown["Utilities"])
	}
}

// --- ComputeGeographicAllocation tests ---

func TestComputeGeographicAllocation_HappyPath(t *testing.T) {
	positions := []PositionWithDetails{
		// ETF: 50% of portfolio, diversified geographically
		getETFWithGeographicAllocations("VT", 50, []symbol.GeographicAllocation{
			{Country: "United States", Percent: 60},
			{Country: "China", Percent: 15},
			{Country: "Japan", Percent: 10},
			{Country: "United Kingdom", Percent: 10},
			{Country: "Other", Percent: 5},
		}),
		// Stock: 30% of portfolio, US company
		getStockWithCountry("AAPL", 30, "United States"),
		// Stock: 20% of portfolio, Japanese company
		getStockWithCountry("7203.T", 20, "Japan"),
	}

	result := ComputeGeographicAllocation(positions)

	// VT: 50*60/100=30 US, 50*15/100=7.5 China, 50*10/100=5 Japan, 50*10/100=5 UK, 50*5/100=2.5 Other
	// AAPL: 30 US
	// 7203.T: 20 Japan
	// Expected: US=60, China=7.5, Japan=25, UK=5, Other=2.5

	want := map[string]float64{
		"United States":  60.0,
		"China":          7.5,
		"Japan":          25.0,
		"United Kingdom": 5.0,
		"Other":          2.5,
	}

	for country, wantWeight := range want {
		got := result.Breakdown[country]
		if got != wantWeight {
			t.Errorf("%s weight = %f, want %f", country, got, wantWeight)
		}
	}
	if result.UnknownWeightPct != 0 {
		t.Errorf("unknown weight = %f, want 0", result.UnknownWeightPct)
	}
	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestComputeGeographicAllocation_ETFOnly(t *testing.T) {
	positions := []PositionWithDetails{
		getETFWithGeographicAllocations("VEA", 100, []symbol.GeographicAllocation{
			{Country: "Mexico", Percent: 14},
			{Country: "Brazil", Percent: 12},
			{Country: "Chile", Percent: 10},
			{Country: "Other", Percent: 64},
		}),
	}

	result := ComputeGeographicAllocation(positions)

	want := map[string]float64{
		"Mexico": 14.0,
		"Brazil": 12.0,
		"Chile":  10.0,
		"Other":  64.0,
	}

	for country, wantWeight := range want {
		got := result.Breakdown[country]
		if got != wantWeight {
			t.Errorf("%s weight = %f, want %f", country, got, wantWeight)
		}
	}
}

func TestComputeGeographicAllocation_StockOnly(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithCountry("AAPL", 40, "United States"),
		getStockWithCountry("SAP", 30, "Germany"),
		getStockWithCountry("TSM", 30, "Taiwan"),
	}

	result := ComputeGeographicAllocation(positions)

	want := map[string]float64{
		"United States": 40.0,
		"Germany":       30.0,
		"Taiwan":        30.0,
	}

	for country, wantWeight := range want {
		got := result.Breakdown[country]
		if got != wantWeight {
			t.Errorf("%s weight = %f, want %f", country, got, wantWeight)
		}
	}
}

func TestComputeGeographicAllocation_PartialData(t *testing.T) {
	positions := []PositionWithDetails{
		// ETF with no geographic data
		PositionWithDetails{
			Symbol:          "NOGEO",
			PortfolioWeight: 40,
			SymbolDetails: &symbol.SymbolDetails{
				QuoteType: "ETF",
			},
		},
		// Stock with country
		getStockWithCountry("AAPL", 60, "United States"),
	}

	result := ComputeGeographicAllocation(positions)

	if result.Breakdown["United States"] != 60.0 {
		t.Errorf("US weight = %f, want 60", result.Breakdown["United States"])
	}
	if result.UnknownWeightPct != 40.0 {
		t.Errorf("unknown weight = %f, want 40", result.UnknownWeightPct)
	}
	if len(result.Warnings) != 1 {
		t.Errorf("warnings = %d, want 1: %v", len(result.Warnings), result.Warnings)
	}
}

func TestComputeGeographicAllocation_AllMissingData(t *testing.T) {
	positions := []PositionWithDetails{
		PositionWithDetails{
			Symbol:          "X",
			PortfolioWeight: 50,
			SymbolDetails: &symbol.SymbolDetails{
				QuoteType: "ETF",
			},
		},
		PositionWithDetails{
			Symbol:          "Y",
			PortfolioWeight: 50,
		},
	}

	result := ComputeGeographicAllocation(positions)

	if result.UnknownWeightPct != 100.0 {
		t.Errorf("unknown weight = %f, want 100", result.UnknownWeightPct)
	}
	if len(result.Breakdown) != 0 {
		t.Errorf("breakdown should be empty, got: %v", result.Breakdown)
	}
}

func TestComputeGeographicAllocation_SinglePosition(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithCountry("AAPL", 100, "United States"),
	}

	result := ComputeGeographicAllocation(positions)

	if result.Breakdown["United States"] != 100.0 {
		t.Errorf("US weight = %f, want 100", result.Breakdown["United States"])
	}
	if result.UnknownWeightPct != 0 {
		t.Errorf("unknown weight = %f, want 0", result.UnknownWeightPct)
	}
}

func TestComputeGeographicAllocation_Empty(t *testing.T) {
	result := ComputeGeographicAllocation([]PositionWithDetails{})

	if result.Breakdown == nil {
		t.Error("breakdown should be empty map, not nil")
	}
	if len(result.Breakdown) != 0 {
		t.Errorf("breakdown should be empty, got: %v", result.Breakdown)
	}
	if result.Message == "" {
		t.Error("expected empty-state message")
	}
}

// --- AllocationBreakdownSorted tests ---

func TestAllocationBreakdownSorted(t *testing.T) {
	result := &AllocationResult{
		Breakdown: map[string]float64{
			"Technology": 50.0,
			"Healthcare": 30.0,
			"Energy":     20.0,
		},
	}

	entries := AllocationBreakdownSorted(result)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should be sorted descending by weight.
	if entries[0].Category != "Technology" || entries[0].WeightPct != 50.0 {
		t.Errorf("first entry = %+v, want Technology/50", entries[0])
	}
	if entries[1].Category != "Healthcare" || entries[1].WeightPct != 30.0 {
		t.Errorf("second entry = %+v, want Healthcare/30", entries[1])
	}
	if entries[2].Category != "Energy" || entries[2].WeightPct != 20.0 {
		t.Errorf("third entry = %+v, want Energy/20", entries[2])
	}
}

func TestAllocationBreakdownSorted_TieBreaking(t *testing.T) {
	// When weights are equal, should sort alphabetically by category.
	result := &AllocationResult{
		Breakdown: map[string]float64{
			"Zebra":  10.0,
			"Alpha":  10.0,
			"Middle": 20.0,
		},
	}

	entries := AllocationBreakdownSorted(result)

	if entries[0].Category != "Middle" {
		t.Errorf("first = %q, want Middle", entries[0].Category)
	}
	if entries[1].Category != "Alpha" {
		t.Errorf("second = %q, want Alpha (alphabetical tie-break)", entries[1].Category)
	}
	if entries[2].Category != "Zebra" {
		t.Errorf("third = %q, want Zebra", entries[2].Category)
	}
}
