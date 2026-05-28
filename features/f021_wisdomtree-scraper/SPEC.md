# Feature: Data Extractor Framework with WisdomTree Provider

**Feature ID**: f021
**Name**: wisdomtree-scraper
**Date**: 2026-05-26
**Status**: Draft

## Problem

Yahoo Finance provides limited and sometimes outdated data for ETFs, especially illiquid or non-US funds. WisdomTree ETFs publish richer, more current data directly on their website — including full holdings (800+ securities), NAV history, country allocation, market cap breakdown, fund characteristics (P/E, P/B, etc.), and theme allocations. There is no way to extract and use this data in the current system.

Additionally, other data providers will be added in the future, so the architecture must support multiple providers without code changes to the core routing logic.

## User Stories

### Story 1: Provider-based symbol details routing

**As a** portfolio user,
**I want** symbol details to be fetched from the fund provider's website when configured, instead of Yahoo Finance,
**so that** I get richer, more accurate, and more current fund data.

**Acceptance Criteria:**

- Given a symbol has a data provider configured,
  When symbol details are fetched (background refresh or manual trigger),
  Then the system dispatches to the correct provider extractor instead of Yahoo Finance.

- Given a symbol has no data provider configured,
  When symbol details are fetched,
  Then the system uses Yahoo Finance as before (no behavior change).

- Given a symbol has a data provider configured,
  When market data (quotes, historical prices) is fetched,
  Then Yahoo Finance is still used for market data (only symbol details routing changes).

### Story 2: WisdomTree data extraction

**As a** portfolio user with WisdomTree ETFs,
**I want** the system to extract comprehensive fund data from WisdomTree's website,
**so that** I can see full holdings, NAV history, country/sector/theme breakdowns, and fund characteristics.

**Acceptance Criteria:**

- Given a WisdomTree ETF is configured with the WisdomTree provider,
  When the extractor runs,
  Then it extracts the following data:
  - **Overview** (AUM, TER, inception date, fund family, legal type, annual holdings turnover) — stored as structured data replacing the existing fund profile
  - **Full NAV history** — stored as a distinct data type in the market data system, tagged with an extractor source to distinguish from Yahoo-sourced data
  - **Market Capitalization** (total market cap + large/mid/small cap breakdown)
  - **Fund Characteristics** (P/E, Estimated P/E, P/B, P/S, P/CF, dividend yield) — replacing the existing equity valuation data; fields beyond the existing four (P/E, P/B, P/CF, P/S) require extending the stored structure
  - **Full Holdings** (all securities returned by the provider with weights, not limited to top 10; if the provider paginates or caps results, all available pages/entries are fetched)
  - **Theme Breakdown** (theme allocation percentages)
  - **Sector Breakdown** (sector allocation percentages)
  - **Country Allocation** (country/region exposure percentages)

- Given the extractor encounters a parsing error or missing data,
  When the extraction runs,
  Then the error is logged explicitly and the symbol is marked as failed (no silent fallback to partial data).

- Given the extractor successfully parses some sections but fails on others,
  When the extraction runs,
  Then the entire extraction is treated as failed (atomic — partial data is not persisted).

### Story 3: Symbol details page — extractor data display

**As a** portfolio user,
**I want** the symbol details page to show all extracted data with its "as of" date,
**so that** I can see the full picture of what the fund holds and how it is positioned.

**Acceptance Criteria:**

- Given symbol details were fetched from a provider extractor,
  When I view the symbol details page,
  Then I see the following sections (in addition to existing Overview and Fund Profile):
  - **Market Capitalization** — total market cap + large/mid/small cap breakdown, with "as of" date
  - **Fund Characteristics** — P/E, Estimated P/E, P/B, P/S, P/CF, dividend yield, with "as of" date
  - **Full Holdings** — all securities with weights (not limited to top 10), with "as of" date
  - **Theme Breakdown** — theme allocation percentages, with "as of" date
  - **Sector Breakdown** — sector allocation percentages, with "as of" date (replaces existing Yahoo sector weightings when extractor data is present)
  - **Country Allocation** — country/region exposure percentages, with "as of" date (replaces existing Yahoo geographic allocation when extractor data is present)
  - **NAV vs Price Chart** — NAV history and market price history plotted together on the same chart for comparison; the two series may have non-overlapping date ranges (e.g., NAV may start earlier or have gaps) and are displayed as-is without interpolation

- Given the existing symbol details page already shows Top 10 Holdings, Sector Weightings, Fund Profile, and Geographic Allocation from Yahoo,
  When extractor data is present,
  Then the extractor sections replace/augment the Yahoo sections (e.g. "Full Holdings" replaces "Top 10 Holdings", "Country Allocation" replaces "Geographic Allocation").

- Given extractor data includes an "as of" date (the reference date embedded in the provider's data, e.g. "As of 22 May 2026" on the WisdomTree page — **not** the date the system fetched it),
  When I view the symbol details page,
  Then each extractor-sourced section shows its "as of" date.

- Given the "as of" date differs from the fetch date (e.g. data is from 5/22 but fetched on 5/26),
  When I view the symbol details page,
  Then the section shows the provider's "as of" date, not the fetch date.

### Story 4: Background refresh integration

**As a** portfolio user,
**I want** extracted data to stay current through automatic background refresh,
**so that** I always see up-to-date information without manual intervention.

**Acceptance Criteria:**

- Given the periodic background fetcher runs,
  When it encounters a symbol with a data provider configured,
  Then it refreshes symbol details using the provider extractor (not Yahoo).

- Given the periodic background fetcher runs,
  When it encounters a symbol with a data provider configured,
  Then it also refreshes the full NAV history from the provider extractor (not incremental — the complete NAV history is re-fetched).

- Given the manual "refresh all" is triggered,
  When the refresh completes,
  Then all data types are refreshed for provider-configured symbols:
  market data (Yahoo), NAV history (extractor), and symbol details (extractor).

- Given the stale threshold is 7 days (same as existing symbol details),
  When a symbol's details are older than 7 days,
  Then the background fetcher schedules a refresh using the appropriate source (Yahoo or extractor).

### Story 5: Extensible provider architecture

**As a** system maintainer,
**I want** the extractor framework to support new providers without modifying core routing logic,
**so that** adding future providers (e.g., iShares, Vanguard) is straightforward.

**Acceptance Criteria:**

- Given a new provider is added,
  When its extractor is registered with the dispatcher,
  Then symbols assigned to that provider are automatically routed to it without changes to the routing logic.

- Given the WisdomTree extractor is implemented,
  When a new provider is added,
  Then no changes are needed to the dispatcher, routing logic, or database schema (only the new extractor implementation and any provider-specific data mappings).

- Given a symbol is assigned to a provider that has no extractor registered,
  When the system attempts to fetch symbol details for that symbol,
  Then the fetch fails with an explicit error identifying the unregistered provider (no silent fallback to Yahoo).

## Data Concepts

### Provider assignment

A symbol can be associated with an alternative data provider. When present, the system routes symbol details fetching to that provider instead of Yahoo Finance. The assignment is persisted per-symbol and is manually configured (not auto-detected). Market data (quotes, historical prices) always uses Yahoo Finance regardless of provider assignment.

### Extractor data stored alongside existing fields

When a provider extractor supplies data, it replaces or augments the existing symbol details fields:
- **Fund profile** — replaced with provider-extracted overview data (AUM, TER, etc.)
- **Equity valuation** — replaced with provider-extracted fund characteristics (P/E, P/B, etc.)
- **Top holdings** — replaced with full holdings from provider (not just top 10)
- **Sector weightings** — replaced with provider-extracted sector breakdown
- **Geographic allocations** — replaced with provider-extracted country allocation

Additional data stored:
- **Theme breakdown** — theme allocation percentages (WisdomTree-specific, may be extended for other providers)
- **Market cap breakdown** — total market cap + large/mid/small cap split

### NAV history

NAV data is stored in the market data system using a distinct data type from stock prices and FX rates, and tagged with a source that distinguishes it from Yahoo-sourced data. Existing data types (`stock`, `fx`) and sources (`yahoo`) remain unchanged.

> **Cross-layer note**: Any existing query that filters on `data_type` (e.g., `data_type IN ('stock', 'fx')`) must be audited and updated to account for the new NAV data type, or NAV-specific queries must be created. This includes historical price retrieval, gap-fill logic, and chart data endpoints.

### "As of" date

Provider-extracted data carries a reference date (the date embedded in the provider's publication, e.g., "As of 22 May 2026" on the WisdomTree page). This is distinct from the fetch date (when the system actually scraped the data). The reference date is stored per extraction and displayed per section on the symbol details page. For NAV history, each data point carries its own date (the NAV date), so no separate "as of" is needed at the row level.

## Edge Cases

- **Provider assigned but extractor not registered**: Symbol has a provider assignment but no extractor implementation exists — fetch fails with explicit error, no fallback to Yahoo.
- **Partial extraction (some sections parse, others don't)**: Required sections (fund info, holdings, as-of date) are atomic — if any fails, entire extraction is rejected. Optional sections (NAV history, themes, sectors, country allocation, market cap, fund characteristics, fund profile) return empty results when not present on the page; the extraction succeeds with whatever data is available.
- **Different provider pages have different data sections**: Not all WisdomTree pages include all data sections. E.g., WMGT lacks `fundSectorsData`, QGRW.L lacks `fundMarketData` and `fundThemeData`. The extractor handles this gracefully — missing optional sections return nil, present sections are parsed normally.
- **NAV data and price data on same symbol**: NAV and price share the same symbol identifier but are distinguished by data type and source; queries must not mix them.
- **Rate limiting between Yahoo and extractor**: Extractor-configured symbols trigger both Yahoo (market data) and extractor (symbol details + NAV) fetches; rate limiting applies independently to each source.
- **Cloudflare/bot detection blocks the extractor**: The request fails with an explicit error; the symbol is marked as failed and existing cached data is preserved.
- **Provider page structure changes**: Parser fails to find expected data sections — treated as a parsing error, logged with details, symbol marked as failed.
- **Provider returns no data for a symbol**: The page loads but expected data sections are missing — treated as a parsing error (same as above).
- **Holdings list is very large (800+ securities)**: All holdings are fetched and stored; no artificial cap is applied.
- **NAV history has gaps or non-trading dates**: NAV data points are stored as-is from the provider; the chart displays available points without interpolation.
- **"As of" date is missing from provider page**: The extraction fails entirely (no fallback to fetch date). The "as of" date is a required field — without it, the reference date for all extracted data is unknown, and storing data with an incorrect reference date would be worse than failing. The fetch date is already captured in `fetched_at`.

## Non-Goals

- **Not in scope**: Browser automation (chromedp/playwright) — prefer lightweight HTTP + parsing. If certain data requires JS rendering, use a server-side article extraction approach.
- **Not in scope**: Real-time streaming or WebSocket updates.
- **Not in scope**: Support for non-ETF instruments (this is ETF-focused).
- **Not in scope**: Automatic symbol-to-provider mapping (the provider assignment is set manually or via admin API, not auto-detected).
- **Not in scope**: NAV history as a standalone table (it is displayed as a chart alongside price data).

## Constraints

- Rate limiting: 1-2 second delay between requests to the same provider.
- Error handling: Explicit errors, no silent fallbacks. If extraction fails, the symbol is marked as failed.
- Cloudflare/bot detection: Must handle potential challenges gracefully.
- Data consistency: NAV history uses the same symbol identifier as market data (e.g., "WMGT LN"), stored with a distinct data type and source tag to distinguish from Yahoo data.
- Extraction is atomic for required sections (fund info, holdings, as-of date): if any required section fails to parse, the entire extraction is rejected. Optional sections (NAV, themes, sectors, country, market cap, characteristics, profile) may be absent on some pages without causing failure.
- Provider assignment is manual: symbols are associated with providers through configuration, not auto-detection.
- NAV chart displays both series without interpolation: NAV and price data points that don't share dates are shown as-is.

## Dependencies

- Existing `symbol_details` table and service layer (f015).
- Existing `market_data` table and cache layer (f011).
- Existing `MarketCache` periodic fetcher and `RefreshAll` logic.
- Existing `symbols` service (`FetchAndStore`, `RefreshSymbol`, `GetStaleSymbols`).
