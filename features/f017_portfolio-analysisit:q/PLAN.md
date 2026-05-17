# Implementation Plan: Portfolio Analysis

## Overview

Build a portfolio analysis system that computes five analytical lenses (ETF overlap, correlation matrix, sector/geographic allocation, stress testing, factor exposure) from existing cached data (positions, symbol details, historical prices). All computation lives in a new `analysis` domain package. A single API endpoint returns all sections (with optional section filtering). A web page renders the results with ECharts visualizations.

## Task Dependencies

```
Task 1 (domain types)
    ↓
Task 2 (overlap) ←───────────────────────────┐
Task 3 (correlation)                         │
Task 4 (allocation)                          │
Task 5 (stress test) ← Task 4 (sector data) │
Task 6 (factor exposure)                     │
    ↓                                        │
Task 7 (analysis service) ←──────────────────┘
    ↓
Task 8 (API handler)
    ↓
Task 9 (web handler + template)
    ↓
Task 10 (router + nav wiring)
All → Task 11 (validation)
```

Tasks 2–6 are independent of each other after Task 1 (pure computation, no shared state). Tasks 7–10 are sequential integration.

## Tasks

### Task 1: Domain types for analysis [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundational types)
**Description:** Define the domain types, result envelope, and filter structures used by the analysis layer.

- [x] Create `internal/domain/analysis/analysis_types.go`:
  - `AnalysisResult` — top-level envelope with all sections (Overlap, Correlation, SectorAllocation, GeographicAllocation, StressTest, FactorExposure), ComputedAt, PortfolioID, Warnings, Message (for empty state)
  - `AnalysisFilters` — PortfolioID *int64, Section string (optional filter), Period string (for correlation lookback)
  - `OverlapResult` — PairwiseMatrix (map of ETF pair → overlap info), TopConcentratedStocks (sorted list), Message (empty state)
  - `OverlapPair` — ETF A ticker, ETF B ticker, OverlappingCount, CombinedWeightPct
  - `ConcentratedStock` — Symbol, Name, TotalWeightPct, HeldByETFs ([]string)
  - `CorrelationResult` — Matrix ([] []float64 or nullable), Symbols ([]string row/col labels), Period string, Warnings ([]string), Message (empty state)
  - `AllocationResult` — Breakdown (map[string]float64, sector/region → weighted %), UnknownWeightPct, Warnings, Message
  - `StressTestResult` — Scenarios ([]StressScenarioResult), Message (empty state)
  - `StressScenarioResult` — Name, DateRange, EstimatedReturnPct, EstimatedDollarImpact, SectorContributions (map[string]float64)
  - `FactorExposureResult` — ValueGrowthTilt (P/E, P/B vs benchmark), SizeTilt, Concentration (HHI + interpretation), TopHoldingWeightPct, Warnings, Message
- [x] Define `AnalysisSection` const/string type: "overlap", "correlation", "sector_allocation", "geographic_allocation", "stress_test", "factor_exposure"
- [x] Write unit tests for model JSON serialization (marshal/unmarshal of decimal fields, nullable sections)

**Verification:** Types compile, JSON serialization round-trips correctly, nullable sections serialize as `null`. ✅

---

### Task 2: ETF overlap computation [PRIORITY: HIGH]
**Corresponds to:** Scenario: View ETF overlap pairwise matrix, Scenario: View top concentrated stocks across ETFs
**Description:** Pure computation that takes a set of ETF positions with cached holdings and computes pairwise overlap and top concentrated stocks.

- [x] Create `internal/domain/analysis/overlap.go`
- [x] Implement `ComputeOverlap(positions []PositionWithDetails) *OverlapResult`:
  - Filter to ETF positions only (QuoteType == "ETF")
  - For each ETF pair, find common underlying symbols from TopHoldings
  - Compute overlap count and combined portfolio weight (sum of each ETF's weight × position weight for overlapping holdings)
  - Build pairwise matrix (symmetric, diagonal omitted or self-reference)
  - Aggregate all underlying holdings across ETFs: sum weight per symbol across all ETFs holding it
  - Sort by total weight descending, take top 10
  - Handle empty state: no ETFs → message, 1 ETF → message (need 2+ for pairwise)
- [x] Define `PositionWithDetails` input struct: Symbol, PortfolioWeight (position market value / total portfolio value), SymbolDetails (*symbol.SymbolDetails)
- [x] Write table-driven unit tests:
  - Happy path: 3 ETFs with known overlapping holdings
  - Zero overlap between some pairs
  - Single ETF (message)
  - No ETFs (message)
  - ETFs with no cached holdings (excluded, warning)
  - Top concentrated stocks correctly aggregated across ETFs
  - Weighted overlap calculation (ETF weight × holding percent)

**Verification:** Given known ETF holdings and portfolio weights, overlap matrix and concentrated stocks match expected values.

**Technical Decision A — Overlap input:**

| Option | Pros | Cons |
|--------|------|------|
| **A: Accept `[]PositionWithDetails`** (chosen) | Service pre-fetches positions + symbol details in one pass; computation function is pure | Input struct is analysis-specific |
| B: Accept raw positions + symbol details map | More flexible callers | Computation function needs to join data |
| C: Accept raw positions, fetch details internally | Simplest caller API | Couples computation to data access; harder to test |

---

### Task 3: Correlation matrix computation [PRIORITY: HIGH]
**Corresponds to:** Scenario: View correlation matrix of holdings, Scenario: Correlation matrix with insufficient data
**Description:** Pure computation that takes historical price series and computes pairwise Pearson correlation coefficients.

- [ ] Create `internal/domain/analysis/correlation.go`
- [ ] Implement `ComputeCorrelation(prices map[string][]market.HistoricalPrice, period string) *CorrelationResult`:
  - Filter each price series to the lookback period (1Y/3Y/5Y/10Y)
  - Compute daily returns from close prices: (close[t] / close[t-1]) - 1
  - For each pair of symbols, compute Pearson correlation of daily returns over overlapping dates
  - Build N×N matrix (N = number of symbols)
  - Track warnings: data overlap < 60 trading days, insufficient data
  - Handle edge cases: single symbol (message), identical symbols (correlation = 1.0), all NaN (message)
- [ ] Implement `pearsonCorrelation(x, y []float64) (float64, int)` — returns correlation coefficient and sample count
- [ ] Implement `alignReturns(pricesA, pricesB) ([]float64, []float64, int)` — aligns daily returns by date, returns aligned series and overlap count
- [ ] Write table-driven unit tests:
  - Happy path: 3 symbols with known correlated price series
  - Perfect positive correlation (identical series)
  - Perfect negative correlation
  - Zero correlation (uncorrelated series)
  - Insufficient data (< 60 days overlap)
  - Single symbol (message)
  - Two symbols (valid — 1×1 correlation)
  - Mismatched date ranges (uses available overlap)
  - Missing prices for a symbol (excluded, warning)

**Verification:** Correlation matrix matches expected values for known price series; warnings generated for insufficient data.

**Technical Decision B — Correlation math:**

| Option | Pros | Cons |
|--------|------|------|
| **A: float64 for correlation** (chosen) | Correlation is a statistical display metric; float64 has 15 digits of precision; standard for finance | Not decimal.Decimal |
| B: decimal.Decimal throughout | Consistent with project convention | Overkill for display-only metric; complex division/sqrt in decimal |

Pearson correlation involves division and square root — float64 is standard practice for this computation. The result is a display metric, not used for financial decisions.

---

### Task 4: Sector and geographic allocation [PRIORITY: HIGH]
**Corresponds to:** Scenario: View sector allocation (ETF look-through), Scenario: View geographic allocation (ETF look-through), Scenario: Sector/geographic allocation with incomplete data
**Description:** Pure computation that takes positions with cached symbol details and computes weighted sector and geographic allocation.

- [ ] Create `internal/domain/analysis/allocation.go`
- [ ] Implement `ComputeSectorAllocation(positions []PositionWithDetails) *AllocationResult`:
  - For ETF positions: weight each sector by (ETF portfolio weight × sector percent from TopHoldings)
  - For individual stock positions: use the stock's own sector from SymbolDetails (assetProfile.Sector)
  - Sum weights per sector, sort descending
  - Track "Unknown" bucket for positions without sector data
  - Generate warnings for symbols missing data
- [ ] Implement `ComputeGeographicAllocation(positions []PositionWithDetails) *AllocationResult`:
  - Same pattern as sector but using GeographicAllocations
  - For individual stocks: country from assetProfile wrapped as single-element allocation
  - Track "Unknown" bucket and warnings
- [ ] Write table-driven unit tests:
  - Happy path: mixed ETF + stock portfolio with known allocations
  - ETF-only portfolio
  - Stock-only portfolio
  - Partial data (some ETFs missing sector/geographic data → Unknown bucket)
  - All missing data (100% Unknown)
  - Single position (100% in one sector/country)
  - Weighted aggregation correctness (ETF weight × sector %)

**Verification:** Given known positions and symbol details, sector and geographic allocations match expected weighted values.

---

### Task 5: Stress testing [PRIORITY: HIGH]
**Corresponds to:** Scenario: Stress test against historical scenarios, Scenario: Compare multiple stress scenarios, Scenario: Stress test with no sector data
**Description:** Pre-defined historical crisis scenarios and computation of estimated portfolio impact.

- [ ] Create `internal/config/data/stress_scenarios.json` with 6 predefined scenarios:
  - Each scenario: name, date_range, sector_returns (GICS sector → peak-to-trough return %)
  - Scenarios: 2008 GFC, 2000 Dot-Com, 2020 COVID, 2022 Decline, 1997 Asian Crisis, 2011 European Debt
- [ ] Create `internal/domain/analysis/stress_scenarios.go`:
  - Define `StressScenario` struct: Name, DateRange, SectorReturns (map[string]float64, sector → peak-to-trough return %)
  - Define 6 predefined scenarios:
    - 2008 Global Financial Crisis (GFC)
    - 2000 Dot-Com Bubble
    - 2020 COVID Crash
    - 2022 Market Decline
    - 1997 Asian Financial Crisis
    - 2011 European Sovereign Debt Crisis
  - Populate with researched peak-to-trough sector returns (GICS sectors)
  - Implement `LoadPredefinedScenarios() ([]StressScenario, error)` — reads JSON file, validates structure
  - Export as `PredefinedScenarios` (package-level variable, populated via init() or lazy load)
- [ ] Create `internal/domain/analysis/stress_test.go`:
  - Implement `ComputeStressTests(sectorAllocation *AllocationResult, portfolioValue decimal.Decimal) *StressTestResult`:
    - For each scenario: multiply sector return × portfolio weight in that sector, sum for total estimated return
    - Compute dollar impact: portfolio value × estimated return / 100
    - Track sector contribution breakdown
    - Sort by severity (most negative first)
    - Handle empty state: no sector data → message
- [ ] Write table-driven unit tests:
  - Happy path: known sector allocation × known scenario → expected return
  - Single sector allocation (100% in one sector)
  - Equal weight across sectors
  - Unknown sector weight (excluded from calculation, noted in warning)
  - Zero portfolio value
  - Negative portfolio value
  - Empty sector allocation (message)

**Verification:** Given known sector weights and scenario data, estimated returns match manual calculation.

**Technical Decision C — Scenario data source:**

| Option | Pros | Cons |
|--------|------|------|
| A: Hardcoded in Go | No runtime fetch; zero dependency; data is static historical | Must research and hardcode values; can't update without deploy |
| **B: JSON file in `internal/config/data/`** (chosen) | Easier to update scenarios without recompiling; data separated from code | Extra file to maintain; still bundled |
| C: Fetch from external source at startup | Always current | Runtime dependency; fetch failures block startup |

JSON file keeps scenario data separated from Go code, making it easier to update historical values without a deploy. Follows the existing `internal/config/data/` pattern.

**Research note:** Sector returns for each crisis will be researched from historical data (e.g., GICS sector performance during GFC peak-to-trough). Accuracy is approximate — these are stress *estimates*, not precise predictions.

---

### Task 6: Factor exposure computation [PRIORITY: HIGH]
**Corresponds to:** Scenario: View factor exposure proxy, Scenario: Factor exposure with insufficient data
**Description:** Pure computation of proxy-based factor exposure metrics from cached valuation data.

- [ ] Create `internal/domain/analysis/factor_exposure.go`
- [ ] Implement `ComputeFactorExposure(positions []PositionWithDetails, portfolioValue decimal.Decimal) *FactorExposureResult`:
  - **Value vs Growth**: portfolio-weighted P/E and P/B from EquityValuation, compared to benchmark (S&P 500: ~20x P/E, ~4x P/B as hardcoded reference)
  - **Size tilt**: large-cap vs mid-cap based on market cap of underlying holdings (from FundProfile.TotalNetAssets for ETFs, or estimated from position value)
  - **Concentration**: HHI = sum of (weight_i²) for all underlying holdings; interpret as "Well-diversified" (<0.02), "Moderately concentrated" (0.02–0.05), "Highly concentrated" (>0.05)
  - **Top holding weight**: largest single underlying holding as % of portfolio
  - Track warnings for positions missing valuation data
- [ ] Write table-driven unit tests:
  - Happy path: portfolio with known P/E, P/B, market caps
  - Single ETF (simple case)
  - Mixed ETF + stock portfolio
  - Missing valuation data for some positions (partial computation, warning)
  - All missing data (message)
  - HHI calculation correctness
  - Value vs growth axis positioning

**Verification:** Factor exposure metrics match expected values for known input data.

---

### Task 7: Analysis service [PRIORITY: HIGH]
**Corresponds to:** Scenario: Request full portfolio analysis, Scenario: Request a single analysis section, Scenario: Request analysis for a portfolio with no positions, Scenario: Request analysis with missing symbol details
**Description:** Service that orchestrates data fetching and delegates to the computation functions. This is the single entry point for all analysis.

- [ ] Create `internal/domain/analysis/service.go`
- [ ] Define interfaces:
  - `PositionSource` — GetOpenPositions(ctx, accountIDs, limit, offset) ([]position.Position, error), EnrichWithMarketData(ctx, positions, baseCurrency) ([]position.PositionWithMarket, error)
  - `SymbolDetailsSource` — GetByInternalSymbol(ctx, internalSymbol) (*symbol.SymbolDetails, error)
  - `MarketDataHistorySource` — GetHistoricalPrices(ctx, symbol, start, end) ([]market.HistoricalPrice, error)
  - `AccountResolver` — GetAccountsByPortfolio(ctx, portfolioID) ([]AccountRef, error), GetAllAccounts(ctx) ([]AccountRef, error)
  - `PortfolioCurrencySource` — GetPortfolioCurrency(ctx, portfolioID) (string, error)
- [ ] Implement `ComputeAnalysis(ctx context.Context, filters AnalysisFilters) (*AnalysisResult, error)`:
  - Resolve account IDs from filters (portfolio_id or all)
  - Fetch open positions, enrich with market data (for portfolio weights)
  - Fetch symbol details for each position symbol
  - Build `[]PositionWithDetails` (position + market weight + symbol details)
  - Compute each section independently (overlap, correlation, allocation, stress test, factor exposure)
  - Apply section filter if specified
  - Collect warnings from all sections
  - Handle empty state: no positions → all sections null, message field
  - Handle missing data: compute from available, add warnings
  - Trigger background symbol details refresh for stale symbols (>7 days)
- [ ] Implement `ComputeAnalysis` to call each computation function and assemble the result
- [ ] Write service tests with hand-written mocks:
  - Happy path: portfolio with ETFs + stocks, all data available
  - No positions (empty result with message)
  - Missing symbol details for some symbols (partial computation, warnings)
  - Section filter (only requested section computed)
  - Stale symbol details (background refresh triggered)
  - Single-stock portfolio (overlap shows message, other sections work)
  - All ETFs with no cached details (graceful degradation)

**Verification:** Service orchestrates all sections correctly; section filter works; warnings collected; empty states handled.

**Technical Decision D — Service location:**

| Option | Pros | Cons |
|--------|------|------|
| **A: New `analysis` package** (chosen) | Clean separation; all analysis types + computation + service co-located; testable in isolation | New package to maintain |
| B: Add to `position.Service` | Reuses position dependencies | Service already large; analysis is a distinct concern from position computation |
| C: Add to `performance` package | Related analytics domain | Performance is equity curve + returns; analysis is composition/risk — different concern |

---

### Task 8: API handler [PRIORITY: HIGH]
**Corresponds to:** Scenario: Request full portfolio analysis, Scenario: Request a single analysis section
**Description:** REST API endpoint for retrieving portfolio analysis data. Returns all sections as JSON with optional section filtering.

- [ ] Create `internal/api/handlers/analysis.go`
- [ ] Define `AnalysisHandler` struct with `*analysis.Service` dependency
- [ ] Register route: `GET /api/analysis` with query params: `portfolio_id`, `section`, `period`
- [ ] Parse query params into `AnalysisFilters`
- [ ] Call `service.ComputeAnalysis(ctx, filters)` and return JSON response
- [ ] Define response DTO matching `AnalysisResult` structure
- [ ] Handle errors: portfolio not found, internal error
- [ ] Write unit tests (mock service, verify request/response mapping)
- [ ] Write tests for section filter parameter

**Verification:** API returns correct JSON for valid requests; section filter works; appropriate error codes for invalid inputs.

---

### Task 9: Web handler + template [PRIORITY: HIGH]
**Corresponds to:** All web UI scenarios (view overlap, correlation, allocation, stress test, factor exposure, empty states)
**Description:** Server-rendered analysis page with tabs/sections for each analytical lens, ECharts visualizations, and period selector.

- [ ] Create `internal/api/handlers/analysis_web.go`
- [ ] Define `AnalysisWebHandler` with `*AnalysisHandler` (API), `*portfolio.Service`, `*web.Renderer`
- [ ] Register routes: `GET /analysis`
- [ ] `HandleAnalysis` renders the page:
  - Parse portfolio_id and period from query params
  - Call `apiHandler.computeResult()` (reuse API computation)
  - Serialize chart data for ECharts (correlation matrix, allocation bars, stress test comparison)
  - Pass data to template
- [ ] Create `templates/analysis/index.html`:
  - Portfolio selector dropdown (reuse pattern from performance page)
  - Period selector for correlation matrix (1Y, 3Y, 5Y, 10Y)
  - Section: ETF Overlap — pairwise matrix table + top concentrated stocks table
  - Section: Correlation — ECharts heatmap of correlation matrix
  - Section: Sector Allocation — horizontal bar chart (ECharts) + table
  - Section: Geographic Allocation — horizontal bar chart (ECharts) + table
  - Section: Stress Testing — comparison table sorted by severity
  - Section: Factor Exposure — summary cards (value/growth axis, size, concentration, top holding)
  - Empty state messages per section
  - Warning indicators (stale data, missing data)
- [ ] Write unit tests for handler (mock API handler, verify template rendering)

**Verification:** Analysis page renders correctly with all sections, charts, and controls. Empty states and warnings displayed appropriately.

**Technical Decision E — Chart visualization approach:**

| Decision | Choice | Reason |
|---|---|---|
| Correlation matrix | ECharts heatmap | Color-coded cells; interactive tooltips; consistent with project |
| Sector/geographic allocation | ECharts horizontal bar chart | Shows sorted weights clearly; better than pie for many categories |
| Stress test comparison | HTML table | Simple tabular data; no chart needed |
| Factor exposure | HTML cards + simple axis div | Value↔Growth axis can be a simple CSS bar; no chart library needed |
| Overlap matrix | HTML table | Pairwise matrix is tabular; color-coded cells via CSS |

---

### Task 10: Router wiring + navigation [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (integration)
**Description:** Wire the analysis handlers into the router and add the navigation link.

- [ ] Update `internal/api/router.go`:
  - Create `analysis.Service` with dependencies (positionSvc, symbolDetailsSvc, marketSvc, accountLister, portfolioCurrencyChecker)
  - Create `analysisHandler` and `analysisWebHandler`
  - Register routes on router
- [ ] Update `templates/partials/nav.html`:
  - Add "Analysis" link (alongside "Performance" in the analytics section)
- [ ] Run `go build` and verify no compilation errors
- [ ] Run `go test ./...` and verify all tests pass

**Verification:** Application builds and runs; navigation link works; analysis page is accessible at `/analysis`.

---

### Task 11: Validation / Hardening [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (cross-cutting quality gate)
**Description:** After implementation tasks are complete, validate the feature end-to-end.

- [ ] Run all tests (`go test ./...`) — not just `-short`
- [ ] Verify each spec scenario manually or via integration test
- [ ] Check edge cases from the spec against actual behavior
- [ ] Run `go vet ./...` and linter
- [ ] Review for cross-layer consistency (data types stored match data types read)
- [ ] Verify no TODOs, FIXMEs, or temporary workarounds remain
- [ ] Update `features/README.md` feature index

**Verification:** All tests pass, all spec scenarios validated, no unresolved issues.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| New `analysis` package | Yes | Clean separation; distinct concern from position/performance; co-locates types + computation + service |
| Computation functions | Pure functions in `analysis` package | Testable in isolation; no DB/network dependencies; follows `comparison/compute.go` pattern |
| Correlation math | float64 (not decimal.Decimal) | Statistical display metric; float64 standard for Pearson correlation; avoids complex sqrt/division in decimal |
| Stress scenario data | JSON file in `internal/config/data/` | Separated from code; easier to update values without recompiling; follows existing config/data pattern |
| Service dependencies | Interfaces on `analysis.Service` | Hand-written mocks in tests; follows existing pattern (position.Service uses interfaces) |
| Section filtering | Server-side (skip computation for unrequested sections) | Reduces work for partial requests; consistent with performance handler's field filtering |
| Background refresh | Triggered by analysis service when stale | Reuses existing symbols service; non-blocking; consistent with performance page pattern |
| Allocation computation | Single function per type (sector, geographic) | Clear separation; same input type; independently testable |
| Portfolio weight calculation | Position market value / total portfolio value | Reuses existing `EnrichWithMarketData` from position service |
| API response structure | Single endpoint, optional section filter | Matches spec; consistent with performance endpoint pattern (`?fields=`) |

## Risks

- **Yahoo Finance data gaps**: Symbol details (holdings, sectors) may be missing for some ETFs, especially obscure ones. Mitigation: graceful degradation with "Unknown" bucket and warnings.
- **Correlation computation cost**: Fetching 10 years of daily prices for 50+ symbols could be slow. Mitigation: data is cached in market_data table (f011); computation uses cached data only.
- **Stress scenario accuracy**: Historical sector returns are approximate estimates. Mitigation: clearly labeled as "estimated" in UI; not financial advice.
- **Large number of ETF pairs**: N×N overlap matrix grows quadratically. Mitigation: overlap is ETF-to-ETF only; typical portfolios have <20 ETFs.
- **Performance constraint (5s for ≤50 positions)**: If multiple sections compute slowly, total time may exceed 5s. Mitigation: section filter allows partial computation; analysis is on-demand (no pre-computation).
