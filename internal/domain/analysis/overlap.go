package analysis

import (
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// ComputeOverlap computes ETF pairwise overlap and top concentrated stocks.
//
// It filters positions to ETFs only, then:
//  1. For each ETF pair, finds common underlying symbols from TopHoldings,
//     counts overlapping holdings, and computes combined portfolio weight
//     (sum of each ETF's contribution to the overlapping holdings).
//  2. Aggregates all underlying holdings across all ETFs, sums weight per
//     symbol, and returns the top 10 most concentrated stocks.
//
// Returns an empty-state message when fewer than 2 ETFs are available.
func ComputeOverlap(positions []PositionWithDetails) *OverlapResult {
	etfs := filterETFs(positions)

	if len(etfs) == 0 {
		return &OverlapResult{
			PairwiseMatrix:        []OverlapPair{},
			TopConcentratedStocks: []ConcentratedStock{},
			Message:               "No ETF positions to analyze. ETF overlap requires at least one ETF.",
		}
	}

	var warnings []string
	for _, p := range etfs {
		if len(p.SymbolDetails.TopHoldings) == 0 {
			warnings = append(warnings, "ETF "+p.Symbol+" has no cached holdings data — excluded from overlap analysis")
		}
	}

	// Concentrated stocks computed even for a single ETF.
	concentrated := computeConcentratedStocks(etfs)

	// Pairwise matrix requires 2+ ETFs.
	var pairs []OverlapPair
	var message string
	if len(etfs) < 2 {
		pairs = []OverlapPair{}
		message = "Only 1 ETF position found. ETF overlap requires at least 2 ETFs for pairwise comparison."
	} else {
		pairs = computePairwiseMatrix(etfs)
	}

	return &OverlapResult{
		PairwiseMatrix:        pairs,
		TopConcentratedStocks: concentrated,
		Warnings:              warnings,
		Message:               message,
	}
}

// filterETFs returns only positions with QuoteType == "ETF".
func filterETFs(positions []PositionWithDetails) []PositionWithDetails {
	var etfs []PositionWithDetails
	for _, p := range positions {
		if p.SymbolDetails != nil && p.SymbolDetails.QuoteType == "ETF" {
			etfs = append(etfs, p)
		}
	}
	return etfs
}

// computePairwiseMatrix builds the symmetric ETF overlap matrix (diagonal omitted).
func computePairwiseMatrix(etfs []PositionWithDetails) []OverlapPair {
	var pairs []OverlapPair
	for i := 0; i < len(etfs); i++ {
		for j := i + 1; j < len(etfs); j++ {
			pair := computePairOverlap(etfs[i], etfs[j])
			pairs = append(pairs, pair)
		}
	}
	return pairs
}

// computePairOverlap computes overlap between two ETF positions.
func computePairOverlap(a, b PositionWithDetails) OverlapPair {
	aHoldings := buildHoldingMap(a.SymbolDetails.TopHoldings)
	bHoldings := buildHoldingMap(b.SymbolDetails.TopHoldings)

	overlappingCount := 0
	combinedWeightPct := 0.0

	// Check all symbols in A's holdings against B's holdings.
	for sym, aPct := range aHoldings {
		bPct, ok := bHoldings[sym]
		if !ok {
			continue
		}
		overlappingCount++
		// Each ETF's contribution to the overlapping holding:
		//   ETF portfolio weight × holding percent (both 0-100 scale)
		contributionA := a.PortfolioWeight * aPct / 100.0
		contributionB := b.PortfolioWeight * bPct / 100.0
		combinedWeightPct += contributionA + contributionB
	}

	return OverlapPair{
		ETFA:            a.Symbol,
		ETFB:            b.Symbol,
		OverlappingCount: overlappingCount,
		CombinedWeightPct: roundTo2(combinedWeightPct),
	}
}

// buildHoldingMap creates a symbol → percent map from TopHoldings.
func buildHoldingMap(holdings []symbol.TopHolding) map[string]float64 {
	m := make(map[string]float64, len(holdings))
	for _, h := range holdings {
		m[h.Symbol] = h.Percent
	}
	return m
}

// computeConcentratedStocks aggregates all underlying holdings across ETFs
// and returns the top 10 by total portfolio weight.
func computeConcentratedStocks(etfs []PositionWithDetails) []ConcentratedStock {
	// symbol → aggregated info
	type stockInfo struct {
		name         string
		totalWeight  float64
		heldByETFs   map[string]struct{} // dedup
	}

	agg := make(map[string]*stockInfo)

	for _, p := range etfs {
		for _, h := range p.SymbolDetails.TopHoldings {
			info, ok := agg[h.Symbol]
			if !ok {
				info = &stockInfo{
					name:       h.Name,
					heldByETFs: make(map[string]struct{}),
				}
				agg[h.Symbol] = info
			}
			contribution := p.PortfolioWeight * h.Percent / 100.0
			info.totalWeight += contribution
			info.heldByETFs[p.Symbol] = struct{}{}
		}
	}

	// Convert to sorted slice.
	count := len(agg)
	if count == 0 {
		return []ConcentratedStock{}
	}

	stocks := make([]ConcentratedStock, 0, count)
	for sym, info := range agg {
		etfList := make([]string, 0, len(info.heldByETFs))
		for etf := range info.heldByETFs {
			etfList = append(etfList, etf)
		}
		stocks = append(stocks, ConcentratedStock{
			Symbol:        sym,
			Name:          info.name,
			TotalWeightPct: roundTo2(info.totalWeight),
			HeldByETFs:    etfList,
		})
	}

	// Sort descending by total weight.
	sortByWeightDesc(stocks)

	// Return top 10.
	if len(stocks) > 10 {
		stocks = stocks[:10]
	}

	return stocks
}

// sortByWeightDesc sorts ConcentratedStock in-place by TotalWeightPct descending.
func sortByWeightDesc(stocks []ConcentratedStock) {
	for i := 0; i < len(stocks); i++ {
		for j := i + 1; j < len(stocks); j++ {
			if stocks[j].TotalWeightPct > stocks[i].TotalWeightPct {
				stocks[i], stocks[j] = stocks[j], stocks[i]
			}
		}
	}
}

// roundTo2 rounds a float64 to 2 decimal places.
func roundTo2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
