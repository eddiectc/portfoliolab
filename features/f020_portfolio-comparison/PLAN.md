# Implementation Plan: Portfolio Comparison Engine

## Overview

Build a side-by-side portfolio comparison engine supporting model-vs-model, model-vs-real, and real-vs-real comparisons. The engine simulates buy-and-hold equity curves for model portfolios, reuses existing performance/risk/drawdown infrastructure for both portfolio types, and presents comprehensive comparison metrics via API + web UI.

## Task Dependencies

```
Task 1 (simulation) ──┐
                       ├──► Task 3 (comparison service) ──┐
Task 2 (metrics) ─────┘                                    │
Task 4 (overlap/correlation) ──────────────────────────────┤
                                                           ├──► Task 6 (web UI)
Task 5 (API + web handlers) ───────────────────────────────┘
```

Tasks 1-2 are independent foundations. Task 3 depends on both. Tasks 4-5 can proceed in parallel after Task 3. Task 6 depends on Task 5.

## Tasks

### Task 1: Model Portfolio Equity Curve Simulation [PRIORITY: HIGH]
**Corresponds to:** Scenario "Compare two model portfolios — performance metrics", "Compare two model portfolios — risk metrics", "Compare model portfolio vs real portfolio", "Handle symbols with shorter history", "Comparison with insufficient data"

**Description:** Implement the buy-and-hold simulation that converts a model portfolio (symbol + weight pairs) into an equity curve (daily portfolio values over time) using cached historical prices. This is the foundation for all model portfolio comparison metrics.

- [x] Add `SimulateEquityCurve` function in `internal/domain/comparison/simulation.go` — pure computation: given starting value, weights, and historical prices per symbol, produce daily portfolio values
- [x] Handle FX conversion for foreign-denominated symbols using cached FX rates (reuse `performance.fx_conversion.go` pattern)
- [x] Add period clipping logic: clip to earliest available data across all symbols; track which symbols have limited history
- [x] Add `SimulateEquityCurveTest` table-driven tests in `simulation_test.go` covering: normal case, single symbol, FX conversion, missing data for some symbols, empty input, starting value edge cases
- [x] Wire simulation into the comparison service (Task 3) — wired via `resolveModelPortfolio` → `SimulateEquityCurve`

**Verification:** `go test ./internal/domain/comparison/` passes with >80% coverage on simulation.go. Simulated equity curve for a known 2-symbol portfolio (e.g., 50% AAPL + 50% GOOG, $10,000 start) produces correct daily values when fed manually constructed prices.

---

### Task 2: Comparison-Specific Metrics (CAGR, Beta, Alpha, Portfolio Correlation) [PRIORITY: HIGH]
**Corresponds to:** Scenario "Compare two model portfolios — risk metrics", "View portfolio-to-portfolio correlation"

**Description:** Add pure computation functions for metrics that compare two portfolios against each other. These complement existing `performance.ComputeRiskMetrics` (which computes per-portfolio Sharpe/volatility) and `analysis.ComputeCorrelation` (which computes intra-portfolio symbol correlation).

- [x] Add `ComputeCAGR` in `internal/domain/comparison/metrics.go` — compound annual growth rate from start/end values and days elapsed
- [x] Add `ComputeBetaAlpha` in `internal/domain/comparison/metrics.go` — beta and alpha of portfolio A relative to portfolio B, using aligned daily returns (Pearson regression; reuse `analysis.pearsonCorrelation` and `analysis.alignReturns` patterns)
- [x] Add `ComputePortfolioCorrelation` in `internal/domain/comparison/metrics.go` — overall correlation between two portfolio daily return series (reuse `analysis.alignReturns` + `analysis.pearsonCorrelation`)
- [x] Add `ComputePeriodExtremes` in `internal/domain/comparison/metrics.go` — best/worst month, best/worst year, win rate from equity curve points
- [x] Add `ComputeReturnDistribution` in `internal/domain/comparison/metrics.go` — annual/monthly return frequency histogram buckets
- [x] Write table-driven tests in `metrics_test.go` for each function: normal case, identical portfolios (correlation=1.0, beta=1.0, alpha=0), short data (<30 days → N/A), empty input, zero volatility edge case

**Verification:** `go test ./internal/domain/comparison/` passes. CAGR for a portfolio that doubles in exactly 1 year = ~100%. Beta of identical series = 1.0. Correlation of identical series = 1.0.

---

### Task 3: Comparison Service [PRIORITY: HIGH]
**Corresponds to:** All comparison scenarios (orchestrates data fetching and computation)

**Description:** Create the service layer that resolves portfolio inputs (model or real), fetches required data (historical prices, equity curves, symbol details), and computes the full comparison result. Follows the `analysis.Service` pattern: interface-based dependencies, single `ComputeComparison` entry point.

- [x] Define `ComparisonRequest` and `ComparisonResult` types in `internal/domain/comparison/types.go` — request has portfolio IDs/types, period, base currency, starting value; result has per-portfolio metrics + cross-portfolio metrics + warnings
- [x] Define service interfaces: `ModelPortfolioSource`, `EquityCurveSource` (for real portfolios), `MarketDataHistorySource`, `MarketDataSymbolResolver`, `SymbolDetailsSource`, `FxRateSource`
- [x] Implement `Service` struct and `ComputeComparison` method in `internal/domain/comparison/service.go`
- [x] Handle portfolio type dispatch: model portfolio → simulate equity curve; real portfolio → call existing `position.ComputeEquityCurve`
- [x] Compute per-portfolio metrics: daily returns (reuse `performance.ComputeDailyReturns`), risk metrics (reuse `performance.ComputeRiskMetrics`), drawdown (reuse `performance.ComputeDrawdownAnalysis`), yearly performance (reuse `performance.ComputeYearlyPerformance`)
- [x] **TWR normalization for real portfolios**: before feeding the equity curve to `ComputePeriodExtremes` (and any other simple-return metric), normalize the real portfolio curve to time-weighted returns (reset to 1.0 at each cash flow). Model portfolios skip this step (no cash flows → simple == TWR). This ensures period extremes are comparable across portfolio types.
- [x] Compute cross-portfolio metrics: beta/alpha, correlation
- [x] Compute cross-portfolio overlap (from Task 4) — wired via `AllocationSource` interface that reuses the allocation service's `ComputeAllocation` (resolves accounts → fetches positions → enriches with market data → allocation percentages). Model portfolios use their weights directly. Both paths convert to `PortfolioHolding` for the overlap computation.
- [x] Handle edge cases: real portfolio with no transactions (empty metrics + message), model portfolio with missing data (warning + clipped period), insufficient data for risk metrics (<30 trading days → N/A)
- [x] Write service tests with hand-written mocks in `service_test.go` — mock model portfolio source, equity curve source, market data history. Test: model-vs-model, model-vs-real, real-vs-real, empty real portfolio, missing market data

**Verification:** `go test ./internal/domain/comparison/` passes. Service returns correct ComparisonResult for a model-vs-model scenario with mocked data. Real portfolio with no transactions returns empty metrics with explanatory message.

---

### Task 4: Cross-Portfolio Overlap and Intra-Portfolio Correlation Extension [PRIORITY: MEDIUM]
**Corresponds to:** Scenario "View holdings overlap", "View correlation matrix"

**Description:** Extend existing overlap and correlation computations to support the comparison context (two portfolios side-by-side). The existing `analysis.ComputeOverlap` works on a single portfolio's positions; the comparison needs top-10 holdings per portfolio + overlap percentage.

- [x] Add `ComputeCrossPortfolioOverlap` in `internal/domain/comparison/overlap.go` — takes two sets of (symbol, weight) pairs, resolves ETF holdings for each, produces top-10 per portfolio + overlap percentage. Reuse `analysis.buildHoldingMap` and `symbol.TopHolding` patterns.
- [x] Add `ComputeIntraPortfolioCorrelation` in `internal/domain/comparison/correlation.go` — takes historical prices for symbols within one portfolio, produces correlation matrix. Thin wrapper around `analysis.ComputeCorrelation` adapted for model portfolio weights (not position-based weights).
- [x] Write tests in `overlap_test.go` and `correlation_test.go`: two portfolios with shared ETF holdings, portfolios with no ETFs (atomic symbols), single-symbol portfolio, symbols with no holdings data

**Verification:** `go test ./internal/domain/comparison/` passes. Two portfolios sharing 3 ETF holdings produce correct overlap percentage. Portfolio with only stocks (no ETFs) produces top-10 as the stocks themselves.

---

### Task 5: API Handler and Route Registration [PRIORITY: HIGH]
**Corresponds to:** US-1, US-2, US-3 (all user stories)

**Description:** Create the HTTP handler for the comparison API endpoint. Follows the `analysis` handler pattern: separate API handler + web handler, shared `computeResult` method.

- [x] Create `internal/api/handlers/comparison.go` with `ComparisonHandler` struct, `HandleComparison` (GET `/api/comparison`), query param parsing (portfolio_a_id, portfolio_a_type, portfolio_b_id, portfolio_b_type, period, date_from, date_to, base_currency, starting_value)
- [x] Validate inputs: portfolio types must be "model" or "real", period validation, starting value > 0
- [x] Implement `computeResult` that delegates to `comparison.Service.ComputeComparison`
- [x] Create `internal/api/handlers/comparison_test.go` with handler-level tests using `httptest.NewRecorder`: valid request, invalid portfolio type, missing portfolio ID, invalid period
- [x] Register routes in `internal/api/router.go` (API handler only; web handler in Task 6)
- [x] Wire comparison service into router with all required dependencies (model portfolio service, position service, market service, symbol details, etc.)

**Verification:** `go test ./internal/api/handlers/ -run Comparison` passes. GET `/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model` returns valid JSON ComparisonResult.

---

### Task 6: Web UI — Comparison Page [PRIORITY: HIGH]
**Corresponds to:** All user-facing scenarios (charts, tables, selectors)

**Description:** Build the server-rendered comparison page with ECharts visualizations. Follows the `analysis_web.go` pattern: web handler delegates to API handler's `computeResult`, serializes chart data as JSON, renders template.

- [x] Create `internal/api/handlers/comparison_web.go` with `ComparisonWebHandler`, `HandleComparison` (GET `/comparison`), portfolio selector fetching
- [x] Serialize chart data: drawdown line chart (both portfolios), annual returns bar chart, return frequency histograms, correlation matrix heatmap, overlap visualization
- [x] Create `templates/comparison/index.html` with sections: performance metrics table, risk metrics table, drawdown chart, annual returns chart, period extremes table, overlap section, correlation section
- [x] Add nav link: add "Comparison" entry to `templates/partials/nav.html`
- [x] Register web routes in `internal/api/router.go`
- [x] Create `internal/api/handlers/comparison_web_test.go` with handler tests: page renders with both portfolios selected, page renders error state for missing data

**Verification:** Server starts, GET `/comparison` renders HTML with portfolio selectors and chart containers. ECharts JSON data is valid. Nav includes "Comparison" link.

---

### Task 7: Integration Test [PRIORITY: MEDIUM]
**Corresponds to:** Full-stack verification

**Description:** End-to-end integration test exercising the full comparison stack with real SQLite schema.

- [ ] Create `tests/integration/comparison_test.go` using the shared test helper (`tests/integration/db.go`)
- [ ] Seed test data: create model portfolios, insert historical market data for symbols
- [ ] Test: GET `/api/comparison` with two model portfolios returns valid result with computed metrics
- [ ] Test: GET `/api/comparison` with real portfolio (transactions) vs model portfolio
- [ ] Test: edge case — real portfolio with no transactions returns empty metrics + message
- [ ] Test: period filtering (1Y, 3Y, custom date range)

**Verification:** `go test ./tests/integration/ -run Comparison` passes.

---

### Task 8: Database Migration (if needed) [PRIORITY: LOW]
**Corresponds to:** N/A — comparison is computed on-demand, no persistent storage needed

**Description:** The comparison engine is fully computational — no new database tables are required. Model portfolios already exist in `model_portfolios` table (migration 020). Real portfolio data uses existing `transactions` and `positions` tables.

- [ ] Confirm no migration needed — comparison results are ephemeral (computed on request)

**Verification:** No migration file created. Feature works with existing schema.

---

## Technical Decisions

| Decision | Options | Recommendation | Reason |
|---|---|---|---|
| **Where to put simulation + metrics** | Extend existing `comparison` domain vs new `portfoliocomparison` domain | Extend `comparison` domain | The existing `comparison` package already has MWR/monthly return functions. Extending it keeps related comparison logic together. |
| **Equity curve for model portfolios** | Store precomputed curves in DB vs compute on-demand | Compute on-demand | Spec says "computed on demand". Precomputing would require scheduling, caching, and invalidation logic. On-demand is simpler and matches the "no scheduling" non-goal. |
| **Beta/Alpha computation** | Use `float64` for regression vs `decimal.Decimal` throughout | `float64` for intermediate computation, `decimal.Decimal` for results | Following existing pattern: `analysis.ComputeCorrelation` uses `float64` internally for Pearson correlation. Statistical computations (regression, correlation) are inherently float-based. Results are rounded to `decimal.Decimal` for API consistency. |
| **API endpoint design** | Single endpoint with portfolio_a/portfolio_b params vs separate endpoints | Single endpoint `GET /api/comparison` | Simpler, matches the "compare 2 portfolios" mental model. Query params identify both portfolios and their types. |
| **Risk-free rate for Sharpe** | Hardcode 0% vs configurable vs fetch from market data | Fixed rate, default 0%, displayed to user | Spec says "defaults to 0% if unavailable; the rate used is displayed". Reuses existing `ComputeRiskMetrics` single-rate API. **Follow-up enhancement:** replace with moving daily rate (e.g., daily 3-month Treasury yield) for multi-year accuracy — requires historical Treasury data source and `ComputeRiskMetrics` signature change. |
| **Chart library** | ECharts (existing) vs Chart.js | ECharts | Consistent with existing pages (analysis, performance). Reuse existing JS infrastructure. |
| **FX rate handling for model portfolio simulation** | Fetch historical FX rates vs use current spot rate | Fetch historical FX rates with spot rate fallback + warning | Spec says "current spot rate is used as a fallback; a warning is shown". Use `performance.fx_conversion.go` pattern for historical FX lookup. |
| **Period handling** | Fixed periods only vs fixed + custom date range | Both (from spec) | Spec explicitly supports both fixed periods (1Y, 3Y, 5Y, YTD, All) and custom date ranges. |

## Risks

- **Market data availability** — Model portfolio symbols may lack sufficient historical data. Mitigation: explicit warnings + period clipping + "refresh market data" prompt.
- **Performance of simulation** — Fetching historical prices for many symbols could be slow. Mitigation: use cached data (existing `market_data` table), fail fast on missing data.
- **Complexity of FX conversion** — Multi-currency model portfolios require historical FX rates for each non-base currency. Mitigation: reuse existing `fx_conversion.go` infrastructure; spot rate fallback.
- **Scope creep** — The spec covers many metrics and chart types. Mitigation: strict adherence to task boundaries; each task is independently testable.
