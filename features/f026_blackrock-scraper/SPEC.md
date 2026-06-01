# Feature: BlackRock/iShares Data Extractor

**Feature ID**: f026
**Name**: blackrock-scraper
**Date**: 2026-06-01
**Status**: Draft

## Problem

Yahoo Finance data for iShares/BlackRock ETFs is often incomplete or lacks the specificity needed for deep portfolio analysis. BlackRock publishes comprehensive fund data on the iShares UK website — including fund profile, full holdings (350+ securities), portfolio characteristics (P/E, P/B, beta, etc.), and sector/geography exposure breakdowns. There is no way to extract and use this data in the current system.

## User Stories

### Story 1: Provider-based symbol details routing

**As a** portfolio user,
**I want** symbol details to be fetched from BlackRock/iShares when configured, instead of Yahoo Finance,
**so that** I get richer, more accurate, and more current fund data.

**Acceptance Criteria:**

- Given a symbol has the BlackRock provider configured,
  When symbol details are fetched (background refresh or manual trigger),
  Then the system dispatches to the BlackRock extractor instead of Yahoo Finance.

- Given a symbol has no data provider configured,
  When symbol details are fetched,
  Then the system uses Yahoo Finance as before (no behavior change).

- Given a symbol has the BlackRock provider configured,
  When market data (quotes, historical prices) is fetched,
  Then Yahoo Finance is still used for market data (only symbol details routing changes).

- Given a symbol is switched from Yahoo Finance (no provider) to the BlackRock provider,
  When the next refresh runs,
  Then symbol details are fetched from BlackRock instead of Yahoo, and market data continues to come from Yahoo.

- Given a symbol is assigned to the BlackRock provider but no extractor is available,
  When the system attempts to fetch symbol details,
  Then the fetch fails with an explicit error identifying the unregistered provider (no silent fallback to Yahoo).

- Given the BlackRock extractor is added to the system,
  When it is registered,
  Then no changes are required to the core routing logic or the database schema established in f021.

### Story 2: BlackRock data extraction

**As a** portfolio user with iShares/BlackRock ETFs,
**I want** the system to extract comprehensive fund data from iShares,
**so that** I can see full holdings, fund profile, portfolio characteristics, and sector/geography breakdowns directly from the provider.

**Acceptance Criteria:**

- Given an iShares ETF is configured with the BlackRock provider and a valid source URL,
  When the extractor runs,
  Then it extracts fund identity data (name, ticker, ISIN, currency, inception date) and stores it as structured data.

- Given an iShares ETF is configured with the BlackRock provider and a valid source URL,
  When the extractor runs,
  Then it extracts fund profile data (AUM, TER, asset class, domicile, product structure, methodology, benchmark, use of income, SFDR classification, rebalance frequency, fund manager, custodian, issuing company) and stores it as structured data, replacing the existing fund profile.

- Given the source URL is invalid or the fund cannot be found,
  When the extractor runs,
  Then the extraction fails with an explicit error and the symbol is marked as failed.

- Given an iShares ETF is configured with the BlackRock provider and a valid source URL,
  When the extractor runs,
  Then it extracts all holdings (ticker, name, sector, asset class, market value, weight percent, notional value, shares, price, location, exchange, market currency) and stores them as structured data.

- Given a fund has hundreds of holdings (e.g. 350+),
  When the extractor runs,
  Then all holdings are extracted and stored (no artificial cap).

- Given a fund has zero holdings,
  When the extractor runs,
  Then the holdings section is stored as empty (not treated as a failure).

- Given the holdings data is missing its reference date,
  When the extractor runs,
  Then the extraction fails with an explicit error. The reference date is required — storing holdings with an unknown publication date would be misleading.

- Given an iShares equity ETF is configured with the BlackRock provider and a valid source URL,
  When the extractor runs,
  Then it extracts equity-specific portfolio characteristics (P/E ratio, P/B ratio, 3y beta, standard deviation 3y, number of holdings) and stores them as structured data with "as of" date.

- Given an iShares bond ETF is configured with the BlackRock provider and a valid source URL,
  When the extractor runs,
  Then bond-specific portfolio characteristics (yield to maturity, weighted average YTM, weighted average maturity, modified duration, effective duration, WAL to worst, 3y beta, standard deviation 3y, number of holdings) are extracted instead of equity-specific metrics. Equity-specific fields are null for bond funds and vice versa.

- Given an iShares ETF is configured with the BlackRock provider and a valid source URL,
  When the extractor runs,
  Then it extracts sector allocation (sector name, percent) and stores it as structured data with "as of" date.

- Given an iShares ETF is configured with the BlackRock provider and a valid source URL,
  When the extractor runs,
  Then it extracts geography/country allocation (country name, percent) and stores it as structured data with "as of" date.

- Given a fund has no sector/geography data available,
  When the extractor runs,
  Then those sections are stored as empty (not treated as a failure).

- Given the extractor encounters a parsing error or the source returns an error,
  When the extraction runs,
  Then the error is logged explicitly and the symbol is marked as failed (no silent fallback to partial data).

- Given the fund page cannot be loaded (404, network error, bot detection),
  When the extraction runs,
  Then the entire extraction fails. The error is logged explicitly and the symbol is marked as failed.

- Given the fund page loads but the holdings data cannot be retrieved,
  When the extraction runs,
  Then the entire extraction is treated as failed (atomic — partial data is not persisted). The error is logged explicitly and the symbol is marked as failed.

- Given the page structure changes (e.g. BlackRock redesigns the iShares website),
  When the extraction runs,
  Then unexpected fields are handled gracefully and missing required fields fail with an explicit error.

### Story 3: Symbol details page — BlackRock data display

**As a** portfolio user,
**I want** the symbol details page to show all BlackRock-extracted data with its "as of" date,
**so that** I can see the full picture of what the fund holds and how it is positioned.

**Acceptance Criteria:**

- Given symbol details were fetched from the BlackRock extractor,
  When I view the symbol details page,
  Then I see the following sections (in addition to existing Overview and Fund Profile):
  - **Fund Characteristics** — P/E, P/B, 3y beta, standard deviation, number of holdings, and bond-specific metrics where applicable, with "as of" date.
  - **Full Holdings** — all securities with weights (expandable beyond top 10), with "as of" date.
  - **Sector Allocation** — sector breakdown with percentages, with "as of" date.
  - **Country Allocation** — country/region exposure with percentages, with "as of" date.

- Given the existing symbol details page already shows Top 10 Holdings, Sector Weightings, Fund Profile, and Geographic Allocation from Yahoo,
  When BlackRock extractor data is present,
  Then the BlackRock sections replace the Yahoo sections: "Full Holdings" replaces "Top 10 Holdings", "Sector Allocation" replaces "Sector Weightings", and "Country Allocation" replaces "Geographic Allocation".

- Given extractor data includes an "as of" date (the date embedded in BlackRock's data, **not** the date the system fetched it),
  When I view the symbol details page,
  Then each section shows its own "as of" date (e.g. holdings effective date, characteristics date may differ).

- Given the "as of" date differs from the fetch date (e.g. data is from 29 May but fetched on 30 May),
  When I view the symbol details page,
  Then the section shows the provider's "as of" date, not the fetch date.

### Story 4: Background refresh integration

**As a** portfolio user,
**I want** BlackRock-extracted data to stay current through automatic background refresh,
**so that** I always see up-to-date information without manual intervention.

**Acceptance Criteria:**

- Given a symbol is assigned to the BlackRock provider,
  When the background refresh runs,
  Then both symbol details (via BlackRock) and market data (via Yahoo) are refreshed.

- Given a symbol is assigned to the BlackRock provider,
  When the background refresh completes successfully,
  Then the symbol details page reflects the latest extracted data.

- Given the BlackRock extraction fails during background refresh,
  When the refresh completes,
  Then the symbol is marked as failed and the previous data (if any) is preserved.

## Data Concepts

### Source URL Configuration

The user sets the source URL on a symbol to the iShares fund page URL. The system routes the URL to the BlackRock extractor based on the provider assignment. This reuses the existing mechanism from f021 — no new configuration UI or storage is needed.

### "As Of" Dates

Each extracted section carries its own reference date (the date embedded in BlackRock's publication, **not** the date the system fetched it):

- **Holdings**: reference date from the holdings data
- **Portfolio characteristics**: reference date embedded per metric (different metrics may have different dates)
- **Sector allocation**: reference date from the holdings data (derived)
- **Geography allocation**: reference date from the holdings data (derived)
- **Fund profile**: reference date embedded per metric (e.g. AUM)

These dates are stored per section and displayed per section on the symbol details page.

## Edge Cases

- **URL Pattern**: iShares UK fund pages use a consistent URL pattern. The extractor resolves the fund from the URL.
- **Investor Type Selection**: The product page requires selecting an investor type before displaying content. The extractor handles this transparently.
- **Different Fund Types**: Equity ETFs and bond ETFs have different portfolio characteristics (valuation metrics vs duration/YTM). The extractor handles whatever fields are present.
- **Hybrid Funds**: Funds with both equity and bond characteristics may have both sets of metrics. The extractor handles whatever fields are present.
- **Missing Optional Data**: If certain sections (e.g. sector/geography) are absent on the page, the extractor handles this gracefully.
- **Empty Required Data**: If the response is missing required fields (ticker, name, internal identifier), the extraction fails with an explicit error.
- **Component ID Resolution Failure**: If the page structure changes and the internal component ID cannot be extracted, the extraction fails with an explicit error.
- **404 / Invalid Fund**: If the fund cannot be found for a given URL, the symbol is marked as failed with an explicit error.
- **Rate Limiting**: The extractor respects delays between requests to avoid overloading the source.
- **Page Structure Changes**: The extractor handles unexpected fields gracefully and fails explicitly on missing required fields.
- **Large Holdings Lists**: Funds with 350+ holdings are handled — all entries are extracted.
- **Duplicate Holdings**: If the same security appears multiple times (e.g. different share classes), each entry is preserved as returned by the source.
- **Securities Lending Data**: Present on ETF pages but not extracted (out of scope).
- **Performance/Returns Data**: Present on the page but not extracted (out of scope).
- **Share-Class-Specific Data**: Different accumulating/distributing variants of the same ETF share the same iShares page. The extractor targets fund-level data, not share-class distinctions.

## Non-Goals

- **Not in scope**: Support for non-ETF instruments (ETFs only).
- **Not in scope**: Support for the iShares US site (`ishares.com/us`). The UK site is the target.
- **Not in scope**: Automatic detection of which provider to use for a symbol. The provider assignment and source URL are set manually (same as f021).
- **Not in scope**: Official BlackRock API partnership — this uses the public-facing website.
- **Not in scope**: Real-time price data (market data continues to come from Yahoo Finance).
- **Not in scope**: Performance/returns data (annual returns, distribution history).
- **Not in scope**: Securities lending data (lending summary, collateral snapshot/matrix).
- **Not in scope**: Risk indicator / Morningstar ratings.
- **Not in scope**: Exchange listings data.
- **Not in scope**: Share-class-specific data (accumulating vs distributing variants). The extractor targets fund-level data.

## Constraints

- **Rate limiting**: 1-2 second delay between requests to the same provider.
- **Error handling**: Explicit errors, no silent fallbacks. If extraction fails, the symbol is marked as failed.
- **Bot detection**: Must handle potential Cloudflare/bot detection challenges gracefully.
- **Atomic extraction**: Required sections (fund identity, holdings with as-of date) are atomic — if any fails, entire extraction is rejected. Optional sections (sector, geography, characteristics) may be absent without causing failure.
- **Source URL is set manually**: The source URL on the symbol is set by the user to the iShares fund page URL, not auto-detected.
- **ETFs only**: The extractor is designed for iShares ETFs. Non-ETF funds (tracker funds, mutual funds) are not supported.
- **No schema changes**: The implementation reuses the core routing logic and database schema established in f021.

## Dependencies

- **Feature f021 (WisdomTree Scraper)**: Depends on the provider dispatcher, the source URL mechanism on symbol mappings, the symbol details table, and the UI components for displaying extractor data.
- **Market Data System**: Depends on the ability to store and query symbol details from extractor sources.
- **Existing scraper features (f022 DWS, f023 Dimensional, f024 iMGP, f025 Vanguard)**: Follows the same registration pattern (register extractor with dispatcher).
