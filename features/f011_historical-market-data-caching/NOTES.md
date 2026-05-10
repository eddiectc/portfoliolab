# Notes: Historical Market Data Caching

## Decisions
- 2026-05-10: `GetLatestQuotesBatch` renamed to `GetLatestQuote` (single-symbol). sqlc doesn't support dynamic `IN` clauses with variable-length slices for SQLite. The batch version will be implemented in the repo layer by looping `GetLatestQuote` per symbol.
- 2026-05-10: `MarketDataService` is a concrete struct (not an interface) in `marketservice` package. The `position` package defines its own `MarketDataService` interface for dependency inversion. This lets consumers mock the service without importing the concrete type.
- 2026-05-10: `RefreshQuotes` on `MarketDataService` takes a symbol list and returns `RefreshResult{Refreshed, Failed}`. The caller (e.g., `position.Service.RefreshMarketData`) is responsible for collecting symbols from transactions/positions. This keeps symbol-discovery logic in the position layer where it belongs.

## Deviations from Plan
- Task 1: `GetLatestQuotesBatch` → `GetLatestQuote` (single symbol). Batch logic moved to repo layer (Task 2).

## Implementation Gotchas
- Staleness check uses calendar days (`>1 day old`) not trading days. Simpler and more conservative (warns on weekends too), but never misses stale data. Would need a trading calendar to do trading-day-accurate staleness.
- `MarketDataService` defines its own `MarketDataRepository` and `MarketDataFetcher` interfaces (minimal subsets) rather than importing the full `position.MarketDataRepository` — avoids circular dependencies and keeps the service self-contained. The `position` package defines its own `MarketDataService` interface referencing `marketservice.RefreshResult`, creating a one-way dependency: `position` → `marketservice`.
- **FX normalization (Task 5.6)**: Removed `FxConverter`/`FxRateProvider` and merged FX rate access into `MarketDataService`. FX rates now follow the same cache-read-only pattern as stock data: `GetCurrentFxRate`/`GetHistoricalFxRate` read from cache (returning nil if missing), and `RefreshFxRates` does live fetch + upsert. Callers handle nil explicitly, matching stock quote behavior. This eliminates the inline fallback chain (historical DB → live fetch → spot rate) and simplifies the dependency graph.
- `GetHistoricalFxRate` uses `repo.GetBySourceAndDate` with source hardcoded to `"yahoo"` — matches how the marketcache populates FX data. If another source is added later, this needs updating.
- `ConvertPnlToBase`, `ConventionFxRate`, `FxRateDisplay` were in the deleted `fx_converter.go` — moved to `service.go` since they're pure utility functions with no repo/fetcher deps.
- `market.FxRateFetcher` interface and `YahooFinanceFetcher.FetchRate` removed — `MarketDataFetcher.FetchFxRate` in marketservice subsumes its role. `FxError` also removed (never used in production).
- Refactoring `position.Service` from two deps (`marketFetcher` + `marketDataRepo`) to one (`marketService`) required updating mocks in 4 test files (service, equity_curve, refresh, performance) and removing the now-unused `mockMarketDataFetcher`/`mockMarketDataRepo` from service_test.go.
- Historical dates are stored as `YYYY-MM-DD` (not RFC3339), so `parseTime()` doesn't work for them — use `time.Parse("2006-01-02", s)` instead
- SQLite `MAX(date)` returns `interface{}` that can be `string` or `[]byte` depending on driver — handle both cases
- Extending `MarketDataRepository` interface requires updating mocks in 5 test files (equity_curve, fx_converter, refresh, service, performance)
- Concurrent protection for `ScheduleSymbolFetch` requires both an `inProgress` set (worker processing) AND a `queued` set (in channel but not yet picked up). Without `queued`, two rapid calls both pass the check before the worker sets `inProgress`.
- `UpsertHistoricalPrices` accepts a `dataType` parameter ("stock" or "fx") — added to support both stock and FX historical data through the same batch method instead of individual `Upsert` calls.

## Future Improvements

## Known Issues
