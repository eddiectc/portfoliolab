# Notes: Portfolio Performance

## Decisions
- 2026-05-09: Placed domain types in `internal/domain/position/performance.go` alongside existing position types, following the plan's Technical Decision B (add to `position.Service` package).

## Deviations from Plan
- Task 2: Used `multi.NewTickers` + per-ticker `History()` instead of `multi.Download` because `multi.Download` returns `[]models.Bar` without currency info. The `History()` call on each ticker caches `ChartMeta` (including currency) accessible via `GetHistoryMetadata()`. Same shared HTTP client is used via `multi.NewTickers`.

## Future Improvements
- N/A

## Known Issues
- None.
