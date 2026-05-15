# Feature: Symbol Details

## Description

A symbol details system that fetches and caches rich metadata about each symbol from Yahoo Finance — generic info (name, exchange) for all symbols, plus ETF-specific data (top holdings, sector weightings, fund profile) for ETFs. This lays the foundation for future analysis (e.g., understanding actual underlying holdings across all ETFs in a portfolio) while giving the user a way to verify data via API and a read-only web page.

Data is fetched immediately when a symbol is added to the system, and refreshed by a background job when stale (>7 days). Latest price is shown on the details page via a live Yahoo call at render time (not stored — handled by the existing price refresh job).

## User Stories

### US-1: Fetch Symbol Details on Symbol Creation
As a user, when I add a new symbol mapping, I want its details (name, exchange, and ETF holdings if applicable) to be fetched automatically so that the data is available immediately.

### US-2: View Symbol Details via API
As a developer (or future mobile client), I want to fetch symbol details for any symbol via the API so that I can programmatically access name, exchange, and ETF holdings data.

### US-3: View Symbol Details in Web UI
As a user, I want to browse a read-only symbol details page so that I can verify the name, exchange, and ETF holdings data for any symbol.

### US-4: Background Refresh of Stale Symbol Details
As a user, I want symbol details to be refreshed automatically when they become stale (>7 days) so that the data stays current without manual intervention.

### US-5: Live Price on Details Page
As a user viewing symbol details, I want to see the latest price alongside the static details so that I have a complete picture, without duplicating the existing price caching system.

## Scenarios

### Scenario: Fetch generic symbol details on symbol creation
**Given** I create a new symbol mapping with internal symbol `AAPL` and market data provider symbol `AAPL`
**When** the symbol mapping is saved
**Then** the system fetches generic details (name, exchange) for `AAPL` from Yahoo Finance
**And** the details are stored in the database
**And** if the fetch fails, the symbol mapping is still created successfully (details fetch is non-blocking)

### Scenario: Fetch ETF-specific details on symbol creation
**Given** I create a new symbol mapping for `WMGG.L` (an ETF)
**When** the symbol mapping is saved
**Then** the system fetches generic details (name, exchange) plus ETF-specific data (top holdings, sector weightings, fund profile, aggregate positions)
**And** all data is stored in the database

### Scenario: Symbol details fetch fails on creation
**Given** I create a new symbol mapping for a symbol that Yahoo Finance doesn't recognize
**When** the symbol mapping is saved
**Then** the symbol mapping is created successfully
**And** no symbol details are stored
**And** the failure is logged but not surfaced to the user

### Scenario: View symbol details via API (generic symbol)
**Given** symbol `AAPL` has cached symbol details
**When** I request the symbol details via the API
**Then** I receive the name, exchange, and other generic fields
**And** no ETF-specific data is included

### Scenario: View symbol details via API (ETF symbol)
**Given** symbol `WMGG.L` has cached symbol details including ETF data
**When** I request the symbol details via the API
**Then** I receive the generic fields (name, exchange) plus ETF-specific data (top holdings, sector weightings, fund profile, aggregate positions)
**And** the response includes metadata (fetched at, source)

### Scenario: View symbol details via API (no cached data)
**Given** symbol `XYZ` exists but has no cached symbol details
**When** I request the symbol details via the API
**Then** I receive a response indicating no details are available

### Scenario: View symbol details in web UI
**Given** symbol `WMGG.L` has cached symbol details
**When** I navigate to the symbol details page for `WMGG.L`
**Then** I see the symbol name, exchange, and latest price (fetched live)
**And** I see the top 10 holdings with symbol, name, and allocation percentage
**And** I see sector weightings sorted by weight descending
**And** I see aggregate positions (stock %, bond %, cash %)
**And** I see fund profile data (family, legal type, net assets, expense ratio)
**And** the page is read-only (no edit controls)

### Scenario: View symbol details in web UI (generic symbol)
**Given** symbol `AAPL` has cached symbol details (no ETF data)
**When** I navigate to the symbol details page for `AAPL`
**Then** I see the symbol name, exchange, and latest price (fetched live)
**And** no ETF-specific sections are shown

### Scenario: View symbol details in web UI (no cached data)
**Given** symbol `XYZ` has no cached symbol details
**When** I navigate to the symbol details page for `XYZ`
**Then** I see a message indicating no details are available
**And** the page does not error

### Scenario: Background refresh fetches stale symbol details
**Given** symbol `WMGG.L` has cached symbol details that are 8 days old
**When** the background refresh job runs
**Then** the system fetches fresh details from Yahoo Finance
**And** the cached data is updated
**And** the fetched-at timestamp is updated

### Scenario: Background refresh skips fresh symbol details
**Given** symbol `WMGG.L` has cached symbol details that are 2 days old
**When** the background refresh job runs
**Then** the system skips fetching details for this symbol
**And** the existing cached data is unchanged

### Scenario: Background refresh handles fetch failure gracefully
**Given** symbol `WMGG.L` has cached symbol details that are stale
**And** Yahoo Finance is temporarily unavailable
**When** the background refresh job runs
**Then** the failure is logged
**And** the existing cached data is preserved (not cleared)
**And** other symbols in the refresh batch are not affected

### Scenario: Live price shown on details page
**Given** symbol `WMGG.L` has cached symbol details
**When** I view the symbol details page
**Then** the latest price is fetched live from Yahoo Finance at render time
**And** the price is displayed alongside the cached details
**And** if the live price fetch fails, the page still renders with cached details and shows the price as unavailable

## Edge Cases

- **Symbol not found on Yahoo**: Symbol mapping exists but Yahoo has no data — details fetch silently fails, symbol mapping is created
- **ETF with no holdings data**: Yahoo returns the symbol but topHoldings module is empty — generic details stored, ETF sections show "no data available"
- **Partial data**: Yahoo returns generic details but not ETF data (or vice versa) — whatever is available is stored
- **International holding symbols**: ETF holdings may reference symbols on non-US exchanges (e.g., `ABBN.SW`, `7011.T`, `601126.SS`) — stored as-is
- **Stale data display**: Cached data older than 7 days — details page shows a "last updated" timestamp
- **Concurrent fetch**: Symbol details being fetched by background job while user views the page — user sees cached data (if any) plus live price
- **Yahoo API rate limit**: Fetch fails with 429 — logged, not retried immediately; cached data preserved
- **Symbol type changes**: A symbol that was previously an ETF is no longer classified as one by Yahoo — next refresh updates accordingly

## Constraints

- **Data source**: Yahoo Finance `quoteSummary` API (`topHoldings`, `fundProfile` modules) and `Info()` / `Quote()` for generic data
- **Auth**: Same crumb/cookie authentication flow as existing market data
- **Storage**: Symbol details stored in a dedicated database table (not the `market_data` table)
- **Generic fields stored**: name (short name, long name), exchange, currency, quote type (ETF/stock/etc.)
- **ETF fields stored**: top 10 holdings (symbol, name, percent), sector weightings, aggregate positions (stock/bond/cash/convertible/preferred/other), fund profile (family, legal type, net assets, expense ratio, turnover), equity valuation ratios (P/E, P/B, P/CF, P/S)
- **Latest price**: fetched live on the details page via Yahoo call at render time; NOT stored in the symbol details table (handled by existing price refresh)
- **Fetch on creation**: triggered after symbol mapping is saved; non-blocking — symbol creation succeeds even if details fetch fails
- **Background refresh**: runs periodically, checks `fetched_at` timestamp, refreshes if >7 days old
- **Read-only UI**: symbol details page displays data only; no edit/delete controls
- **Error responses**: `{"error": "message", "code": "ERROR_CODE"}`
- **Number formatting**: percentages with 2 decimal places; monetary values with 2 decimal places and thousands separator

## Non-Goals

- Editing symbol details (read-only)
- Manual refresh trigger for individual symbols (background job only; manual refresh can be added later)
- Historical holdings snapshots (point-in-time composition)
- Drill-down into individual holding details from within an ETF's holdings
- Portfolio-level aggregate sector exposure (aggregating across all ETF holdings)
- Support for market data providers other than Yahoo Finance
- Symbol search/discovery
- Displaying holdings for non-ETF instruments
- Price caching in symbol details (prices handled by existing market data system)

## Dependencies

- **f003_symbol-map** (done) — symbol details are keyed by internal symbol; market data provider symbol (Yahoo ticker) resolved via symbol mapping
- **f014_benchmark-selection** — symbol details fetch on creation may be needed before benchmark selection uses the data
- **Market data layer** (`internal/market/`) — existing crumb/cookie auth and Yahoo Finance integration pattern reused
- **Market cache** (`internal/domain/marketcache/`) — background refresh pattern followed for symbol details refresh job
