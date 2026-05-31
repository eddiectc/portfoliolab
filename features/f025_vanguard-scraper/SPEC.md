# Feature: Vanguard Data Extractor

**Feature ID**: f025
**Name**: vanguard-scraper
**Date**: 2026-05-30
**Status**: Draft

## Problem

Yahoo Finance data for Vanguard ETFs and mutual funds is often incomplete or lacks the specificity needed for deep portfolio analysis. Vanguard publishes comprehensive fund data on their UK investor website (`vanguardinvestor.co.uk`) via a public API, including fund profile, individual holdings, sector allocation, country/region allocation with benchmark comparison, fund characteristics (P/E, P/B, ROE, etc.), and NAV/price history. There is no way to extract and use this data in the current system.

## User Stories

### Story 1: Fund identity and profile extraction

**As a** portfolio user with Vanguard funds,
**I want** the system to extract basic fund identity and profile data from Vanguard,
**so that** I can see accurate fund metadata directly from the provider.

**Acceptance Criteria:**

- Given a Vanguard fund is configured with the Vanguard provider and a valid source URL,
  When the extractor runs,
  Then it extracts fund identity data (name, ticker, SEDOL, ISIN, currency, inceptionDate) and stores it as structured data replacing the existing fund profile.

- Given a Vanguard fund is configured with the Vanguard provider and a valid source URL,
  When the extractor runs,
  Then it extracts fund profile data (managementStrategy, assetClassification, productType, marketRegionFocus, distributionStrategy, benchmark, expense ratio) and stores it as structured data.

- Given the source URL is invalid or the fund cannot be found,
  When the extractor runs,
  Then the extraction fails with an explicit error and the symbol is marked as failed.

### Story 2: Holdings extraction

**As a** portfolio user with Vanguard funds,
**I want** the system to extract the full holdings list from Vanguard,
**so that** I can see every security the fund holds with its weight, not just the top 10.

**Acceptance Criteria:**

- Given a Vanguard fund is configured with the Vanguard provider and a valid source URL,
  When the extractor runs,
  Then it extracts all holdings (issuerName, securityLongDescription, marketValuePercentage, securityType) and stores them as structured data.

- Given a fund has optional bond-specific fields (couponRate, finalMaturity),
  When the extractor runs,
  Then those fields are extracted where present and handled gracefully where absent (e.g. equity holdings have no coupon/maturity).

- Given a fund has zero holdings,
  When the extractor runs,
  Then the holdings section is stored as empty (not treated as a failure).

- Given the holdings data is missing its reference date (effectiveDate),
  When the extractor runs,
  Then the extraction fails with an explicit error. The reference date is required — storing holdings with an unknown publication date would be misleading.

### Story 3: Sector and country allocation with benchmark comparison

**As a** portfolio user with Vanguard funds,
**I want** the system to extract sector and country allocations alongside benchmark comparisons,
**so that** I can see how the fund's allocation differs from its benchmark.

**Acceptance Criteria:**

- Given a Vanguard fund is configured with the Vanguard provider and a valid source URL,
  When the extractor runs,
  Then it extracts sector allocation (sectorName, fundPercent, benchmarkPercent, date using ICB 12-sector standard) and stores it as structured data with benchmark comparison.

- Given a Vanguard fund is configured with the Vanguard provider and a valid source URL,
  When the extractor runs,
  Then it extracts country/region allocation (countryName, countryCode, fundMktPercent, benchmarkMktPercent, regionName, regionCode, date) and stores it as structured data with benchmark comparison.

- Given a fund has a large number of countries (100+),
  When the extractor runs,
  Then all countries are extracted and stored.

### Story 4: Fund characteristics extraction

**As a** portfolio user with Vanguard funds,
**I want** the system to extract fund characteristics from Vanguard,
**so that** I can see valuation and risk metrics (P/E, P/B, market cap, ROE, etc.) with benchmark comparison.

**Acceptance Criteria:**

- Given a Vanguard fund is configured with the Vanguard provider and a valid source URL,
  When the extractor runs,
  Then it extracts fund characteristics (P/E ratio, P/B ratio, median market cap, forward 5Y ROE, forward 5Y EPS growth, revenue/revenue prior year) and stores them as structured data with benchmark comparison where available.

- Given a fund is a bond fund rather than an equity fund,
  When the extractor runs,
  Then bond-specific characteristics (average coupon, average maturity, average quality, average duration) are extracted instead of equity-specific metrics. Equity-specific fields are null for bond funds and vice versa.

### Story 5: NAV and price history extraction

**As a** portfolio user with Vanguard funds,
**I want** the system to extract NAV and market price history from Vanguard,
**so that** I can compare NAV against market prices across exchange listings.

**Acceptance Criteria:**

- Given a Vanguard fund is configured with the Vanguard provider and a valid source URL,
  When the extractor runs,
  Then it extracts NAV prices (price, asOfDate, currencyCode) and stores them as structured data.

- Given a fund trades on multiple exchanges,
  When the extractor runs,
  Then market prices per exchange listing (price, asOfDate, currencyCode) are extracted and stored alongside NAV data.

- Given the price history query returns data for a date range,
  When the extractor runs,
  Then the data is stored as-is without interpolation for gaps or non-trading dates.

### Story 6: Error handling

**As a** portfolio user with Vanguard funds,
**I want** extraction failures to be reported explicitly,
**so that** I know when data is stale or unavailable.

**Acceptance Criteria:**

- Given the extractor encounters a parsing error or the source returns an error,
  When the extraction runs,
  Then the error is logged explicitly and the symbol is marked as failed (no silent fallback to partial data).

- Given the fund identity lookup fails (Phase 1 — required to resolve the fund's internal identifier),
  When the extraction runs,
  Then the entire extraction fails. Without the internal identifier, deep data queries cannot be executed.

- Given the fund identity lookup succeeds but one or more deep data queries fail (Phase 2 — holdings, sectors, countries, characteristics, prices),
  When the extraction runs,
  Then the entire extraction is treated as failed (atomic — partial data is not persisted). The error is logged explicitly and the symbol is marked as failed.

- Given the API structure changes (reverse-engineered API, not officially documented),
  When the extraction runs,
  Then unexpected fields are handled gracefully and missing required fields fail with an explicit error.

### Story 7: Integration with extractor framework

**As a** system maintainer,
**I want** the Vanguard provider to integrate seamlessly with the existing extractor framework,
**so that** symbols assigned to the Vanguard provider are routed and processed automatically.

**Acceptance Criteria:**

- Given the Vanguard extractor is registered with the provider dispatcher,
  When the system fetches symbol details for a symbol assigned to the Vanguard provider,
  Then the Vanguard extractor is invoked instead of Yahoo Finance.

- Given a symbol is assigned to the Vanguard provider,
  When the background refresh runs,
  Then both symbol details (via Vanguard) and market data (via Yahoo) are refreshed.

- Given a symbol is switched from Yahoo Finance (no provider) to the Vanguard provider,
  When the next refresh runs,
  Then symbol details are fetched from Vanguard instead of Yahoo, and market data continues to come from Yahoo.

- Given a symbol is assigned to the Vanguard provider but no extractor is registered,
  When the system attempts to fetch symbol details,
  Then the fetch fails with an explicit error identifying the unregistered provider (no silent fallback to Yahoo).

- Given the Vanguard extractor implementation,
  When it is added to the system,
  Then no changes are required to the core routing logic or the database schema established in f021.

### Story 8: Symbol details page — Vanguard data display

**As a** portfolio user,
**I want** the symbol details page to show all Vanguard-extracted data with its "as of" date,
**so that** I can see the full picture of what the fund holds and how it is positioned.

**Acceptance Criteria:**

- Given symbol details were fetched from the Vanguard extractor,
  When I view the symbol details page,
  Then I see the following sections (in addition to existing Overview and Fund Profile):
  - **Fund Characteristics** — P/E, P/B, median market cap, ROE, EPS growth, and bond-specific metrics where applicable, with benchmark comparison and "as of" date.
  - **Top 10 Holdings** — all securities with weights (expandable beyond top 10), with "as of" date.
  - **Sector Allocation** — ICB 12-sector breakdown with fund vs benchmark percentages, with "as of" date.
  - **Country Allocation** — country/region exposure with fund vs benchmark percentages, with "as of" date.
  - **NAV vs Price Chart** — NAV history and market price history plotted together; multiple exchange listings are shown as separate series; the two series may have non-overlapping date ranges and are displayed as-is without interpolation.

- Given the existing symbol details page already shows Top 10 Holdings, Sector Weightings, Fund Profile, and Geographic Allocation from Yahoo,
  When Vanguard extractor data is present,
  Then the Vanguard sections replace/augment the Yahoo sections (e.g. "Country Allocation" replaces "Geographic Allocation").

- Given extractor data includes an "as of" date (the date embedded in Vanguard's data, **not** the date the system fetched it),
  When I view the symbol details page,
  Then each section shows its own "as of" date (e.g. holdings effectiveDate, sector allocation date, country allocation date may differ).

- Given the "as of" date differs from the fetch date (e.g. data is from 30 Apr but fetched on 30 May),
  When I view the symbol details page,
  Then the section shows the provider's "as of" date, not the fetch date.

## Data Concepts

### Source URL Configuration

The user sets the `data_source_url` on a symbol to the Vanguard fund page URL (e.g. `https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing`). The dispatcher routes the URL to the Vanguard extractor, which resolves the fund's internal identifier from the URL. This reuses the existing mechanism from f021 — no new configuration UI or storage is needed.

### Extraction Phases

The extraction proceeds in two phases:

1. **Phase 1 — Identifier Resolution**: Resolves the fund's internal identifier from the source URL and extracts fund identity and profile overview. If this fails, the entire extraction fails — deep data queries require the internal identifier.

2. **Phase 2 — Deep Data**: Independent queries for holdings, sector allocation, country allocation, fund characteristics, and NAV/price history. If any query fails, the entire extraction fails — partial data is not persisted.

See RESEARCH.md for API endpoint details, query specifications, and pagination mechanics.

### "As Of" Dates

Each extracted section carries its own reference date (the date embedded in Vanguard's publication, **not** the date the system fetched it):

- **Holdings**: `effectiveDate` from the holdings response
- **Sector allocation**: `date` from the sector response
- **Country allocation**: `date` from the country response
- **Fund characteristics**: derived from the analytics period
- **NAV/price history**: each data point carries its own `asOfDate`

These dates are stored per section and displayed per section on the symbol details page.

## Edge Cases

- **URL Pattern**: Vanguard UK fund pages use a consistent URL pattern with a kebab-case fund slug. The extractor resolves the fund from this slug.
- **Holdings Pagination**: Funds with many holdings require multiple requests. The extractor handles pagination transparently with appropriate delays between requests.
- **Missing Optional Fields**: If certain fields (e.g. couponRate, finalMaturity for equity holdings) are null, the extractor handles this gracefully.
- **Empty Required Data**: If the response is missing required fields (ticker, name, internal identifier), the extraction fails with an explicit error.
- **404 / Invalid Fund Slug**: If the fund cannot be found for a given URL, the symbol is marked as failed with an explicit error.
- **Rate Limiting**: The extractor respects delays between requests to avoid overloading the source.
- **API Changes**: Since the API is reverse-engineered (not officially documented), field names or structure may change. The extractor handles unexpected fields gracefully and fails explicitly on missing required fields.
- **Different Fund Types**: ETFs and mutual funds may return different data (e.g. bond funds have coupon/maturity data, equity funds don't). The extractor handles whatever fields are present.
- **Empty Holdings**: If the holdings query returns zero items, the holdings section is stored as empty (not a failure).
- **Large Country Lists**: Some funds have 100+ countries in market allocation. All are extracted and stored.
- **Benchmark Comparison**: Sector and country allocations include benchmark percentages. Stored alongside fund percentages for comparison display.
- **Multiple Exchange Listings**: ETFs may trade on multiple exchanges with different currency listings. All are extracted and displayed.

## Non-Goals

- **Not in scope**: Browser automation (chromedp/playwright). The public API approach is sufficient.
- **Not in scope**: Support for the Vanguard US site (`investor.vanguard.com`). The UK site provides a richer public API.
- **Not in scope**: Automatic detection of which provider to use for a symbol. The provider assignment and source URL are set manually (same as f021).
- **Not in scope**: Official Vanguard API partnership — this uses the public-facing API of the UK investor website.
- **Not in scope**: Real-time price data (market data continues to come from Yahoo Finance).
- **Not in scope**: Performance/returns data beyond what the API provides (annual NAV returns).

## Constraints

- **All-or-nothing**: If any query fails (Phase 1 or Phase 2), the entire extraction fails. Partial data is not persisted. The error is logged explicitly and the symbol is marked as failed.
- **Phase 1 prerequisite**: Required data from the initial lookup (fund identity including ticker, name, internal identifier) must be present for the extraction to proceed. Without the internal identifier, deep data queries cannot be executed.
- **Rate limiting**: Delays between requests to avoid overloading the source.
- **Error handling**: Explicit errors, no silent fallbacks. If extraction fails, the symbol is marked as failed and existing cached data is preserved.
- **Source URL is set manually**: The `data_source_url` on the symbol is set by the user to the Vanguard fund page URL, not auto-detected.
- **Reverse-engineered API**: The API endpoints are not officially documented. The extractor must be resilient to structural changes and fail explicitly when expected fields are missing.

## Dependencies

- **Feature f021 (WisdomTree Scraper)**: Depends on the provider dispatcher, the `data_source_url` column on `symbol_mappings`, the `symbol_details` table, and the UI components for displaying extractor data.
- **Market Data System**: Depends on the ability to store and query symbol details from extractor sources.
- **Feature f022 (DWS Scraper) / f023 (Dimensional Scraper) / f024 (iMGP Scraper)**: Follows the same registration pattern (register extractor with dispatcher).
