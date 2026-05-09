# Feature: Historical Market Data Caching

## Description

The system automatically fetches and caches daily historical prices (stock and FX) in the background so that user-facing pages render instantly without blocking on external market data API calls. Users can see the caching status and trigger manual refreshes for all symbols or individual ones.

This is an enhancement of f010 (Portfolio Performance). The performance page currently fetches live data on each request — this feature adds a persistent historical price cache that is kept up-to-date asynchronously.

## User Stories

1. **As a user**, I want historical prices to be refreshed in the background, so that I can open the performance page without waiting for market data to load.

2. **As a user**, I want to see the status of historical price data (last updated, currently refreshing), so that I know whether my charts are showing current information.

3. **As a user**, I want to trigger a manual refresh of all historical data, so that I can update prices on demand after market close or after adding new transactions.

## Scenarios

### Scenario: Background fetch triggered by new symbol (transaction save)
**Given** a transaction is added for a symbol that has no historical prices cached
**When** the transaction is successfully saved
**Then** a background fetch is scheduled for that symbol's historical prices starting from the date of the earliest transaction for that symbol
**And** the fetch runs asynchronously without blocking the user's current action

### Scenario: Background fetch triggered by new symbol (position recalculation)
**Given** position recalculation discovers an open position for a symbol that has no historical prices cached
**When** the recalculation completes
**Then** a background fetch is scheduled for that symbol's historical prices starting from the date of the earliest transaction for that symbol
**And** the fetch runs asynchronously without blocking the user's current action

### Scenario: Background fetch triggered by new FX pair (transaction save)
**Given** a transaction is added for a currency pair that has no historical FX data cached
**When** the transaction is successfully saved
**Then** a background fetch is scheduled for that FX pair's historical prices starting from the date of the earliest transaction requiring that pair
**And** the fetch runs asynchronously without blocking the user's current action

### Scenario: Background fetch triggered by new FX pair (position recalculation)
**Given** position recalculation discovers an open position requiring an FX rate for a currency pair that has no historical FX data cached
**When** the recalculation completes
**Then** a background fetch is scheduled for that FX pair's historical prices starting from the date of the earliest transaction requiring that pair
**And** the fetch runs asynchronously without blocking the user's current action

### Scenario: Periodic refresh of existing symbols
**Given** historical prices are already cached for some symbols
**When** a new trading day has passed since the last cached price
**Then** the background process fetches and appends the missing price(s) for those symbols
**And** existing cached prices are never overwritten or deleted

### Scenario: Manual refresh of all symbols
**Given** the user triggers a refresh of all historical data
**When** the refresh runs
**Then** historical prices are fetched for all symbols with outstanding positions or transactions
**And** missing dates in the existing cache are filled in
**And** the operation runs in the background without blocking the user

> Note: This supersedes the "Manually refresh historical market data" scenario from f010, which was limited to symbols in the visible chart period. f011 extends the scope to all symbols with transactions.

### Scenario: Performance page uses cached data
**Given** historical prices are cached for the symbols in the user's portfolio
**When** the user opens the performance page
**Then** the page renders using cached historical prices from the database without waiting for external API calls
**And** the page loads quickly even if the market data provider is slow or unavailable

### Scenario: Positions page uses cached data
**Given** historical prices are cached for the symbols in the user's portfolio
**When** the user opens the positions page
**Then** the page renders using cached prices from the database without fetching market data directly
**And** the page loads quickly even if the market data provider is slow or unavailable

### Scenario: Pages show staleness warning
**Given** historical prices are cached but the most recent price for one or more symbols is more than one trading day old
**When** the user opens the performance page or the positions page
**Then** the page renders using the available cached data
**And** a warning indicates which symbols have stale data

### Scenario: Status shows last update time
**Given** historical data has been cached for some symbols
**When** the user views the data status
**Then** the status shows when each symbol's historical data was last updated

### Scenario: Status shows refresh in progress
**Given** a background or manual refresh is currently running
**When** the user views the data status
**Then** the status indicates that a refresh is in progress

### Scenario: Initial cache population
**Given** the system starts up and the portfolio has symbols with no cached historical data
**When** the system completes its startup sequence
**Then** a background fetch is scheduled for all uncached symbols
**And** the fetch runs asynchronously without delaying the availability of the web UI

### Scenario: Partial fetch failure is tolerated
**Given** a batch fetch is running for multiple symbols
**When** one or more symbols fail to fetch (e.g., delisted, API error)
**Then** the successfully fetched symbols are still cached
**And** the failed symbols are reported in the status without blocking the rest

### Scenario: Full fetch failure is handled gracefully
**Given** the market data provider is completely unreachable (e.g., network outage)
**When** a background or manual refresh is triggered
**Then** the refresh is reported as failed in the status
**And** existing cached data is preserved and served as-is
**And** the system does not enter a retry loop that blocks further operations

### Scenario: Non-trading days are not filled
**Given** the system is refreshing historical prices
**When** the date range includes weekends or market holidays
**Then** only actual trading days with price data are cached
**And** the system does not create placeholder or zero-price entries for dates with no price data

## Edge Cases

- **Symbol no longer exists** (delisted, ticker changed): The system records the fetch failure and reports it in the status, but does not delete existing cached data.
- **Very long history** (symbol with transactions going back many years): The system fetches the full range but does not block or time out — the background process handles large ranges.
- **Concurrent refresh attempts**: If a refresh is already running for a symbol, a new trigger for the same symbol is ignored or queued (not run twice simultaneously).
- **Empty portfolio**: If there are no positions or transactions, no historical data is fetched.
- **FX pair with no market data**: Some exotic currency pairs may not have historical data available; the system records the failure gracefully.
- **Provider rate limiting**: If the market data provider throttles requests, the system backs off gracefully rather than failing the entire batch.

## Constraints

- Historical data is append-only — once a price is cached for a given symbol + date, it is not overwritten.
- Only daily (end-of-day) prices are cached; no intraday data.
- The system uses a single market data provider; no provider failover or fallback.
- Historical prices are persisted in the existing price cache.
- Background fetches must not block the HTTP server or degrade responsiveness of user-facing pages.

## Non-Goals

- **No real-time streaming** — prices are fetched periodically or on-demand, not pushed live.
- **No alternative data providers** — Yahoo Finance is the sole source.
- **No intraday data** — only daily closing prices.
- **No dedicated management page** — status and refresh controls are inline with existing UI, not a separate page.
- **No price adjustment logic** — unadjusted close prices are cached as-is.
- **No per-symbol manual refresh** — user-facing refresh is all-or-nothing; background fetches may target individual symbols automatically. Per-symbol manual refresh may be added later if the batch operation becomes slow with many symbols.

## Dependencies

- **f010 (Portfolio Performance)** — this feature enhances the performance page by providing cached historical data for charts.
- **f009 (Positions)** — the system uses the positions table to determine which symbols and FX pairs need historical data.
- **f004 (Transaction CRUD)** — the system monitors transactions for new symbols that need historical data.
