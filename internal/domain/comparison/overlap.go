package comparison

import (
	"sort"
	"strings"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// PortfolioHolding describes a single holding in a portfolio for overlap
// computation. It can be either a direct holding (stock) or an ETF entry
// (with TopHoldings populated for expansion).
type PortfolioHolding struct {
	Symbol                string
	Weight                decimal.Decimal // fraction of portfolio (0.0-1.0)
	Name                  string
	QuoteType             string // "ETF" or "EQUITY" (or other)
	TopHoldings           []symbol.TopHolding
	Sector                string                   // primary sector for individual stocks
	SectorWeightings      []symbol.SectorWeighting // sector breakdown for ETFs
	GeographicAllocations []symbol.GeographicAllocation
}

// CrossPortfolioOverlapInput holds the two portfolios to compare.
type CrossPortfolioOverlapInput struct {
	PortfolioA []PortfolioHolding
	PortfolioB []PortfolioHolding
}

// ComputeCrossPortfolioOverlap computes the holdings overlap between two
// portfolios.
//
// For each portfolio:
//   - ETF holdings are expanded to their underlying symbols (weight × percent)
//   - Direct holdings (stocks) are kept as-is
//   - The top-10 underlying holdings are returned
//
// Overlap percentage uses the industry-standard weighted overlap:
//
//	overlap = Σ min(w_A,i, w_B,i) for all shared holdings i
//
// This gives the percentage of each portfolio invested in the same securities.
//
// Key resolution: ISIN → Symbol → Name (normalized). Both portfolios
// are matched at the lowest common key level.
//
// Returns warnings when ETFs have no cached holdings data.
func ComputeCrossPortfolioOverlap(input CrossPortfolioOverlapInput) *OverlapResult {
	topA, warningsA := expandToTopHoldingsWithWarnings(input.PortfolioA, 10)
	topB, warningsB := expandToTopHoldingsWithWarnings(input.PortfolioB, 10)

	warnings := append(warningsA, warningsB...)

	// Determine the lowest common key level across both portfolios.
	level := minKeyLevel(input.PortfolioA, input.PortfolioB)

	// Expand all holdings (weighting naturally downweights tiny positions).
	expandedA := expandETFHoldings(input.PortfolioA, level)
	expandedB := expandETFHoldings(input.PortfolioB, level)

	overlapPct, matchWarnings := computeWeightedOverlap(expandedA, expandedB)
	warnings = append(warnings, matchWarnings...)

	return &OverlapResult{
		TopHoldingsA: topA,
		TopHoldingsB: topB,
		OverlapPct:   &overlapPct,
		Warnings:     warnings,
	}
}

// expandToTopHoldingsWithWarnings expands ETF holdings to underlying symbols
// and returns the top N holdings by weight, plus any warnings.
func expandToTopHoldingsWithWarnings(holdings []PortfolioHolding, limit int) ([]HoldingWeight, []string) {
	var warnings []string

	// Check for holdings that look like ETFs (have symbol details but no holdings data).
	for _, h := range holdings {
		if h.QuoteType == "ETF" && len(h.TopHoldings) == 0 {
			warnings = append(warnings, "ETF "+h.Symbol+" has no cached holdings data — treated as atomic holding")
		}
	}

	expanded := expandETFHoldingsDisplay(holdings)

	// Sort descending by weight.
	type holdingEntry struct {
		isin   string
		symbol string
		weight decimal.Decimal
		name   string
	}
	entries := make([]holdingEntry, 0, len(expanded))
	for _, info := range expanded {
		entries = append(entries, holdingEntry{
			isin:   info.isin,
			symbol: info.symbol,
			weight: info.weight,
			name:   info.name,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].weight.Cmp(entries[j].weight) > 0
	})

	// Take top N.
	if len(entries) > limit {
		entries = entries[:limit]
	}

	result := make([]HoldingWeight, len(entries))
	for i, e := range entries {
		result[i] = HoldingWeight{
			ISIN:   e.isin,
			Symbol: e.symbol,
			Weight: e.weight,
			Name:   e.name,
		}
	}

	return result, warnings
}

// holdingInfo holds aggregated info for an underlying holding.
type holdingInfo struct {
	weight decimal.Decimal // aggregated weight as fraction (0.0-1.0)
	name   string
}

// holdingInfoDisplay holds aggregated info with separate ISIN and Symbol for display.
type holdingInfoDisplay struct {
	weight decimal.Decimal
	name   string
	isin   string
	symbol string
}

// isETF returns true if the holding is an ETF.
// Uses TopHoldings presence as the primary signal — QuoteType may be
// empty for older records stored before the QuoteType field was added.
func isETF(h PortfolioHolding) bool {
	return len(h.TopHoldings) > 0
}

// keyLevel indicates what identifier is available for holdings.
type keyLevel int

const (
	keyNone keyLevel = iota
	keyName
	keySymbol
	keyISIN
	keyBest // use best available per holding (ISIN > Symbol > Name)
)

// holdingKeyLevel returns the highest key level available for a TopHolding.
func holdingKeyLevel(uh symbol.TopHolding) keyLevel {
	if uh.ISIN != "" && uh.ISIN != "-" {
		return keyISIN
	}
	if uh.Symbol != "" {
		return keySymbol
	}
	if uh.Name != "" {
		return keyName
	}
	return keyNone
}

// holdingKey returns a key for a TopHolding at the given level.
func holdingKey(uh symbol.TopHolding, level keyLevel) string {
	switch level {
	case keyBest:
		if uh.ISIN != "" && uh.ISIN != "-" {
			return uh.ISIN
		}
		if uh.Symbol != "" {
			return strings.ToUpper(uh.Symbol)
		}
		if uh.Name != "" {
			return normalizeName(uh.Name)
		}
		return ""
	case keyISIN:
		if uh.ISIN != "" && uh.ISIN != "-" {
			return uh.ISIN
		}
	case keySymbol:
		if uh.Symbol != "" {
			return strings.ToUpper(uh.Symbol)
		}
	case keyName:
		if uh.Name != "" {
			return normalizeName(uh.Name)
		}
	}
	return ""
}

// normalizeName normalizes a holding name for comparison:
// uppercase, strip trailing periods and whitespace.
func normalizeName(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ToUpper(s)
	// Strip trailing periods (e.g. "Apple Inc." → "Apple Inc")
	s = strings.TrimRight(s, ".")
	return s
}

// minKeyLevel returns the minimum key level across all holdings in both portfolios.
// This ensures both sides use the same key type.
func minKeyLevel(holdingsA, holdingsB []PortfolioHolding) keyLevel {
	minA := keyISIN
	hasETFA := false
	for _, h := range holdingsA {
		if !isETF(h) {
			continue
		}
		hasETFA = true
		for _, uh := range h.TopHoldings {
			level := holdingKeyLevel(uh)
			if level < minA {
				minA = level
			}
		}
	}
	if !hasETFA {
		minA = keySymbol
	}

	minB := keyISIN
	hasETFB := false
	for _, h := range holdingsB {
		if !isETF(h) {
			continue
		}
		hasETFB = true
		for _, uh := range h.TopHoldings {
			level := holdingKeyLevel(uh)
			if level < minB {
				minB = level
			}
		}
	}
	if !hasETFB {
		minB = keySymbol
	}

	if minA < minB {
		return minA
	}
	return minB
}

// expandETFHoldings expands ETF holdings to their underlying symbols.
// Direct holdings (non-ETF) are kept as-is. Returns a map of key → info.
func expandETFHoldings(holdings []PortfolioHolding, level keyLevel) map[string]*holdingInfo {
	agg := make(map[string]*holdingInfo)

	for _, h := range holdings {
		weight, _ := h.Weight.Float64()

		if isETF(h) {
			// Expand ETF to underlying holdings.
			for _, uh := range h.TopHoldings {
				key := holdingKey(uh, level)
				if key == "" {
					continue
				}
				info, ok := agg[key]
				if !ok {
					info = &holdingInfo{}
					agg[key] = info
				}
				// Contribution: portfolio weight (fraction) × holding percent / 100.
				contribution := weight * uh.Percent / 100.0
				contribDec, _ := decimal.NewFromFloat64(contribution)
				info.weight, _ = info.weight.Add(contribDec)
				// Use the name from the first occurrence.
				if info.name == "" {
					info.name = uh.Name
				}
			}
		} else {
			// Direct holding — keep as-is.
			key := h.Symbol
			if level == keyName && h.Name != "" {
				key = normalizeName(h.Name)
			} else if level == keySymbol {
				key = strings.ToUpper(key)
			}
			info, ok := agg[key]
			if !ok {
				info = &holdingInfo{}
				agg[key] = info
			}
			info.weight, _ = info.weight.Add(h.Weight)
			if info.name == "" {
				info.name = h.Name
			}
		}
	}

	return agg
}

// expandETFHoldingsDisplay expands ETF holdings for display purposes,
// tracking ISIN and Symbol separately. Direct holdings are kept as-is.
func expandETFHoldingsDisplay(holdings []PortfolioHolding) map[string]*holdingInfoDisplay {
	agg := make(map[string]*holdingInfoDisplay)

	for _, h := range holdings {
		weight, _ := h.Weight.Float64()

		if isETF(h) {
			for _, uh := range h.TopHoldings {
				// Use ISIN as primary key; fall back to uppercase symbol, then name.
				key := holdingKey(uh, keyBest)
				if key == "" {
					continue
				}
				info, ok := agg[key]
				if !ok {
					info = &holdingInfoDisplay{}
					agg[key] = info
				}
				contribution := weight * uh.Percent / 100.0
				contribDec, _ := decimal.NewFromFloat64(contribution)
				info.weight, _ = info.weight.Add(contribDec)
				if info.name == "" {
					info.name = uh.Name
				}
				// Populate ISIN and Symbol from first occurrence that has them.
				if info.isin == "" && uh.ISIN != "" && uh.ISIN != "-" {
					info.isin = uh.ISIN
				}
				if info.symbol == "" && uh.Symbol != "" {
					info.symbol = strings.ToUpper(uh.Symbol)
				}
			}
		} else {
			key := h.Symbol
			if h.Name != "" && key == "" {
				key = normalizeName(h.Name)
			}
			info, ok := agg[key]
			if !ok {
				info = &holdingInfoDisplay{
					symbol: strings.ToUpper(h.Symbol),
				}
				agg[key] = info
			}
			info.weight, _ = info.weight.Add(h.Weight)
			if info.name == "" {
				info.name = h.Name
			}
		}
	}

	return agg
}

// computeWeightedOverlap computes the industry-standard weighted overlap
// between two portfolios.
//
//	overlap = Σ min(w_A,i, w_B,i) for all shared holdings i
//
// Weights are fractions (0.0-1.0). Result is expressed as a percentage.
func computeWeightedOverlap(a, b map[string]*holdingInfo) (decimal.Decimal, []string) {
	if len(a) == 0 || len(b) == 0 {
		return decimal.Zero, nil
	}

	// Sum of min(weights) for shared holdings.
	var overlap decimal.Decimal
	for key, infoA := range a {
		if infoB, ok := b[key]; ok {
			minW := infoA.weight
			if infoB.weight.Cmp(minW) < 0 {
				minW = infoB.weight
			}
			overlap, _ = overlap.Add(minW)
		}
	}

	// Convert fraction to percentage.
	overlapF, _ := overlap.Float64()
	pctRounded := stats.RoundTo2(overlapF * 100.0)
	pct, _ := decimal.NewFromFloat64(pctRounded)
	return pct, nil
}
