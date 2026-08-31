# Feature: iM Global Partner (iMGP) Scraper

**Feature ID**: f024
**Name**: imgp-scraper
**Date**: 2026-05-30
**Status**: Draft

## Problem

Yahoo Finance data for iM Global Partner (iMGP) funds is often incomplete or insufficient, particularly for alternative and multi-asset strategies. iMGP publishes monthly factsheet PDFs for each fund share class containing comprehensive data — fund facts, risk measures, and portfolio breakdown (asset class allocation, equity derivatives by region, currency derivatives allocation). There is no way to extract and use this data in the current system.

## User Stories

### Story 1: iMGP data extraction

**As a** portfolio user with iMGP funds,
**I want** the system to extract comprehensive fund data from iMGP factsheet PDFs,
**so that** I can see accurate fund facts, performance, risk measures, and portfolio composition.

**Acceptance Criteria:**

- Given a symbol has its `data_source_url` set to an iMGP fund page URL (e.g. `https://www.imgp.com/fund/LU2951555585` or the US variant `https://www.imgp.com/us/fund/US53700T8273-imgp-dbi-managed-futures-strategy-etf`),
  When the extractor runs,
  Then it extracts the following data:
  - **Fund Facts** (AUM, inception date, ISIN, share class name, fee) — stored as structured data replacing the existing fund profile. The fee is "Management Fees" on EU share class factsheets and "Gross Expense Ratio" on US share class factsheets (both stored as the annual expense ratio fraction).
  - **Risk Measures** (volatility, Sharpe ratio, information ratio, beta, correlation, tracking error) — stored as fund characteristics.
  - **Portfolio Composition** — replaces "Top Holdings" and "Sector Weightings" for funds where this data is the primary portfolio breakdown. Three tiers:
  - **Asset Class Allocation** (equities, bonds, gold, oil, cash & others) — exposure relative to AUM; values can be negative (short positions) and do not sum to 100%.
  - **Equity Derivatives by Region** (North America, Europe, Asia, Emerging Countries, etc.) — composition within the equity sleeve; values sum to 100% but individual values can be negative.
  - **Currency Derivatives Allocation** (USD, EUR, JPY, etc.) — composition within the currency sleeve; values sum to 100% but individual values can be negative.
  - **Reference Date** — the "as of" date embedded in the factsheet (e.g. "April 30, 2026"), used as the reference date for all extracted sections.

- Given the extractor encounters a parsing error or the factsheet is unavailable,
  When the extraction runs,
  Then the error is logged explicitly and the symbol is marked as failed (no silent fallback to partial data).

- Given the extractor successfully parses some sections but fails on required ones (Fund Facts, Reference Date),
  When the extraction runs,
  Then the entire extraction is treated as failed (atomic — partial data is not persisted).

- Given the sample factsheet PDF for `LU2951555585` (stored in `features/f024_imgp-scraper/samples/`),
  When the extractor runs against it,
  Then it successfully extracts Fund Facts, Reference Date, and all present portfolio sections with values matching the PDF.

### Story 2: Integration with Extractor Framework

**As a** system maintainer,
**I want** the iMGP extractor to register with the existing dispatcher,
**so that** symbols with an iMGP `data_source_url` are routed and processed automatically.

**Acceptance Criteria:**

- Given the iMGP extractor is registered with the dispatcher and implements `URLMatcher` for iMGP domains,
  When the system fetches symbol details for a symbol whose `data_source_url` points to an iMGP URL,
  Then the dispatcher routes the request to the iMGP extractor instead of Yahoo Finance.

- Given a symbol has its `data_source_url` set to an iMGP factsheet URL,
  When the background refresh runs,
  Then both symbol details (via iMGP extractor) and market data (via Yahoo Finance) are refreshed.

- Given the iMGP extractor is registered,
  When it is added to the system,
  Then no changes are required to the core routing logic (dispatcher, registry, `FindByURL`).
  Storage extensions are required for iMGP-specific data types (risk measures, asset class allocation, equity derivatives by region, currency derivatives allocation).

### Story 3: Symbol details page — extractor data display

**As a** portfolio user,
**I want** the symbol details page to show all extracted iMGP data with its reference date,
**so that** I can see the full picture of what the fund holds and how it is positioned.

**Acceptance Criteria:**

- Given symbol details were fetched from the iMGP extractor,
  When I view the symbol details page,
  Then I see the following sections (replacing/augmenting existing Yahoo-sourced sections):
  - **Fund Facts** — AUM, inception date, fees, with reference date.
  - **Risk Measures** — volatility, Sharpe ratio, and other risk metrics, with reference date.
  - **Asset Class Allocation** — exposure relative to AUM (equities, bonds, gold, oil, cash), values can be negative, with reference date.
  - **Equity Derivatives by Region** — composition within the equity sleeve, with reference date.
  - **Currency Derivatives Allocation** — composition within the currency sleeve, with reference date.

- Given the Asset Class Allocation includes negative values (short positions) and does not sum to 100%,
  When I view the symbol details page,
  Then the section is rendered as a list/table, not a pie chart or stacked bar.

- Given extractor data includes a reference date (the "as of" date embedded in the factsheet PDF — **not** the date the system fetched it),
  When I view the symbol details page,
  Then each extractor-sourced section shows its reference date.

## Data Concepts

### Source URL Configuration

The user sets the `data_source_url` on a symbol to the iMGP factsheet page URL (e.g. `https://www.imgp.com/fund/LU2951555585`). The ISIN is embedded in the URL path. The dispatcher's `FindByURL` matches the URL against the iMGP extractor's `URLMatcher`. This reuses the existing mechanism from f021 — no new configuration UI or storage is needed. Market data (quotes, prices) continues to use Yahoo Finance.

### Fund identity (page variants)

iMGP has two fund page variants with different URL layouts and factsheet layouts:

- **EU share classes** — `https://www.imgp.com/fund/{ISIN}` (e.g. `.../fund/LU2951555585`). Day-first dates, the factsheet PDF includes the ISIN, share class name and ongoing charges.
- **US share classes** — `https://www.imgp.com/us/fund/{ISIN}-{slug}` (e.g. `.../us/fund/US53700T8273-imgp-dbi-managed-futures-strategy-etf`). Month-first dates; the factsheet PDF lists the CUSIP instead of the ISIN and omits the share class name and ongoing charges.

Fund identity (ISIN + fund name) comes from the structured `const fund = {...}` JSON embedded in the page HTML (present on both variants). The URL path is the fallback: `{ISIN}` directly for EU, `{ISIN}-{slug}` prefix for US. When the factsheet PDF omits the ISIN (US variant), the extractor back-fills it from the resolved fund identity. If no ISIN can be found in either source, the extraction fails with an explicit error.

Multiple share classes of the same fund have different ISINs and different URLs.

### Data Mapping

Extractor data replaces or augments existing fields:
- **Fund profile** &rarr; replaced with iMGP fund facts (AUM, inception date, fees, ISIN, share class name). Extended with iMGP-specific fields beyond the existing fund profile structure.
- **Top holdings** &rarr; replaced with asset class allocation (exposure relative to AUM; values can be negative, do not sum to 100%). Data shape differs from existing holdings (named categories instead of security symbols).
- **Sector weightings** &rarr; replaced with equity derivatives by region (composition within equity sleeve; sums to 100%).
- **Fund characteristics** &rarr; replaced with risk measures (volatility, Sharpe ratio, information ratio, beta, correlation, tracking error). Data shape differs from existing equity valuation fields.
- **New data** — currency derivatives allocation has no existing equivalent and requires new storage.

Storage extensions are required for the new data types (risk measures, asset class allocation, equity derivatives by region, currency derivatives allocation).

### Portfolio Composition Structure

iMGP factsheets present portfolio data in three tiers with different semantics:

1. **Asset Class Allocation** — exposure relative to AUM (e.g. equities 20.3%, bonds -25.7%, gold 15.4%). Values can be **negative** (short futures) and do **not** sum to 100%. This represents the fund's directional views expressed through derivatives. The UI must not render this as a pie chart.

2. **Equity Derivatives by Region** — composition within the equity sleeve (e.g. North America 68.3%, Emerging Countries 15.8%). Values sum to ~100%. Individual values can be negative if the fund has short regional exposure.

3. **Currency Derivatives Allocation** — composition within the currency sleeve (e.g. JPY -89.4%, USD 4.8%). Values sum to ~100%. Individual values can be negative (short currency positions).

Not all fund types include all three tiers. Equity-focused funds may show traditional top holdings instead. The extractor handles whatever sections are present on the factsheet.

### Reference Date

The "as of" date found in the factsheet PDF (e.g. "Fact Sheet &ndash; April 30, 2026") is stored as the reference date for the extraction. This is the date the provider published the data, distinct from when the system fetched it.

### Factsheet Publication Cadence

Factsheets are published monthly. The same URL is reused for each month's edition (the PDF is replaced on the server). The extractor fetches the latest available factsheet on each run.

## Edge Cases

- **Factsheet URL returns 404 or unrelated content**: If the configured `data_source_url` returns a 404 or non-PDF content (e.g. HTML error page), the symbol is marked as failed with an explicit error.
- **Missing optional data**: If certain sections (e.g. risk measures for very new funds, or currency breakdown for non-multi-asset funds) are absent from the factsheet, the extractor handles this gracefully without failing, provided required sections (Fund Facts, Reference Date) are present.
- **Empty required data**: If the factsheet loads but required sections are missing or empty, the extraction fails with an explicit error.
- **PDF format changes**: If the factsheet layout changes and parsing fails, the extraction is treated as a parsing error, logged with details, and the symbol is marked as failed.
- **Different fund types have different portfolio sections**: Equity funds may show traditional top holdings and sector weightings, while alternative/CTA funds show derivatives allocation by asset class, region, and currency. The extractor handles whatever sections are present on the factsheet.
- **Rate limiting**: The extractor respects a delay between requests to avoid being blocked.
- **Cloudflare/bot detection**: If the request is blocked, the error is explicit and existing cached data is preserved.
- **Multiple share classes**: A single fund may have multiple share classes (e.g. A, I, C shares), each with a different ISIN and factsheet URL. Each is configured as a separate symbol with its own `data_source_url`.
- **PDF language variants**: iMGP factsheets are published in multiple languages. The `data_source_url` determines which language edition is fetched (e.g. the URL path or query parameter selects English). The extractor parses whatever language the URL resolves to.
- **US share class pages**: US pages use a different URL layout (`/us/fund/{ISIN}-{slug}`) and their factsheets use month-first dates (e.g. `05/07/2019` = 7 May 2019), list the CUSIP instead of the ISIN, label the fee "Gross Expense Ratio", and omit the share class name and ongoing charges. The extractor selects the date layout from the URL region, accepts either fee label, and back-fills the ISIN from the page's structured data. Sample factsheet: `features/f024_imgp-scraper/samples/DBMF_FACTSHEETS_EN.pdf`.
- **Corrupted or incomplete PDF download**: If the downloaded PDF is truncated or corrupted, the parsing fails with an explicit error and the symbol is marked as failed.

## Non-Goals

- **Not in scope**: Browser automation (chromedp/playwright). The factsheet is a direct PDF download.
- **Not in scope**: Support for non-fund products offered by iMGP.
- **Not in scope**: Automatic mapping of tickers to ISINs (the `data_source_url` is set manually by the user).
- **Not in scope**: NAV history extraction (NAV/price data continues to come from Yahoo Finance).
- **Not in scope**: Historical factsheet archive (only the latest factsheet at the URL is fetched).

## Constraints

- **Atomicity**: Required sections (Fund Facts including reference date) must be present for a successful extraction. Optional sections (risk measures, asset class allocation, regional breakdown, currency breakdown) may be absent without causing failure.
- **PDF parsing**: The extractor reads the factsheet as a PDF document and extracts structured data from it.
- **Rate limiting**: 1-2 second delay between requests to the same provider.
- **Error handling**: Explicit errors, no silent fallbacks. If extraction fails, the symbol is marked as failed and existing cached data is preserved.
- **Source URL is set manually**: The `data_source_url` on the symbol is set by the user to the iMGP factsheet page URL, not auto-detected.
- **ISIN is in the URL**: The ISIN is embedded in the `data_source_url` path; it is not stored separately.

## Dependencies

- **Feature f021 (WisdomTree Scraper)**: Depends on the extractor dispatcher (`FindByURL`), the `data_source_url` column on `symbol_mappings`, the `symbol_details` table, and the UI components for displaying extractor data.
- **Market Data System**: Depends on the ability to store and query symbol details from extractor sources.
- **Feature f022 (DWS Scraper) / f023 (Dimensional Scraper)**: Follows the same registration pattern (implement `Extractor` + `URLMatcher`, register with dispatcher).
