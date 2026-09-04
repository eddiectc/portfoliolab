# Feature: Seed Data for New Deployment

## Description

A fresh deployment of Arch Portfolio Lab should not start as an empty, confusing state. This feature seeds a realistic sample portfolio into a new deployment so that a first-time user can immediately explore a fully working demo — positions, multi-year P&L, benchmark comparison, allocation, model portfolio, and symbol detail pages.

Seeding happens **automatically at startup**: if no portfolio exists, the sample data is created; if any portfolio exists, nothing is seeded.

The sample dataset is fixed and fully specified in [Sample Data](#sample-data): four real, popular symbols (two UCITS ETFs — a major app feature the demo must showcase — and two US single stocks), two accounts in different currencies (GBP and USD), eight transactions spanning 2024-07-15 to 2025-07-15 (~2 years of history, including a mid-way rebalance), and one model portfolio. The seed does **not** generate market data: the existing startup market-data fetch covers the seeded symbols. The sample portfolio is clearly identifiable as sample data and can be deleted at any time through the normal delete flow (the sample model portfolio is deleted separately — see Scenarios).

## User Stories

1. **As a user** deploying the app for the first time, I want sample data loaded automatically on first startup, so that I can immediately explore a fully working demo instead of an empty app.

2. **As a user**, I want the sample to use real, popular symbols including UCITS ETFs (VWRP.L, VUTA.L), so that the demo feels authentic and demonstrates UCITS ETF support.

3. **As a user**, I want the sample to cover the full feature set — multiple accounts in different currencies (GBP and USD), target allocations, a model portfolio, benchmark symbols, and symbol details — so that I can exercise every page without entering any data myself.

4. **As a user**, I want the sample transaction history to start on 2024-07-15, include one rebalance, and stay small, so that the demo is representative but easy to read.

5. **As a user**, I want the sample data to be clearly identifiable as sample data, so that I can distinguish it from my own portfolios.

6. **As a user**, I want to delete the sample portfolio and sample model portfolio through the normal delete flows, so that I can remove the demo data when I start entering real data.

7. **As a user**, I want VWRP.L and VUTA.L to be selectable as benchmarks on the performance page, so that I can try benchmark comparison out of the box.

## Sample Data

### Symbols

| Symbol | Type | Role |
|---|---|---|
| VWRP.L | UCITS ETF (global equity, GBP) | sample holding + benchmark |
| VUTA.L | UCITS ETF (US equity, GBP) | sample holding + benchmark |
| GOOG | US single stock | sample holding |
| BRK-B | US single stock | sample holding |

- All four symbols are created with symbol details — **fixed embedded values that ship with the seed dataset** (sector/geographic data) — so the analysis pages render. After seeding, the normal background symbol-details refresh may update them from the provider.
- VWRP.L and VUTA.L are marked as benchmark symbols.
- If a symbol mapping for one of these symbols already exists (e.g., leftover from a previous sample), the seed reuses it rather than duplicating it.

### Portfolio and accounts

- One sample portfolio named **"Sample"** — base currency **GBP**.
- Account **`sample-gbp`** (GBP) — starts with £100,000 cash. The account names are recognizable as sample data.
- Account **`sample-usd`** (USD) — starts with $100,000 cash.

### Transactions

Eight transactions total (4 on 2024-07-15, 4 on 2025-07-15):

| Date | Account | Symbol | Type | Price | Quantity |
|---|---|---|---|---|---|
| 2024-07-15 | sample-gbp | VWRP.L | buy | 105.00 | 666 |
| 2024-07-15 | sample-gbp | VUTA.L | buy | 19.70 | 1522 |
| 2024-07-15 | sample-usd | GOOG | buy | 185.00 | 270 |
| 2024-07-15 | sample-usd | BRK-B | buy | 430.00 | 116 |
| 2025-07-15 | sample-gbp | VWRP.L | sell | 115.00 | 18 |
| 2025-07-15 | sample-gbp | VUTA.L | buy | 19.60 | 108 |
| 2025-07-15 | sample-usd | GOOG | sell | 180.00 | 170 |
| 2025-07-15 | sample-usd | BRK-B | buy | 475.00 | 64 |

The 2025-07-15 GBP trades (sell 18 VWRP.L, buy 108 VUTA.L) form the **rebalance** that restores the 70/30 split. Quantities are whole shares, sized as follows:

- **2024 GBP buys:** 666 × VWRP.L (≈£69,930.00) + 1522 × VUTA.L (≈£29,983.40) leaves a cash remainder of £86.60 — ≈70% / ≈30% of the £100,000.
- **2025 GBP rebalance:** at the 2025-07-15 prices the portfolio is worth ≈£106,507.80; 70% → 648 VWRP.L (sell 18), 30% → 1630 VUTA.L (buy 108), leaving ≈69.97% / ≈29.99% and £39.80 cash.
- **USD account:** the 2024 buys leave $170 cash; the 2025 trades (sell 170 GOOG, buy 64 BRK-B) leave 100 GOOG, 180 BRK-B, and $370 cash.

The 70/30 split is approximate (whole shares); that is by design, not an error.

### Model portfolio

- One model portfolio named **"Sample Model"**.
- Target allocation: **70% VWRP.L / 30% VUTA.L** — mirrors the GBP account strategy.
- The sample portfolio **also** carries a target allocation of **70% VWRP.L / 30% VUTA.L**, so the allocation page (actual vs. target) and the model-vs-real comparison pages render out of the box.

## Scenarios

### Scenario: Automatic seed on startup of an empty deployment
**Given** a deployment with no portfolios (the database may be completely empty, or may contain only shared catalog data such as symbol mappings)
**When** the server starts
**Then** the seed completes before the market-data cache's first refresh pass
**And** the sample dataset from [Sample Data](#sample-data) is created: the "Sample" portfolio (base currency GBP) with its two accounts, all eight transactions, symbol mappings and embedded symbol details for all four symbols, the 70/30 target allocation on the sample portfolio, and the "Sample Model" model portfolio with its 70/30 allocation
**And** VWRP.L and VUTA.L are marked as benchmark symbols
**And** the user can open the app and see a fully working demo (positions, P&L, benchmark comparison, allocation, model portfolio)
**And** market data for the seeded symbols and required currency pairs is fetched by the existing startup market-data fetch

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
**Given** the sample data is present and market data for the seeded symbols is available
**When** the user opens the main pages
**Then** each page renders with sample data — no empty states:
**And** the positions page lists all four symbols across both the GBP and USD accounts
**And** the performance page shows continuous history from 2024-07-15
**And** the allocation page shows the 70/30 target against the actual allocation
**And** the model portfolio list shows "Sample Model" and the model-vs-real comparison renders
**And** the symbol detail pages render with sector/geographic data for all four symbols
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
- **Seed failure:** if any step of the seed fails (e.g., a database error), the whole seed is rolled back (atomic — no partial sample data remains). The server starts normally, the failure is logged, and the seed is retried on the next startup.
- **Whole-share rounding:** the 70/30 split is approximate because quantities are whole shares; this is expected, not an error.

## Constraints

- The sample dataset is **fixed and fully specified** in [Sample Data](#sample-data) — the same data is created on every deployment. No per-run randomization.
- Transaction history starts **2024-07-15** with a rebalance point on **2025-07-15**; the dataset contains exactly **8 transactions**. The fixed dates are canonical (~2 years of history as of writing).
- The sample does **not** include dividend or interest transactions.
- The seed does **not** simulate broker imports — sample data is created directly as application data, not processed through import formats.
- The seed does **not** generate market data; seeded symbols and required currency pairs are covered by the existing startup market-data fetch (f011).
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
- Dividend or interest transactions in the sample data.
- A manual seed trigger of any kind (no CLI option, no web UI) — the only trigger is the automatic seed at startup of a deployment with no portfolio.
- One-click removal of all sample data — full cleanup is two normal deletions (sample portfolio, then sample model portfolio).
- Multi-user or access-control considerations for sample data.

## Dependencies

- f001 (Portfolio CRUD), f002 (Account CRUD), f003 (Symbol Map), f004 (Transaction CRUD) — sample data is created through these existing capabilities.
- f011 (Historical Market Data Caching) — startup fetch covers the seeded symbols and currency pairs.
- f014 (Benchmark Selection) — VWRP.L and VUTA.L marked as benchmark symbols.
- f015 (Symbol Details) — symbol details for the four sample symbols.
- f018 (Allocation), f019 (Model Portfolio) — sample model portfolio and target allocation.
