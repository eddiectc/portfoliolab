# Notes: Historical Market Data Caching

## Decisions
- 2026-05-10: `GetLatestQuotesBatch` renamed to `GetLatestQuote` (single-symbol). sqlc doesn't support dynamic `IN` clauses with variable-length slices for SQLite. The batch version will be implemented in the repo layer by looping `GetLatestQuote` per symbol.

## Deviations from Plan
- Task 1: `GetLatestQuotesBatch` → `GetLatestQuote` (single symbol). Batch logic moved to repo layer (Task 2).

## Implementation Gotchas
- Historical dates are stored as `YYYY-MM-DD` (not RFC3339), so `parseTime()` doesn't work for them — use `time.Parse("2006-01-02", s)` instead
- SQLite `MAX(date)` returns `interface{}` that can be `string` or `[]byte` depending on driver — handle both cases
- Extending `MarketDataRepository` interface requires updating mocks in 5 test files (equity_curve, fx_converter, refresh, service, performance)

## Future Improvements

## Known Issues
