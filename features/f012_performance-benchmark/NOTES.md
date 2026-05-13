# Notes: Performance Benchmark

## Decisions
- 2026-05-12: Added `MarketDataService` as a second dependency to `PerformanceHandler` (alongside `positionSvc`) to fetch benchmark historical prices. This follows the existing pattern where the handler layer accesses market data directly (e.g., `PositionWebHandler` has `marketCache`).
- 2026-05-12: Duplicated the `determineDateRange` logic from `position/equity_curve.go` into `performance.go` as an unexported helper. This avoids adding a dependency cycle and keeps the handler self-contained. The logic mirrors the service-layer version exactly.
- 2026-05-12: Added `MarketDataService` as a new dependency to `PerformanceWebHandler` (4th param, after `marketCache`). This mirrors the API handler pattern and lets the web handler fetch benchmark prices independently. The `router.go` wiring passes the same `marketSvc` instance.
- 2026-05-12: Invalid benchmark tickers in the web handler are silently dropped (set to empty) rather than returning a 400 error. The API handler returns 400 for invalid benchmarks, but the web handler is more forgiving — it just shows no benchmark. This avoids breaking the page on malformed URLs.

## Deviations from Plan
- None.

## Template Escaping Notes (Task 5)
- Go's `html/template` encodes `^` differently in `value` vs `data-*` attributes: `value` keeps `^` as-is, `data-url` URL-encodes it to `%5e`. Tests must match the actual encoding used.
- `&` in URLs is always HTML-escaped to `&amp;` in attribute values.
- The `value` attribute on `<option>` elements is treated specially by html/template — it may clear the attribute and fall back to text content. The browser handles this correctly (uses text as the option value), so the `onchange` handler works as expected.
- Benchmark names JS object is built inline via `{{range .BenchmarkNames}}` rather than JSON-encoding the map (no `json:` template function available).

## Post-Review Fixes
- 2026-05-12: Fixed benchmark selector option values — changed from full URLs to ticker symbols so the form submit path (POST with `benchmark=^GSPC`) works correctly alongside the `onchange` JavaScript navigation path. `onchange` now uses `this.dataset.url` instead of `this.value`.

## Future Improvements
- Consider extracting `determineDateRange` to a shared package if it grows in callers.

## Bugs Fixed Post-Merge

### Monthly heatmap repeated rows (2026-05-13)
- **Symptom:** Each year's row was duplicated N times (N = number of months with data that year). E.g., 2024 with 11 months showed 11 identical rows.
- **Root cause:** `computeMonthlyReturnsFromCurve` returned a flat `[]monthlyReturnData` (one entry per month), but the template iterated each entry as a table row and then did an O(n²) inner loop to look up all 12 months' data. So N months → N identical year rows.
- **Fix:** Restructured data to `[]yearReturnData` (one per year) with `Months map[int]monthCellData`. Template simplified to `index $row.Months $m` — no nested lookup loop.

### Monthly returns counted deposits as profit (2026-05-13)
- **Symptom:** A mid-month deposit inflated the monthly return. E.g., depositing 100k on a 100k portfolio mid-month, then 5% market gain, showed 110% return instead of 5%.
- **Root cause:** Monthly return used simple return `(last_PV / first_PV - 1)` which doesn't account for cash flows within the month. The overall TWR metric was correct (uses pre-cash-flow breakpoints), but the monthly heatmap wasn't.
- **Fix:** Replaced simple return with `computeMonthlyTWR` — detects cash flow dates from `NetDeposit` changes in the equity curve, computes pre-cash-flow value as `PV - delta_ND`, and geometrically links sub-period returns. Same TWR logic as the overall metric, scoped per-month.
- **Test added:** `mid-month deposit — TWR isolates cash flow` verifies the 100k→200k→210k scenario yields 5% (not 110%).

## Known Issues
- None.
