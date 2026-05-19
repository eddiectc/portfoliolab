# Feature: allocation

## Description

A dedicated **Allocation** page that shows the investor how their portfolio is distributed across symbols, with the ability to define target weights and track drift. Unlike the Positions page (which shows per-account holdings), the Allocation view aggregates by symbol across accounts, so the investor sees their total exposure to each holding regardless of which account holds it.

The feature supports three modes:
- **Current allocation** — actual % weight per symbol across selected portfolio(s)
- **Target allocation** — user-defined desired % weights per symbol
- **Drift & rebalancing** — comparison of actual vs target, with suggestions to close the gap

Cash is included in the allocation, aggregated to the portfolio base currency using current FX rates. The investor can scope the view to all portfolios or select one or more specific portfolios.

## User Stories

### US-1: View Current Allocation
As an investor, I want to see my current allocation broken down by symbol, so that I understand how my capital is distributed across holdings.

### US-2: Filter Allocation by Portfolio(s)
As an investor with multiple portfolios, I want to view the allocation for all portfolios combined or select one or specific portfolios, so that I can analyze allocation at different levels.

### US-3: Drill Down to Per-Account Breakdown
As an investor, I want to expand a symbol's allocation row to see how it is distributed across individual accounts, so that I understand where each holding lives.

### US-4: Set Target Allocation
As an investor, I want to define my desired target % weight for each symbol, so that I have a reference point for how my portfolio should be allocated.

### US-5: Compare Actual vs Target
As an investor, I want to see my actual allocation side by side with my target allocation, so that I can identify where my portfolio has drifted.

### US-6: Rebalancing Suggestions
As an investor, I want to see suggestions for trades that would bring my portfolio closer to target, so that I know what to buy or sell to rebalance.

## Scenarios

### Scenario: View allocation across all portfolios
**Given** I have multiple portfolios with holdings across several symbols
**When** I navigate to the Allocation page with no portfolio filter selected
**Then** I see a table showing each symbol's allocation as a percentage of total portfolio value
**And** each row shows the symbol name, current market value, and allocation percentage
**And** cash holdings are included as a row, converted to the base currency

### Scenario: View allocation for a single portfolio
**Given** I have multiple portfolios
**When** I select a single portfolio from the portfolio filter
**Then** I see the allocation recalculated for only that portfolio
**And** percentages reflect the share within that portfolio's total value

### Scenario: View allocation for multiple selected portfolios
**Given** I have multiple portfolios
**When** I select two or more portfolios from the portfolio filter
**Then** I see the allocation aggregated across the selected portfolios
**And** percentages reflect the share within the combined total value

### Scenario: Drill down to per-account breakdown
**Given** I am viewing the allocation page and a symbol is held across multiple accounts
**When** I expand the row for that symbol
**Then** I see a breakdown showing the quantity, market value, and percentage contribution from each account
**And** the sub-rows sum to the parent row's total

### Scenario: Create a target allocation
**Given** I am on the Allocation page with my current holdings visible
**When** I enter a target percentage for each symbol
**And** I save the target allocation
**Then** the target percentages are stored
**And** the save is rejected if the percentages do not sum to 100%
**And** an error message indicates the current total and how far it is from 100%

### Scenario: Edit an existing target allocation
**Given** I have previously saved a target allocation
**When** I modify the target percentage for one or more symbols
**And** I save the changes
**Then** the updated target percentages are stored
**And** the save is rejected if the percentages do not sum to 100%
**And** an error message indicates the current total and how far it is from 100%

### Scenario: Compare actual vs target allocation
**Given** I have saved a target allocation
**When** I view the Allocation page
**Then** I see columns for actual %, target %, and the drift (actual minus target)
**And** rows with positive drift are visually distinguished from rows with negative drift

### Scenario: Rebalancing suggestions
**Given** my actual allocation has drifted from my target allocation
**When** I request rebalancing suggestions
**Then** I see a list of suggested trades showing the symbol, direction (buy/sell), share quantity, and estimated dollar value
**And** each suggestion indicates how much it would reduce the drift for that symbol
**And** the suggestions are ordered by the magnitude of drift (largest first)

### Scenario: Rebalancing suggestions with no drift
**Given** my actual allocation matches my target allocation within a 5% tolerance for every symbol
**When** I request rebalancing suggestions
**Then** I see a message indicating the portfolio is balanced and no trades are suggested

### Scenario: View allocation without a saved target
**Given** I am on the Allocation page for a portfolio
**And** I have not saved a target allocation for this portfolio
**When** I view the Allocation page
**Then** I see the actual % column for each symbol
**And** the target % and drift columns are empty or show "—"
**And** a prompt suggests saving a target allocation to enable drift tracking

### Scenario: Reject target allocation with invalid percentages
**Given** I am editing my target allocation
**When** I enter a negative percentage for a symbol
**And** I attempt to save
**Then** the save is rejected
**And** an error message indicates that percentages must be between 0% and 100%

### Scenario: View allocation for a cash-only portfolio
**Given** a portfolio has no symbol holdings, only cash balances
**When** I view the Allocation page for that portfolio
**Then** I see a single row for cash showing 100% allocation
**And** the market value equals the total cash balance in the portfolio base currency

### Scenario: Multi-currency cash aggregation
**Given** a portfolio with base currency USD
**And** account 3 holds $5,000 USD cash and account 5 holds £3,000 GBP cash
**And** the current FX rate GBP/USD is 1.27
**When** I view the Allocation page for that portfolio
**Then** I see a single cash row with value $8,810 USD ($5,000 + £3,000 × 1.27)
**And** the allocation percentage for cash is computed against the total portfolio value including all symbol holdings

### Scenario: Rebalancing suggestions with stale market data
**Given** my actual allocation has drifted from my target allocation
**And** market data for one symbol is unavailable or stale
**When** I request rebalancing suggestions
**Then** suggestions are shown for symbols with available market data
**And** the symbol with unavailable data shows a warning that the suggestion cannot be computed
**And** a "last updated" timestamp is shown for the market data used

### Scenario: Delete a target allocation
**Given** I have previously saved a target allocation for a portfolio
**When** I delete the target allocation
**Then** the target percentages are removed
**And** the Allocation page shows only actual % with empty target % and drift columns
**And** rebalancing suggestions are no longer shown

### Scenario: Target allocation includes symbols not yet held
**Given** I want to set a target for a symbol I don't currently hold
**When** I add a new symbol to my target allocation with a non-zero percentage
**Then** the symbol appears in the target allocation
**And** the rebalancing suggestion for that symbol shows a buy recommendation for the full target amount

### Scenario: Allocation excludes symbols with zero weight in target
**Given** I have a target allocation and a symbol's target weight is 0%
**When** I view the rebalancing suggestions
**Then** the symbol shows a sell recommendation for the full current holding

## Edge Cases

- **Empty portfolio** — No holdings exist; allocation page shows an empty state with a helpful message
- **Single holding** — Only one symbol held (100% allocation); target and drift still work normally
- **Target percentages don't sum to 100%** — Save is rejected; user sees the current total and must adjust before saving. The current total is shown during editing so the user can self-correct.
- **Symbol held in multiple currencies** — Values are converted to the portfolio base currency using current FX rates before calculating percentages
- **FX rate unavailable** — If an FX rate cannot be fetched, the symbol's value is excluded from the total and flagged with a warning
- **Zero or negative portfolio value** — Allocation percentages are undefined; the page shows a warning instead of misleading numbers
- **Rapid price movement** — Allocation percentages reflect the latest cached market data; a "last updated" timestamp is shown
- **Target saved but symbol no longer held** — The symbol still appears in the target allocation with 0% actual and a buy suggestion
- **Cash allocation drift** — Cash weight changes naturally as other positions move; drift reflects this like any other symbol
- **Drift within tolerance** — Symbols with |actual - target| ≤ 5% are considered "balanced"; rebalancing suggestions are not generated for them
- **Symbol in target but not in symbol map** — Target can reference a symbol not yet in the system; rebalancing suggestion shows the full target amount with a note that the symbol is not yet mapped
- **Deleting a symbol from target** — Setting a symbol's target weight to 0% is equivalent to removing it from the target; the symbol still appears in actual allocation if held

## Constraints

- Allocation percentages are based on current market value, not cost basis
- Cash is aggregated to the portfolio base currency using current spot FX rates
- Rebalancing suggestions are informational only — no trading execution
- Target allocation is stored per portfolio; the "all portfolios" view shows each portfolio's target independently
- Target percentages must sum to exactly 100% — saves are rejected otherwise
- Percentages are rounded to 1 decimal place for display (stored precision is higher)
- Drift tolerance is 5% — symbols with |actual - target| ≤ 5% are considered balanced and excluded from rebalancing suggestions
- Rebalancing suggestions show both share quantity (rounded to 2 decimal places) and estimated dollar value
- Cash can have a target weight; it is treated as a regular entry in the target allocation
- Error responses follow the standard format: `{"error": "message", "code": "ERROR_CODE"}`

## Non-Goals

- **Automatic rebalancing** — Suggestions are advisory; the investor executes trades manually
- **Tax-aware rebalancing** — Suggestions do not consider tax implications (capital gains, wash sales)
- **Allocation history** — Tracking how allocation changed over time (future feature)
- **Multiple target allocation profiles** — Only one target set per portfolio (e.g., no "conservative" vs "aggressive" profiles)
- **Rebalancing thresholds/alerts** — No configurable drift threshold or notification; drift is always shown

## Testing Requirements
- **Web rendering regression tests:** Allocation page must have integration tests that verify it renders 200 OK with HTML content, both with data and empty. These catch template errors (undefined fields, nil pointers) before they reach production.
- **Drift tolerance boundary tests:** Table-driven tests for the 5% tolerance boundary (e.g., 4.9% drift → balanced, 5.1% drift → suggestion generated).
- **Percentage validation tests:** Table-driven tests for target percentage validation (negative, >100%, sum ≠ 100%, valid combinations).
- **Multi-currency cash aggregation tests:** Integration tests verifying cash from multiple currencies is correctly converted and aggregated to the portfolio base currency.
- **Rebalancing calculation tests:** Hand-written tests verifying share quantity and dollar value calculations for rebalancing suggestions against known-correct values.

## Dependencies

- **f009_positions** — Open positions data (per-account quantities, symbols)
- **f010_portfolio-performance** — Portfolio base currency and multi-currency FX conversion
- **f011_historical-market-data-caching** — Current market prices for calculating market values
- **f001_portfolio-crud** — Portfolio list for filtering
