# Notes: Overlap Enhancement

## Implementation Notes

### Task 9: Update comparison template with enhanced overlap sections

- **Template functions added**: `weightPct` (converts decimal.Decimal fraction to percentage string) and `mapKeys` (returns map keys for iteration) added to `internal/web/renderer.go` FuncMap.
- **Sector/Country allocation tables**: Iterate Portfolio A's breakdown first, then add Portfolio B's unique keys. Each row shows both portfolios' percentages. "Unknown" row shown when either portfolio has unknown weight.
- **Empty-state handling**: When either portfolio has a `Message` field (e.g. "no holdings"), the section shows the message text instead of the table.
- **Merged holdings table**: Uses `weightPct` template function to convert decimal.Decimal weights to percentage display. Shows overlap % as pre-computed percentage points.
- **Overweight/Underweight/Neutral tables**: Use `weightPct` for weight display. Overweight shows "+X.XXpp" (heat-positive), underweight shows "X.XXpp" (heat-negative), neutral shows "0.00pp" (heat-neutral). Difference column uses `printf "%.2f" .Difference` directly since `Difference` is already a float64 percentage point value.
- **ECharts drift charts**: Two new chart scripts for sector and country drift. Diverging bar charts with blue (#4575b4) for positive (A overweight) and red (#d73027) for negative (B overweight). Labels show signed value with "pp" suffix. Y-axis reversed for top-to-bottom reading.
- **ECharts script condition**: Updated to include `.SectorDriftChart` and `.CountryDriftChart` so the library is loaded when drift charts are present.
- **Portfolio names throughout**: All section headers use `{{if .Result.PortfolioA}}{{.Result.PortfolioA.Name}}{{end}}` pattern for proper portfolio name display.

### Task 8: Chart serialization for sector/country drift charts

- **Drift chart data structure**: `driftChartData` with `Categories []string`, `Values []float64` (percentage points, positive = A overweight), `NameA`, `NameB`, `Note` (optional truncation message). Sorted by absolute difference descending, alphabetically for ties.
- **Country drift limit**: `serializeCountryDriftChart` truncates to top 15 countries by absolute difference (spec edge case). A `Note` field is set with the count of omitted countries. Sector drift chart has no limit (typically 11 GICS sectors). Constant `countryDriftLimit = 15` in `comparison_web.go`.
- **`computeAllocationDrift` helper**: Takes two breakdown maps (fractions 0.0-1.0), computes union of categories, drift = (A-B)*100 in percentage points. Returns sorted `allocationDrift` struct.
- **`roundTo2` helper**: Correctly handles negative numbers (Go's `int()` truncates toward zero, so `int(-5499.5) = -5499` not `-5500`). Uses sign extraction for correct rounding.
- **`mergedHoldingsData`**: Converts `MergedHolding` domain types to display-ready rows with weights as percentages and overlap as percentage points.
- **Wired into `buildPageData`**: Three new fields in `comparisonPageData` (`SectorDriftChart`, `CountryDriftChart`, `MergedHoldingsData`) populated alongside existing charts.
- **Empty-state handling**: All three functions return `{}` when data is unavailable (nil result, nil overlap, nil allocation, or empty holdings).

### Task 7: Wire enhanced overlap into the comparison service

- **Enrichment already present**: The `buildModelHoldings()` and `buildRealHoldings()` functions in `service.go` already had the enrichment code for `Sector`, `SectorWeightings`, and `GeographicAllocations` from a previous session. Verified by reading the code.
- **`computeOverlap` wiring**: The `computeOverlap` method already passes `PortfolioAName`/`PortfolioBName` to `CrossPortfolioOverlapInput`, which feeds into the sector/country warning prefixes.
- **Service-level tests added**: Two new tests in `service_test.go`:
  - `TestComputeComparison_EnhancedOverlap_FullPipeline` — model-vs-model with ETFs having sector/geographic/top holdings data, verifies full pipeline produces populated OverlapResult (sector allocations, country allocations, merged holdings, overweight/underweight)
  - `TestComputeComparison_EnhancedOverlap_IdenticalPortfolios` — identical portfolios verify zero drift and all neutral holdings; overlap percentage reflects expanded top holdings proportion (not 100% since top holdings are a subset of full ETF holdings)
- **Breakdown values are fractions**: `SectorAllocationResult.Breakdown` and `CountryAllocationResult.Breakdown` use fraction values (0.58 = 58%), not percentage values. Test assertions adjusted accordingly.
- **`strings` import added**: Added `"strings"` to the import block in `service_test.go` for the warning prefix check.

### Task 6: Extend OverlapResult type and integrate in ComputeCrossPortfolioOverlap

- **Types placement follows NOTES.md from Tasks 2–5**: `SectorAllocationResult`, `CountryAllocationResult`, and `AllocationEntry` live in `allocation.go` (not `types.go`). `MergedHolding` and `WeightDifferenceHolding` already live in `types.go` since `OverlapResult` references them directly.
- **Warnings from allocation computations**: Sector/country warnings use portfolio names (e.g. `[sector My Model]`, `[country My Real]`). `CrossPortfolioOverlapInput` gained `PortfolioAName`/`PortfolioBName` fields; defaults to "A"/"B" when names are empty (backward compatible for tests).
- **Existing test fix**: `TestComputeCrossPortfolioOverlap_ETFWithNoHoldings` expected exactly 1 warning — updated to check `>= 1` and verify the UNKNOWN_ETF warning is present by prefix, since enhanced overlap adds sector/country warnings.
- **Integration tests**: Three new tests — populated fields (ETF+stock with sector/geo data), empty portfolio B (graceful degradation), identical portfolios (100% overlap, zero drift, all neutral).

### Task 4: Merged holdings computation

- **MergedHolding type in types.go**: The plan placed `MergedHolding` in `types.go` (TD-4) and it was added there since `OverlapResult` (in Task 6) will reference it. This follows the cross-reference rule.
- **Key resolution uses existing `expandETFHoldingsDisplay`**: Reuses the existing `keyBest` expansion from `overlap.go` rather than creating a separate key level resolution. Both portfolios use the same best-available key (ISIN > Symbol > Name) for matching.
- **`selectTopN` helper**: Extracted as a standalone function for selecting top N holdings from an expanded map. Used by `ComputeMergedHoldings` to apply the per-portfolio limit before merging.
- **`expandETFHoldingsWithKey` removed**: Was added as a variant accepting a `keyLevel` parameter but never called by `ComputeMergedHoldings` (which uses `expandETFHoldingsDisplay` with `keyBest` directly). Removed during implementation review as dead code.
- **Overlap % rounding**: Uses `stats.RoundTo2` for consistency with other percentage calculations in the comparison domain.

### Task 5: Overweight/underweight/neutral holdings computation

- **`WeightDifferenceHolding` type in types.go**: Added alongside `MergedHolding` since both are display-oriented result types referenced by `OverlapResult` (Task 6).
- **Difference as percentage points**: `Difference` field is `(weightA - weightB) * 100` in percentage points, using `stats.RoundTo2` for consistency with other percentage calculations.
- **Three return values**: `overweight, underweight, neutral`. Zero-difference holdings (identical weights in both portfolios) go into `neutral`, sorted by weight desc. This shows where portfolios align — the point of the overlap analysis.
- **Reuses `expandETFHoldingsDisplay`**: Same expansion pattern as Task 4 (merged holdings), using `keyBest` for matching.

### Task 2+3: Sector and country allocation computation

### Task 2+3: Sector and country allocation computation

- **Tasks 2 and 3 implemented together**: Both functions share the same file (`allocation.go`), test file, result type pattern, and `normalizeSector` helper. Combined them in one pass rather than splitting across two sessions.
- **Result types in allocation.go, not types.go**: The plan placed `SectorAllocationResult` and `CountryAllocationResult` in `types.go` (TD-3). They live in `allocation.go` instead since they're only used by the allocation functions, not by other domain code. `AllocationEntry` and the sorted helper functions follow the same placement.

### Task 1: PortfolioHolding enrichment

- **Sector field added alongside SectorWeightings**: The plan specified `SectorWeightings []symbol.SectorWeighting` and `GeographicAllocations []symbol.GeographicAllocation`, but stocks use `SymbolDetails.Sector` (a single primary sector string), not `SectorWeightings` (a slice used by ETFs). Added `Sector string` to `PortfolioHolding` to cover both cases, matching the pattern in `analysis/allocation.go`.

### Task 10: End-to-end integration test

- **Two of three test cases already covered by Task 7**: `TestComputeComparison_EnhancedOverlap_FullPipeline` (full pipeline) and `TestComputeComparison_EnhancedOverlap_IdenticalPortfolios` (identical portfolios) were added during Task 7 and satisfy those Task 10 requirements.
- **New test added**: `TestComputeComparison_EnhancedOverlap_OneSideMissingData` — Portfolio A (VOO with full sector/geographic/top holdings data) vs Portfolio B (VXUS with zero sector/geographic/top holdings data). Verifies:
  - SectorAllocationA and CountryAllocationA populated from VOO data
  - SectorAllocationB/CountryAllocationB degrade gracefully (Unknown bucket or empty)
  - MergedHoldings still produced (3 entries: VOO's top holdings expanded, VXUS as atomic)
  - Warnings include `[sector Minimal Data] VXUS: no sector data for ETF` and `[country Minimal Data] VXUS: no geographic data`
  - No panic, result struct fully valid

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
