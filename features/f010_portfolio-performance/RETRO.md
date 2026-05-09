# Retrospective: Portfolio Performance

## What Went Well

- **Comprehensive test coverage**: 50+ tests across 6 test files covering domain logic, API handlers, web handlers, and model serialization. Table-driven tests for return metrics and equity curve helpers are thorough and readable.
- **All 11 spec scenarios implemented**: Every scenario from SPEC.md has corresponding implementation and test coverage, including all edge cases (empty state, negative net deposit, mismatched currencies, missing market data, FX rate gaps).
- **Clean separation of concerns**: Equity curve computation (`equity_curve.go`), return metrics (`return_metrics.go`), and refresh (`refresh.go`) are well-separated with clear responsibilities. Pure functions (`ComputeReturnMetrics`, `walkTransactions`, `interpolateDaily`) are easily testable.
- **Pragmatic technical decisions**: Using `math.Pow` for CAGR exponentiation (instead of implementing ln/exp for decimal) was the right call for a display-only metric. The `multi.NewTickers` + per-ticker `History()` approach correctly addressed the currency metadata gap in `multi.Download`.
- **Graceful degradation**: Missing market data, FX rate gaps, and fetch failures are all handled with warnings rather than hard errors. The equity curve still renders with partial data.
- **Web UI completeness**: Template includes portfolio selector, period buttons (all 8 periods), ECharts integration, summary metric cards, refresh button, empty state, error state, and warning display — all matching the spec.

## What Could Be Improved

- **File naming clarity**: The plan specified `performance.go` for domain types, but the actual file is `performance_types.go`. While the split into separate files (`performance_types.go`, `equity_curve.go`, `return_metrics.go`, `refresh.go`) is better organization, the plan should have anticipated this.
- **Period label inconsistency**: The spec says "All Time" but the implementation uses "All" as the period key. Functionally correct, but the UI label and internal key diverge from the spec wording.
- **No result caching**: Performance data is computed on every page load, including fetching historical prices from Yahoo Finance. For portfolios with many symbols or long date ranges, this could be slow. The spec mentions "pre-computed and cached" as a non-goal, but a simple in-memory or DB cache would be a reasonable follow-up.
- **Interpolation generates many points**: For a 5-year period, daily interpolation produces ~1,800 data points passed to the browser as JSON. ECharts handles this fine, but it increases page weight. Consider downsampling for long periods in a future iteration.
- **Refresh only updates current prices**: The chosen approach (Technical Decision D) only refreshes current quotes, not historical prices. This means the equity curve for past dates may use stale prices. The plan acknowledges this as a known limitation.

## Spec vs Reality

| Spec Item | Status | Notes |
|---|---|---|
| Equity curve with portfolio value + net deposit | ✅ Implemented | Daily interpolation, FX conversion, all covered |
| Summary return metrics (total return %, CAGR) | ✅ Implemented | Pure function, comprehensive edge case handling |
| Multi-currency FX conversion | ✅ Implemented | Historical rates with spot rate fallback |
| Period selector (1W/1M/3M/1Y/3Y/5Y/YTD/All) | ✅ Implemented | All 8 periods supported; label is "All" not "All Time" |
| Single portfolio view | ✅ Implemented | Portfolio filter via `portfolio_id` param |
| All portfolios view (matching currencies) | ✅ Implemented | Currency consistency check with clear error |
| All portfolios view (mismatched currencies) | ✅ Implemented | Returns `mismatched_currencies` error |
| Empty state (no transactions) | ✅ Implemented | Empty curve + "Insufficient data" flag |
| Only deposits (no investments) | ✅ Implemented | Portfolio value = net deposit, 0% return |
| Negative net deposit | ✅ Implemented | Total return shows N/A, curve renders correctly |
| Manual data refresh | ✅ Implemented | Current prices + FX rates, partial failure handling |
| Missing market data warning | ✅ Implemented | Position excluded, warning added to result |
| FX rate gap fallback | ✅ Implemented | Falls back to original value when no rate found |

**Scenarios the spec missed (none discovered during implementation).**

## Plan vs Reality

| Plan Aspect | Assessment | Notes |
|---|---|---|
| Task breakdown | ✅ Effective | 8 tasks were well-scoped; each was independently testable |
| Task sizing | ⚠️ Task 3 was larger than expected | Equity curve computation required 8+ helper functions and ~300 lines. Could have been split into "walk transactions" and "build curve points" as separate tasks |
| Dependencies | ✅ Accurate | The dependency graph (Task 1 → Tasks 2,3 → Tasks 4,5 → Tasks 6-8) was followed correctly |
| Technical decisions | ✅ All documented | 7 decisions recorded in PLAN.md; all were sound and referenced existing patterns |
| Deviations | ✅ All tracked | 3 deviations documented in NOTES.md (historical fetcher approach, handler deps, URL building) |
| Test strategy | ✅ Exceeded expectations | More comprehensive than planned; handler tests include integration-style scenarios |

## Learnings

- **Always check library APIs for metadata before committing to a batch approach**: The `multi.Download` vs `multi.NewTickers` + `History()` deviation was caught during Task 2 implementation. Adding a quick API spike before planning would have avoided this.
- **Go `html/template` URL expression limitation is a recurring gotcha**: Pre-building URLs in the handler (instead of inline template expressions) is the right pattern. This lesson from NOTES.md should be added to CONVENTIONS.md.
- **Shadowing bugs in variable declarations are easy to miss**: The `pricesBySymbol` shadowing bug in `ComputeEquityCurve` was caught by tests but could have been caught earlier with `staticcheck` or similar linter.
- **Equity curve computation is naturally complex**: Walking transactions, tracking positions/cash/net deposit, fetching prices, FX conversion, and interpolation all in one flow. Consider extracting `walkTransactions` into its own package if this logic grows further.
- **`decimal.Decimal` pointer fields for optional values work well**: Using `*decimal.Decimal` for `TotalReturnPct` and `AnnualizedReturnPct` with `omitempty` cleanly handles the N/A cases in JSON serialization.

## Action Items

- [x] Update `features/README.md` to mark f010 as "done" — **done**
- [x] Add Go `html/template` URL expression guidance to `docs/CONVENTIONS.md` — **done** (pre-build URLs in handler, `{{index .Map "key"}}` for map access)
- [ ] Add `staticcheck` or `govet` to CI pipeline to catch variable shadowing bugs earlier
- [ ] For future analytics features: add a result caching layer (keyed by portfolio_id + period + last_updated) to avoid recomputing equity curves on every request
- [ ] Evaluate ECharts data downsampling for periods > 1 year to reduce JSON payload size
