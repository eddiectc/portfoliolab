# Feature: Dimensional Fund Advisors (DFA) Scraper

**Feature ID**: f023
**Name**: dimensional-scraper
**Date**: 2026-05-29
**Status**: Draft

## Problem

Yahoo Finance data for Dimensional Fund Advisors (DFA) ETFs is often incomplete, lacking full holdings lists and detailed fund characteristics. To provide users with high-fidelity portfolio analysis, the system needs a specialized extractor for Dimensional funds to fetch comprehensive data directly from the provider.

## User Stories

### Story 1: Dimensional data extraction

**As a** portfolio user with Dimensional ETFs,
**I want** the system to extract comprehensive fund data from Dimensional's official sources,
**so that** I can see full holdings, accurate costs, NAV history, and detailed allocations.

**Acceptance Criteria:**

- Given a Dimensional ETF is configured with the Dimensional provider and a valid identifier,
  When the extractor runs,
  Then it extracts the following data:
  - **Fund Overview** (AUM, TER, inception date, etc.) — stored as structured data replacing the existing fund profile.
  - **Full Holdings** (all securities with names and weights) — replaces "Top 10 Holdings".
  - **Country Allocation** — extracted or aggregated from holdings, replacing "Geographic Allocation".
  - **Sector Weightings** — extracted or aggregated from holdings, replacing "Sector Weightings".
  - **Full NAV history** — stored as a distinct data type tagged with the `dimensional` source.
  - **Reference Date** — the "As of" date provided by the source, used as the reference date for all extracted sections.

- Given the extractor encounters a parsing error or the source returns an error,
  When the extraction runs,
  Then the error is logged explicitly and the symbol is marked as failed (no silent fallback to partial data).

- Given the extractor successfully parses some sections but fails on required ones (Holdings, Reference Date),
  When the extraction runs,
  Then the entire extraction is treated as failed (atomic — partial data is not persisted).

### Story 2: Integration with Extractor Framework

**As a** system maintainer,
**I want** the Dimensional provider to integrate seamlessly with the existing extractor framework,
**so that** symbols assigned to the Dimensional provider are routed and processed automatically.

**Acceptance Criteria:**

- Given the Dimensional extractor is registered with the provider dispatcher,
  When the system fetches symbol details for a symbol assigned to the Dimensional provider,
  Then the Dimensional extractor is invoked instead of Yahoo Finance.

- Given a symbol is assigned to the Dimensional provider,
  When the background refresh runs,
  Then both symbol details (via Dimensional) and market data (via Yahoo) are refreshed.

- Given the Dimensional extractor implementation,
  When it is added to the system,
  Then no changes are required to the core routing logic or the database schema established in f021.

## Data Concepts

### Provider Assignment

A symbol is associated with the Dimensional provider via configuration. When present, the system routes symbol details and NAV history fetching to the Dimensional extractor instead of Yahoo Finance. Market data (quotes, prices) continues to use Yahoo Finance.

### Data Mapping

Extractor data replaces or augments existing fields:
- **Fund profile** $\rightarrow$ replaced with Dimensional overview data.
- **Top holdings** $\rightarrow$ replaced with full holdings list.
- **Sector weightings** $\rightarrow$ replaced with Dimensional sector breakdown.
- **Geographic allocations** $\rightarrow$ replaced with Dimensional country allocation.

### NAV History

NAV data is stored using the `nav` data type and `dimensional` source tag, ensuring it is distinguished from Yahoo-sourced market prices in the database and UI.

### Reference Date

The "As of" date found in the provider's data is stored as the reference date for the extraction, ensuring users know the exact date the holdings and allocations were published.

## Edge Cases

- **Identifier Mismatch**: If the provider requires a specific ID (slug) different from the ticker, the provider configuration must support this identifier.
- **Missing Optional Data**: If certain fields (e.g., specific fund characteristics) are missing for a particular fund, the extractor handles this gracefully without failing, provided required sections (Holdings, Reference Date) are present.
- **Empty Required Data**: If the source returns a success response but the holdings list is empty for a fund that should have holdings, this is treated as a parsing error and the extraction fails.
- **API/Web Rate Limiting**: The extractor must respect a delay between requests to avoid being blocked.
- **Invalid Identifier**: If the source returns a 404 or "Not Found" for a given identifier, the symbol is marked as failed with an explicit error.
- **Large Holdings List**: The extractor must process all entries in the holdings table regardless of size.

## Non-Goals

- **Not in scope**: Browser automation (chromedp/playwright). An official public API is available and preferred.
- **Not in scope**: Support for non-ETF products offered by Dimensional.
- **Not in scope**: Automatic mapping of tickers to provider-specific identifiers.

## Constraints

- **Atomicity**: Required sections (Holdings, Reference Date) must be present and non-empty for a successful extraction.
- **Data Integrity**: All weights must be parsed and stored with high precision.

## Dependencies

- **Feature f021 (WisdomTree Scraper)**: Depends on the provider dispatcher, the `symbol_details` storage extensions, and the UI components for displaying extractor data.
- **Market Data System**: Depends on the ability to store and query `nav` data types.
