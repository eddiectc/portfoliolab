# Notes: Portfolio Performance

## Decisions
- 2026-05-09: Placed domain types in `internal/domain/position/performance.go` alongside existing position types, following the plan's Technical Decision B (add to `position.Service` package).
- 2026-05-09: Task 3 implementation split into `equity_curve.go` (logic) and `equity_curve_test.go` (tests), co-located in `internal/domain/position/`.

## Deviations from Plan
- **Task 4: Time-Weighted Return instead of simple total return/CAGR**. The plan specified `TotalReturnPct = (current_value - net_deposit) / net_deposit` and `AnnualizedReturnPct` (CAGR). During implementation, TWR was chosen instead because it isolates investment performance from the timing and magnitude of deposits/withdrawals. TWR geometrically links sub-period returns between cash flow breakpoints. Fields renamed: `TotalReturnPct` → `TWRPct`, `AnnualizedReturnPct` → `AnnualizedTWRPct`. When there are no cash flows, TWR degenerates to the simple return.
- Task 2: Used `multi.NewTickers` + per-ticker `History()` instead of `multi.Download` because `multi.Download` returns `[]models.Bar` without currency info. The `History()` call on each ticker caches `ChartMeta` (including currency) accessible via `GetHistoryMetadata()`. Same shared HTTP client is used via `multi.NewTickers`.
- Task 6: `PerformanceHandler` only needs `*position.Service` (not `*portfolio.Service` as planned) because `ComputeEquityCurve` already handles portfolio resolution internally via the account lister.
- Task 7: Pre-built URLs in handler (`RefreshURL`, `PeriodURLs` map) instead of inline template expressions in href/action attributes. Go's `html/template` rejects template expressions inside URLs as "ambiguous context" — it can't distinguish `&` as HTML entity vs query param separator.

## Lessons Learned
- Go `html/template` requires `{{index .Map "key"}}` for map access with string keys containing digits/special chars — dot notation `{{.Map."1W"}}` causes a parse error (`bad character U+0022 '"'`).

## Bugs Found
- **Go shadowing bug in `ComputeEquityCurve`**: `pricesBySymbol, failedSymbols := fetcher.FetchHistoricalPricesBatch(...)` created a new local `pricesBySymbol` that shadowed the outer variable. The outer `pricesBySymbol` (initialized as empty map) was passed to `buildEquityCurvePoints`, resulting in zero position values. Fixed by declaring `var pricesBySymbol map[string][]market.HistoricalPrice` before the if-block and using `=` assignment inside.

## Future Improvements
- Add a result caching layer for equity curve data (keyed by portfolio_id + period + last_updated) to avoid recomputing on every request
- Evaluate ECharts data downsampling for periods > 1 year (~1,800 daily points) to reduce JSON payload size
- Consider adding `staticcheck` or `govet` to CI to catch variable shadowing bugs earlier

## Known Issues
- None.
