# Implementation Plan: Efficient Frontier

## Overview

Portfolio optimization via the efficient frontier method. A new domain package `efficientfrontier` provides the optimization engine (pure Go, no external libraries). A service layer fetches historical prices via the existing `marketservice`, computes the frontier, and returns results to API and web handlers. The web UI presents an interactive ECharts scatter plot, allocation weight tables, and the ability to save optimized allocations as model portfolios.

## Task Dependencies

```
Task 1 (optimization engine)
    └── Task 2 (service layer)
            └── Task 3 (API endpoint)
                    └── Task 4 (web page — symbol selection + compute)
                            └── Task 5 (web page — save as model portfolio)
Task 6 (integration tests) — parallel with Tasks 3-5, depends on Task 2
Task 7 (nav + docs) — after all above
```

## Tasks

### Task 1: Optimization engine — types, returns, covariance, frontier [PRIORITY: HIGH]

**Corresponds to:** All scenarios (core computation)

**Description:** Create `internal/domain/efficientfrontier/` with the pure-Go optimization engine. This package has no dependencies on databases, HTTP, or external services — it operates on `[]float64` slices and produces typed results.

- [x] Define types: `FrontierRequest`, `FrontierResult`, `FrontierPoint`, `OptimizedPortfolio` (with `Symbol`, `Weight` as fraction 0.0–1.0), `FrontierError`
- [x] Implement `ComputeReturns(prices []market.HistoricalPrice) ([]float64, error)` — daily log returns from close prices
- [x] Implement `ComputeAnnualizedReturn(returns []float64, tradingDays int) float64`
- [x] Implement `ComputeAnnualizedVolatility(returns []float64, tradingDays int) float64`
- [x] Implement `ComputeCovarianceMatrix(returnsBySymbol map[string][]float64) ([][]float64, []string, error)` — aligned returns, sample covariance
- [x] Implement `ComputeMinVariance(covMatrix [][]float64, n int) ([]float64, error)` — analytical global minimum variance portfolio via `w = Σ⁻¹·1 / (1'·Σ⁻¹·1)`; falls back to nil on singular matrix
- [x] Implement `ComputeFrontier(request FrontierRequest) (*FrontierResult, error)` — the main entry point:
  - Compute expected returns and covariance from aligned price data
  - Analytical min-variance portfolio (exact anchor point via matrix inversion)
  - Grid search: sample ~2000 random portfolios on the simplex (Dirichlet distribution)
  - Evaluate each: portfolio return = w'μ, portfolio volatility = sqrt(w'Σw)
  - Filter to Pareto frontier (efficient points only); merge analytical min-variance point
  - Identify max Sharpe ratio portfolio from combined set
  - Return frontier points (20–30 sampled from the efficient set) + key portfolios
  - Cap candidate symbols at 10; reject with error if more than 10 provided
- [x] Handle edge cases: singular covariance matrix, insufficient data, single symbol
- [x] Write unit tests: `types_test.go`, `returns_test.go`, `covariance_test.go`, `minvariance_test.go`, `frontier_test.go` (table-driven, no DB, no network)

**Verification:** `go test ./internal/domain/efficientfrontier/...` passes with 89.6% coverage. All edge cases return typed errors.

---

### Task 2: Service layer — data fetching, symbol resolution, orchestration [PRIORITY: HIGH]

**Corresponds to:** Scenarios: compute with defaults, custom period, custom risk-free rate; Error scenarios (missing data, all missing, numerical failure)

**Description:** Create `Service` in `internal/domain/efficientfrontier/` that orchestrates data fetching and delegates to the computation functions. Follows the `analysis.Service` pattern: defines interfaces for dependencies, fetches data, collects warnings, returns structured result.

- [ ] Define service interfaces: `MarketDataHistorySource` (reuse from `marketservice`), `MarketDataSymbolResolver`, `SymbolLister` (for autocomplete)
- [ ] Implement `ComputeFrontier(ctx, request) (*ServiceResult, error)`:
  - Resolve market data symbols for each candidate
  - Fetch historical prices for the selected period (1Y/3Y/5Y → date range)
  - Check data sufficiency per symbol (minimum ~60 trading days)
  - Build aligned price map, pass to `ComputeFrontier` engine
  - Collect warnings for symbols with limited/missing data
  - Return `ServiceResult` with frontier data + warnings + excluded symbols
- [ ] Implement `GetCandidateSymbols(ctx) ([]string, error)` — distinct symbols from symbol map for autocomplete
- [ ] Implement `GetSymbolsFromPortfolio(ctx, portfolioID) ([]string, error)` — copy from real portfolio
- [ ] Implement `GetSymbolsFromModelPortfolio(ctx, modelPortfolioID) ([]string, error)` — copy from model portfolio
- [ ] Handle currency conversion: if symbols have different currencies, convert prices to base currency using cached FX rates (warn if FX data missing)
- [ ] Write unit tests: `service_test.go` with hand-written mocks for all interfaces

**Verification:** `go test ./internal/domain/efficientfrontier/...` passes. Service correctly collects warnings for partial data and returns explicit errors for complete failure.

---

### Task 3: API endpoint — POST /api/efficient-frontier/compute [PRIORITY: HIGH]

**Corresponds to:** Scenarios: compute with defaults, custom period, custom risk-free rate; Error scenarios

**Description:** Create `efficientfrontier.go` and `efficientfrontier_test.go` in `internal/api/handlers/`. Follows the `analysis` pattern: handler defines a service interface, delegates computation, returns JSON.

- [ ] Define `efficientFrontierService` interface (subset of methods the handler needs)
- [ ] Implement `HandleComputeFrontier` (POST `/api/efficient-frontier/compute`):
  - Parse request: `[]string Symbols`, `string Period`, `float64 RiskFreeRate`
  - Validate: at least 2 symbols, max 10 symbols, valid period
  - Call service.ComputeFrontier
  - Return JSON response with frontier points, key portfolios, warnings
  - Error responses for: empty candidate set, numerical failure, all symbols missing data
- [ ] Implement `HandleGetCandidateSymbols` (GET `/api/efficient-frontier/symbols`) — autocomplete source
- [ ] Implement `HandleGetPortfolioSymbols` (GET `/api/efficient-frontier/portfolio/{id}/symbols`) — copy from real portfolio
- [ ] Implement `HandleGetModelPortfolioSymbols` (GET `/api/efficient-frontier/model-portfolio/{id}/symbols`) — copy from model portfolio
- [ ] Register routes in `RegisterRoutes`
- [ ] Wire handler in `router.go`
- [ ] Write handler tests: `efficientfrontier_test.go` with mock service

**Verification:** `go test ./internal/api/handlers/... -run Efficient` passes. API returns consistent error responses.

---

### Task 4: Web page — symbol selection, frontier chart, allocation weights [PRIORITY: HIGH]

**Corresponds to:** Scenarios: select candidate symbols, compute with defaults/custom period/custom risk-free rate, view allocation weights, error scenarios

**Description:** Create `efficientfrontier_web.go` and `efficientfrontier_web_test.go` in `internal/api/handlers/`. Create template `templates/efficient_frontier/index.html`. Follows the `analysis_web.go` pattern: web handler delegates to API handler for computation, adds presentation concerns.

- [ ] Create `EfficientFrontierWebHandler` with routes: `GET /efficient-frontier`
- [ ] Implement `HandleEfficientFrontier` (GET `/efficient-frontier`):
  - Parse query params: symbols (comma-separated), period, risk-free rate, source portfolio/model-portfolio ID
  - Call API handler for computation
  - Serialize frontier data as JSON for ECharts scatter chart
  - Build template data: candidate symbols list, period buttons, URLs
  - Render template
- [ ] Create template `templates/efficient_frontier/index.html`:
  - Symbol input with autocomplete (datalist from symbol map)
  - "Copy from portfolio" / "Copy from model portfolio" dropdowns
  - Period selector buttons (1Y, 3Y, 5Y)
  - Risk-free rate input field (default: current US Treasury rate, e.g. 4.5%)
  - Compute button
  - ECharts scatter plot: x-axis = volatility (%), y-axis = return (%), frontier curve + max Sharpe + min variance markers
  - Click handler on chart points → show allocation weights table
  - Allocation weights table (symbol, weight %, return contribution)
  - Warning/error display area
  - "Save as Model Portfolio" button (pre-fills model portfolio form)
- [ ] Register web handler in `router.go`
- [ ] Add nav link in `templates/partials/nav.html`
- [ ] Write web handler tests: `efficientfrontier_web_test.go`

**Verification:** Page renders correctly. Chart displays frontier curve. Clicking points shows allocation weights. Copy from portfolio/model-portfolio pre-fills symbols.

---

### Task 5: Save optimized allocation as model portfolio [PRIORITY: MEDIUM]

**Corresponds to:** Scenario: save optimized allocation as a model portfolio

**Description:** Add a POST endpoint and web form to save the currently displayed frontier portfolio as a model portfolio. Reuses existing `modelportfolio` service.

- [ ] Implement `HandleSaveAsModelPortfolio` (POST `/api/efficient-frontier/save`):
  - Parse request: `string Name`, `[]FrontierAllocation Entries`
  - Map to `modelportfolio.CreateRequest`
  - Call existing model portfolio service.Create
  - Return created model portfolio
- [ ] Add web form section in template: name input + save button, appears after frontier is computed
- [ ] On save success, redirect to model portfolio list with flash message
- [ ] Wire handler in `router.go`
- [ ] Write tests

**Verification:** Selecting a frontier point and saving creates a model portfolio visible in `/model-portfolios`.

---

### Task 6: Integration tests [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (end-to-end verification)

**Description:** Integration tests using in-memory SQLite with real schema, exercising the full stack.

- [ ] Create `tests/integration/efficient_frontier_test.go`
- [ ] Test: compute frontier with 2 symbols (happy path) — verify frontier points, key portfolios
- [ ] Test: compute frontier with 3+ symbols — verify weights sum to 1.0
- [ ] Test: error — empty candidate set
- [ ] Test: error — single symbol
- [ ] Test: warning — some symbols missing data (partial optimization)
- [ ] Test: error — all symbols missing data
- [ ] Test: save as model portfolio (full path: compute → save → verify in DB)
- [ ] Use shared test helper `tests/integration/db.go` for in-memory DB

**Verification:** `go test ./tests/integration/... -run Efficient` passes.

---

### Task 7: Nav link, API.md, feature index update [PRIORITY: LOW]

**Corresponds to:** N/A (documentation)

**Description:** Final housekeeping.

- [ ] Add "Efficient Frontier" nav link in `templates/partials/nav.html` (after "Comparison")
- [ ] Update `API.md` with efficient frontier endpoints
- [ ] Update `features/README.md` feature index (add f028, status = "in-progress")
- [ ] Run `goimports -w .` and `go test ./...`

**Verification:** Nav link visible. API.md documents all endpoints. Feature index updated.

---

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| **Optimization algorithm** | Analytical min-variance + grid search hybrid | Min-variance portfolio computed exactly via Σ⁻¹·1 / (1'·Σ⁻¹·1). Rest of frontier from ~2000 random simplex samples filtered to Pareto set. Pure Go, no external libraries (per spec Non-Goals). Cap at 10 symbols keeps matrix ops fast and stable. If Σ is singular, falls back to grid search only. |
| **Package location** | `internal/domain/efficientfrontier/` | Matches existing domain package pattern (analysis, comparison, modelportfolio). Self-contained with engine + service + types. |
| **Service interfaces** | Define new interfaces in efficientfrontier package | Follows `analysis.Service` pattern: service defines what it needs via interfaces, concrete implementations wired in `router.go`. Keeps package boundaries clean. |
| **Currency handling** | Convert to base currency using cached FX rates; warn if missing | Spec requires handling multi-currency symbols. Reuses existing FX infrastructure from `marketservice`. Warns rather than fails if FX data unavailable. |
| **Frontier point count** | 20–30 points on the efficient curve | Enough for smooth chart rendering without overwhelming the user. Max Sharpe + min variance always included. |
| **Default risk-free rate** | 4.5% (configurable) | Approximate current US Treasury yield. User can override. Stored as a constant with comment noting it should be updated periodically. |
| **Data sufficiency threshold** | Minimum 60 trading days (~3 months) | Enough for meaningful return/volatility estimates. Below this, results are unreliable. Spec notes "very short time period" edge case. |
| **Chart library** | ECharts scatter series | Consistent with existing analytics pages (analysis, comparison). Supports click callbacks for showing allocation weights. |
| **Save as model portfolio** | Reuse existing modelportfolio service | No new database tables needed. Follows API-first architecture: web handler delegates to service. |

### Alternative Considered: Pure Grid Search (No Analytical Component)

**Rejected:** Grid search alone without the analytical min-variance anchor.

**Reason:** The minimum variance portfolio is the most commonly referenced frontier point. Computing it analytically guarantees exactness at that anchor. The hybrid approach adds ~50 lines (matrix multiply + solve 10×10 system) for a meaningful accuracy gain at the most important point.

### Alternative Considered: Full QP Solver

**Rejected:** Active-set or interior-point quadratic programming for exact frontier computation.

**Pros:** Mathematically exact; handles long-only constraints natively; industry-standard.

**Cons:** ~300-500 lines of code; matrix operations (Cholesky, inversion); complex to test; numerical stability concerns; over-engineered for N ≤ 10 symbols where the hybrid approach is sufficient.

---

## Risks

- **Numerical stability:** The covariance matrix may be near-singular with highly correlated assets. Mitigation: add regularization (add small epsilon to diagonal) and detect singularity before inversion.
- **Large symbol sets:** Grid search complexity grows with N dimensions; matrix inversion is O(N³). Mitigation: hard cap at 10 candidate symbols; reject with error if more selected.
- **FX data gaps:** Multi-currency portfolios may have missing FX rates. Mitigation: warn user, proceed with available data, note limitation in result.
