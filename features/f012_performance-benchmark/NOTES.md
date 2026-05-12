# Notes: Performance Benchmark

## Decisions
- 2026-05-12: Added `MarketDataService` as a second dependency to `PerformanceHandler` (alongside `positionSvc`) to fetch benchmark historical prices. This follows the existing pattern where the handler layer accesses market data directly (e.g., `PositionWebHandler` has `marketCache`).
- 2026-05-12: Duplicated the `determineDateRange` logic from `position/equity_curve.go` into `performance.go` as an unexported helper. This avoids adding a dependency cycle and keeps the handler self-contained. The logic mirrors the service-layer version exactly.

## Deviations from Plan
- None yet.

## Future Improvements
- Consider extracting `determineDateRange` to a shared package if it grows in callers.

## Known Issues
- None.
