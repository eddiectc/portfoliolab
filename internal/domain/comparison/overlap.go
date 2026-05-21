package comparison

import (
	"sort"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// PortfolioHolding describes a single holding in a portfolio for overlap
// computation. It can be either a direct holding (stock) or an ETF entry
// (with TopHoldings populated for expansion).
type PortfolioHolding struct {
	Symbol      string
	WeightPct   decimal.Decimal // weight as percentage (0-100)
	Name        string
	QuoteType   string          // "ETF" or "EQUITY" (or other)
	TopHoldings []symbol.TopHolding
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
// Overlap percentage is the Jaccard similarity of underlying symbol sets:
//   overlap = |A ∩ B| / |A ∪ B| × 100
//
// Returns warnings when ETFs have no cached holdings data.
func ComputeCrossPortfolioOverlap(input CrossPortfolioOverlapInput) *OverlapResult {
	topA, warningsA := expandToTopHoldingsWithWarnings(input.PortfolioA, 10)
	topB, warningsB := expandToTopHoldingsWithWarnings(input.PortfolioB, 10)

	warnings := append(warningsA, warningsB...)

	// Compute overlap percentage from expanded underlying symbol maps.
	expandedA := expandETFHoldings(input.PortfolioA)
	expandedB := expandETFHoldings(input.PortfolioB)

	overlapPct := computeOverlapPercentage(expandedA, expandedB)

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

	// Check for ETFs with no holdings data.
	for _, h := range holdings {
		if h.QuoteType == "ETF" && len(h.TopHoldings) == 0 {
			warnings = append(warnings, "ETF "+h.Symbol+" has no cached holdings data — treated as atomic holding")
		}
	}

	expanded := expandETFHoldings(holdings)

	// Convert to sortable slice.
	type holdingEntry struct {
		symbol string
		weight decimal.Decimal
		name   string
	}
	entries := make([]holdingEntry, 0, len(expanded))
	for sym, info := range expanded {
		entries = append(entries, holdingEntry{
			symbol: sym,
			weight: info.weight,
			name:   info.name,
		})
	}

	// Sort descending by weight.
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
			Symbol: e.symbol,
			Weight: e.weight,
			Name:   e.name,
		}
	}

	return result, warnings
}

// holdingInfo holds aggregated info for an underlying symbol.
type holdingInfo struct {
	weight decimal.Decimal // aggregated weight as percentage (0-100)
	name   string
}

// expandETFHoldings expands ETF holdings to their underlying symbols.
// Direct holdings (non-ETF) are kept as-is. Returns a map of symbol → info.
func expandETFHoldings(holdings []PortfolioHolding) map[string]*holdingInfo {
	agg := make(map[string]*holdingInfo)

	for _, h := range holdings {
		weightPct, _ := h.WeightPct.Float64()

		if h.QuoteType == "ETF" && len(h.TopHoldings) > 0 {
			// Expand ETF to underlying holdings.
			for _, uh := range h.TopHoldings {
				info, ok := agg[uh.Symbol]
				if !ok {
					info = &holdingInfo{}
					agg[uh.Symbol] = info
				}
				// Contribution: portfolio weight × holding percent / 100.
				contribution := weightPct * uh.Percent / 100.0
				contribDec, _ := decimal.NewFromFloat64(contribution)
				info.weight, _ = info.weight.Add(contribDec)
				// Use the name from the first occurrence.
				if info.name == "" {
					info.name = uh.Name
				}
			}
		} else {
			// Direct holding — keep as-is.
			info, ok := agg[h.Symbol]
			if !ok {
				info = &holdingInfo{}
				agg[h.Symbol] = info
			}
			info.weight, _ = info.weight.Add(h.WeightPct)
			if info.name == "" {
				info.name = h.Name
			}
		}
	}

	return agg
}

// computeOverlapPercentage computes the Jaccard similarity between two
// sets of underlying symbols, expressed as a percentage.
//
//	overlap = |A ∩ B| / |A ∪ B| × 100
//
// Returns 0 when either set is empty.
func computeOverlapPercentage(a, b map[string]*holdingInfo) decimal.Decimal {
	if len(a) == 0 || len(b) == 0 {
		return decimal.Zero
	}

	// Build sets.
	setA := make(map[string]bool, len(a))
	for sym := range a {
		setA[sym] = true
	}
	setB := make(map[string]bool, len(b))
	for sym := range b {
		setB[sym] = true
	}

	// Intersection.
	intersection := 0
	for sym := range setA {
		if setB[sym] {
			intersection++
		}
	}

	// Union.
	union := len(setA)
	for sym := range setB {
		if !setA[sym] {
			union++
		}
	}

	if union == 0 {
		return decimal.Zero
	}

	pctF := float64(intersection) / float64(union) * 100.0
	pctRounded := stats.RoundTo2(pctF)
	pct, _ := decimal.NewFromFloat64(pctRounded)
	return pct
}
