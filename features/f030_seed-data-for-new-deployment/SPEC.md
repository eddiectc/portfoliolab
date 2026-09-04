# Feature: Seed data for new deployment

**Status:** Approved
**Created:** 2026-07-21
**Revised:** 2026-09-04 (see [Changelog](#changelog))

## Overview

A brand-new deployment of Arch Portfolio Lab starts empty: no portfolios, accounts, transactions, or history — the dashboard, positions, performance, and allocation pages are all empty states.

This feature seeds a small, realistic sample dataset on the automatic first startup (a server start when no portfolio exists yet), so a new user immediately sees a working demo: a "Sample" portfolio with two accounts, about two years of deposit, buy, and sell history across four real symbols, a 70/30 model allocation, and benchmark symbols.

The sample data uses real symbols (VWRP.L, VUTA.L, GOOG, BRK-B) with representative prices and quantities. Symbol details and live prices are **not** embedded in the seed: the seed creates the symbol mappings (including the two ETFs' provider page URLs), and the existing startup background refresh fetches details and market data exactly as it does for user-created symbols.

## User Stories

- As a new user, when I start the app for the first time I want to see sample data in the portfolio, positions, performance, and allocation pages, so that I can see what the app looks like when it's in use.
- As a new user, I want the sample data to clearly be a sample (a recognizable name), so that I don't confuse it with my real portfolio.
- As a user of an existing deployment, I want starting the app to never create, modify, or delete my data, so that the sample data never leaks into my real data.
- As a new user, I want the sample portfolio to have a model allocation and benchmark symbols, so that the allocation and benchmark comparison features are immediately demoable.
- As a new user, I want to be able to delete the sample portfolio and start from a completely clean state, so that the sample data is not a trap.

## Sample Data

### Symbols

| Symbol | Type | Role |
|---|---|---|
| VWRP.L | UCITS ETF (global equity, GBP) | sample holding + benchmark |
| VUTA.L | UCITS ETF (USD investment-grade bond, GBP) | sample holding + benchmark |
| GOOG | US single stock | sample holding |
| BRK-B | US single stock | sample holding |

The two ETFs carry their provider page in the symbol mapping (`data_source_url`):

| Symbol | Data source URL |
|--------|-----------------|
| VWRP.L | https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-accumulating |
| VUTA.L | https://www.vanguardinvestor.co.uk/investments/vanguard-usd-treasury-bond-ucits-etf-usd-accumulating |

- VWRP.L and VUTA.L are marked as benchmark symbols.
- **Symbol details are not embedded in the seed.** The seed writes the symbol mappings only. Details are fetched by the existing startup background refresh: the Vanguard extractor (f025) for the two ETFs (full fund profile plus region and sector breakdowns), Yahoo (f015) for GOOG/BRK-B (sector and country). A symbol with no details row is always considered stale, so a failed fetch is retried automatically on the next refresh cycle.
- If a symbol mapping for one of these symbols already exists (e.g., created by the user), the seed reuses it and never modifies it.

### Portfolio and accounts

- One sample portfolio named **"Sample"** — base currency **GBP**.
- Account **"Main Investment"** — trades in GBP.
- Account **"US Brokerage"** — trades in USD.
- Each account's opening cash is a first-class deposit transaction (below), so cash balances are derived from transaction history exactly like a user's accounts.

### Transactions

Ten transactions total: 2 initial deposits, 6 on 2024-07-01, 2 on 2025-07-15, 2 on 2026-03-10.

| Date | Account | Symbol | Type | Price | Quantity | Net cash |
|---|---|---|---|---|---|---|
| 2024-07-01 | Main Investment | $CASH-GBP | deposit | 1.00 | 100,000.00 | +100,000.00 |
| 2024-07-01 | Main Investment | VWRP.L | buy | 105.00 | 666 | −69,930.00 |
| 2024-07-01 | Main Investment | VUTA.L | buy | 19.70 | 1,522 | −29,983.40 |
| 2024-07-01 | US Brokerage | $CASH-USD | deposit | 1.00 | 30,000.00 | +30,000.00 |
| 2024-07-01 | US Brokerage | GOOG | buy | 148.50 | 40 | −5,940.00 |
| 2024-07-01 | US Brokerage | BRK-B | buy | 412.30 | 12 | −4,947.60 |
| 2025-07-15 | Main Investment | VWRP.L | sell | 144.60 | 18 | +2,602.80 |
| 2025-07-15 | Main Investment | VUTA.L | buy | 19.65 | 108 | −2,122.20 |
| 2026-03-10 | US Brokerage | GOOG | buy | 152.30 | 4 | −609.20 |
| 2026-03-10 | US Brokerage | BRK-B | buy | 418.90 | 1 | −418.90 |

- The 2025-07-15 trades (sell 18 VWRP.L, buy 108 VUTA.L) form the **rebalance** that moves the GBP holdings back toward the 70/30 split.
- Resulting positions: 648 VWRP.L, 1,630 VUTA.L, 44 GOOG, 13 BRK-B.
- Quantities are whole shares; the 70/30 split is approximate (whole-share rounding), by design and not an error.

### Model portfolio

- One model portfolio named **"Sample Model"**.
- Target allocation: **70% VWRP.L (global equity) / 30% VUTA.L (USD bond)** — a classic equity/bond model reflecting the GBP account strategy.
- The sample portfolio **also** carries a target allocation of **70% VWRP.L / 30% VUTA.L**, so the allocation page (actual vs. target) and the model-vs-real comparison pages render out of the box.

## Scenarios

### Scenario: Automatic seed on startup of an empty deployment
**Given** a deployment with no portfolios (the database may be completely empty, or may contain only shared catalog data such as symbol mappings)
**When** the server starts
**Then** the seed completes before the market-data cache's first refresh pass
**And** the sample dataset from [Sample Data](#sample-data) is created: the "Sample" portfolio (base currency GBP) with its two accounts, all ten transactions, the symbol mappings for all four symbols (with the benchmark flags and the two ETFs' Vanguard source URLs), the 70/30 target allocation on the sample portfolio, and the "Sample Model" model portfolio with its 70/30 allocation
**And** VWRP.L and VUTA.L are marked as benchmark symbols
**And** the user can open the app and see a fully working demo (positions, P&L, benchmark comparison, allocation, model portfolio)
**And** market data for the seeded symbols and required currency pairs is fetched by the existing startup market-data fetch
**And** symbol details for the four symbols are fetched by the existing startup background refresh (Vanguard extractor for the two ETFs, Yahoo for the two stocks)

### Scenario: No re-seeding when a portfolio exists
**Given** a deployment with at least one portfolio (seeded sample data, user data, or both)
**When** the server starts
**Then** no sample data is created or modified
**And** no existing data is ever modified or deleted by seeding

### Scenario: Benchmark symbols are selectable
**Given** the sample data is present
**When** the user opens the benchmark selector on the performance page
**Then** VWRP.L and VUTA.L are listed as available benchmarks
**And** selecting one renders the benchmark comparison (chart overlay and stats) against it

### Scenario: Demo is fully functional
**Given** the sample data is present, market data for the seeded symbols is available, and the startup background refresh has fetched symbol details
**When** the user opens the main pages
**Then** each page renders with sample data — no empty states:
**And** the positions page lists all four symbols across both the GBP and USD accounts
**And** the performance page shows continuous history from 2024-07-01
**And** the allocation page shows the 70/30 target against the actual allocation
**And** the model portfolio list shows "Sample Model" and the model-vs-real comparison renders
**And** the symbol details pages render with provider-sourced data — region/sector breakdowns from the Vanguard extractor for the two ETFs, sector/country from Yahoo for the two stocks
**And** multi-currency values display correctly across the GBP and USD accounts, including cross-currency conversion into the portfolio base currency (GBP)

### Scenario: Sample portfolio can be deleted
**Given** the sample data is present
**When** the user deletes the sample portfolio through the normal delete flow
**Then** the portfolio, its accounts, its transactions, and its target allocation are removed
**And** the symbol mappings (and their benchmark flags) are shared catalog entries and remain in place
**And** the "Sample Model" model portfolio is **not** removed by the portfolio deletion — it is a top-level entity and is deleted separately through the model portfolio delete flow
**And** the app continues to function normally for any remaining data

## Edge Cases

- **Startup with the sample portfolio already present:** the automatic seed is a no-op — no duplicates, no error.
- **User deletes the sample portfolio but keeps their own data:** subsequent startups do not re-seed, because at least one portfolio exists.
- **User deletes all portfolios, including the sample:** the next startup finds no portfolio, so the sample data is seeded again (documented, predictable behavior). If the "Sample Model" model portfolio was not deleted, the seed reuses it (its name is unique) rather than failing or creating a duplicate.
- **Symbol overlap between sample and user data:** the sample uses real tickers (GOOG, BRK-B, VWRP.L, VUTA.L) that a user may also trade. Seeding must not break or modify existing symbol mappings — shared symbols are simply referenced by both.
- **Market data unavailable (offline or provider failure):** seeding itself still succeeds; pages that depend on prices degrade exactly as they do today when market data is missing (existing behavior, no new handling).
- **Symbol details fetch failure:** if the provider page or Yahoo is unreachable when the startup background refresh runs, seeding is unaffected (details are not part of the seed write); the affected symbols simply have no details row yet, remain stale, and are retried automatically on the next refresh cycle.
- **Seed failure:** if any step of the seed fails (e.g., a database error), the whole seed is rolled back (atomic — no partial sample data remains). The server starts normally, the failure is logged, and the seed is retried on the next startup.
- **Whole-share rounding:** the 70/30 split is approximate because quantities are whole shares; this is expected, not an error.

## Constraints

- The sample dataset is **fixed and fully specified** in [Sample Data](#sample-data) — the same data is created on every deployment. No per-run randomization.
- Transaction history starts **2024-07-01**, with a rebalance point on **2025-07-15** and final purchases on **2026-03-10**; the dataset contains exactly **10 transactions** (2 deposits + 8 trades). The fixed dates are canonical (~2 years of history as of writing).
- The sample does **not** include dividend or interest transactions.
- The seed does **not** simulate broker imports — sample data is created directly as application data, not processed through import formats.
- The seed does **not** generate market data and does **not** fetch or embed symbol details; both are covered by the existing startup fetches (market data: f011; symbol details: the background refresh, f015/f025).
- The seed completes **before** the market-data cache's first refresh pass at startup, so that startup fetch covers the seeded symbols and currency pairs.
- The seed is **atomic**: it is applied as a single database transaction. If any step fails, the entire seed is rolled back (no partial sample data), the server still starts, and the failure is logged.
- Repeated startups while any portfolio exists are **no-ops**. After all portfolios are deleted, re-seeding reuses surviving shared entries (symbol mappings, model portfolio) instead of duplicating them, and never modifies existing data.
- The sample portfolio, model portfolio, and accounts are identifiable by name and deletable through the normal delete flows — no seeded record is protected from deletion.
- The automatic seed applies **only** when no portfolio exists; it never runs against a database that contains one or more portfolios. Other data (symbol mappings, cached market data) may be present and is left untouched.

## Non-Goals

- Editing, customizing, or partially selecting the sample data (no option to "pick which samples to include").
- Randomizing sample data per run.
- Seeding from an export or backup of a user's real data (data migration is a separate concern).
- Generating or backfilling market data as part of the seed.
- Fetching or embedding symbol details as part of the seed — details are the responsibility of the existing startup background refresh.
- Dividend or interest transactions in the sample data.
- A manual seed trigger of any kind (no CLI option, no web UI) — the only trigger is the automatic seed at startup of a deployment with no portfolio.
- One-click removal of all sample data — full cleanup is two normal deletions (sample portfolio, then sample model portfolio).
- Multi-user or access-control considerations for sample data.

## Dependencies

- f001 (Portfolio CRUD), f002 (Account CRUD), f003 (Symbol Map), f004 (Transaction CRUD) — sample data is created through these existing capabilities.
- f011 (Historical Market Data Caching) — startup fetch covers the seeded symbols and currency pairs.
- f014 (Benchmark Selection) — VWRP.L and VUTA.L marked as benchmark symbols.
- f015 (Symbol Details), f025 (Vanguard Scraper) — details for the four sample symbols are fetched by the existing startup background refresh (Vanguard extractor for the two ETFs, Yahoo for the two stocks).
- f018 (Allocation), f019 (Model Portfolio) — sample model portfolio and target allocation.

## Changelog

- 2026-09-04 — Symbol details are no longer embedded in the seed dataset. The seed writes symbol mappings only (internal + market data symbols, `is_benchmark` flags, and the Vanguard `data_source_url` for VWRP.L/VUTA.L); details and live prices are produced by the existing startup background refresh, which automatically retries missing or stale rows. VUTA.L description corrected from "US equity" to "USD investment-grade bond" (confirmed against the provider page).
- 2026-07-21 — Initial deposits are first-class `type='deposit'` transactions (one per account); the dataset totals 10 transactions (2 deposits + 8 trades); accounts renamed to "Main Investment"/"US Brokerage"; dataset values confirmed against the demo deployment.
