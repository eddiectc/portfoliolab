# Feature: Symbol Details — Geographic Data

## Description

Extend the existing symbol details system (f015) to capture geographic/country breakdown data from Yahoo Finance. Currently, symbol details stores top holdings, sector weightings, aggregate positions, fund profile, and equity valuation — but not the geographic allocation that shows which countries/regions an ETF is exposed to.

This is a prerequisite for portfolio-level geographic allocation analysis (f017), which aggregates ETF geographic data through look-through to show the investor's true country/region exposure.

## User Stories

- As an investor with ETF holdings, I want geographic data to be captured for each ETF so that I can later see my portfolio's country/region exposure.
- As an investor, I want geographic data to be refreshed alongside existing symbol details so that the data stays current without a separate process.

## Scenarios

### Scenario: Fetch geographic data for an ETF on symbol creation
**Given** I create a new symbol mapping for `VWRP.L` (an ETF)
**When** the symbol details are fetched from Yahoo Finance
**Then** geographic/country breakdown data is fetched and stored alongside the existing symbol details
**And** the data includes: country name and allocation percentage for each country
**And** if geographic data is unavailable from Yahoo, the field is stored as null/empty without blocking the rest of the details fetch

### Scenario: Fetch geographic data for a generic symbol (non-ETF)
**Given** I create a new symbol mapping for `AAPL` (an individual stock)
**When** the symbol details are fetched from Yahoo Finance
**Then** the country of incorporation/headquarters is captured (from assetProfile or equivalent)
**And** for individual stocks, a single country is stored rather than a breakdown

### Scenario: Geographic data included in cached symbol details
**Given** symbol `VWRP.L` has cached symbol details including geographic data
**When** I request the symbol details via the API
**Then** the response includes the geographic breakdown alongside existing fields (top holdings, sectors, etc.)
**And** the geographic data is structured as an array of {country, percent} entries

### Scenario: Geographic data included in web UI symbol details page
**Given** symbol `VWRP.L` has cached symbol details including geographic data
**When** I navigate to the symbol details page for `VWRP.L`
**Then** I see a geographic/country allocation section showing the breakdown by country
**And** the section is sorted by weight descending
**And** for individual stocks, the country is shown as a single value (not a breakdown)

### Scenario: Geographic data fetched during background refresh
**Given** symbol `VWRP.L` has cached symbol details that are stale (>7 days)
**When** the background refresh job runs
**Then** geographic data is re-fetched alongside other symbol details
**And** the cached geographic data is updated
**And** if the geographic fetch fails but other details succeed, the other details are still updated and the geographic data is preserved from the previous fetch

### Scenario: Geographic data unavailable from Yahoo
**Given** I create a new symbol mapping for an ETF that Yahoo has no geographic data for
**When** the symbol details are fetched
**Then** the geographic field is stored as null/empty
**And** the rest of the symbol details (holdings, sectors, etc.) are still stored
**And** the symbol details page shows "no geographic data available" for that symbol

### Scenario: Symbol details API response includes geographic data
**Given** symbol `VWRP.L` has cached symbol details with geographic data
**When** I call `GET /api/symbols/{id}`
**Then** the response includes a `geographic_allocations` field in the `symbol_details` object
**And** the field is an array of objects with `country` and `percent` fields, sorted by percent descending
**And** if no geographic data is available, the field is null

## Edge Cases

- **Yahoo returns no geographic data**: Some ETFs (especially smaller or non-US funds) may have no country breakdown — stored as null
- **Partial geographic data**: Yahoo returns geographic data but it doesn't sum to 100% — stored as-is, "Other" or "Unknown" bucket not invented
- **Country name inconsistencies**: Yahoo may use different country names (e.g., "United States" vs "USA") — stored as-is from Yahoo; normalization can be done at display time
- **International symbols in holdings**: Geographic data is at the ETF level, not per-holding — individual holding countries are not resolved
- **Currency/region mismatch**: An ETF may be domiciled in one country but invested in another — geographic data reflects investment exposure, not domicile
- **Stale geographic data**: Like other symbol details, geographic data is considered stale after 7 days and refreshed by the background job
- **New ETF with no historical data**: Newly launched ETFs may have no geographic data from Yahoo — stored as null until the provider populates it
- **Existing symbols**: Symbols already in the database before this feature is deployed will receive geographic data on their next scheduled refresh; no automatic backfill is performed

## Constraints

- **Data source**: Yahoo Finance — geographic data obtained through the existing market data provider integration
- **ETF geographic data**: Country/region breakdown (array of country and allocation percentage)
- **Individual stock country**: Single country value from the symbol's profile data
- **Storage**: Added to the existing symbol details storage alongside other detail types
- **Fetch**: Integrated into the existing symbol details fetch flow — same creation hook and background refresh
- **Non-blocking**: Geographic fetch failure does not block symbol creation or other details from being stored
- **Read-only**: Geographic data is fetched from the market data provider, not user-editable

## Non-Goals

- Geographic data for bond ETFs or fixed-income instruments (equity-focused)
- Region aggregation (e.g., grouping countries into "Developed Markets", "Emerging Markets") — stored as individual countries, aggregation done at display/analysis time
- Historical geographic snapshots (point-in-time composition)
- Manual override of geographic data
- Support for market data providers other than Yahoo Finance

## Dependencies

- **f015 Symbol Details** (done) — extends the existing fetcher, schema, service, and UI; reuses all infrastructure
