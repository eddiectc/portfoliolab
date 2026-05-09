# Notes: Portfolio Performance

## Decisions
- 2026-05-09: Placed domain types in `internal/domain/position/performance.go` alongside existing position types, following the plan's Technical Decision B (add to `position.Service` package).
- 2026-05-09: Task 3 implementation split into `equity_curve.go` (logic) and `equity_curve_test.go` (tests), co-located in `internal/domain/position/`.

## Deviations from Plan
- Task 2: Used `multi.NewTickers` + per-ticker `History()` instead of `multi.Download` because `multi.Download` returns `[]models.Bar` without currency info. The `History()` call on each ticker caches `ChartMeta` (including currency) accessible via `GetHistoryMetadata()`. Same shared HTTP client is used via `multi.NewTickers`.
- Task 6: `PerformanceHandler` only needs `*position.Service` (not `*portfolio.Service` as planned) because `ComputeEquityCurve` already handles portfolio resolution internally via the account lister.
- Task 7: Pre-built URLs in handler (`RefreshURL`, `PeriodURLs` map) instead of inline template expressions in href/action attributes. Go's `html/template` rejects template expressions inside URLs as "ambiguous context" — it can't distinguish `&` as HTML entity vs query param separator.

## Lessons Learned
- Go `html/template` requires `{{index .Map "key"}}` for map access with string keys containing digits/special chars — dot notation `{{.Map."1W"}}` causes a parse error (`bad character U+0022 '"'`).

## Bugs Found
- **Go shadowing bug in `ComputeEquityCurve`**: `pricesBySymbol, failedSymbols := fetcher.FetchHistoricalPricesBatch(...)` created a new local `pricesBySymbol` that shadowed the outer variable. The outer `pricesBySymbol` (initialized as empty map) was passed to `buildEquityCurvePoints`, resulting in zero position values. Fixed by declaring `var pricesBySymbol map[string][]market.HistoricalPrice` before the if-block and using `=` assignment inside.

## Future Improvements
- N/A

## Known Issues
- None.
