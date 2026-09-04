# Notes: Portfolio Performance

## Decisions
- 2026-05-09: Placed domain types in `internal/domain/position/performance.go` alongside existing position types, following the plan's Technical Decision B (add to `position.Service` package).
- 2026-05-09: Task 3 implementation split into `equity_curve.go` (logic) and `equity_curve_test.go` (tests), co-located in `internal/domain/position/`.

## Post-Retro Enhancements
- **2026-05-11: Money-Weighted Return (MWR)** added alongside TWR. MWR (Internal Rate of Return) complements TWR by accounting for the timing and magnitude of cash flows. While TWR isolates pure investment performance, MWR shows the actual return experienced by the investor. Implemented via bisection-based IRR solver on cash flows derived from the equity curve's NetDeposit column. Added `MWRPct` (annualized) and `HoldingPeriodMWRPct` (period return) to `ReturnMetrics`, plus two new summary cards on the performance page.
- **2026-05-14: Simple Return formula changed** from raw value-based return (`end_pv / begin_pv - 1`) to profit-over-deposits (`(end_pv - end_nd) / end_nd × 100`). The old formula produced misleadingly large numbers for portfolios with significant deposits (e.g. 5404% when 98% of growth was from deposits, not investment performance). The new formula answers "for every unit of currency deposited, how much profit was made?" and is always defined as long as net deposit > 0. Added `SimpleReturnPct` and `AnnualizedSimpleReturnPct` to `ReturnMetrics`. Removed unused `ComputeSimpleReturn` function (P&L-delta version that was never called).

## Deviations from Plan
- **Task 4: Time-Weighted Return instead of simple total return/CAGR**. The plan specified `TotalReturnPct = (current_value - net_deposit) / net_deposit` and `AnnualizedReturnPct` (CAGR). During implementation, TWR was chosen instead because it isolates investment performance from the timing and magnitude of deposits/withdrawals. TWR geometrically links sub-period returns between cash flow breakpoints. Fields renamed: `TotalReturnPct` → `TWRPct`, `AnnualizedReturnPct` → `AnnualizedTWRPct`. When there are no cash flows, TWR degenerates to the simple return.
- Task 2: Used `multi.NewTickers` + per-ticker `History()` instead of `multi.Download` because `multi.Download` returns `[]models.Bar` without currency info. The `History()` call on each ticker caches `ChartMeta` (including currency) accessible via `GetHistoryMetadata()`. Same shared HTTP client is used via `multi.NewTickers`.
- Task 6: `PerformanceHandler` only needs `*position.Service` (not `*portfolio.Service` as planned) because `ComputeEquityCurve` already handles portfolio resolution internally via the account lister.
- Task 7: Pre-built URLs in handler (`RefreshURL`, `PeriodURLs` map) instead of inline template expressions in href/action attributes. Go's `html/template` rejects template expressions inside URLs as "ambiguous context" — it can't distinguish `&` as HTML entity vs query param separator.

## Lessons Learned
- Go `html/template` requires `{{index .Map "key"}}` for map access with string keys containing digits/special chars — dot notation `{{.Map."1W"}}` causes a parse error (`bad character U+0022 '"'`).

## Bugs Found
- **Flat equity curve between transactions (fixed 2026-07-15)**: `InterpolateDaily` carried the last *transaction date's* portfolio value forward flat to the next transaction, instead of revaluing the point-in-time portfolio at each day's price. Result: the curve only moved on trade dates (visible on the seeded portfolio as a flat line from 2024-07-01 to 2025-07-15). The spec (f010) requires daily market value with carry-forward only for non-trading days, so this was a code bug, not a spec issue. Fix: `InterpolateDaily` now takes the per-snapshot portfolio states (`[]dateSnapshot`) and revalues the last snapshot on/before each date at that date's forward-filled prices and FX rates; non-trading days fall out of the same mechanism (no price that day → forward-fill carries the last trading day's value). The old behavior also wrongly carried forward *net deposits* at the old FX rate; net deposit is now reconverted at each date's FX rate. Pinned by `TestInterpolateDaily_MarkToMarketBetweenTransactions` and `TestComputeEquityCurve_MarkToMarketBetweenTransactions` (both failed red before the fix).
- **Go shadowing bug in `ComputeEquityCurve`**: `pricesBySymbol, failedSymbols := fetcher.FetchHistoricalPricesBatch(...)` created a new local `pricesBySymbol` that shadowed the outer variable. The outer `pricesBySymbol` (initialized as empty map) was passed to `buildEquityCurvePoints`, resulting in zero position values. Fixed by declaring `var pricesBySymbol map[string][]market.HistoricalPrice` before the if-block and using `=` assignment inside.

## Future Improvements
- Add a result caching layer for equity curve data (keyed by portfolio_id + period + last_updated) to avoid recomputing on every request
- Evaluate ECharts data downsampling for periods > 1 year (~1,800 daily points) to reduce JSON payload size
- Consider adding `staticcheck` or `govet` to CI to catch variable shadowing bugs earlier

## Known Issues
- None.
