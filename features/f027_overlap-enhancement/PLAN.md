# Implementation Plan: Overlap Enhancement

## Overview

Enhance the Holdings Overlap section on the portfolio comparison page with sector overlap, country allocation, and richer holdings analysis (merged table, overweight/underweight).

## Technical Decisions

### TD-1: Where to compute sector/country allocation for comparison

**Decision**: Add new computation functions in `internal/domain/comparison/allocation.go` that accept `[]PortfolioHolding` (the comparison domain's holding type) and produce sector/country breakdowns. These mirror the logic from `analysis/allocation.go` but operate on the comparison-specific data structures.

**Rationale**:
- The comparison service already builds `[]PortfolioHolding` with symbol details (TopHoldings, QuoteType, Name). Adding `SectorWeightings` and `GeographicAllocations` to this enrichment is a small change.
- The analysis domain's `ComputeSectorAllocation` works on `[]PositionWithDetails` (float64 weights + SymbolDetails pointer). The comparison domain uses `decimal.Decimal` weights and inline fields. Duplicating the computation logic avoids introducing a cross-domain dependency and keeps the comparison self-contained.
- The computation functions are ~40 lines each and are pure functions (easy to test).

**Pros**: Self-contained comparison domain, no cross-dependency, follows existing pattern (overlap.go already has pure computation functions)
**Cons**: Some code duplication with analysis/allocation.go (~80 lines total)

### TD-2: How to enrich PortfolioHolding with sector/country data

**Decision**: Add `SectorWeightings` and `GeographicAllocations` fields to the `PortfolioHolding` struct, populated from `SymbolDetails` during the `buildModelHoldings` and `buildRealHoldings` calls in service.go.

**Rationale**: The symbol details already contain this data. It's a natural extension of the existing enrichment pattern (QuoteType, ShortName, TopHoldings are already pulled from SymbolDetails).

**Pros**: Minimal change to service layer, data available at computation time
**Cons**: Adds 2 fields to PortfolioHolding (only used by allocation computation)

### TD-3: How to structure the enhanced overlap result

**Decision**: Extend the existing `OverlapResult` type with new optional fields:
- `SectorAllocationA`, `SectorAllocationB` (map[string]float64)
- `CountryAllocationA`, `CountryAllocationB` (map[string]float64)
- `MergedHoldings` ([]MergedHolding)
- `OverweightHoldings` ([]WeightDifferenceHolding)
- `UnderweightHoldings` ([]WeightDifferenceHolding)
- `SectorMissingSymbolsA`, `SectorMissingSymbolsB` ([]string)
- `CountryMissingSymbolsA`, `CountryMissingSymbolsB` ([]string)

**Rationale**: Keeps everything under `CrossMetrics.Overlap` in the template. No need for new top-level result types.

**Pros**: Single access point in template, backward compatible
**Cons**: OverlapResult grows significantly (mitigated by using omitempty)

### TD-4: Merged holdings — data structure

**Decision**: New types in `comparison/types.go`:
- `MergedHolding` — symbol, name, weightA, weightB (as fractions), overlapPct
- `WeightDifferenceHolding` — symbol, name, weightA, weightB, difference

**Rationale**: Clean separation from existing `HoldingWeight` type. The merged table has different semantics (overlap % as absolute pp of min weight) from the existing top holdings display.

### TD-5: Chart serialization for drift charts

**Decision**: Add new serialization functions in `comparison_web.go` that produce JSON for ECharts diverging bar charts. The drift charts (sector and country) use a single series with positive/negative values, colored by CSS class or ECharts visualMap.

**Rationale**: Follows existing pattern (serializeValueGrowthChartData, etc.). ECharts is already used on the comparison page.

---

## Task List

### Task 1: Add sector/country fields to PortfolioHolding and enrich from SymbolDetails
**Files**: `internal/domain/comparison/overlap.go`, `internal/domain/comparison/service.go`

- [x] Add `Sector` (string, for stocks), `SectorWeightings []symbol.SectorWeighting` and `GeographicAllocations []symbol.GeographicAllocation` fields to `PortfolioHolding`
- [x] In `buildModelHoldings()` and `buildRealHoldings()`, populate these fields from SymbolDetails (alongside existing QuoteType, ShortName, TopHoldings)
- [x] Add `portfolioHoldingETFWithSector` and `portfolioHoldingStockWithSector` test helpers for enriched holdings
- [x] Update existing tests to compile

**Tests**: All existing overlap_test.go and service_test.go tests pass with new fields.

---

### Task 2: Implement sector allocation computation for comparison
**Files**: `internal/domain/comparison/allocation.go`, `internal/domain/comparison/allocation_test.go`

- [x] Create `ComputeSectorAllocationForHoldings(holdings []PortfolioHolding) *SectorAllocationResult`
- [x] Logic mirrors `analysis/allocation.go::ComputeSectorAllocation` but operates on `PortfolioHolding`:
  - For ETFs: iterate SectorWeightings, weight = holding.Weight × sector.Percent/100
  - For stocks: use SymbolDetails.Sector (primary sector), full weight
  - Missing data → "Unknown" bucket + warning list
- [x] Return type: `SectorAllocationResult` with `Breakdown map[string]float64`, `UnknownWeightPct float64`, `Warnings []string`, `MissingSymbols []string`
- [x] Add `normalizeSector` function (reuse from analysis or copy)

**Tests**:
- [x] ETF with sector weightings → correct weighted breakdown
- [x] Stock with primary sector → full weight to that sector
- [x] Mixed ETF + stock portfolio
- [x] Missing sector data → Unknown bucket + warning
- [x] Empty holdings → empty result with message

---

### Task 3: Implement country allocation computation for comparison
**Files**: `internal/domain/comparison/allocation.go`, `internal/domain/comparison/allocation_test.go`

- [x] Create `ComputeCountryAllocationForHoldings(holdings []PortfolioHolding) *CountryAllocationResult`
- [x] Logic mirrors `analysis/allocation.go::ComputeGeographicAllocation` but operates on `PortfolioHolding`:
  - For all holdings: iterate GeographicAllocations, weight = holding.Weight × country.Percent/100
  - Missing data → "Unknown" bucket + warning list
- [x] Return type: `CountryAllocationResult` with `Breakdown map[string]float64`, `UnknownWeightPct float64`, `Warnings []string`, `MissingSymbols []string`

**Tests**:
- [x] ETF with geographic allocations → correct weighted breakdown
- [x] Stock with single country → full weight to that country
- [x] Mixed portfolio
- [x] Missing geographic data → Unknown bucket + warning
- [x] Empty holdings → empty result

---

### Task 4: Implement merged holdings computation
**Files**: `internal/domain/comparison/overlap_enhanced.go`, `internal/domain/comparison/overlap_enhanced_test.go`

- Create `ComputeMergedHoldings(holdingsA, holdingsB []PortfolioHolding, limit int) []MergedHolding`
- Expand both portfolios to underlying holdings (reuse `expandETFHoldingsDisplay`)
- Merge into single list: shared holdings first (sorted by min-weight overlap % desc), then unique holdings (sorted by their weight desc, interleaved)
- Limit to top N from A + top N from B (deduplicated)
- Overlap % = min(weightA, weightB) × 100 (absolute percentage points)
- For unique holdings: overlap % = 0

**Tests**:
- Two portfolios with shared and unique holdings
- All holdings shared (identical portfolios) → 100% overlap for all
- No shared holdings → all unique, sorted by weight
- Limit respected (top 10 + top 10 deduplicated)
- ETF expansion works correctly

---

### Task 5: Implement overweight/underweight holdings computation
**Files**: `internal/domain/comparison/overlap_enhanced.go`, `internal/domain/comparison/overlap_enhanced_test.go`

- Create `ComputeWeightDifferences(holdingsA, holdingsB []PortfolioHolding, limit int) ([]WeightDifferenceHolding, []WeightDifferenceHolding)`
- Expand both portfolios to underlying holdings
- Compute difference = weightA - weightB for each holding
- Positive differences → overweight (A > B), sorted by difference desc, limited to top N
- Negative differences → underweight (B > A), sorted by abs(difference) desc, limited to top N
- Include holdings where one weight is 0 (not present in one portfolio)

**Tests**:
- Clear overweight/underweight cases
- Identical portfolios → both lists empty
- One portfolio empty → all holdings are overweight for the non-empty side
- Limit respected

---

### Task 6: Extend OverlapResult type and integrate in ComputeCrossPortfolioOverlap
**Files**: `internal/domain/comparison/types.go`, `internal/domain/comparison/overlap.go`

- Add new fields to `OverlapResult`:
  - `SectorAllocationA`, `SectorAllocationB *SectorAllocationResult`
  - `CountryAllocationA`, `CountryAllocationB *CountryAllocationResult`
  - `MergedHoldings []MergedHolding`
  - `OverweightHoldings []WeightDifferenceHolding`
  - `UnderweightHoldings []WeightDifferenceHolding`
- Add new types to `types.go`: `SectorAllocationResult`, `CountryAllocationResult`, `MergedHolding`, `WeightDifferenceHolding`, `AllocationEntry`
- In `ComputeCrossPortfolioOverlap`, after computing existing overlap:
  - Call `ComputeSectorAllocationForHoldings` for both portfolios
  - Call `ComputeCountryAllocationForHoldings` for both portfolios
  - Call `ComputeMergedHoldings` with limit 10
  - Call `ComputeWeightDifferences` with limit 10
- The function signature changes: it now needs the full `PortfolioHolding` slices (not just the expanded versions), which it already has via `input.PortfolioA` and `input.PortfolioB`

**Tests**: Integration test that verifies all new fields are populated when sector/country data is available.

---

### Task 7: Wire enhanced overlap into the comparison service
**Files**: `internal/domain/comparison/service.go`, `internal/domain/comparison/service_test.go`

- In `buildModelHoldings()` and `buildRealHoldings()`, enrich with `SectorWeightings` and `GeographicAllocations` from SymbolDetails
- The `computeOverlap` method already passes `[]PortfolioHolding` to `ComputeCrossPortfolioOverlap`, so the enhanced computation is automatic
- Verify that sector/country data flows through for both model and real portfolios

**Tests**:
- Model portfolio with ETF having sector/geographic data → data present in holdings
- Real portfolio with allocation source → data present in holdings
- Service-level test verifying OverlapResult has sector/country fields populated

---

### Task 8: Add chart serialization for sector/country drift charts
**Files**: `internal/api/handlers/comparison_web.go`

- Add `serializeSectorDriftChart(result *ComparisonResult) string` — produces diverging bar chart JSON
- Add `serializeCountryDriftChart(result *ComparisonResult) string` — produces diverging bar chart JSON
- Add `serializeMergedHoldings(result *ComparisonResult) string` — produces merged holdings data
- Add corresponding JSON types for chart data
- Add new fields to `comparisonPageData`: `SectorDriftChart`, `CountryDriftChart`, etc.

**Tests**: Unit tests for serialization functions (verify JSON structure and values).

---

### Task 9: Update comparison template with enhanced overlap sections
**Files**: `templates/comparison/index.html`

- Replace existing "Holdings Overlap" section with enhanced version:
  1. **Sector Allocation** — side-by-side table (sector | Portfolio A % | Portfolio B %) + drift chart
  2. **Country Allocation** — side-by-side table (country | Portfolio A % | Portfolio B %) + drift chart
  3. **Merged Holdings** — single table (Name | Weight A | Weight B | Overlap %)
  4. **Overweight Holdings** — table (Name | Weight A | Weight B | Difference)
  5. **Underweight Holdings** — table (Name | Weight A | Weight B | Difference)
- Add warning messages for missing sector/country data
- Add empty-state messages when no allocation data available
- Add ECharts script blocks for sector/country drift charts (diverging bar charts)
- Use portfolio names (not "A"/"B") in all labels

**Tests**: Manual verification (template rendering). Integration test that verifies page renders without errors when overlap data is present.

---

### Task 10: End-to-end integration test
**Files**: `internal/domain/comparison/service_test.go` or new `internal/domain/comparison/allocation_integration_test.go`

- Full pipeline test: model portfolios with ETFs having sector/geographic/top holdings data → ComputeComparison → verify OverlapResult has all new fields populated
- Edge case: one portfolio with data, one without → verify graceful degradation
- Edge case: both portfolios identical → verify zero drift, empty overweight/underweight

---

## Task Dependencies

```
Task 1 (enrich PortfolioHolding)
    ├── Task 2 (sector allocation computation)
    ├── Task 3 (country allocation computation)
    ├── Task 4 (merged holdings)
    └── Task 5 (overweight/underweight)
            ├── Task 6 (extend OverlapResult + integrate)
            └── Task 7 (wire into service)
                    ├── Task 8 (chart serialization)
                    └── Task 9 (template)
                            └── Task 10 (integration test)
```

Tasks 2-5 can be done in parallel after Task 1. Tasks 6-7 depend on 2-5. Tasks 8-9 depend on 7. Task 10 is final.

## Risk Assessment

| Risk | Mitigation |
|------|-----------|
| Sector/country data missing for some symbols | "Unknown" bucket + warning message, as per spec |
| Real portfolios may not have sector data for all positions | Allocation service already resolves symbol details; if missing, falls back to Unknown |
| Merged holdings limit (top 10 + top 10) may hide important holdings | Spec constraint; documented in UI |
| Template complexity grows significantly | Each section is self-contained; follows existing pattern |
