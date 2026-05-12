# Implementation Plan: Performance Benchmark

## Overview

Add benchmark comparison to the existing performance page. Five pre-defined benchmarks are just tickers (e.g. `^GSPC`) fetched and cached through the existing market data infrastructure — no special types, no special storage. The user selects a benchmark via a dropdown in the filter bar; the selection is shared across the equity chart, MWR statistics, and a new monthly return heatmap.

No database migration is needed — benchmarks use the existing `market_data` table with `data_type = 'stock'`. The "Refresh All" button is extended to also fetch benchmark data.

**Design principle:** The comparison/computation layer is generic and works with *any* ticker. The UI constraint (5 predefined tickers, dropdown-only) is enforced at the handler/template layer. This keeps the door open for future features (custom comparison tickers, Sharpe ratio, beta, correlation) without refactoring the core logic.

## Task Dependencies

```
Task 1 (constants + compute)
    ↓
Task 2 (extend MarketCache.RefreshAll) ← Task 1
    ↓
Task 3 (extend API endpoint) ← Task 1, Task 2
    ↓
Task 4 (extend web handler) ← Task 3
    ↓
Task 5 (template: chart + MWR + selector) ← Task 4
    ↓
Task 6 (monthly heatmap) ← Task 4, Task 5
    ↓
Task 7 (validation)
```

Tasks 1–2 are backend foundation. Tasks 3–6 add user-facing layers incrementally. Task 7 is the cross-cutting quality gate.

## Tasks

### Task 1: Comparison constants and computation [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundational data and math)
**Description:** Define the five predefined benchmark tickers with display names and provide pure computation functions for MWR and monthly returns from historical prices. The computation functions work on any ticker's price data — they are not benchmark-specific.

- [x] Create `internal/domain/comparison/comparison.go` with:
  - `var Predefined = map[string]string{...}` — ticker → display name (e.g. `"^GSPC": "S&P 500"`) for the 5 predefined benchmarks
  - `func IsValidPredefined(ticker string) bool` — checks if ticker is one of the predefined benchmarks (used by handler to validate dropdown selection)
  - `func GetPredefined() map[string]string` — returns the map (for dropdown options)
- [x] Create `internal/domain/comparison/compute.go` with:
  - `func ComputeMWR(prices []market.HistoricalPrice) *decimal.Decimal` — simple holding-period return: `(end/start - 1) * 100`. Returns nil if fewer than 2 prices or start ≤ 0.
  - `func ComputeMWRForPeriod(prices []market.HistoricalPrice, from, to time.Time) *decimal.Decimal` — filters prices to [from, to] then computes MWR
  - `func ComputeMonthlyReturns(prices []market.HistoricalPrice) map[string]*decimal.Decimal` — groups prices by year-month, returns monthly return % as map of `"YYYY-MM" -> return`. Used by heatmap (Task 6) and future analytics.
- [x] Write tests for `comparison.go`:
  - `IsValidPredefined` returns true for each of the 5 tickers
  - `IsValidPredefined` returns false for random/unknown tickers
  - `GetPredefined` returns all 5 entries
- [x] Write tests for `compute.go` (table-driven):
  - `ComputeMWR`: normal case (5+ prices up/down), single price → nil, empty → nil, declining → negative, flat → zero, start = 0 → nil
  - `ComputeMWRForPeriod`: filters correctly and computes MWR for the window
  - `ComputeMonthlyReturns`: multiple months with varying returns, single month, empty prices, partial month data

**Verification:** `go test ./internal/domain/comparison/...` passes.

---

### Task 2: Extend MarketCache.RefreshAll to include benchmarks [PRIORITY: HIGH]

**Corresponds to:** Scenario "Select a benchmark for the first time" (data availability)
**Description:** When the user clicks "Refresh All" on the performance page, also fetch historical data for all 5 predefined benchmark tickers. Benchmarks are treated like any other symbol.

- [x] Add `func (m *MarketCache) RefreshPredefinedBenchmarks(ctx context.Context)` to `marketcache.go`:
  - Iterates over the 5 predefined benchmark tickers from `comparison.Predefined`
  - For each ticker: fetches historical prices from earliest available to now via `fetchHistoricalDirect`
  - Upserts via `repo.UpsertHistoricalPrices(ctx, ticker, prices, "stock")`
  - Logs fetch success/failure per ticker
- [x] Modify `func (m *MarketCache) RefreshAll(ctx context.Context)` to also call `RefreshPredefinedBenchmarks` after refreshing portfolio symbols
- [x] Write tests:
  - `RefreshPredefinedBenchmarks` with mock fetcher: all 5 tickers fetched and upserted
  - `RefreshPredefinedBenchmarks` with partial fetch failure: failed tickers logged, others succeed
  - `RefreshAll` includes benchmarks (verifies call order via mock)

**Verification:** `go test ./internal/domain/marketcache/...` passes; RefreshAll now covers benchmarks.

---

### Task 3: Extend performance API endpoint [PRIORITY: HIGH]

**Corresponds to:** Scenario "Select a benchmark for the first time", "Switch between different benchmarks", "Clear the benchmark selection", "Benchmark changes with date range"
**Description:** Extend `GET /api/performance` to accept an optional `benchmark` query parameter. When present, fetch cached prices for the ticker, compute MWR, and include in the response. The computation functions work with any ticker — the predefined constraint is enforced at the validation layer.

- [ ] Modify `parsePerformanceFilters` in `performance.go` to extract `benchmark` query param (ticker string, empty = no benchmark)
- [ ] Validate against `comparison.IsValidPredefined` — reject non-predefined tickers with 400
- [ ] Extend `PerformanceFilters` in `position/performance_types.go` with `Benchmark string` field
- [ ] Extend `PerformanceResult` in `position/performance_types.go` with:
  - `BenchmarkTicker string` — selected benchmark ticker (empty if none)
  - `BenchmarkPrices []market.HistoricalPrice` — cached benchmark prices for the period
  - `BenchmarkMWRPct *decimal.Decimal` — benchmark MWR for the period
  - `BenchmarkCurrency string` — benchmark currency (from price data)
  - `BenchmarkWarning string` — data availability warning (empty if OK)
- [ ] Modify `HandlePerformance` in `performance.go`:
  - After computing portfolio result, if `filters.Benchmark` is set and valid:
    - Read cached prices via `marketService.GetHistoricalPrices(ctx, ticker, dateFrom, dateTo)`
    - Compute MWR via `comparison.ComputeMWRForPeriod(prices, dateFrom, dateTo)`
    - Populate benchmark fields on the result
    - Set warning if no data available
  - If invalid ticker, return 400 error
- [ ] Write tests (table-driven, using hand-written mock for MarketDataService):
  - No benchmark: response unchanged (benchmark fields empty/nil)
  - Valid benchmark with data: prices, MWR, currency populated
  - Valid benchmark, no cached data: warning set, prices empty
  - Invalid (non-predefined) benchmark ticker: 400 error
  - Benchmark with partial data: prices for available range, MWR computed

**Verification:** `go test ./internal/api/handlers/... -run Performance` passes.

---

### Task 4: Extend performance web handler [PRIORITY: HIGH]

**Corresponds to:** Scenario "Select a benchmark for the first time", "Benchmark selection is shared across views"
**Description:** Wire benchmark data through the web handler to the template. No benchmark service needed — the handler reads cached prices directly via the existing MarketDataService.

- [ ] Modify `HandlePerformance` in `performance_web.go`:
  - Parse `benchmark` from query params
  - Pass benchmark ticker through `PerformanceFilters` to `ComputeEquityCurve` (benchmark data already included in the API result)
  - If benchmark data exists, serialize benchmark prices for chart JSON
  - Compute benchmark monthly returns for heatmap
- [ ] Extend `performancePageData` struct with:
  - `SelectedBenchmark string` — ticker of selected benchmark
  - `BenchmarkNames map[string]string` — available benchmarks for dropdown
  - `BenchmarkTicker string` — same as SelectedBenchmark (for template clarity)
  - `BenchmarkChartData string` — JSON-serialized benchmark prices for ECharts (`[{date, price}, ...]`)
  - `BenchmarkMWRPct *decimal.Decimal` — benchmark MWR for display
  - `BenchmarkCurrency string` — benchmark currency
  - `BenchmarkWarning string` — data availability warning
  - `BenchmarkURLs map[string]string` — pre-built URLs for each benchmark option
  - `MonthlyReturns []monthlyReturnData` — monthly return data for heatmap (added in Task 6, but struct field added here)
- [ ] Add `buildBenchmarkURLs(selectedTicker, portfolioID, period string) map[string]string` — pre-built URLs preserving portfolio_id, period, and setting/changing benchmark
- [ ] Modify `buildPeriodURLs` to preserve `benchmark` param in URLs
- [ ] Wire in `router.go`: no new dependency needed (comparison is just a package import for constants + compute)
- [ ] Write tests:
  - Handler with no benchmark: benchmark fields empty, page renders as before
  - Handler with benchmark: benchmark data populated in page data
  - Handler with benchmark, no data: warning populated
  - `buildBenchmarkURLs` preserves other query params
  - `buildPeriodURLs` preserves benchmark param

**Verification:** `go test ./internal/api/handlers/... -run PerformanceWeb` passes.

---

### Task 5: Update performance template (chart + MWR + selector) [PRIORITY: HIGH]

**Corresponds to:** Scenario "Overlay a benchmark on the performance chart", "View MWR comparison on the performance page", "Clear the benchmark selection", "Switch between different benchmarks", "Benchmark changes with date range"
**Description:** Add benchmark selector to filter bar, overlay benchmark line on equity chart with dual Y-axes, show benchmark MWR alongside portfolio MWR.

- [ ] Add benchmark selector dropdown to the filter bar (after period buttons, before apply button):
  - Options: "None" + 5 benchmarks (display name + ticker, e.g. "S&P 500 (^GSPC)")
  - Uses pre-built URLs from `BenchmarkURLs` map
  - Selected state highlighted via active class
- [ ] Update ECharts config for dual Y-axes:
  - Left Y-axis: portfolio value (absolute, existing behavior)
  - Right Y-axis: benchmark price (absolute value) — shown only when benchmark selected
  - Portfolio line on left axis, benchmark line on right axis
  - Legend includes benchmark name when selected
  - Tooltip shows portfolio value and benchmark price
  - Benchmark line uses a distinct color (e.g. gray/dashed)
- [ ] Add benchmark MWR card next to portfolio MWR card:
  - Label: "Benchmark MWR"
  - Value: benchmark MWR percentage with positive/negative coloring
  - Hidden when no benchmark selected
- [ ] Show benchmark warning (e.g. badge or small text) when data is stale/unavailable
- [ ] Write template render tests:
  - With benchmark: selector shows selected, chart has dual axes + benchmark series, MWR card visible
  - Without benchmark: selector shows "None", single Y-axis, MWR card hidden
  - With benchmark warning: warning visible
  - Switch benchmarks: URLs correctly change ticker param

**Verification:** Page renders correctly with and without benchmark; ECharts shows dual-axis chart; template tests pass.

---

### Task 6: Monthly return heatmap [PRIORITY: MEDIUM]

**Corresponds to:** Scenario "Monthly heatmap without benchmark", "Monthly heatmap with benchmark comparison", "Heatmap updates when benchmark changes", "Heatmap updates when date range changes"
**Description:** Add a monthly return heatmap section below the equity chart. Without benchmark: color by absolute return. With benchmark: color by relative performance (outperformed/underperformed).

- [ ] Compute monthly returns from equity curve in the web handler:
  - Group equity curve points by year-month
  - For each month: return = `(end_value / start_value - 1) * 100`
  - Produce `[]monthlyReturnData` struct: `Year int, Month int, PortfolioReturn string, BenchmarkReturn string, Diff string`
- [ ] Compute benchmark monthly returns using `comparison.ComputeMonthlyReturns(prices)` (group by month, same formula)
- [ ] Add `Diff` field: portfolio return − benchmark return (for relative coloring)
- [ ] Extend `performancePageData` with `MonthlyReturns []monthlyReturnData` (struct field added in Task 4)
- [ ] Add heatmap section to template (below equity chart, above footer):
  - HTML table: months as columns (Jan–Dec), years as rows
  - Without benchmark: cells colored by absolute return (green positive, red negative)
  - With benchmark: cells colored by diff (warmer = outperformed, cooler = underperformed)
  - Cell text: portfolio return % (with benchmark: shows "portfolio% vs benchmark%" or diff)
  - Toggle: when benchmark selected, show relative coloring; "None" shows absolute
  - Uses CSS classes for color coding (add to style.css)
- [ ] Handle edge cases: months with no data (empty/gray cell), partial months
- [ ] Write template render tests:
  - Heatmap without benchmark: absolute return coloring
  - Heatmap with benchmark: relative coloring
  - Empty data: empty state message

**Verification:** Heatmap renders correctly in both modes; template tests pass.

---

### Task 7: Validation / Hardening [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (cross-cutting quality gate)
**Description:** After implementation tasks are complete, validate the feature end-to-end before declaring it done.

- [ ] Run all tests (`go test ./...`) — not just `-short`
- [ ] Verify each spec scenario manually or via integration test
- [ ] Check edge cases from the spec against actual behavior:
  - Benchmark data unavailable → warning shown, portfolio data still displays
  - Partial data overlap → benchmark line starts from first available point
  - Empty portfolio → benchmark can still display independently
  - Very short date range → comparison works
  - Date range changes → benchmark updates
  - Benchmark data has gaps → line breaks at discontinuities
- [ ] Run `go vet ./...` and linter
- [ ] Review for cross-layer consistency (data types stored match data types read)
- [ ] Verify no TODOs, FIXMEs, or temporary workarounds remain
- [ ] Verify benchmark tickers match spec exactly: `^GSPC`, `^IXIC`, `VWRP.L`, `VUSA.L`, `XNAQ.L`
- [ ] Check that GBp→GBP conversion in `FetchHistoricalPricesBatch` handles UK benchmarks (VWRP.L, VUSA.L, XNAQ.L)

**Verification:** All tests pass, all spec scenarios validated, no unresolved issues.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Benchmark data storage | Reuse existing `market_data` table with `data_type = 'stock'` | No migration needed; benchmarks are just tickers |
| Benchmark types | `map[string]string` ticker → display name, no special struct | Benchmarks are regular stock quotes; only need display names |
| Comparison/compute layer | Generic functions in `internal/domain/comparison/compute.go` | Work with any ticker's prices; future features (Sharpe, beta, correlation) reuse them without refactoring |
| Predefined constraint | Enforced at handler layer via `comparison.IsValidPredefined` | UI limited to 5 tickers now, but backend is open to any ticker |
| Benchmark refresh | Integrated into existing `MarketCache.RefreshAll` | "Refresh All" refreshes all market data including benchmarks |
| Benchmark MWR | Simple holding-period return `(end/start - 1) * 100` | No cash flows for an index; MWR = simple return |
| Chart dual Y-axes | Portfolio value (left) + benchmark price (right) | Both show absolute values on their own scale |
| Monthly heatmap | HTML table with CSS color classes | Simple, no new JS dependency |
| Benchmark selection | Query parameter `?benchmark=^GSPC` | Stateless, bookmarkable, shared across views |

## Future Extension Points

The generic comparison layer enables these features without refactoring the core logic:

- **Custom comparison tickers:** Remove the `IsValidPredefined` check in the handler, add a text input alongside the dropdown. The compute functions already work with any ticker.
- **Sharpe ratio:** Add `func ComputeSharpe(prices []market.HistoricalPrice, riskFreeRate decimal.Decimal) *decimal.Decimal` to `compute.go`. Uses `ComputeMonthlyReturns` internally.
- **Beta / Correlation:** Add `func ComputeBeta(portfolioReturns, benchmarkReturns map[string]*decimal.Decimal) *decimal.Decimal` to `compute.go`. Both use `ComputeMonthlyReturns` as input.
- **Multiple benchmarks:** Add `?compare2=`, `?compare3=` params. Chart adds more series. Compute functions are already per-ticker.

## Risks

- **Yahoo Finance index ticker support:** Index tickers like `^GSPC` and `^IXIC` may behave differently than stock tickers in go-yfinance. Mitigation: test fetching each benchmark ticker early (Task 1/2) and fall back to a warning if data is unavailable.
- **UK-domiciled ETF GBp pricing:** VWRP.L, VUSA.L, XNAQ.L may return prices in pence (GBp). Mitigation: the existing `FetchHistoricalPricesBatch` already handles GBp→GBP conversion; verify this works for these tickers.
- **Benchmark data gaps:** Index data may have more gaps than stock data. Mitigation: chart handles gaps gracefully (line breaks at discontinuities).
