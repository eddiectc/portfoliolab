# Notes: Historical Market Data Caching

## Decisions
- 2026-05-10: `GetLatestQuotesBatch` renamed to `GetLatestQuote` (single-symbol). sqlc doesn't support dynamic `IN` clauses with variable-length slices for SQLite. The batch version will be implemented in the repo layer by looping `GetLatestQuote` per symbol.
- 2026-05-10: `MarketDataService` is a concrete struct (not an interface) in `marketservice` package. The `position` package defines its own `MarketDataService` interface for dependency inversion. This lets consumers mock the service without importing the concrete type.
- 2026-05-10: `RefreshQuotes` on `MarketDataService` takes a symbol list and returns `RefreshResult{Refreshed, Failed}`. The caller (e.g., `position.Service.RefreshMarketData`) is responsible for collecting symbols from transactions/positions. This keeps symbol-discovery logic in the position layer where it belongs.
- 2026-05-10: `MarketCacheScheduler` interface defined in `marketcache` package. Consumer services (position, transaction) depend on the interface, not the concrete `MarketCache` type. This avoids circular imports.
- 2026-05-10: `MarketCache` created inside `router.go` (not `main.go`) because it needs the `YahooFinanceFetcher`, `MarketDataRepository`, and `position.Service` (as SymbolDiscoverer) which are all constructed in the router. The router returns the cache alongside the `http.Handler` for lifecycle management in `main.go`.
- 2026-05-10: Transaction service uses separate optional interfaces (`MarketDataScheduler`, `EarliestDateFinder`, `AccountPortfolioFinder`, `PortfolioCurrencyResolver`) instead of a combined interface. This keeps dependencies minimal and testable.

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
- **Task 6 sqlc queries**: New queries in `transaction.sql` (`GetSymbolsWithEarliestDate`, `GetSymbolsByOpenPositions`, `GetFxPairsByOpenPositions`, `GetEarliestDateBySymbol`) use `MIN(date)` which returns `interface{}` from SQLite. The `parseInterfaceTime` helper in `transaction_repo.go` handles both `string` and `[]byte` representations.
- **Task 6 sqlc comments**: sqlc fails with "edited query syntax is invalid" if there are inline comments between the `-- name:` annotation and the SQL. Remove comments or put them on a separate line before the annotation.
- **Task 6 circular dependency**: `MarketCache` needs `position.Service` as SymbolDiscoverer, and `position.Service` needs `MarketCache` for scheduling. Resolved by creating `position.Service` first, then `MarketCache`, then wiring `MarketCache` into `position.Service` via `WithMarketCache`.
- **Task 6 interface explosion**: Adding 4 new methods to `TransactionRepository` in the position package required updating 5 mock implementations across test files (service_test, refresh_test, performance_test, position_test). Each mock got stub implementations returning nil.
- **Task 6 router signature change**: `Router()` now returns `(http.Handler, *marketcache.MarketCache)` instead of just `http.Handler`. All integration tests updated with `, _` to discard the cache reference.
- **Task 7 handler interface**: `MarketDataHandler` depends on a private `marketCacheStatus` interface (not the concrete `*marketcache.MarketCache`) so it can be mocked in tests. Follows the same pattern as other handlers that use interfaces for cross-package dependencies.
- **Task 7 cross-task**: Creating and registering `MarketDataHandler` also satisfied the corresponding Task 9 sub-task. Task 9 now only has one remaining item: passing MarketCache to the performance and position web handlers.
- **Task 8 cache status interface**: `cacheStatusProvider` interface defined in `performance_web.go` (not a separate file) since both web handlers are in the same `handlers` package. Same pattern as `marketCacheStatus` in `market_data.go`.
- **Task 8 refresh behavior change**: `HandleRefresh` now calls `marketCache.RefreshAll()` (full refresh of ALL symbols) instead of `positionSvc.RefreshMarketData()` (limited to visible period symbols). Button renamed "Refresh All" and `buildRefreshURL` simplified — no longer includes period query param since refresh is always global.
- **Task 8 stale symbol extraction**: `extractStaleSymbols()` parses warning strings ("stale market data for SYMBOL (...)" / "missing market data for SYMBOL") to build the `StaleSymbols` list for the aggregate indicator. Only used on the performance page — the positions page has no equivalent warnings from `EnrichWithMarketData`.
- **Task 8 positions page staleness**: Positions template uses `CacheStatus.FailedSymbols` as the staleness proxy (non-empty = stale) since there's no per-symbol staleness check in `EnrichWithMarketData`. This is less precise than the performance page but sufficient for the aggregate indicator.

## Post-Completion Fixes (2026-05-10)

### GBp (pence) → GBP conversion
- Yahoo returns some UK stocks (e.g. XNAQ.L, QGRP.L) in pence with `currency: "GBp"`
- Fix at fetcher layer (`FetchQuotesBatch` + `FetchHistoricalPricesBatch` in `internal/market/quote.go`): detect `GBp`, divide by 100, store as `GBP`
- Moved from position service workaround to fetcher layer — single source of truth

### RefreshAll context bug
- `RefreshAll` was passing `r.Context()` to background goroutine, cancelled on HTTP response
- Changed to use `m.ctx` (internal lifecycle context from `Start`/`Stop`)

### Weekend staleness + gap-fill
- Added `tradingDayBeforeOrOn()` to skip weekend staleness checks
- Added `nextTradingDay()` to skip weekends when scheduling gap fetches
- Truncated dates to midnight UTC before comparison to eliminate false positives

### RealizedPnL for open positions
- Open positions always get `RealizedPnL = 0` (including partial sells)
- Sell proceeds already in cash balance; remaining shares valued via market price
- Showing partial realized P&L double-counted against equity curve

### walkTransactions double-negated sell quantities (CRITICAL)
- Sell transactions store **negative** quantities in DB (matching calculator convention)
- `walkTransactions` did `txn.Quantity.Neg()`, turning `-228` into `+228`
- This **added** to the position instead of subtracting — positions doubled after sells
- Fix: remove `Neg()`, add `txn.Quantity` directly (sign already correct)

### Equity curve stopping at last transaction date
- `interpolateDaily` only filled between transaction dates with flat carry-forward
- Curve stopped at 2026-04-06 (last txn) instead of extending to today
- Fix: pass `dateTo`, positions, cash, and cached prices to `interpolateDaily`
- Extended dates compute real portfolio value from cached prices

### Refactor: WalkPortfolioState
- Position quantity tracking was duplicated in `walkTransactions` and the calculator
- The double-negation bug was exactly this divergence
- Extracted `WalkPortfolioState` (quantities + cash + net deposit) as single source of truth
- `walkTransactions` delegates to it, only adds position currency tracking
- Old `WalkPositionQuantities` kept as thin wrapper for backward compat

### FX forward-fill SQL filter bug (CRITICAL)
- `GetHistoricalPricesBySymbolAndRange` filtered `data_type = 'stock'` only
- FX rates stored with `data_type = 'fx'` were never returned
- FX forward-fill lookup was always empty → non-base-currency values passed through unconverted
- USD values added directly to GBP portfolio, inflating by ~30%
- Fix: SQL changed to `data_type IN ('stock', 'fx')`
- `convertWithFxLookup` now returns `(value, bool)` — callers log WARN on missing rates
- Added tests: `TestComputeEquityCurve_FXForwardFillWeekend`, `TestComputeEquityCurve_MissingFXRateWarns`

### Period filter slices output curve, not transactions
- Period filter was filtering transactions, causing missing positions from buys before the period
- Fix: always walk ALL transactions for correct portfolio state; period only slices final output curve
- Renamed `TotalReturnPct` to `PeriodReturnPct` — uses `(end-begin)/begin` instead of net deposit

## Future Improvements
- Recalculate hooks schedule fetches for ALL open position symbols regardless of whether cache already exists. The market cache's concurrent protection (queued + in-progress sets) deduplicates, but this means extra channel messages. Could optimize by checking cache first, but the trade-off is an extra DB query per recalc.

## Known Issues
