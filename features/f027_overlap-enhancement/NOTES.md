# Notes: Overlap Enhancement

## Implementation Notes

### Task 1: PortfolioHolding enrichment

- **Sector field added alongside SectorWeightings**: The plan specified `SectorWeightings []symbol.SectorWeighting` and `GeographicAllocations []symbol.GeographicAllocation`, but stocks use `SymbolDetails.Sector` (a single primary sector string), not `SectorWeightings` (a slice used by ETFs). Added `Sector string` to `PortfolioHolding` to cover both cases, matching the pattern in `analysis/allocation.go`.

## Planning Notes

### Codebase Analysis

- **Existing overlap**: `comparison/overlap.go` has `ComputeCrossPortfolioOverlap` that expands ETFs to underlying holdings and computes weighted overlap. The `PortfolioHolding` type already carries `TopHoldings` (for ETF expansion), `QuoteType`, `Name`, `Symbol`, `Weight`.
- **Existing sector/country**: `analysis/allocation.go` has `ComputeSectorAllocation` and `ComputeGeographicAllocation` that work on `[]PositionWithDetails` (analysis domain type with `float64` weights + `*symbol.SymbolDetails`). These functions do ETF look-through for sectors/countries.
- **Comparison service enrichment**: `buildModelHoldings()` and `buildRealHoldings()` in `service.go` already pull `QuoteType`, `ShortName`, `TopHoldings` from `SymbolDetails`. Adding `SectorWeightings` and `GeographicAllocations` is a natural extension.
- **Symbol details**: `symbol.SymbolDetails` already has `SectorWeightings`, `GeographicAllocations`, `Sector` (primary for stocks), `Country` (via GeographicAllocations).

### Key Design Decisions

1. **Duplicate allocation computation logic** rather than introducing cross-domain dependency. The comparison domain already has self-contained computation (overlap.go, metrics.go, correlation.go). Adding allocation.go follows this pattern.

2. **Extend existing `OverlapResult`** rather than creating new result types. Keeps the template access path simple (`Result.CrossMetrics.Overlap.SectorAllocationA`).

3. **New types in `types.go`**: `SectorAllocationResult`, `CountryAllocationResult`, `MergedHolding`, `WeightDifferenceHolding`, `AllocationEntry` — all with `json` tags for API compatibility.

4. **Merged holdings file**: `overlap_enhanced.go` for merged + overweight/underweight computations, keeping the existing `overlap.go` focused on the core overlap computation.

### Spec Clarifications

- **Overlap % for merged table**: Spec says "absolute percentage points of the smaller weight (min of the two weights)". This means `min(weightA, weightB) * 100` as a percentage, not a ratio.
- **Merged holdings limit**: "top 10 from A + top 10 from B (deduplicated)" — meaning up to 20 unique holdings.
- **Drift direction**: Always Portfolio A minus Portfolio B (selection order).
- **Country limit**: Top 15 by absolute difference for drift chart, with note about remaining.

### Potential Spec Drift

- The spec mentions "sector drift chart" and "country drift chart" as diverging bar charts. The existing comparison page uses ECharts. These will be implemented as ECharts bar charts with positive/negative values.
- The spec says "holdings unique to either portfolio are interleaved, not separated into subsections" in the merged table. This means unique holdings from A and B are mixed together, sorted by their weight in their respective portfolio.
