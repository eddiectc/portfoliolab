# Feature: Market Data Override

**Status:** Draft — pending user review
**Created:** 2026-09-17

## Overview

Market data providers are occasionally wrong. A real incident motivated this feature: on 2025-10-24 Yahoo Finance served corrupted historical bars for four London-listed securities (e.g. XNAQ.L at 58.65, the *USD* NAV of the underlying fund, instead of the ~44.02 GBP LSE close). The upstream provider **still serves the corrupted bars today** — verified by live re-fetch — so neither deleting the bad rows nor re-fetching fixes them: any full refresh re-downloads the provider's full history and re-writes the bad values.

This feature lets the user record a **price override** for a symbol on a specific date. For that symbol and date, the override replaces whatever the provider fetched. Overrides are stored separately from fetched provider data, so they are immune to refreshes: a full refresh re-fetches provider history on top of the existing rows but never touches override rows, and every price read in the app resolves through the override first.

Overrides apply to historical prices (stocks, FX rates, and fund NAVs). They do not apply to the live "current" quote (which has no historical date).

## User Stories

- As a user, I want to correct a specific historical price that the app displays incorrectly (because the provider delivered a bad value), so that the equity curve and P&L reflect the true value.
- As a user, I want my correction to survive market-data refreshes (periodic background refreshes and manual full refreshes), so that I never have to re-apply the fix.
- As a user, I want to see which overrides are currently active, so that I can audit what I have corrected.
- As a user, I want to remove an override, so that the provider's fetched price applies again if the provider's data was corrected later.
- As a user, I want to change an override I previously set, so that I can refine a correction without removing and re-adding it.

## Behavior

### Setting an override

- An override is recorded for one **symbol**, one **date** (calendar date, `YYYY-MM-DD`), one **data type** (`stock`, `fx`, or `nav`), one **price**, and one **currency**.
- Setting an override for a symbol/date/data-type that already has an override **replaces** the previous value (last write wins).
- No symbol existence check is performed: the symbol is stored as given. An override for a symbol with no fetched market data is inert — it simply has no effect on reads.
- The date must be a valid past-or-present calendar date; a future date is a validation error.
- The price must be positive.
- The currency is stored as given; it is not validated against the symbol's currency.
- The data type must be one of `stock`, `fx`, `nav`; beyond that it is stored as given — no cross-check against the symbol's kind, and a mismatched override simply has no effect on reads.
- An override is a *historical* correction: it is never used as the live "current" quote.

### Precedence

- For any historical price read of symbol S, data type T, on date D:
  - if an override exists for (S, D, T), the override's price (and currency) is used;
  - otherwise the price the app resolves from provider rows using its normal multi-source resolution is used — i.e. the same value the app would show without the override.
- The override replaces the provider row for that date **within the read** — it does not delete or modify the fetched row. Both values continue to exist; the override simply wins.
- Dates for which the provider has no row but an override does exist are included in the price history (the override is an additional price point).

### Where overrides take effect

Overrides are honored by all historical price reads, including:

- the **performance equity curve** (daily portfolio valuation),
- **position P&L** computation, including historical FX conversion (an FX override for date D is the rate used for valuations on D and afterwards, subject to the usual forward-fill: a later fetched or overridden rate takes precedence),
- the **symbol detail page** price history and NAV chart.

Overrides do **not** affect:

- the live "current" quote shown for positions or fetched on demand,
- the *staleness detection* logic that decides which date range a refresh must back-fill (an override is not treated as evidence that provider history is missing or present on that date).

### Surviving refreshes

- A **periodic refresh** (gap-fill: fetch from the latest cached date to today) must leave overrides in place. Since gap-fill only re-fetches the *tail* of the series, interior override dates are never re-fetched; tail dates are re-fetched under the provider's own source and do not affect override rows.
- A **manual full refresh** (re-fetch of the provider's full history for every symbol) must leave overrides in place: the refresh upserts provider rows, and provider rows and override rows are distinct records that can coexist for the same symbol and date.
- After any refresh, the app's reads continue to resolve to the override for its date.
- Refreshes never automatically remove, modify, or flag overrides.

### Listing and removing

- Active overrides can be listed, optionally filtered by symbol. Each entry reports: symbol, date, data type, price, currency. Entries are returned in a stable order: symbol, then date, then data type.
- An override can be removed by symbol, date, and data type. After removal, the provider-fetched price for that date applies again (or, if the provider has no row for that date, the date has no price at all).
- Removing an override that does not exist is an error (the caller asked to remove something that isn't there).

## API

Overrides are managed through the REST API (a web UI is out of scope for this feature). The endpoint contract — methods, parameters, and status codes — is defined in [API.md](../../API.md) under **Market Data → Price Overrides**.

Because FX pair symbols contain `/` (e.g. `GBP/USD`), overrides are addressed in the **request body / query parameters**, not in the URL path.

## Scenarios

### Scenario: Correcting a corrupted historical bar
**Given** the app has cached Yahoo historical prices for XNAQ.L, and the cached price for 2025-10-24 is the corrupted value 58.6523 (while the surrounding days are ~44–45)
**When** the user sets an override for XNAQ.L, 2025-10-24, stock, 44.02, GBP
**Then** the price history for XNAQ.L on 2025-10-24 is 44.02
**And** the performance equity curve on 2025-10-24 reflects 44.02 (no spike)
**And** the cached Yahoo row for 2025-10-24 still exists and still reads 58.6523

### Scenario: Override survives a manual full refresh
**Given** the override from the previous scenario is active, and the provider still serves the corrupted 2025-10-24 bar
**When** the user triggers a manual full market-data refresh and it completes
**Then** the override for XNAQ.L on 2025-10-24 is still active and unchanged
**And** the price history for XNAQ.L on 2025-10-24 is still 44.02
**And** the equity curve on 2025-10-24 still reflects 44.02

### Scenario: Override survives periodic background refreshes
**Given** the override from the first scenario is active
**When** one or more periodic background refreshes run
**Then** the override is unchanged and still takes effect on reads

### Scenario: Provider row and override coexist
**Given** Yahoo has a fetched row for XNAQ.L on 2025-10-24 and an override exists for the same symbol and date
**When** the app reads the price for XNAQ.L on 2025-10-24
**Then** the override's value is returned
**And** a read that asks specifically for the Yahoo-fetched value for that symbol and date still returns 58.6523

### Scenario: Override on a date the provider has no row for
**Given** the provider's cached history for symbol S has no row on date D (e.g. a data gap)
**When** the user sets an override for S, date D, stock
**Then** the price history for S includes a point on date D with the override's price
**And** the equity curve uses the override's price on date D (and forward-fills normally from it on later dates)

### Scenario: FX override affects historical conversions
**Given** the app has cached GBP/USD FX history and the user sets an FX override for GBP/USD, 2025-10-24, 1.3250
**When** position P&L is computed for a valuation date on or after 2025-10-24 (before any later rate)
**Then** the 1.3250 override rate is used for the conversion
**And** a valuation date after a later fetched or overridden rate uses that later rate instead (forward-fill semantics are unchanged)

### Scenario: NAV override affects the symbol detail page
**Given** the symbol detail page for a fund shows the NAV history and the NAV on date D is wrong
**When** the user sets a NAV override for the symbol on date D
**Then** the NAV chart on the symbol detail page shows the overridden value on date D

### Scenario: Live quote is unaffected
**Given** an override exists for symbol S on an old historical date D
**When** the app fetches or displays the current quote for S
**Then** the current quote is the provider's live value, not the override

### Scenario: Updating an existing override
**Given** an override for XNAQ.L on 2025-10-24 with price 44.02 is active
**When** the user sets a new override for XNAQ.L on 2025-10-24 with price 44.10
**Then** there is exactly one override for XNAQ.L on 2025-10-24
**And** its price is 44.10

### Scenario: Removing an override restores the fetched price
**Given** an override for XNAQ.L on 2025-10-24 is active and Yahoo has a fetched row for that date
**When** the user removes the override
**Then** the price history for XNAQ.L on 2025-10-24 is the Yahoo-fetched value again
**And** the equity curve is recomputed with the fetched value on that date
**And** the Yahoo row itself was not modified by the removal

### Scenario: Removing a non-existent override is an error
**Given** no override exists for symbol S on date D
**When** the user requests removal of the override for S on date D
**Then** the API reports that no such override exists (404)

### Scenario: Listing overrides
**Given** overrides are active for XNAQ.L (2025-10-24) and GBP/USD (2025-10-24, fx)
**When** the user lists overrides without a filter
**Then** both overrides are returned with their symbol, date, data type, price, and currency
**And** filtering by symbol returns only that symbol's overrides

### Scenario: Validation rejects a non-positive price
**When** the user submits an override with a price that is zero or negative
**Then** the API rejects the request with 400 and a message identifying the price as invalid
**And** no override is created or modified

### Scenario: Validation rejects an invalid date
**When** the user submits an override with a date that does not parse as `YYYY-MM-DD` or that is in the future
**Then** the API rejects the request with 400 and a message identifying the date as invalid
**And** no override is created or modified

### Scenario: Validation rejects an unknown data type
**When** the user submits an override with a data type other than `stock`, `fx`, or `nav`
**Then** the API rejects the request with 400 and a message identifying the data type as invalid
**And** no override is created or modified

## Edge Cases

- **Override for a symbol with no fetched market data**: The override is stored but inert — reads resolve only from provider rows, so the override has no effect.
- **Data type mismatched with the symbol's kind** (e.g. an `fx` override for a stock symbol): Stored as given; no read matches it, so it has no effect.
- **Currency that does not match the symbol's currency**: Stored as given; when the override wins, reads see the override's currency.
- **Provider rows from multiple sources on the override's date**: The override wins over whatever the multi-source resolution would have picked.
- **Override with date = today**: Allowed (past-or-present); never used as the live current quote.
- **Removal with a data type that does not match the stored override**: All three keys must match; otherwise removal reports 404 even if an override exists for the same symbol and date under another data type.

## Constraints

- Overrides are consulted on every historical price read; the lookup must not meaningfully degrade read performance (a single key lookup per symbol/date/data type).
- Prices and FX rates are decimal values (`decimal.Decimal`, never `float64` — project convention).
- Single-user scale: no permissions, no concurrency control, no audit trail beyond the override rows themselves.

## Dependencies

- **f011 Historical Market Data Caching** (done) — market data storage, provider rows, and the historical price read paths the override merges into
- **f010 Portfolio Performance** (done) — equity curve and position P&L, the primary consumers of overridden values
- **f003 Symbol Map** (done) — the symbol catalog (context for symbols; this feature does not enforce an existence check)

## Out of Scope

- **Automatic corruption detection.** Deciding *which* provider bars are wrong (e.g. a spike-and-revert heuristic) is a separate concern; this feature only provides the mechanism for a user to correct them.
- **Web UI.** Overrides are managed through the REST API in this version. A UI for viewing/editing overrides is a candidate follow-up.
- **Overriding the live current quote.** Current quotes (no historical date) are always the provider's live value.
- **Bulk operations** beyond one override per request.

## Notes for Review

- The motivating incident: 2025-10-24 bars for XNAQ.L, QGRP.L, ARCI.L, WOEE.L. Estimated true LSE closes (provider USD NAV × USD/GBP ≈ 0.7505, consistent with Friday↔Monday interpolation to ~0.1%): XNAQ.L 44.02, QGRP.L 28.33, ARCI.L 8.29, WOEE.L 4.73. After implementation, these four overrides are the first real data entry.
- Staleness/gap-fill detection is deliberately left to consider provider rows only (see Behavior → Surviving refreshes); an alternate design (override extends the "latest date") was rejected as it would cause unnecessary back-fills.
- Whether `GET` of a specific provider-sourced row should remain possible alongside the precedence rule: yes — scenario "Provider row and override coexist" makes explicit that reads keyed on a specific source keep returning the raw fetched value.
