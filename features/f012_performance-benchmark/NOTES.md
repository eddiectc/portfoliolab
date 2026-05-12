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

## Future Improvements
- Consider extracting `determineDateRange` to a shared package if it grows in callers.

## Known Issues
- None.
