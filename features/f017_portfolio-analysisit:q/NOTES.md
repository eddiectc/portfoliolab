# Notes: Portfolio Analysis

## Decisions
- 2025-05-17: Task 1 complete — domain types defined in `internal/domain/analysis/analysis_types.go`. All types use `float64` for percentage/weight fields (consistent with `symbol.SymbolDetails` which stores percentages as float64). `EstimatedDollarImpact` uses `decimal.Decimal` since it represents an actual monetary amount.
- 2025-05-17: `PositionWithDetails` was already defined in Task 1 (`analysis_types.go`), so Task 2 reused it without modification.
- 2025-05-17: `roundTo2` helper uses `int(v*100+0.5)/100` for float64 rounding — simple, no math library dependency, consistent with display-only precision.
- 2026-05-17: Task 3 complete — `roundTo2` fixed to use `math.Round(v*100)/100` instead of `int(v*100+0.5)/100`. The original formula truncated toward zero for negative values (e.g. -0.999 → -0.99 instead of -1.0), which broke negative correlation display. `math.Round` handles both positive and negative correctly.

## Deviations from Plan
- Task 2: Plan said single ETF returns empty for both pairwise and concentrated stocks. Changed to still compute concentrated stocks for 1 ETF — useful to see what a single ETF is most concentrated in, even without pairwise comparison. Pairwise still requires 2+ ETFs.

## Future Improvements
- N/A

## Known Issues
- None.

## Session 2026-05-17 (Task 4)

### Prerequisite: Add `sector` field for individual stocks
The plan referenced `assetProfile.Sector` for individual stocks, but this field was fetched from Yahoo Finance and not persisted. Added:
- Migration 018: `sector TEXT` column on `symbol_details` table
- `Sector` field on `SymbolDetails` struct (in `internal/types/symbol/symbol_details.go`)
- Fetcher populates `Sector` from `assetProfile.Sector` for non-ETF symbols
- Updated sqlc schema + queries (`symbol_details.sql`, `schema.sql`)
- Updated repo to serialize/deserialize the field
- Updated test schema in `symbol_details_repo_test.go`
- Ran `sqlc generate` to regenerate types

### Implementation
- `internal/domain/analysis/allocation.go`: `ComputeSectorAllocation` and `ComputeGeographicAllocation`
- `internal/domain/analysis/allocation_test.go`: 17 unit tests covering happy paths, edge cases, partial data, empty states, and sorting
- Helper functions `getETFWithSectorWeightings`, `getStockWithSector`, `getETFWithGeographicAllocations`, `getStockWithCountry` for test fixtures
- `AllocationBreakdownSorted` convenience function for ordered output
- `normalizeSector` placeholder for consistent sector naming (currently passthrough — Yahoo sectors are already consistent)

## Session 2026-05-18 (Task 5)

### Implementation
- `internal/domain/analysis/stress_scenarios.json`: 6 predefined historical crisis scenarios with GICS sector peak-to-trough returns (2008 GFC, 2000 Dot-Com, 2020 COVID, 2022 Decline, 1997 Asian Crisis, 2011 European Debt)
- `internal/domain/analysis/stress_scenarios.go`: `StressScenario` struct, `LoadPredefinedScenarios` loader, `PredefinedScenarios` package-level variable populated via `init()` with `//go:embed`
- `internal/domain/analysis/stress_test.go`: `ComputeStressTests` — applies sector returns × portfolio weights, computes dollar impact, sorts by severity
- `internal/domain/analysis/stress_test_test.go`: 13 unit tests (happy path, single sector, equal weight, unknown weight warning, zero/negative portfolio value, empty/nil allocation, sector not in scenario, sort order, mixed positive/negative returns, scenario loading)

### Decision: JSON file location
Plan specified `internal/config/data/stress_scenarios.json` but `//go:embed` only supports files in the same directory or subdirectories of the Go source file. Moved JSON to `internal/domain/analysis/stress_scenarios.json` alongside the Go code. This keeps the data co-located with the code that consumes it.

## Session 2026-05-18 (Task 6)

### Implementation
- `internal/domain/analysis/factor_exposure.go`: `ComputeFactorExposure` — computes value/growth tilt (weighted P/E, P/B vs S&P 500 reference), size tilt (large/mid/small cap from TotalNetAssets), concentration (HHI from underlying holdings), top holding weight
- `internal/domain/analysis/factor_exposure_test.go`: 14 unit tests + 22 sub-tests for helper functions (classifyTilt, combineTilts, classifySizeTilt, classifyHHI)

### Decision: Function signature
Plan specified `ComputeFactorExposure(positions []PositionWithDetails, portfolioValue decimal.Decimal)` but `portfolioValue` isn't used in the computation (unlike stress tests which compute dollar impact). Omitted the unused parameter to follow Go conventions.

### Decision: Size classification thresholds
- Large cap: ≥ $10B TotalNetAssets
- Mid cap: ≥ $2B TotalNetAssets
- Small cap: < $2B TotalNetAssets
- Stocks without FundProfile are excluded from size classification (no market cap data in cached symbol details)

### Decision: Value/growth tilt logic
- Benchmark: S&P 500 reference P/E=20, P/B=4
- Neutral band: ±15% of benchmark (P/E: 17-23, P/B: 3.4-4.6)
- P/E and P/B tilts combined: if both agree → that tilt; if one empty → use the other; if disagree → "neutral"; if both empty → "neutral"

### Decision: HHI computation
- Weights as fractions (0-1), not percentages
- For ETFs: looks through to top holdings (portfolio_weight × holding_percent / 10000)
- For stocks: position weight / 100
- For ETFs without holdings data: falls back to position weight / 100 with warning
- Thresholds: <0.02 well-diversified, 0.02-0.05 moderately-concentrated, >0.05 highly-concentrated
- Uses `roundTo4` (4dp) instead of `roundTo2` — HHI operates in 0-1 range where 2dp zeros out diversified portfolios (e.g., 0.0026 → 0.00)

### Silent failure fixes
- **HHI precision**: Changed from `roundTo2` to `roundTo4` so diversified portfolios show meaningful HHI values (e.g., 0.0026 not 0.00)
- **Partial size coverage**: Added warning when `sizeTrackedWeight < 100` (some positions lack FundProfile), e.g., "size data available for only 60.00% of portfolio"
- **Tilt sentinel**: When no valuation data exists, tilt is "unavailable" (not "neutral") — distinguishes "no data" from "within benchmark band"
- **Size percentages**: Changed from "relative to tracked weight" to "relative to total portfolio" — so they sum to <100% when coverage is partial. The gap is the signal (e.g., 60% large + 40% gap = missing data).
- **Concentration for nil SymbolDetails**: Instead of silently skipping positions with no symbol details, use position weight directly for HHI/top-holding (same fallback as ETFs without holdings data). Warning updated to say "using position weight for concentration only".
- Added `roundTo4` helper to `overlap.go` alongside existing `roundTo2`

### Additional factors (Quality, Cost, Momentum, Volatility)
- **Signature change**: `ComputeFactorExposure` now takes `pricesBySymbol map[string][]market.HistoricalPrice` as second param (pass `nil` for static-only)
- **Quality**: Weighted P/CF and P/Sales from `EquityValuation`, compared to S&P 500 reference (10x P/CF, 2.5x P/Sales) with ±20% neutral band. Lower = better quality.
- **Cost**: Weighted expense ratio and holdings turnover from `FundProfile`. No tilt classification — reported as raw values.
- **Momentum**: Portfolio-weighted 3M/6M/12M returns from price history. Tilt based on majority of windows above/below ±2% threshold.
- **Volatility**: Portfolio-weighted annualized volatility (daily std dev × √252). Thresholds: ≤10% low, ≤20% medium, >20% high.
- `getETFWithFullData` test helper added for tests needing P/CF, P/Sales, expense ratio, and turnover.
- `makePriceMap` test helper for constructing price history from `priceEntry` slices.

## Session 2026-05-18 (Task 8)

### Implementation
- `internal/api/handlers/analysis.go`: `AnalysisHandler` with `analysisService` interface, `HandleAnalysis` handler, `parseAnalysisFilters` query param parser
- `internal/api/handlers/analysis_test.go`: 17 tests covering success, no params, section filter, period filter, all filters combined, internal error, empty state, warnings, route registration, filter parsing (6 tests), JSON round-trip, and omitempty behavior

### Decision: Interface on handler
Plan specified `*analysis.Service` as the handler dependency. Changed to an `analysisService` interface (`ComputeAnalysis` method) to enable hand-written mocks in tests. This follows Go best practices (depend on interfaces, not concrete types) and matches the testing pattern used throughout the project.

### Decision: No response DTO
Plan mentioned "Define response DTO matching `AnalysisResult` structure." Skipped this — `AnalysisResult` already has proper `json` struct tags and is the canonical response type. Creating a separate DTO would be redundant duplication. The `AnalysisResult` type IS the response DTO.

## Session 2026-05-18 (Task 7)

### Bug fix: `stress_test.go` naming
- File `stress_test.go` contained implementation code but was named with `_test.go` suffix, causing Go to exclude it from normal builds. Renamed to `stress.go`.

### Implementation
- `internal/domain/analysis/service.go`: `Service` struct with 7 interfaces, `ComputeAnalysis()` orchestration method
- `internal/domain/analysis/service_test.go`: 10 service tests with hand-written mocks

### Interfaces defined
- `PositionSource` — get/enrich positions (matches `position.Service`)
- `SymbolDetailsSource` — get symbol details (matches `symbols.Service`)
- `MarketDataHistorySource` — get historical prices (matches `position.MarketDataService`)
- `AccountResolver` — resolve accounts from portfolio or all
- `PortfolioCurrencySource` — get portfolio base currency
- `MarketDataSymbolResolver` — map internal symbol → market data provider symbol (new, needed for fetching historical prices)
- `SymbolRefresher` — trigger background refresh of stale symbol details (optional, set via `WithSymbolRefresher`)

### Design decisions
- **Nil-tolerant dependencies**: `symbolDetails`, `marketHistory`, `marketDataSymbol`, `symbolRefresher` can be nil — service returns empty results rather than panicking
- **Stress test auto-computes sector allocation**: When only `stress_test` section is requested, sector allocation is computed internally (not included in result) since stress tests depend on it
- **Background refresh is fire-and-forget**: Stale symbol refresh runs in a goroutine without blocking the response
- **Portfolio weight from enriched positions**: Uses `MarketValueBase` if available (FX-converted), falls back to `MarketValue` in position currency
- **Period defaults to 1Y**: When filters.Period is empty, defaults to "1Y" for correlation lookback

### Silent failure audit
Fixed 9 silent failures/fallbacks:
1. **Overlap condition**: Rewrote tangled boolean (`computeSection && ... || filters.Section == ""`) to simple `wantAll || filters.Section == string(SectionX)` pattern
2. **resolveAccounts currency error**: Now returns warning "could not determine portfolio base currency" instead of logging at Debug only
3. **resolveAccounts no-portfolio path**: Error from GetPortfolioCurrency no longer silently discarded
4. **fetchSymbolDetails nil source**: Returns warning "symbol details source not configured" instead of empty map silently
5. **fetchSymbolDetails missing symbols**: Returns warning listing missing symbols instead of Debug log only
6. **fetchHistoricalPrices nil deps**: Returns warning "historical price data source not configured" instead of empty map silently
7. **fetchHistoricalPrices missing symbols**: Tracks and reports symbols with no price data
8. **buildPositionWithDetails zero total**: Returns warning "no market data available" instead of empty slice silently
9. **EnrichWithMarketData missing data**: Added check counting positions with `MarketDataAvailable=false` and emits warning

All data-fetching helpers now return `([]string)` warnings alongside their results. Warnings are collected in `ComputeAnalysis` before section computation and prepended to `result.Warnings`.
