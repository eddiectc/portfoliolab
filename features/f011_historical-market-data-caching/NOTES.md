# Notes: Historical Market Data Caching

## Decisions
- 2026-05-10: `GetLatestQuotesBatch` renamed to `GetLatestQuote` (single-symbol). sqlc doesn't support dynamic `IN` clauses with variable-length slices for SQLite. The batch version will be implemented in the repo layer by looping `GetLatestQuote` per symbol.

## Deviations from Plan
- Task 1: `GetLatestQuotesBatch` → `GetLatestQuote` (single symbol). Batch logic moved to repo layer (Task 2).

## Implementation Gotchas
- Staleness check uses calendar days (`>1 day old`) not trading days. Simpler and more conservative (warns on weekends too), but never misses stale data. Would need a trading calendar to do trading-day-accurate staleness.
- Historical dates are stored as `YYYY-MM-DD` (not RFC3339), so `parseTime()` doesn't work for them — use `time.Parse("2006-01-02", s)` instead
- SQLite `MAX(date)` returns `interface{}` that can be `string` or `[]byte` depending on driver — handle both cases
- Extending `MarketDataRepository` interface requires updating mocks in 5 test files (equity_curve, fx_converter, refresh, service, performance)
- Concurrent protection for `ScheduleSymbolFetch` requires both an `inProgress` set (worker processing) AND a `queued` set (in channel but not yet picked up). Without `queued`, two rapid calls both pass the check before the worker sets `inProgress`.
- `UpsertHistoricalPrices` accepts a `dataType` parameter ("stock" or "fx") — added to support both stock and FX historical data through the same batch method instead of individual `Upsert` calls.

## Future Improvements

## Known Issues
