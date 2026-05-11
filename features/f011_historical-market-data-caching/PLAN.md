# Implementation Plan: Historical Market Data Caching

## Overview

Add a background caching layer so that historical prices (stock and FX) are pre-fetched and stored in the `market_data` table asynchronously. User-facing pages (performance, positions) read from the cache instead of fetching live from Yahoo Finance, rendering instantly even when the provider is slow or unavailable. A `MarketCache` service orchestrates background fetches triggered by transaction saves, position recalculations, server startup, and a periodic ticker. An aggregate status indicator on existing pages shows data freshness.

## Task Dependencies

```
Task 1 (sqlc queries) → Task 2 (repo methods)
                              ↓
Task 3 (MarketCache service)
                              ↓
          ┌───────────────────┼───────────────────┐
          ↓                   ↓                   ↓
Task 4 (equity curve)   Task 5 (positions)    Task 6 (hooks + startup)
          ↓                   ↓                   ↓
          └───────────────────┼───────────────────┘
                              ↓
Task 5.5 (MarketDataService abstraction)
                              ↓
Task 7 (API endpoints) ←─────┘
                              ↓
Task 8 (web UI)
                              ↓
Task 9 (router wiring)
```

Tasks 4, 5, 6 are independent after Task 3. Task 5.5 consolidates the cache access pattern. Tasks 7 and 8 are sequential integration.

## Tasks

### Task 1: sqlc queries for cache reading [PRIORITY: HIGH]

**Corresponds to:** Scenarios: Performance page uses cached data, Positions page uses cached data, Status shows last update time

**Description:** Add SQL queries to read cached historical data and check cache status.

- [x] Add to `internal/data/queries/market_data.sql`:
  - `GetHistoricalPricesBySymbolAndRange` — historical prices for one symbol within [date_from, date_to], sorted by date ASC
  - `GetLatestQuote` — latest (date='') entry for a single symbol (batch version handled in repo layer, since sqlc doesn't support dynamic IN for SQLite)
  - `GetLatestPriceDatePerSymbol` — MAX(date) per symbol for stock data (to detect staleness)
  - `GetDistinctCachedSymbols` — distinct symbols with cached data (stock or fx)
- [x] Run `sqlc generate` to regenerate Go code
- [x] Verify generated code compiles

**Verification:** `sqlc generate` succeeds; new methods appear in `market_data.sql.go`; `go build ./...` passes.

---

### Task 2: Repository methods for reading cached data [PRIORITY: HIGH]

**Corresponds to:** Scenarios: Performance page uses cached data, Positions page uses cached data

**Description:** Add methods to `MarketDataRepository` that expose the new sqlc queries, and extend the `MarketDataRepository` interface in the position package.

- [x] Add to `internal/data/market_data_repo.go`:
  - `GetHistoricalPricesBySymbol(ctx, symbol, start, end) ([]market.HistoricalPrice, error)` — reads cached historical prices for one symbol in a date range
  - `GetLatestQuotesBatch(ctx, symbols []string) map[string]*market.MarketData` — reads latest quotes for multiple symbols; missing symbols omitted from result
  - `GetLatestPriceDatePerSymbol(ctx, symbols []string) map[string]*time.Time` — latest cached date per symbol; missing symbols omitted
- [x] Extend `MarketDataRepository` interface in `internal/domain/position/fx_converter.go` with the new methods
- [x] Write unit tests for each new repo method using the existing `setupMarketDataDB` pattern

**Verification:** Repo methods return correct data from in-memory SQLite; missing symbols handled gracefully.

---

### Task 3: MarketCache background service [PRIORITY: HIGH]

**Corresponds to:** Scenarios: Background fetch triggered by new symbol, Periodic refresh, Manual refresh, Initial cache population, Partial/full fetch failure, Non-trading days not filled, Concurrent refresh attempts, Status shows refresh in progress

**Description:** New package `internal/domain/marketcache` with a service that orchestrates background fetching of historical prices and FX rates.

- [x] Create `internal/domain/marketcache/marketcache.go`:
  - `MarketCache` struct with: fetcher (`market.MarketDataFetcher`), repo (`MarketDataRepository`), discoverer (`SymbolDiscoverer`), logger, in-memory status (mutex-protected)
  - `SymbolDiscoverer` interface (implemented by position package):
    - `ActiveSymbols(ctx) (map[string]time.Time, error)` — symbols with **open** positions → need current quotes + historical (value: earliest transaction date)
    - `AllSymbols(ctx) (map[string]time.Time, error)` — all symbols with **any** transactions → need historical only (value: earliest transaction date)
    - `ActiveFxPairs(ctx) (map[string]time.Time, error)` — FX pairs needed for open positions → need current rates + historical
  - Status fields: `inProgress map[string]bool` (per-symbol), `lastRefresh time.Time`, `failedSymbols map[string]string`, `refreshAllInProgress bool`
  - `Start(ctx context.Context)` — launches background worker goroutine + 2-minute periodic ticker for current quotes
  - `Stop()` — cancels context, stops goroutines
  - `ScheduleSymbolFetch(symbol string, fromDate time.Time)` — sends fetch request via channel; ignores if already in progress for that symbol
  - `ScheduleFxPairFetch(baseCurrency, quoteCurrency string, fromDate time.Time)` — converts to Yahoo symbol, schedules fetch, stores with `FormatFxPair` symbol
  - `RefreshAll(ctx context.Context)` — triggers full refresh of all symbols; sets `refreshAllInProgress` flag
  - `GetStatus() CacheStatus` — returns aggregate status: last refresh time, refresh in progress, failed symbols count
  - `CacheStatus` struct: `LastRefresh time.Time`, `Refreshing bool`, `FailedSymbols []string`, `TotalSymbols int`
- [x] Background worker:
  - Channel-based: receives `fetchRequest{symbol, fromDate, isFx, fxPair}` structs
  - For each request: check in-progress set → skip if running → mark in-progress → fetch → store → clear in-progress
  - Fetch logic: call `FetchHistoricalPricesBatch(ctx, []string{symbol}, fromDate, now)` → `UpsertHistoricalPrices`
  - FX fetch: convert pair to Yahoo symbol via `FxPairToYahooSymbol`, fetch, store with `FormatFxPair` as symbol and `data_type="fx"`
  - Partial failure: successful symbols cached, failed symbols recorded in status
  - Full failure (provider unreachable): existing cache preserved, failure recorded
  - Non-trading days: Yahoo returns no bars for weekends/holidays → nothing stored (natural behavior)
- [x] Periodic refresh (ticker, every 2 minutes):
  - **Discover**: call `SymbolDiscoverer` to get active symbols, all symbols, and FX pairs with earliest dates
  - **Current quotes**: refresh `date=''` for active symbols (open positions only) and active FX pairs via `FetchQuotesBatch` → `Upsert`
  - **Historical gap-fill**: for all symbols (open + closed), compare earliest transaction date against cached date range → only fetch missing dates:
    - No cache at all → fetch from earliest transaction date to now
    - Cache exists but latest cached date is before most recent trading day → fetch from (latest cached + 1 day) to now
    - Fully covered → skip
  - Skips historical gap-fill if a full manual refresh is already in progress (current quotes still refresh)
- [x] Create `internal/domain/marketcache/marketcache_test.go`:
  - Table-driven tests with hand-written mocks for fetcher and repo
  - Test: ScheduleSymbolFetch triggers a fetch
  - Test: Concurrent fetch protection (second request for same symbol ignored)
  - Test: Partial failure (some symbols fail, others cached)
  - Test: Full failure (all symbols fail, existing data preserved)
  - Test: FX pair fetch
  - Test: Status tracking (in progress, completed, failed)
  - Test: Periodic ticker refreshes current quotes every 2 minutes
  - Test: Periodic ticker detects and fills historical gaps for new symbols
  - Test: Periodic ticker skips historical gap-fill when manual refresh is in progress
  - Test: Empty portfolio (no symbols to fetch)
  - Test: Start/Stop lifecycle

**Verification:** MarketCache runs background fetches without blocking, prevents concurrent fetches for same symbol, tracks status correctly.

**Technical Decision A — Background fetcher approach:**

| Option | Pros | Cons |
|--------|------|------|
| **A: Channel-based worker** (chosen) | Simple, no external deps, natural Go pattern, easy to test | Single worker (sequential); fine for typical portfolio sizes |
| B: Worker pool (N goroutines) | Parallel fetches | More complex; diminishing returns for <50 symbols |
| C: Cron library | Structured scheduling | External dependency; overkill for a 2-minute ticker |

**Technical Decision B — Where to put MarketCache:**

| Option | Pros | Cons |
|--------|------|------|
| **A: New `marketcache` package** (chosen) | Clean separation; dedicated concern | New package to wire |
| B: Add to `position.Service` | No new package | Service already large; mixes concerns |
| C: Add to `market` package | Near fetcher | `market` is infrastructure; caching is domain logic |

---

### Task 4: Modify ComputeEquityCurve to use cached data [PRIORITY: HIGH]

**Corresponds to:** Scenario: Performance page uses cached data, Scenario: Pages show staleness warning

**Description:** Change `ComputeEquityCurve` in `position.Service` to read historical prices from the DB cache instead of fetching live from Yahoo Finance.

- [x] In `internal/domain/position/equity_curve.go`, modify the price-fetching step:
  - Replace `s.marketFetcher.FetchHistoricalPricesBatch(ctx, symbols, dateFrom, dateTo)` with a loop calling `s.marketDataRepo.GetHistoricalPricesBySymbol(ctx, sym, dateFrom, dateTo)` for each symbol
  - Build the same `pricesBySymbol` map from cached data
- [x] Add staleness detection:
  - After reading cache, check which symbols have no data in the requested range
  - For symbols with data, check if the latest cached date is >1 trading day old using `GetLatestPriceDatePerSymbol`
  - Add warnings: `"stale market data for SYMBOL (last updated X days ago)"` and `"missing market data for SYMBOL"`
- [x] Remove the live fetch and upsert logic (no longer needed — MarketCache handles that)
- [x] Update unit tests in `equity_curve_test.go`:
  - Mock `GetHistoricalPricesBySymbol` to return cached data
  - Mock `GetLatestPriceDatePerSymbol` for staleness checks
  - Verify staleness warnings appear correctly
  - Verify missing data warnings
  - Verify page renders with partial cache (some symbols cached, some not)

**Verification:** Equity curve computes from cached data; no external API calls during computation; staleness warnings generated correctly.

---

### Task 5: Modify EnrichWithMarketData to use cached data [PRIORITY: HIGH]

**Corresponds to:** Scenario: Positions page uses cached data

**Description:** Change `EnrichWithMarketData` in `position.Service` to read current quotes from the DB cache instead of fetching live.

- [x] In `internal/domain/position/service.go`, modify `EnrichWithMarketData`:
  - Replace `s.marketFetcher.FetchQuotesBatch(ctx, symbols)` with `s.marketDataRepo.GetLatestQuotesBatch(ctx, symbols)`
  - Remove the live-upsert-after-fetch logic (MarketCache handles caching)
  - Keep all enrichment logic (market value, unrealized P&L, FX conversion) unchanged
- [x] Update unit tests in `service_test.go`:
  - Mock `GetLatestQuotesBatch` to return cached quotes
  - Verify enrichment works with cached data
  - Verify missing quotes handled gracefully (MarketDataAvailable=false)

**Verification:** Positions page enriches from cached quotes; no external API calls during rendering.

---

### Task 5.5: MarketDataService abstraction [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundational abstraction)

**Description:** Create a `MarketDataService` in `internal/domain/marketservice/` that centralizes market data retrieval (quotes, historical prices, refresh) so consumer services don't know whether data comes from cache or live fetch.

- [x] Create `internal/domain/marketservice/marketservice.go` with:
  - `Service` struct holding `MarketDataFetcher` and `MarketDataRepository`
  - `GetQuotes(ctx, symbols)` — reads latest quotes from cache
  - `GetHistoricalPrices(ctx, symbol, start, end)` — reads cached historical prices
  - `GetLatestPriceDatePerSymbol(ctx, symbols)` — reads latest cached dates per symbol
  - `RefreshQuotes(ctx, symbols)` — fetches live quotes and upserts to cache, returns `RefreshResult` (succeeded/failed symbols)
  - `RefreshResult` struct with `Refreshed []string` and `Failed []string`
- [x] Create `internal/domain/marketservice/marketservice_test.go` with comprehensive tests:
  - GetQuotes from cache, no repo, empty symbols
  - GetHistoricalPrices from cache, no data, no repo
  - GetLatestPriceDatePerSymbol, no repo
  - RefreshQuotes success, partial failure, no fetcher, empty symbols, upsert error
- [x] Update `position.Service` to depend on `MarketDataService` interface instead of `marketFetcher` + `marketDataRepo`:
  - Replace `marketFetcher` and `marketDataRepo` fields with `marketService`
  - Rename `WithMarketDataFetcher()` → `WithMarketDataService()`
  - Update `EnrichWithMarketData`, `ComputeEquityCurve`, `RefreshMarketData` to use `marketService`
- [x] Update `router.go` to instantiate `marketservice.New(fetcher, repo)` and wire it
- [x] Update all tests across `position/`, `api/handlers/` to use mock `MarketDataService`

**Verification:** `go build ./...` and `go test -short ./...` pass; all consumers use the abstraction.

---

### Task 5.6: Normalize FX rate access through MarketDataService [PRIORITY: HIGH]

**Corresponds to:** Scenarios: Positions page uses cached data, Performance page uses cached data

**Description:** Remove the `FxConverter`/`FxRateProvider` abstraction and merge FX rate access into `MarketDataService`. This eliminates the inline fallback chain (historical DB → live fetch → spot rate) and normalizes FX to the same cache-read-only pattern as stock data.

- [x] Add FX methods to `MarketDataService` interface:
  - `GetCurrentFxRate(ctx, base, quote string) (*FxRate, error)` — spot rate from cache
  - `GetHistoricalFxRate(ctx, base, quote string, date time.Time) (*FxRate, error)` — historical rate from cache
  - `RefreshFxRates(ctx, pairs []FxPair) FxRefreshResult` — live fetch + upsert
- [x] Add FX methods to `marketservice.Service` implementation
- [x] Extend `MarketDataFetcher` interface with `FetchFxRate`
- [x] Extend `MarketDataRepository` interface with `GetCurrentFxRate`, `GetBySourceAndDate`
- [x] Remove `FxConverter` from `position.Service` (delete `fx_converter.go`)
- [x] Update `position.Service.convertPnlToBase` to use `marketService.GetCurrentFxRate` / `GetHistoricalFxRate`
- [x] Update `position.Service.convertValuesToBase` to use `marketService.GetCurrentFxRate`
- [x] Update `position/equity_curve.go` to use `marketService.GetHistoricalFxRate`
- [x] Update `position/refresh.go` to use `marketService.RefreshFxRates`
- [x] Update `router.go` to remove FxConverter instantiation
- [x] Update all tests (service_test.go, equity_curve_test.go, refresh_test.go, performance_test.go, position_test.go)
- [x] Move `ConvertPnlToBase`, `ConventionFxRate`, `FxRateDisplay` to `service.go`
- [x] Remove obsolete `FxRateProvider` interface and `mockFxRateProvider` from tests

**Verification:** `go build ./...` and `go test ./internal/... ./tests/...` pass; no references to `FxConverter`/`FxRateProvider` remain.

---

### Task 6: Integration hooks — transaction save, position recalc [PRIORITY: MEDIUM]

**Corresponds to:** Scenarios: Background fetch triggered by new symbol (transaction save), Background fetch triggered by new symbol (position recalculation), Background fetch triggered by new FX pair

**Description:** Wire MarketCache into the transaction lifecycle so new symbols trigger background fetches automatically.

- [x] Add `SymbolDiscoverer` methods to `position.Service`:
  - `ActiveSymbols(ctx) (map[string]time.Time, error)` — symbols with open positions, keyed by symbol, value is earliest transaction date
  - `AllSymbols(ctx) (map[string]time.Time, error)` — all symbols with any transactions, same format
  - `ActiveFxPairs(ctx) (map[string]time.Time, error)` — FX pairs needed for open positions, keyed by "BASE/QUOTE", value is earliest transaction date
  - These use new sqlc queries (GROUP BY symbol, MIN(date)) filtered by open/closed position state
- [x] Add `ScheduleSymbolFetch`/`ScheduleFxPairFetch` methods to `position.Service` (delegates to MarketCache if configured)
- [x] Add `WithMarketCache(marketcache.MarketCacheScheduler)` setter on `position.Service`
- [x] Define `MarketCacheScheduler` interface: `ScheduleSymbolFetch(symbol string, fromDate time.Time)`, `ScheduleFxPairFetch(base, quote string, fromDate time.Time)`, `RefreshAll(ctx context.Context)`
- [x] In `transaction.Service.Create` and `transaction.Service.Update` and `transaction.Service.Delete`:
  - After position recalculation, schedule cache fetch for the symbol (skipping $CASH symbols)
  - Uses `EarliestDateFinder` to find the earliest transaction date for the full range
  - For non-portfolio-currency transactions, schedules FX pair fetch too
  - Uses separate optional interfaces: `MarketDataScheduler`, `EarliestDateFinder`, `AccountPortfolioFinder`, `PortfolioCurrencyResolver`
- [x] In `position.Service.RecalculateAccount` (and RecalculatePortfolio/RecalculateAll):
  - After recalculation, check open positions for symbols/FX pairs without cached data
  - Schedule background fetches for any missing symbols
- [x] In `cmd/server/main.go`:
  - `MarketCache` created inside `router.go` (has all dependencies) and returned for lifecycle management
  - `marketCache.Start(ctx)` called before HTTP server starts
  - `marketCache.Stop()` called during graceful shutdown
- [x] Wire MarketCache through router.go to position service and transaction service

**Verification:** After saving a transaction for a new symbol, a background fetch is scheduled.

**Technical Decision C — How to hook into transaction save:**

| Option | Pros | Cons |
|--------|------|------|
| **A: Add hook to transaction.Service** (chosen) | Transaction service already triggers position recalc; natural place to add cache trigger | Transaction service gains another dependency |
| B: Hook in HTTP handler | No domain layer changes | Duplicates logic across API + web handlers; misses programmatic callers |
| C: Event bus | Decoupled | Over-engineered for two subscribers |

**Technical Decision D — How to find earliest transaction date:**

| Option | Pros | Cons |
|--------|------|------|
| **A: Query transaction repo** (chosen) | Accurate; uses existing data | Extra DB query per trigger |
| B: Pass date from caller | No extra query | Caller must know; error-prone |
| C: Always fetch from beginning of time | Simple | Wastes API calls on already-cached history |

---

### Task 7: Manual refresh + status API endpoints [PRIORITY: MEDIUM]

**Corresponds to:** Scenario: Manual refresh of all symbols, Scenario: Status shows last update time, Scenario: Status shows refresh in progress

**Description:** API endpoints for triggering a full historical data refresh and checking cache status.

- [x] Create `internal/api/handlers/market_data.go`:
  - `MarketDataHandler` struct with `*marketcache.MarketCache` dependency
  - `POST /api/market-data/refresh` — triggers `marketCache.RefreshAll(ctx)`, returns 202 Accepted (background operation)
  - `GET /api/market-data/status` — returns `CacheStatus` as JSON: `{last_refresh, refreshing, failed_symbols: [], total_symbols: N}`
- [x] Register routes in handler
- [x] Write unit tests:
  - POST /api/market-data/refresh returns 202
  - GET /api/market-data/status returns correct JSON
  - Status reflects in-progress refresh

**Verification:** API endpoints return correct responses; refresh triggers background operation without blocking.

---

### Task 8: Web UI — staleness warnings + aggregate status indicator [PRIORITY: MEDIUM]

**Corresponds to:** Scenarios: Pages show staleness warning, Status shows last update time, Status shows refresh in progress

**Description:** Add aggregate status indicator to performance and positions pages. Show staleness warnings. Update refresh button behavior.

- [x] Update `performancePageData` struct in `performance_web.go`:
  - Add `CacheStatus` field (last refresh time, refreshing flag, failed count)
  - Add `StaleSymbols` field ([]string) for the staleness warning banner
- [x] Update `HandlePerformance` in `performance_web.go`:
  - After `ComputeEquityCurve`, check cache status via `marketCache.GetStatus()`
  - Pass status and stale symbols to template
- [x] Update `templates/performance/index.html`:
  - Replace the existing Refresh button form to POST to `/performance/refresh` (triggers full refresh via marketCache.RefreshAll)
  - Add aggregate status text near the filter bar:
    - Data current: *"Just now"* (green/subtle)
    - Stale: *"3 symbol(s) stale · Updated 2d ago"* (amber)
    - Refreshing: *"Refreshing..."* (blue/italic)
  - Keep existing warning banner for missing/stale data details
- [x] Update `openPositionListPageData` in `position_web.go`:
  - Add `CacheStatus`, `HasCacheStatus`, `LastRefreshText` fields
- [x] Update `HandleOpenPositions` in `position_web.go`:
  - Check cache status, pass to template
- [x] Update `templates/position/open.html`:
  - Add aggregate status indicator near page header (same style as performance page)
- [x] Write unit tests for handlers verifying status is passed to templates

**Verification:** Performance and positions pages show aggregate cache status; refresh button triggers full historical refresh; staleness warnings appear when data is old.

---

### Task 4.1: Post-completion bug fixes [PRIORITY: HIGH]

**Corresponds to:** Production bugs discovered after initial implementation

**Description:** Fix critical bugs found during production use of the equity curve.

- [x] **Fix GBp (pence) → GBP conversion**: Yahoo returns some UK stocks in pence with `currency: "GBp"`. Detect at the fetcher layer (`FetchQuotesBatch` and `FetchHistoricalPricesBatch` in `internal/market/quote.go`), divide price by 100, store as `"GBP"`. Single source of truth — position service no longer handles currency quirks.
- [x] **Fix RefreshAll context bug**: Background refresh was cancelled when the HTTP request completed because `r.Context()` was passed. Changed to use internal lifecycle context (`m.ctx`) managed by `Start`/`Stop`.
- [x] **Fix weekend staleness warnings**: Added `tradingDayBeforeOrOn()` helper to skip weekend staleness checks. Added `nextTradingDay()` helper to skip weekends when scheduling gap fetches. Truncated dates to midnight UTC before comparison.
- [x] **Fix RealizedPnL for open positions**: Open positions always get `RealizedPnL = 0` (including partial sells). Sell proceeds are already reflected in cash, and remaining shares are valued via market price. Showing partial realized P&L double-counted against the equity curve.
- [x] **Fix walkTransactions double-negated sell quantities**: Sell transactions store **negative** quantities in the DB, but `walkTransactions` did `Neg()` on them, turning `-228` into `+228` and **adding** to the position instead of subtracting. Fixed by removing the `Neg()` — just add `txn.Quantity` directly.
- [x] **Fix equity curve stopping at last transaction date**: `interpolateDaily` only filled gaps between transaction dates with flat carry-forward. Now extends through `dateTo` using cached historical prices for current positions + cash, so the curve reflects actual price changes through the end date.
- [x] **Refactor: extract shared WalkPortfolioState**: Position quantity tracking was duplicated in `walkTransactions` (equity curve) and the calculator. Extracted `WalkPortfolioState` as the single source of truth for portfolio state (quantities, cash, net deposit) tracking. `walkTransactions` delegates to it and only adds position currency tracking on top.
- [x] Add `TestWalkTransactions_NegativeSellQuantity` to enforce the signed-quantity convention
- [x] Add comprehensive tests for `WalkPortfolioState` / `WalkPositionQuantities`
- [x] **Fix FX forward-fill for equity curve**: FX rates were fetched via `GetHistoricalPrices` but the SQL query filtered `data_type = 'stock'` only, excluding FX data (`data_type = 'fx'`). The FX forward-fill lookup was always empty, causing non-base-currency values to pass through unconverted — inflating GBP portfolios with USD assets by ~30%. Fixed SQL to `data_type IN ('stock', 'fx')`. `convertWithFxLookup` now returns `(value, bool)` so callers log WARN when FX conversion fails instead of silently using unconverted values.
- [x] Add `TestComputeEquityCurve_FXForwardFillWeekend` — verifies FX rates forward-fill on Sat/Sun
- [x] Add `TestComputeEquityCurve_MissingFXRateWarns` — verifies unconverted value when FX data is absent

**Verification:** `go build ./...` and `go test ./...` pass; equity curve shows correct values after sells; curve extends to today with real prices.

---

### Task 9: Router wiring [PRIORITY: LOW]

**Corresponds to:** All scenarios (integration)

**Description:** Wire MarketCache and new handlers into the router.

- [x] Update `internal/api/router.go`:
  - `MarketCache` created inside router with fetcher, repo, position service (as SymbolDiscoverer), logger
  - Returned alongside http.Handler for lifecycle management in main.go
  - Pass MarketCache to position service via `WithMarketCache`
  - Wire transaction service with cache scheduler, earliest date finder, account portfolio finder, portfolio currency resolver
- [x] Update `cmd/server/main.go`:
  - Receives MarketCache from router, calls `Start(ctx)` before HTTP server
  - Calls `marketCache.Stop()` during graceful shutdown
- [x] Run `go build ./...` — verify no compilation errors
- [x] Run `go test ./...` — verify all tests pass
- [x] Create and register `MarketDataHandler` (for manual refresh + status API) — done in Task 7
- [x] Pass MarketCache to performance web handler and position web handler

**Verification:** Application builds and runs; MarketCache starts on boot, stops on shutdown; all handlers wired correctly.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Background fetcher | Channel-based single worker | Simple Go pattern; sufficient for typical portfolio sizes; no external deps |
| Package location | New `internal/domain/marketcache` | Clean separation of concerns; dedicated caching orchestration |
| Status UI | Aggregate text indicator (no per-symbol) | Minimal UI; satisfies spec; inline with existing pages |
| Cache reading | New repo methods + sqlc queries | Follows existing sqlc pattern; type-safe; testable |
| Symbol discovery | `SymbolDiscoverer` interface on position.Service | MarketCache stays autonomous; open vs closed positions distinguished (open = quotes + historical, closed = historical only) |
| Transaction hook | Add to transaction.Service | Natural extension of existing position recalc hook; single place |
| Periodic refresh | 2-minute ticker for current quotes + historical gap-fill | Keeps P&L fresh; catches new symbols added while server is running |
| Concurrent protection | In-memory set (mutex) | Simple; prevents duplicate fetches for same symbol |
| FX historical data | Same market_data table, data_type="fx" | Reuses existing schema; no migration needed |

## Risks

- **Yahoo Finance rate limits during bulk initial fetch**: If the portfolio has many symbols, the first ticker pass could hit rate limits. Mitigation: sequential fetches via channel worker; ticker only touches current quotes (lightweight), historical fetches are one-time per symbol.
- **Cache empty on first page load**: Before the first ticker pass completes, pages may show empty or incomplete data. Mitigation: `Start()` does immediate first pass; user sees "Refreshing..." status until cache populates.
- **Stale data during trading hours**: 2-minute ticker keeps current quotes reasonably fresh. Historical data is immutable once the day closes.
- **Context cancellation during long fetch**: If server shuts down during a fetch, in-progress work is lost. Mitigation: acceptable — next startup will re-discover and re-fetch.
