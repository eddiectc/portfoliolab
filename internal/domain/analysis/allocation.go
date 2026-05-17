package analysis

import (
	"sort"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// ComputeSectorAllocation computes the weighted sector allocation of a
// portfolio, doing ETF look-through for ETF positions and using the primary
// sector for individual stock positions.
//
// For each ETF position, each sector is weighted by:
//
//	ETF portfolio weight × sector percent from SectorWeightings
//
// For each individual stock position, the stock's primary sector (from
// SymbolDetails.Sector) receives the full position weight.
//
// Positions without sector data are accumulated into the "Unknown" bucket.
// Warnings are generated for each symbol missing sector data.
func ComputeSectorAllocation(positions []PositionWithDetails) *AllocationResult {
	if len(positions) == 0 {
		return &AllocationResult{
			Breakdown:        map[string]float64{},
			UnknownWeightPct: 0,
			Message:          "No positions to analyze. Sector allocation requires at least one position.",
		}
	}

	breakdown := make(map[string]float64)
	var warnings []string
	unknownWeight := 0.0

	for _, p := range positions {
		if p.SymbolDetails == nil {
			unknownWeight += p.PortfolioWeight
			warnings = append(warnings, p.Symbol+": no symbol details available")
			continue
		}

		if p.SymbolDetails.QuoteType == "ETF" {
			// ETF look-through: weight each sector by ETF weight × sector %.
			if len(p.SymbolDetails.SectorWeightings) == 0 {
				unknownWeight += p.PortfolioWeight
				warnings = append(warnings, p.Symbol+": no sector data for ETF — weight counted as unknown")
				continue
			}
			for _, sw := range p.SymbolDetails.SectorWeightings {
				weighted := p.PortfolioWeight * sw.Percent / 100.0
				breakdown[normalizeSector(sw.Sector)] += weighted
			}
		} else {
			// Individual stock: use primary sector.
			if p.SymbolDetails.Sector == "" {
				unknownWeight += p.PortfolioWeight
				warnings = append(warnings, p.Symbol+": no sector data — weight counted as unknown")
				continue
			}
			breakdown[normalizeSector(p.SymbolDetails.Sector)] += p.PortfolioWeight
		}
	}

	// Round values and sort.
	for k, v := range breakdown {
		breakdown[k] = roundTo2(v)
	}
	unknownWeight = roundTo2(unknownWeight)

	return &AllocationResult{
		Breakdown:        breakdown,
		UnknownWeightPct: unknownWeight,
		Warnings:         warnings,
	}
}

// ComputeGeographicAllocation computes the weighted geographic allocation of
// a portfolio, doing ETF look-through for ETF positions and using the
// country data for individual stock positions.
//
// For each ETF position, each country is weighted by:
//
//	ETF portfolio weight × country percent from GeographicAllocations
//
// For each individual stock position, the stock's country (from
// GeographicAllocations, which is a single 100% entry for stocks) receives
// the full position weight.
//
// Positions without geographic data are accumulated into the "Unknown" bucket.
// Warnings are generated for each symbol missing geographic data.
func ComputeGeographicAllocation(positions []PositionWithDetails) *AllocationResult {
	if len(positions) == 0 {
		return &AllocationResult{
			Breakdown:        map[string]float64{},
			UnknownWeightPct: 0,
			Message:          "No positions to analyze. Geographic allocation requires at least one position.",
		}
	}

	breakdown := make(map[string]float64)
	var warnings []string
	unknownWeight := 0.0

	for _, p := range positions {
		if p.SymbolDetails == nil {
			unknownWeight += p.PortfolioWeight
			warnings = append(warnings, p.Symbol+": no symbol details available")
			continue
		}

		if len(p.SymbolDetails.GeographicAllocations) == 0 {
			unknownWeight += p.PortfolioWeight
			warnings = append(warnings, p.Symbol+": no geographic data — weight counted as unknown")
			continue
		}

		for _, ga := range p.SymbolDetails.GeographicAllocations {
			// For ETFs: weight by ETF weight × country %.
			// For stocks: GeographicAllocations is a single 100% entry,
			// so this effectively assigns the full position weight.
			weighted := p.PortfolioWeight * ga.Percent / 100.0
			breakdown[ga.Country] += weighted
		}
	}

	// Round values.
	for k, v := range breakdown {
		breakdown[k] = roundTo2(v)
	}
	unknownWeight = roundTo2(unknownWeight)

	return &AllocationResult{
		Breakdown:        breakdown,
		UnknownWeightPct: unknownWeight,
		Warnings:         warnings,
	}
}

// normalizeSector converts a sector name to a consistent display format.
// Handles common Yahoo Finance sector name variations.
func normalizeSector(sector string) string {
	// Trim and return as-is for now; Yahoo Finance sectors are already
	// reasonably consistent (e.g. "Technology", "Healthcare").
	// Can be extended with a lookup table if needed.
	return sector
}

// AllocationBreakdownSorted returns the allocation breakdown as a sorted
// slice of entries, ordered by weight descending. This is a convenience
// function for consumers that need ordered output (e.g., charts, tables).
type AllocationEntry struct {
	Category string
	WeightPct float64
}

func AllocationBreakdownSorted(result *AllocationResult) []AllocationEntry {
	entries := make([]AllocationEntry, 0, len(result.Breakdown))
	for cat, weight := range result.Breakdown {
		entries = append(entries, AllocationEntry{Category: cat, WeightPct: weight})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].WeightPct != entries[j].WeightPct {
			return entries[i].WeightPct > entries[j].WeightPct
		}
		return entries[i].Category < entries[j].Category
	})
	return entries
}

// getETFWithSectorWeightings returns a PositionWithDetails configured as an
// ETF with the given sector weightings, for testing.
func getETFWithSectorWeightings(sym string, portfolioWeight float64, sectors []symbol.SectorWeighting) PositionWithDetails {
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: portfolioWeight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType:        "ETF",
			SectorWeightings: sectors,
		},
	}
}

// getStockWithSector returns a PositionWithDetails configured as an
// individual stock with the given primary sector, for testing.
func getStockWithSector(sym string, portfolioWeight float64, sector string) PositionWithDetails {
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: portfolioWeight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType: "EQUITY",
			Sector:    sector,
		},
	}
}

// getETFWithGeographicAllocations returns a PositionWithDetails configured
// as an ETF with the given geographic allocations, for testing.
func getETFWithGeographicAllocations(sym string, portfolioWeight float64, allocations []symbol.GeographicAllocation) PositionWithDetails {
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: portfolioWeight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType:             "ETF",
			GeographicAllocations: allocations,
		},
	}
}

// getStockWithCountry returns a PositionWithDetails configured as an
// individual stock with the given country (as a single 100% geographic
// allocation), for testing.
func getStockWithCountry(sym string, portfolioWeight float64, country string) PositionWithDetails {
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: portfolioWeight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType: "EQUITY",
			GeographicAllocations: []symbol.GeographicAllocation{
				{Country: country, Percent: 100},
			},
		},
	}
}
