package comparison

import (
	"sort"

	"github.com/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// SectorAllocationResult holds the weighted sector allocation of a portfolio
// computed from PortfolioHolding data (comparison domain).
type SectorAllocationResult struct {
	Breakdown        map[string]float64 `json:"breakdown"`
	UnknownWeightPct float64            `json:"unknown_weight_pct"`
	Warnings         []string           `json:"warnings,omitempty"`
	MissingSymbols   []string           `json:"missing_symbols,omitempty"`
	Message          string             `json:"message,omitempty"`
}

// CountryAllocationResult holds the weighted geographic (country) allocation
// of a portfolio computed from PortfolioHolding data (comparison domain).
type CountryAllocationResult struct {
	Breakdown        map[string]float64 `json:"breakdown"`
	UnknownWeightPct float64            `json:"unknown_weight_pct"`
	Warnings         []string           `json:"warnings,omitempty"`
	MissingSymbols   []string           `json:"missing_symbols,omitempty"`
	Message          string             `json:"message,omitempty"`
}

// ComputeSectorAllocationForHoldings computes the weighted sector allocation
// of a portfolio from comparison-domain PortfolioHolding data.
//
// For each ETF holding, each sector is weighted by:
//
//	holding weight (fraction) × sector percent from SectorWeightings
//
// For each individual stock holding, the stock's primary sector (from
// PortfolioHolding.Sector) receives the full holding weight.
//
// Holdings without sector data are accumulated into the "Unknown" bucket.
// Warnings and missing symbols are tracked per-holding.
func ComputeSectorAllocationForHoldings(holdings []PortfolioHolding) *SectorAllocationResult {
	if len(holdings) == 0 {
		return &SectorAllocationResult{
			Breakdown:        map[string]float64{},
			UnknownWeightPct: 0,
			Message:          "No holdings to analyze. Sector allocation requires at least one holding.",
		}
	}

	breakdown := make(map[string]float64)
	var warnings []string
	var missingSymbols []string
	var unknownWeight float64

	for _, h := range holdings {
		weight, _ := h.Weight.Float64()

		switch h.QuoteType {
		case "ETF":
			if len(h.SectorWeightings) == 0 {
				unknownWeight += weight
				warnings = append(warnings, h.Symbol+": no sector data for ETF — weight counted as unknown")
				missingSymbols = append(missingSymbols, h.Symbol)
				continue
			}
			for _, sw := range h.SectorWeightings {
				weighted := weight * sw.Percent / 100.0
				breakdown[normalizeSector(sw.Sector)] += weighted
			}
		default:
			// Individual stock: use primary sector.
			if h.Sector == "" {
				unknownWeight += weight
				warnings = append(warnings, h.Symbol+": no sector data — weight counted as unknown")
				missingSymbols = append(missingSymbols, h.Symbol)
				continue
			}
			breakdown[normalizeSector(h.Sector)] += weight
		}
	}

	// Round values.
	for k, v := range breakdown {
		breakdown[k] = stats.RoundTo2(v)
	}
	unknownWeight = stats.RoundTo2(unknownWeight)

	return &SectorAllocationResult{
		Breakdown:        breakdown,
		UnknownWeightPct: unknownWeight,
		Warnings:         warnings,
		MissingSymbols:   missingSymbols,
	}
}

// ComputeCountryAllocationForHoldings computes the weighted geographic
// (country) allocation of a portfolio from comparison-domain PortfolioHolding
// data.
//
// For all holdings, each country is weighted by:
//
//	holding weight (fraction) × country percent from GeographicAllocations
//
// Holdings without geographic data are accumulated into the "Unknown" bucket.
func ComputeCountryAllocationForHoldings(holdings []PortfolioHolding) *CountryAllocationResult {
	if len(holdings) == 0 {
		return &CountryAllocationResult{
			Breakdown:        map[string]float64{},
			UnknownWeightPct: 0,
			Message:          "No holdings to analyze. Country allocation requires at least one holding.",
		}
	}

	breakdown := make(map[string]float64)
	var warnings []string
	var missingSymbols []string
	var unknownWeight float64

	for _, h := range holdings {
		weight, _ := h.Weight.Float64()

		if len(h.GeographicAllocations) == 0 {
			unknownWeight += weight
			warnings = append(warnings, h.Symbol+": no geographic data — weight counted as unknown")
			missingSymbols = append(missingSymbols, h.Symbol)
			continue
		}

		for _, ga := range h.GeographicAllocations {
			weighted := weight * ga.Percent / 100.0
			breakdown[ga.Country] += weighted
		}
	}

	// Round values.
	for k, v := range breakdown {
		breakdown[k] = stats.RoundTo2(v)
	}
	unknownWeight = stats.RoundTo2(unknownWeight)

	return &CountryAllocationResult{
		Breakdown:        breakdown,
		UnknownWeightPct: unknownWeight,
		Warnings:         warnings,
		MissingSymbols:   missingSymbols,
	}
}

// normalizeSector converts a sector name to a consistent display format.
// Currently a pass-through; Yahoo Finance sectors are already reasonably
// consistent (e.g. "Technology", "Healthcare"). Can be extended with a
// lookup table if multiple data sources with varying naming conventions are added.
func normalizeSector(sector string) string {
	return sector
}

// SectorAllocationSorted returns the sector allocation breakdown as a sorted
// slice of entries, ordered by weight descending.
type AllocationEntry struct {
	Category string
	Weight   float64 // fraction 0.0-1.0, not percentage
}

func SectorAllocationSorted(result *SectorAllocationResult) []AllocationEntry {
	entries := make([]AllocationEntry, 0, len(result.Breakdown))
	for cat, weight := range result.Breakdown {
		entries = append(entries, AllocationEntry{Category: cat, Weight: weight})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Weight != entries[j].Weight {
			return entries[i].Weight > entries[j].Weight
		}
		return entries[i].Category < entries[j].Category
	})
	return entries
}

func CountryAllocationSorted(result *CountryAllocationResult) []AllocationEntry {
	entries := make([]AllocationEntry, 0, len(result.Breakdown))
	for cat, weight := range result.Breakdown {
		entries = append(entries, AllocationEntry{Category: cat, Weight: weight})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Weight != entries[j].Weight {
			return entries[i].Weight > entries[j].Weight
		}
		return entries[i].Category < entries[j].Category
	})
	return entries
}

// --- test helpers ---

// getETFWithSectorWeightings returns a PortfolioHolding configured as an ETF
// with the given sector weightings, for testing.
func getETFWithSectorWeightings(sym string, weight float64, sectors []symbol.SectorWeighting) PortfolioHolding {
	d, _ := decimal.NewFromFloat64(weight)
	return PortfolioHolding{
		Symbol:           sym,
		Weight:           d,
		Name:             sym,
		QuoteType:        "ETF",
		SectorWeightings: sectors,
	}
}

// getStockWithSector returns a PortfolioHolding configured as an individual
// stock with the given primary sector, for testing.
func getStockWithSector(sym string, weight float64, sector string) PortfolioHolding {
	d, _ := decimal.NewFromFloat64(weight)
	return PortfolioHolding{
		Symbol:    sym,
		Weight:    d,
		Name:      sym,
		QuoteType: "EQUITY",
		Sector:    sector,
	}
}

// getETFWithGeographicAllocations returns a PortfolioHolding configured as an
// ETF with the given geographic allocations, for testing.
func getETFWithGeographicAllocations(sym string, weight float64, allocations []symbol.GeographicAllocation) PortfolioHolding {
	d, _ := decimal.NewFromFloat64(weight)
	return PortfolioHolding{
		Symbol:                sym,
		Weight:                d,
		Name:                  sym,
		QuoteType:             "ETF",
		GeographicAllocations: allocations,
	}
}

// getStockWithCountry returns a PortfolioHolding configured as an individual
// stock with the given country (as a single 100% geographic allocation),
// for testing.
func getStockWithCountry(sym string, weight float64, country string) PortfolioHolding {
	d, _ := decimal.NewFromFloat64(weight)
	return PortfolioHolding{
		Symbol:    sym,
		Weight:    d,
		Name:      sym,
		QuoteType: "EQUITY",
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: country, Percent: 100},
		},
	}
}
