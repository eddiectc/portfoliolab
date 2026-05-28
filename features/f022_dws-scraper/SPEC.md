# Feature: Data Extractor Framework with DWS Provider

**Feature ID**: f022
**Name**: dws-scraper
**Date**: 2026-05-28
**Status**: Draft

## Problem

Yahoo Finance data for DWS Xtrackers ETFs is often incomplete or lacks the granularity needed for deep portfolio analysis. DWS provides detailed fund data, including full holdings, precise TER (Total Expense Ratio), and a comprehensive NAV history via their own API. To provide users with the highest quality data, the system needs a specialized extractor for DWS funds.

## User Stories

### Story 1: DWS data extraction

**As a** portfolio user with DWS Xtrackers ETFs,
**I want** the system to extract comprehensive fund data from DWS's API,
**so that** I can see full holdings, accurate costs, NAV history, and detailed allocations.

**Acceptance Criteria:**

- Given a DWS ETF is configured with the DWS provider and a valid DWS slug,
  When the extractor runs,
  Then it extracts the following data via the DWS JSON API:
  - **Fund Overview** (Product type, internal identifier) — stored as structured data replacing the existing fund profile.
  - **Total Expense Ratio (TER)** — extracted from the costs and fees data, replacing/augmenting the existing fund profile.
  - **Full Holdings** (all securities with ISIN, Name, and Weight) — replaces "Top 10 Holdings".
  - **Country Allocation** — aggregated from the holdings data (sum of weights per country), replacing "Geographic Allocation".
  - **Sector Weightings** — aggregated from the holdings data (sum of weights per industry), replacing "Sector Weightings".
  - **Full NAV history** — extracted from the performance chart endpoint, stored as a distinct data type tagged with the DWS source.
  - **Reference Date** — the "As of" date provided by the DWS API, used as the reference date for all extracted sections.

- Given the extractor encounters a parsing error or the API returns an error,
  When the extraction runs,
  Then the error is logged explicitly and the symbol is marked as failed (no silent fallback to partial data).

- Given the extractor successfully parses some sections but fails on required ones (Holdings, Reference Date),
  When the extraction runs,
  Then the entire extraction is treated as failed (atomic — partial data is not persisted).

### Story 2: Integration with Extractor Framework

**As a** system maintainer,
**I want** the DWS provider to integrate seamlessly with the existing extractor framework,
**so that** symbols assigned to the DWS provider are routed and processed automatically.

**Acceptance Criteria:**

- Given the DWS extractor is registered with the provider dispatcher,
  When the system fetches symbol details for a symbol assigned to the DWS provider,
  Then the DWS extractor is invoked instead of Yahoo Finance.

- Given a symbol is assigned to the DWS provider,
  When the background refresh runs,
  Then both symbol details (via DWS) and market data (via Yahoo) are refreshed.

- Given the DWS extractor implementation,
  When it is added to the system,
  Then no changes are required to the core routing logic or the database schema established in f021.

## Data Concepts

### API-Based Extraction

Unlike HTML scraping, the DWS provider utilizes a REST API. The extractor will make requests to `https://etf.dws.com/api/pdp/en-gb/etf/{identifier}/{endpoint}`.

### Data Mapping

| DWS API Endpoint | Portfolio Lab Domain Field | Logic |
|---|---|---|
| `pdpSettings` | Fund Profile | Map product type and identifier. |
| `pdpMetaTagsTealium` | Fund Info, Fund Profile | Extract Fund Name, Total AUM, and TER. |
| `holdings` | Full Holdings | Extract ISIN, Name, and Weight. |
| `holdings` | Country Allocation | Aggregate weights grouped by Country. |
| `holdings` | Sector Weightings | Aggregate weights grouped by Industry. |
| `performancechart` | NAV History | Map timestamps and values to market data points. |
| `performancechart` | "As of" Date | Use the API's provided reference date as the extraction date. |

### NAV History

NAV data is stored using the `nav` data type and `dws` source tag, ensuring it is distinguished from Yahoo-sourced market prices in the database and UI.

## Edge Cases

- **Identifier Mismatch**: The DWS API uses a "slug" (e.g., `IE00BGV...-artificial-intelligence...`) rather than a simple ticker. The provider configuration must include this slug for the extractor to function.
- **Missing Data Sections**: If a specific fund lacks TER or certain holdings metadata, the extractor should handle the missing JSON keys gracefully without failing the entire extraction, provided the "required" sections (Holdings, Reference Date) are present.
- **Empty Required Data**: If the API returns a success response but the holdings list is empty for a fund that should have holdings, this is treated as a parsing error and the extraction fails.
- **API Rate Limiting**: The extractor must respect a delay between requests to the same API to avoid being blocked.
- **Invalid Slug**: If the API returns a 404 for a given identifier, the symbol is marked as failed with an "Identifier not found" error.
- **Large Holdings List**: The extractor must process all entries in the holdings table regardless of size.

## Non-Goals

- **Not in scope**: Browser automation (chromedp/playwright) — the research confirms JSON API is sufficient.
- **Not in scope**: Support for non-ETF products offered by DWS.
- **Not in scope**: Automatic mapping of tickers to DWS slugs (slugs are provided via configuration/API).

## Constraints

- **Atomicity**: Required sections (Holdings, Reference Date) must be present and non-empty for a successful extraction.
- **Data Integrity**: Aggregated Country and Sector weights must be derived from the raw weight values to maintain precision.

## Dependencies

- **Feature f021 (WisdomTree Scraper)**: Depends on the provider dispatcher, the `symbol_details` storage extensions, and the UI components for displaying extractor data.
- **Market Data System**: Depends on the ability to store and query `nav` data types.
