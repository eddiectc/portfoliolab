# Retrospective: Performance Benchmark

**Feature:** f012_performance-benchmark
**Date:** 2026-05-13
**Status:** Complete (with post-review fixes)

---

## Spec vs Reality

### What the spec got right
- **5 predefined benchmarks** — all tickers (`^GSPC`, `^IXIC`, `VWRP.L`, `VUSA.L`, `XNAQ.L`) matched exactly
- **Three comparison views** (chart overlay, MWR stat, monthly heatmap) all implemented as specified
- **Edge cases** — data unavailable, partial overlap, empty portfolio, short date range all handled
- **Constraints** — single benchmark at a time, return-based (percentage) comparison, no custom benchmarks
- **Dependencies** — reused f010 performance page and f011 market data caching without breaking changes

### What the spec missed
- **Chart mode divergence:** The spec described a dual Y-axis chart (portfolio value left, benchmark price right). The final implementation uses two distinct chart modes:
  - **No benchmark:** Two-panel layout (value + return indicator sub-panel) — a user-requested enhancement not in the spec
  - **With benchmark:** Percentage comparison (both lines as % return from start) — differs from the spec's "absolute price on right axis" design
- **Benchmark MWR label:** Spec said "Benchmark MWR" but the card shows "Benchmark MWR (currency)" to disambiguate multi-currency benchmarks
- **Heatmap cell format:** Spec described "portfolio% vs benchmark%" text. Final UX uses portfolio return as main value + `▲/▼ diff%` alpha indicator below — cleaner but not specified

### What was over-specified
- **Risk-adjusted metrics** (alpha, beta, Sharpe) were explicitly non-goals and remained so — good boundary
- **Multiple simultaneous benchmarks** — correctly excluded, single benchmark constraint held

---

## Plan vs Reality

### Task execution
| Task | Status | Notes |
|------|--------|-------|
| 1: Comparison constants + compute | ✅ Complete | `comparison.go` + `compute.go` with comprehensive table-driven tests |
| 2: Extend MarketCache.RefreshAll | ✅ Complete | `RefreshPredefinedBenchmarks` added; later enhanced with `gapFillBenchmarks` in `Start()` |
| 3: Extend API endpoint | ✅ Complete | Benchmark validation + data fetching in API handler |
| 4: Extend web handler | ✅ Complete | Benchmark data wiring, URL builders, monthly returns computation |
| 5: Template (chart + MWR + selector) | ✅ Complete | Dual-mode chart, benchmark selector dropdown, MWR card |
| 6: Monthly heatmap | ✅ Complete | HTML table with absolute/relative coloring modes |
| 7: Validation | ✅ Complete | All tests pass, all spec scenarios validated |

### Plan deviations
- **None documented.** The 7-task plan was followed sequentially as designed.

### Technical decisions validated
| Decision | Outcome |
|----------|---------|
| Reuse `market_data` table with `data_type = 'stock'` | ✅ Worked seamlessly, no migration needed |
| Generic compute functions in `comparison/compute.go` | ✅ Proved valuable — user-requested chart improvements reused `ComputeMonthlyReturns` |
| Predefined constraint at handler layer | ✅ Clean separation; backend remains open to any ticker |
| Benchmark refresh in `RefreshAll` | ⚠️ Partial — required post-fix: `Start()` also calls `gapFillBenchmarks` |
| Dual Y-axes (portfolio value + benchmark price) | ❌ Changed — final chart uses percentage comparison mode instead |
| Monthly heatmap as HTML table | ✅ Simple, no new JS dependency |
| Query parameter `?benchmark=` | ✅ Stateless, bookmarkable, shared across views |

---

## Bugs Found Post-Review

### Severity: High

**1. Monthly heatmap repeated rows**
- **Symptom:** Each year row duplicated N times (N = months with data that year)
- **Root cause:** `computeMonthlyReturnsFromCurve` returned flat `[]monthlyReturnData`, template iterated each entry as a row + O(n²) inner lookup
- **Fix:** Restructured to `[]yearReturnData` with `Months map[int]monthCellData`
- **Lesson:** Template rendering bugs with nested loops are hard to catch in unit tests. Consider adding a snapshot/render test that counts rows.

**2. Monthly returns counted deposits as profit**
- **Symptom:** Mid-month deposit inflated monthly return (e.g., 100k→200k deposit + 5% gain = 110% return instead of 5%)
- **Root cause:** Simple return `(last_PV / first_PV - 1)` didn't account for intra-month cash flows
- **Fix:** Replaced with `computeMonthlyTWR` — detects cash flow dates from `NetDeposit` changes, geometrically links sub-period returns
- **Lesson:** Any return calculation touching equity curve values must account for cash flows. The overall TWR metric was correct (uses pre-cash-flow breakpoints), but the monthly version wasn't. This is a domain logic trap — the fix mirrors the existing TWR logic.

### Severity: Medium

**3. Benchmark data not fetched automatically**
- **Symptom:** Benchmark symbols had no data after initial setup; user had to click "Refresh All" manually
- **Root cause:** `RefreshPredefinedBenchmarks` only called by `doRefreshAll` (manual refresh), not by periodic `doRefresh` cycle
- **Fix:** Added `go m.gapFillBenchmarks(ctx)` in `MarketCache.Start()` for background gap-fill on startup
- **Lesson:** When adding new data sources to a caching layer, ensure they're covered by both manual refresh AND automatic background refresh. The periodic cycle and startup initialization are two separate paths.

**4. Heatmap UX improvements**
- **Cell display:** Two values in one cell was confusing → changed to main value + alpha indicator
- **Color scale:** Blue for underperform was inconsistent → changed to red (consistent with absolute mode)
- **Empty cells:** Benchmark-only months (before portfolio existed) cluttered the view → filtered to portfolio months only
- **Chart error handling:** Added try-catch around ECharts init
- **Lesson:** UI polish often emerges only during visual review. Consider a "UI review" checklist item in Task 7.

---

## Test Coverage Assessment

### What tests caught
- `comparison/compute_test.go`: 24 table-driven test cases covering MWR, MWRForPeriod, MonthlyReturns — caught edge cases (empty, single price, zero start, partial months)
- `handlers/performance_test.go`: Handler tests with mock `MarketDataService` — caught validation logic (invalid benchmark → 400), no-data scenarios
- `handlers/performance_web_test.go`: Web handler tests — caught URL building edge cases, benchmark data population

### What tests missed
- **Heatmap row duplication:** The template render tests may have checked for correct data presence but not row count. A test asserting "N years → N rows" would have caught this.
- **Monthly TWR bug:** The `computeMonthlyTWR` function didn't exist when tests were written. The original `computeMonthlyReturnsFromCurve` used simple return and tests passed. Adding a test with mid-month cash flows would have caught this immediately.
- **Benchmark auto-fetch:** No test verified that `Start()` triggers benchmark fetching. The `RefreshPredefinedBenchmarks` test covered manual refresh only.
- **Chart mode:** No automated test for the two chart rendering modes (benchmark vs no-benchmark). The JS chart code is hard to test without a browser.

### Recommendations
- Add a test case to `computeMonthlyReturnsFromCurve` with mid-month cash flows asserting TWR isolates the investment return
- Add a test for `MarketCache.Start()` verifying benchmark gap-fill is triggered (mock the fetcher)
- Consider a template render test that counts `<tr>` elements in the heatmap

---

## Code Quality

### Strengths
- **Generic comparison layer:** `compute.go` functions work with any ticker's prices, enabling future extensions (Sharpe, beta, correlation) without refactoring
- **Hand-written mocks:** Follow project convention — `mockPosRepoForPerf` and `mockMarketDataService` simulate real behavior
- **Decimal usage:** All monetary values use `decimal.Decimal`, never `float64` (except intermediate calculations in compute functions, which is acceptable for ratios)
- **URL builders:** `buildBenchmarkURLs` and `buildPeriodURLs` pre-build URLs preserving query params — clean separation from template
- **Error handling:** Benchmark fetch failures gracefully degrade (warning message, portfolio data still shows)

### Areas for improvement
- **`determineDateRange` duplication:** The function is duplicated between `position/equity_curve.go` and `performance.go`. Noted in NOTES.md as a future extraction candidate.
- **Float64 in compute functions:** `ComputeMWR` and `ComputeMonthlyReturns` use `Float64()` for ratio calculation. For extreme values, this could lose precision. Acceptable for benchmark returns but worth noting.
- **Inline JSON in template:** Chart data is JSON-serialized server-side and embedded in `<script>` tags. Works but limits client-side interactivity.
- **Bubble sort:** `computeMonthlyReturnsFromCurve` uses bubble sort for months and years. Fine for the small data set (< 10 years) but `sort.Strings` / `sort.Ints` would be more idiomatic.

---

## Spec Accuracy Score

| Aspect | Score | Notes |
|--------|-------|-------|
| User stories | 5/5 | All 3 user stories fully implemented |
| Scenarios | 4.5/5 | All 12 scenarios covered; chart mode differs from spec but is a UX improvement |
| Edge cases | 5/5 | All 8 edge cases handled correctly |
| Constraints | 5/5 | All 5 constraints respected |
| Non-goals | 5/5 | No scope creep into risk metrics, custom benchmarks, etc. |
| **Overall** | **4.8/5** | Excellent spec accuracy; chart mode change was a justified UX improvement |

---

## Lessons Learned

### What went well
1. **Generic compute layer:** The decision to make `comparison/compute.go` work with any ticker (not just predefined benchmarks) paid off immediately when user-requested chart improvements needed the same functions
2. **Incremental task plan:** The 7-task dependency chain allowed each task to be independently tested before proceeding
3. **Reuse over new types:** Treating benchmarks as regular stock symbols (`data_type = 'stock'`) avoided database migrations and kept the codebase simple
4. **Query parameter for state:** `?benchmark=` is stateless, bookmarkable, and shared across all views on the page

### What to improve
1. **TWR for monthly returns:** Any return calculation on equity curve data must use TWR (not simple return) to isolate investment performance from cash flows. Add this to the AGENTS.md best practices.
2. **Template rendering tests:** Add row-count assertions and structural checks for complex templates (heatmaps, tables with nested loops)
3. **Auto-refresh coverage:** When adding new data sources to the cache, verify both manual refresh AND background/periodic refresh paths
4. **Chart design in spec:** The spec described a dual Y-axis chart but the final percentage comparison chart is more useful. Consider describing the *comparison intent* (side-by-side % return) rather than the *visual implementation* (dual Y-axes) in future specs
5. **UI review checklist:** Add visual review items (color consistency, empty states, error states) to the Task 7 validation checklist

### Process improvements
- **Pre-review checklist:** Before declaring a feature "done," verify: (a) auto-refresh covers new data, (b) return calculations use TWR where cash flows are possible, (c) template row counts match expected output
- **Cross-cutting concern:** The TWR bug affected the monthly heatmap but not the overall TWR metric because they were computed by different functions. Consider a shared `computeTWR` function that both paths use.

---

## Future Extension Points (validated by implementation)

The generic comparison layer enables these without refactoring:
- **Custom comparison tickers:** Remove `IsValidPredefined` check, add text input
- **Sharpe ratio:** Add `ComputeSharpe` to `compute.go`, uses `ComputeMonthlyReturns`
- **Beta / Correlation:** Add `ComputeBeta` to `compute.go`, uses `ComputeMonthlyReturns` for both inputs
- **Multiple benchmarks:** Add `?compare2=`, `?compare3=` params, chart adds more series
