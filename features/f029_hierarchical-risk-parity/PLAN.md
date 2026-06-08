# Implementation Plan: Hierarchical Risk Parity

## Overview

Build a Hierarchical Risk Parity (HRP) portfolio optimization feature. The user selects candidate symbols and a historical period, and the system computes four HRP allocations (one per linkage method: single, complete, average, Ward) using hierarchical clustering and recursive bisection. Results are displayed as a weights table and four dendrograms. Any allocation can be saved as a model portfolio.

The implementation follows the same layered architecture as f028 Efficient Frontier:
- **Domain** (`internal/domain/hierarchicalriskparity/`): types, computation engine, service
- **Data adapters** (`internal/data/hrp_adapters.go`): bridge between domain interfaces and existing repos/services
- **API handlers** (`internal/api/handlers/hrp.go`): REST endpoints
- **Web handlers** (`internal/api/handlers/hrp_web.go`): server-rendered pages
- **Template** (`templates/hierarchical_risk_parity/index.html`): UI with ECharts dendrograms

## Task Dependencies

```
Task 1 (types)
    ↓
Task 2 (math utilities: returns, correlation, distance)
    ↓
Task 3 (hierarchical clustering — 4 linkage methods)
    ↓
Task 4 (quasi-diagonalization + recursive bisection)
    ↓
Task 5 (HRP computation engine — ties it all together)
    ↓
Task 6 (service layer — data fetching, FX, orchestration)
    ↓
Task 7 (data adapters)
    ↓
Task 8 (API handler)
    ↓
Task 9 (web handler)
    ↓
Task 10 (template + dendrogram rendering)
    ↓
Task 11 (router wiring + nav link)
```

## Tasks

### Task 1: Domain types and error definitions [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundational)
**Description:** Define the request, result, and error types for the HRP domain.

- [x] Create `internal/domain/hierarchicalriskparity/types.go`
- [x] Define `HrpRequest` (symbols, prices map, period)
- [x] Define `HrpResult` with four `HrpAllocation` (one per linkage method), each containing symbol weights and dendrogram tree
- [x] Define `HrpAllocation` (linkage method name, weights map, dendrogram data)
- [x] Define `DendrogramNode` for tree serialization (name, children, distance/height)
- [x] Define linkage method constants (`single`, `complete`, `average`, `ward`)
- [x] Define typed errors: `ErrInsufficientSymbols`, `ErrTooManySymbols`, `ErrInsufficientData`, `ErrNumericalFailure`
- [x] Write `types_test.go` with serialization round-trip tests

**Verification:** `go test ./internal/domain/hierarchicalriskparity/...` passes; types serialize to/from JSON correctly.

---

### Task 2: Math utilities — returns, correlation, distance matrix [PRIORITY: HIGH]
**Corresponds to:** Scenario: Compute HRP allocations (data preparation step)
**Description:** Implement the mathematical building blocks needed by the HRP engine.

- [x] Create `internal/domain/hierarchicalriskparity/returns.go`
- [x] Implement `ComputeDailyReturns(prices) → []float64` (simple returns, sorted by date)
- [x] Implement `AlignReturns(pricesBySymbol, symbols) → ([][]float64, int)` — align by date, return only dates where ALL symbols have data
- [x] Create `internal/domain/hierarchicalriskparity/correlation.go`
- [x] Implement `ComputeCorrelationMatrix(alignedReturns) → [][]float64` using `stats.PearsonCorrelation`
- [x] Create `internal/domain/hierarchicalriskparity/distance.go`
- [x] Implement `CorrelationToDistance(corrMatrix) → [][]float64` using `d(i,j) = sqrt(2*(1-corr(i,j)))`
- [x] Write tests for each function with known inputs (e.g., perfectly correlated → distance 0, uncorrelated → distance ~1.41)

**Verification:** `go test ./internal/domain/hierarchicalriskparity/...` passes; correlation of identical series = 1.0, distance = 0.0.

---

### Task 3: Hierarchical clustering with four linkage methods [PRIORITY: HIGH]
**Corresponds to:** Scenario: Compute HRP allocations, Scenario: View clustering dendrograms
**Description:** Implement agglomerative hierarchical clustering producing a merge sequence and dendrogram tree for each linkage method.

- [x] Create `internal/domain/hierarchicalriskparity/clustering.go`
- [x] Define `MergeRecord` (cluster1 index, cluster2 index, distance)
- [x] Implement `clusterSingleLinkage(distanceMatrix) → ([]MergeRecord, *DendrogramNode)`
- [x] Implement `clusterCompleteLinkage(distanceMatrix) → ([]MergeRecord, *DendrogramNode)`
- [x] Implement `clusterAverageLinkage(distanceMatrix) → ([]MergeRecord, *DendrogramNode)`
- [x] Implement `clusterWardLinkage(distanceMatrix) → ([]MergeRecord, *DendrogramNode)`
- [x] Implement shared `buildDendrogramTree(mergeRecords, nSymbols) → *DendrogramNode`
- [x] Write `clustering_test.go` with table-driven tests for 2-5 symbol cases, verify merge order and tree structure

**Verification:** `go test ./internal/domain/hierarchicalriskparity/...` passes; 2-symbol case produces single merge at correct distance; tree has correct leaf count.

---

### Task 4: Quasi-diagonalization and recursive bisection [PRIORITY: HIGH]
**Corresponds to:** Scenario: Compute HRP allocations (core algorithm)
**Description:** Implement the two key HRP steps: ordering assets by cluster proximity, then recursively allocating risk budget.

- [x] Create `internal/domain/hierarchicalriskparity/quasidiag.go`
- [x] Implement `QuasiDiagonalize(mergeRecords, nSymbols) → []int` — return sorted indices from the dendrogram leaves left-to-right
- [x] Create `internal/domain/hierarchicalriskparity/bisection.go`
- [x] Implement `recursiveBisect(sortedIndices, covarianceMatrix) → map[int]float64` — recursive risk-parity allocation
  - Split list into two halves at the optimal bisection point (minimizing cross-covariance)
  - Allocate risk budget equally between the two groups
  - Within each group, compute minimum variance weights for risk contribution
  - Recurse until single assets remain
- [x] Implement `findBisectionPoint(indices, covarianceMatrix) → int` — find the split point that minimizes cross-group covariance
- [x] Implement `minVarianceWeights(indices, covarianceMatrix) → []float64` — analytical minimum variance for a subset (using matrix inversion, same as f028)
- [x] Write `bisection_test.go` with known 2-asset and 3-asset cases; verify weights sum to 1.0 and are non-negative

**Verification:** `go test ./internal/domain/hierarchicalriskparity/...` passes; 2-asset case produces equal-risk weights; weights always sum to 1.0.

---

### Task 5: HRP computation engine [PRIORITY: HIGH]
**Corresponds to:** Scenario: Compute HRP allocations with default period, Scenario: Compute HRP allocations with custom period
**Description:** Tie together all components into the main `ComputeHrp` function that produces four allocations.

- [x] Create `internal/domain/hierarchicalriskparity/hrp.go`
- [x] Implement `ComputeHrp(request HrpRequest) (*HrpResult, error)`
  - Validate symbol count (2-20)
  - Compute daily returns per symbol
  - Align returns across all symbols
  - Compute correlation matrix
  - Compute distance matrix
  - For each linkage method: cluster → quasi-diagonalize → recursive bisection → weights
  - Build result with four allocations and four dendrograms
- [x] Handle edge cases: single symbol (error), no data (empty state message), insufficient data (warning)
- [x] Write `hrp_test.go` with table-driven tests covering 2-symbol, 3-symbol, 5-symbol cases
- [x] Write test for edge cases: single symbol, empty symbols, too many symbols (21), zero-return symbol

**Verification:** `go test ./internal/domain/hierarchicalriskparity/...` passes; four allocations returned, each with weights summing to ~1.0; dendrograms have correct leaf count.

---

### Task 6: Service layer — data fetching, FX conversion, orchestration [PRIORITY: HIGH]
**Corresponds to:** Scenario: Compute HRP allocations, Scenario: Warning — insufficient data, Scenario: Error — symbol with no data
**Description:** Service that resolves symbols, fetches historical prices, handles FX conversion, and delegates to the computation engine.

- [x] Create `internal/domain/hierarchicalriskparity/service.go`
- [x] Define interfaces: `MarketDataHistorySource`, `MarketDataSymbolResolver`, `SymbolLister`, `PortfolioSymbolSource`, `ModelPortfolioSource`, `FxRateSource` (same signatures as efficient frontier)
- [x] Define `ServiceResult` (Result, Warnings, ExcludedSymbols, SymbolDataSpan)
- [x] Define `ComputeHrpRequest` (Symbols, Period, BaseCurrency)
- [x] Implement `Service` struct and `NewService` constructor
- [x] Implement `ComputeHrp(ctx, req) → (*ServiceResult, error)` — mirrors efficient frontier service pattern
- [x] Implement `GetCandidateSymbols`, `GetSymbolsFromPortfolio`, `GetSymbolsFromModelPortfolio`
- [x] Implement `fetchPricesForSymbols` (resolve market symbols, fetch prices, collect warnings/excluded)
- [x] Implement `convertToBaseCurrency` (FX conversion using cached rates)
- [x] Implement `computeDataSpan` and period helpers
- [x] Write `service_test.go` with hand-written mocks (same pattern as efficient frontier)

**Verification:** `go test ./internal/domain/hierarchicalriskparity/...` passes; service correctly resolves symbols, fetches prices, converts FX, and delegates to engine.

---

### Task 7: Data adapters [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (infrastructure)
**Description:** Bridge between HRP domain interfaces and existing repository/service implementations.

- [x] Create `internal/data/hrp_adapters.go`
- [x] Implement `HrpSymbolListerImpl` (delegates to SymbolListerImpl)
- [x] Implement `HrpPortfolioSymbolSourceImpl` (delegates to PortfolioSymbolSourceImpl)
- [x] Implement `HrpModelPortfolioSourceImpl` (wraps ModelPortfolioSourceImpl, converts ModelPortfolioRef type)
- [x] Implement `HrpFxRateSourceImpl` (wraps FxRateSourceImpl, converts FxRate type)
- [x] Reuse existing patterns from `efficient_frontier_adapters.go`

**Verification:** Adapters compile and satisfy HRP domain interfaces (structural typing).

---

### Task 8: API handler [PRIORITY: MEDIUM]
**Corresponds to:** Scenario: Select candidate symbols, Scenario: Copy symbols from portfolio/model portfolio, Scenario: Compute HRP allocations, Scenario: Save as model portfolio
**Description:** REST API endpoints for HRP computation and symbol management.

- [x] Create `internal/api/handlers/hrp.go`
- [x] Define `hrpService` interface and `HrpHandler` struct
- [x] Implement `POST /api/hrp/compute` — validate symbols (2-20), period (1Y/3Y/5Y), call service
- [x] Implement `GET /api/hrp/symbols` — candidate symbols for autocomplete
- [x] Implement `GET /api/hrp/portfolio/{id}/symbols` — copy from real portfolio
- [x] Implement `GET /api/hrp/model-portfolio/{id}/symbols` — copy from model portfolio
- [x] Implement `POST /api/hrp/save` — save allocation as model portfolio (reuse model portfolio creator)
- [x] Implement `RegisterRoutes`
- [x] Write `hrp_test.go` with handler-level tests using `httptest`

**Verification:** `go test ./internal/api/handlers/... -run Hrp` passes; endpoints return correct status codes and JSON.

---

### Task 9: Web handler [PRIORITY: MEDIUM]
**Corresponds to:** Scenario: View allocation weights table, Scenario: View clustering dendrograms
**Description:** Server-rendered web page handler that delegates to the API service and prepares template data.

- [x] Create `internal/api/handlers/hrp_web.go`
- [x] Define `HrpWebHandler` struct with dependencies
- [x] Define `hrpPageData` struct (Result, Warnings, ExcludedSymbols, CandidateSymbols, Portfolios, ModelPortfolios, SelectedSymbols, SelectedPeriod, SelectedBaseCurrency, PeriodURLs, serialized JSON for ECharts)
- [x] Implement `GET /hrp` handler — parse query params, call service, build page data
- [x] Implement `POST /hrp/save` handler — save selected allocation as model portfolio
- [x] Implement `serializeHrpChartData` — convert HrpResult to JSON for ECharts (four dendrogram trees + weights table data)
- [x] Implement `RegisterRoutes`
- [x] Implement helper functions: `parseHrpSymbols`, `buildHrpPeriodURLs`, `fetchPortfolios`, `fetchModelPortfolios`, `fetchCandidateSymbols`
- [x] Write `hrp_web_test.go` with handler tests

**Verification:** `go build ./...` succeeds; serialization, parsing, URL building, error message, and data span tests pass. Handler/template tests pass once Task 10 (template) is complete.

---

### Task 10: Template — HRP page with weights table and dendrograms [PRIORITY: MEDIUM]
**Corresponds to:** Scenario: View allocation weights table, Scenario: View clustering dendrograms, Scenario: Save an HRP allocation as a model portfolio, Scenario: Error — insufficient candidate symbols
**Description:** Server-rendered HTML template with symbol input, period selector, weights table, four ECharts dendrograms, and save form.

- [ ] Create `templates/hierarchical_risk_parity/index.html`
- [ ] Symbol input with datalist autocomplete (same pattern as efficient frontier)
- [ ] "Copy from" dropdown for real portfolios and model portfolios
- [ ] Period buttons (1Y, 3Y, 5Y)
- [ ] Currency selector (USD, EUR, GBP, JPY, CHF, CAD, AUD, CNY)
- [ ] Compute button
- [ ] Error/warning display sections
- [ ] Weights table: rows = symbols, columns = four linkage methods (single, complete, average, Ward), weights as percentages
- [ ] Four ECharts tree charts (dendrograms), one per linkage method
- [ ] Save form: name input + hidden fields for selected allocation weights
- [ ] JavaScript for: copy-from functionality, dendrogram rendering (ECharts tree chart), save form population
- [ ] Follow efficient frontier template as reference for layout and conventions

**Verification:** Page renders without errors; weights table displays four columns; dendrograms render with correct leaf labels.

---

### Task 11: Router wiring and navigation [PRIORITY: LOW]
**Corresponds to:** All scenarios (integration)
**Description:** Wire HRP into the application router and add navigation link.

- [ ] Add HRP service instantiation in `internal/api/router.go`
- [ ] Add HRP handler registration in `internal/api/router.go`
- [ ] Add HRP web handler registration in `internal/api/router.go`
- [ ] Add "Hierarchical Risk Parity" link to `templates/partials/nav.html`
- [ ] Verify the application builds cleanly (`go build ./...`)
- [ ] Run full test suite (`go test ./...`)

**Verification:** `go build ./...` succeeds; `go test ./...` passes; nav link appears in rendered pages.

---

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| **Package name** | `hierarchicalriskparity` | Follows convention: lowercase, no underscores, descriptive |
| **Dendrogram rendering** | ECharts tree chart (orthogonal layout) | ECharts already used in project; tree chart supports hierarchical data with labels; consistent with existing charting approach |
| **Distance metric** | `d(i,j) = sqrt(2*(1-corr(i,j)))` | Standard HRP distance from correlation; produces values in [0, 2] |
| **Bisection split point** | Minimize cross-group covariance | Standard HRP approach; finds the cut that best separates the cluster into two independent groups |
| **Minimum variance in bisection** | Analytical matrix inversion (same as f028) | Reuses existing pattern; works for small subsets (2-10 assets per bisection level) |
| **Service interfaces** | Same signatures as efficient frontier | Enables reusing data adapters; consumer-defined interfaces per convention |
| **Data adapters** | New file `hrp_adapters.go` | Keeps adapters co-located by domain; explicit about which domain uses which adapters |
| **Returns/correlation** | Implement in HRP package (not shared) | Efficient frontier versions are tailored to that domain; HRP needs slightly different signatures (e.g., aligned returns matrix for correlation). Keeping them separate avoids coupling. |
| **Max symbols** | 20 (vs efficient frontier's 10) | Spec constraint; HRP is more robust with many assets (no numerical optimization) |
| **Periods** | 1Y, 3Y, 5Y only | Spec constraint; matches efficient frontier |
| **Base currency** | User-selectable, same set as efficient frontier | Spec requirement for multi-currency symbols |

## Risks

- **Hierarchical clustering correctness**: The four linkage methods must produce correct merge sequences. Mitigation: test against known small datasets and verify merge distances are monotonically increasing.
- **Matrix inversion in bisection**: If a subset of assets has a singular covariance matrix at some bisection level, the minimum variance calculation fails. Mitigation: add diagonal loading (small epsilon) as fallback, same approach as f028.
- **Dendrogram rendering with many symbols**: 20 symbols may make dendrograms crowded. Mitigation: ECharts tree chart handles this with auto-layout; symbol labels can be rotated or abbreviated.
- **Cross-package coupling**: The HRP service interfaces mirror efficient frontier's. If efficient frontier changes its interfaces, HRP won't break (structural typing), but the data adapters may need updates. Mitigation: keep adapter implementations minimal and focused.
