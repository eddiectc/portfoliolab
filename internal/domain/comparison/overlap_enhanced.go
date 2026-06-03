package comparison

import (
	"sort"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/govalues/decimal"
)

// ComputeMergedHoldings produces a merged list of holdings from two portfolios,
// showing each holding's weight in both portfolios and the overlap percentage.
//
// The list is ordered: shared holdings first (sorted by overlap % descending),
// then unique holdings (sorted by their weight descending, interleaved).
//
// Overlap % for shared holdings = min(weightA, weightB) * 100 (absolute percentage points).
// Overlap % for unique holdings = 0.
//
// The limit parameter controls how many holdings are selected from each portfolio
// before merging (top N from A + top N from B, deduplicated).
func ComputeMergedHoldings(holdingsA, holdingsB []PortfolioHolding, limit int) []MergedHolding {
	if len(holdingsA) == 0 && len(holdingsB) == 0 {
		return nil
	}

	// Expand both portfolios to underlying holdings.
	expandedA := expandETFHoldingsDisplay(holdingsA)
	expandedB := expandETFHoldingsDisplay(holdingsB)

	// Select top N from each portfolio by weight.
	topA := selectTopN(expandedA, limit)
	topB := selectTopN(expandedB, limit)

	// Build a unified key set from both selections.
	// Use ISIN as primary key for matching; fall back to uppercase symbol, then name.
	type keyInfo struct {
		key    string
		isin   string
		symbol string
		name   string
	}

	// Collect keys from both sides.
	keyMap := make(map[string]*keyInfo)
	for key, info := range topA {
		keyMap[key] = &keyInfo{key: key, isin: info.isin, symbol: info.symbol, name: info.name}
	}
	for key, info := range topB {
		existing, ok := keyMap[key]
		if !ok {
			keyMap[key] = &keyInfo{key: key, isin: info.isin, symbol: info.symbol, name: info.name}
		} else {
			// Merge name/symbol info (use whichever is more complete).
			if existing.name == "" && info.name != "" {
				existing.name = info.name
			}
			if existing.symbol == "" && info.symbol != "" {
				existing.symbol = info.symbol
			}
		}
	}

	// Classify as shared or unique.
	type holdingEntry struct {
		key        string
		symbol     string
		name       string
		weightA    decimal.Decimal
		weightB    decimal.Decimal
		overlapPct float64
		isShared   bool
	}

	var shared []holdingEntry
	var unique []holdingEntry

	for _, ki := range keyMap {
		infoA, hasA := topA[ki.key]
		infoB, hasB := topB[ki.key]

		weightA := decimal.Zero
		weightB := decimal.Zero
		if hasA {
			weightA = infoA.weight
		}
		if hasB {
			weightB = infoB.weight
		}

		symbol := ki.symbol
		if symbol == "" {
			symbol = ki.key
		}
		name := ki.name

		if hasA && hasB {
			// Shared holding — compute overlap %.
			minW := weightA
			if weightB.Cmp(weightA) < 0 {
				minW = weightB
			}
			minWF, _ := minW.Float64()
			overlapPct := stats.RoundTo2(minWF * 100.0)

			shared = append(shared, holdingEntry{
				key:        ki.key,
				symbol:     symbol,
				name:       name,
				weightA:    weightA,
				weightB:    weightB,
				overlapPct: overlapPct,
				isShared:   true,
			})
		} else {
			// Unique holding.
			unique = append(unique, holdingEntry{
				key:        ki.key,
				symbol:     symbol,
				name:       name,
				weightA:    weightA,
				weightB:    weightB,
				overlapPct: 0,
				isShared:   false,
			})
		}
	}

	// Sort shared by overlap % descending, then by symbol for stability.
	sort.Slice(shared, func(i, j int) bool {
		if shared[i].overlapPct != shared[j].overlapPct {
			return shared[i].overlapPct > shared[j].overlapPct
		}
		return shared[i].symbol < shared[j].symbol
	})

	// Sort unique by their weight (max of A, B) descending, then by symbol.
	sort.Slice(unique, func(i, j int) bool {
		maxI := unique[i].weightA
		if unique[i].weightB.Cmp(maxI) > 0 {
			maxI = unique[i].weightB
		}
		maxJ := unique[j].weightA
		if unique[j].weightB.Cmp(maxJ) > 0 {
			maxJ = unique[j].weightB
		}
		cmp := maxI.Cmp(maxJ)
		if cmp != 0 {
			return cmp > 0
		}
		return unique[i].symbol < unique[j].symbol
	})

	// Build final list: shared first, then unique.
	result := make([]MergedHolding, 0, len(shared)+len(unique))
	for _, e := range shared {
		result = append(result, MergedHolding{
			Symbol:     e.symbol,
			Name:       e.name,
			WeightA:    e.weightA,
			WeightB:    e.weightB,
			OverlapPct: e.overlapPct,
		})
	}
	for _, e := range unique {
		result = append(result, MergedHolding{
			Symbol:     e.symbol,
			Name:       e.name,
			WeightA:    e.weightA,
			WeightB:    e.weightB,
			OverlapPct: e.overlapPct,
		})
	}

	return result
}

// selectTopN selects the top N holdings from an expanded holdings map by weight.
func selectTopN(expanded map[string]*holdingInfoDisplay, limit int) map[string]*holdingInfoDisplay {
	if len(expanded) == 0 || limit <= 0 {
		return nil
	}
	if len(expanded) <= limit {
		// Return a copy to avoid aliasing.
		result := make(map[string]*holdingInfoDisplay, len(expanded))
		for k, v := range expanded {
			result[k] = v
		}
		return result
	}

	// Sort by weight descending.
	type entry struct {
		key  string
		info *holdingInfoDisplay
	}
	entries := make([]entry, 0, len(expanded))
	for k, v := range expanded {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].info.weight.Cmp(entries[j].info.weight) > 0
	})

	result := make(map[string]*holdingInfoDisplay, limit)
	for _, e := range entries[:limit] {
		result[e.key] = e.info
	}
	return result
}
