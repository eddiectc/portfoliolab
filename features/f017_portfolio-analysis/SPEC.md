# Feature: Portfolio Analysis

## Description

A dedicated **Analysis** page that gives the investor a risk and composition view of their portfolio — complementary to the Performance page (which answers "how much did I make?"). This feature answers "what am I exposed to and what could go wrong?"

It provides five analytical lenses:
- **ETF Overlap** — see if ETF holdings duplicate each other
- **Correlation Matrix** — how holdings move together
- **Sector & Geographic Allocation** — portfolio composition via ETF look-through
- **Stress Testing** — how the portfolio fares in historical crisis scenarios
- **Factor Exposure** — style and risk characteristics of the portfolio

## User Stories

- As an investor with multiple ETFs, I want to see which ETFs hold overlapping stocks, so I know if I'm over-concentrated in certain names.
- As an investor, I want to see how my holdings correlate with each other, so I can assess whether my diversification is real or illusory.
- As an investor, I want to see my portfolio's sector and geographic allocation through ETF look-through, so I understand my true exposure beyond the ETF labels.
- As an investor, I want to stress-test my portfolio against historical market crises, so I understand potential downside in worst-case scenarios.
- As an investor, I want to see my portfolio's factor exposure (value vs growth, size, concentration), so I understand the style and risk profile of my holdings.

## Scenarios

### Scenario: Fetch portfolio analysis via API
**Given** a portfolio with open ETF and stock positions and cached symbol details
**When** the API endpoint for portfolio analysis is called
**Then** the response contains a single JSON object with all analysis sections:
  - `overlap` — ETF pairwise overlap matrix and top concentrated stocks
  - `correlation` — correlation matrix of holdings for the requested period
  - `sector_allocation` — sector breakdown with weighted percentages
  - `geographic_allocation` — geographic/region breakdown with weighted percentages
  - `stress_test` — estimated portfolio impact for each predefined scenario
  - `factor_exposure` — value/growth tilt, size, concentration metrics
**And** each section is independently nullable (e.g., `overlap: null` if no ETFs)
**And** the response includes metadata: computed_at timestamp, portfolio_id, and data_freshness warnings

### Scenario: Fetch portfolio analysis via API (single section)
**Given** a client only needs one analysis section (e.g., a mobile chart showing sector allocation)
**When** the API endpoint is called with a section filter parameter
**Then** only the requested section is computed and returned
**And** the response structure is the same (other sections are null/omitted)

### Scenario: Fetch portfolio analysis via API (no positions)
**Given** a portfolio with no open positions
**When** the API endpoint is called
**Then** the response returns all sections as null/empty
**And** a message field explains that analysis requires open positions

### Scenario: Fetch portfolio analysis via API (missing symbol details)
**Given** a portfolio with positions but some symbols lack cached details (sector, holdings, etc.)
**When** the API endpoint is called
**Then** the response computes analysis from available data
**And** a `warnings` array lists which symbols are missing data and which sections are affected

### Scenario: View ETF overlap pairwise matrix
**Given** a portfolio with 3+ ETF positions
**When** the investor views the Analysis page
**Then** they see a pairwise matrix showing the percentage of overlapping underlying stocks between each ETF pair
**And** each cell shows the combined portfolio weight of the overlapping holdings (e.g., "ETF A & ETF B: 12 stocks overlap, 8.3% of portfolio")
**And** pairs with zero overlap show "—" or "0%"

### Scenario: View top concentrated stocks across ETFs
**Given** a portfolio with multiple ETF positions that share underlying holdings
**When** the investor views the Analysis page
**Then** they see a ranked list of the top 10 most concentrated underlying stocks across all ETFs
**And** each entry shows: stock name, ticker, total portfolio weight (sum of its weight across all ETFs holding it), and which ETFs hold it
**And** the list is sorted by total portfolio weight descending

### Scenario: View correlation matrix of holdings
**Given** a portfolio with 3+ positions (ETFs or individual stocks)
**When** the investor views the Analysis page and selects a time period (1Y, 3Y, 5Y, 10Y)
**Then** they see a correlation matrix showing the pairwise correlation coefficient between each holding
**And** cells are color-coded (e.g., red for high positive correlation, blue for negative, white for near-zero)
**And** the matrix includes both ETF positions and individual stock positions
**And** the selected period determines the lookback window for the correlation calculation

### Scenario: Correlation matrix with insufficient data
**Given** a portfolio with holdings that have less historical price data than the selected period
**When** the investor views the correlation matrix
**Then** the correlation is computed using the available data overlap (the shorter of the two data series)
**And** a warning is shown if the data overlap is less than 60 trading days
**And** cells with insufficient data show "—" instead of a correlation value

### Scenario: View sector allocation (ETF look-through)
**Given** a portfolio with ETF positions that have cached sector weightings
**When** the investor views the Analysis page
**Then** they see a sector allocation breakdown showing the portfolio's total exposure to each sector
**And** the allocation is computed by weighting each ETF's sector breakdown by the ETF's portfolio weight
**And** individual stock positions are included using their own sector classification (from symbol details)
**And** sectors are sorted by weight descending
**And** a visual representation (e.g., horizontal bar chart or pie chart) is shown alongside the table

### Scenario: View geographic allocation (ETF look-through)
**Given** a portfolio with ETF positions that have cached geographic/country data
**When** the investor views the Analysis page
**Then** they see a geographic allocation breakdown showing the portfolio's total exposure by region/country
**And** the allocation is computed by weighting each ETF's geographic breakdown by the ETF's portfolio weight
**And** individual stock positions are included using their own country classification
**And** regions are sorted by weight descending

### Scenario: Sector/geographic allocation with incomplete data
**Given** a portfolio where some ETFs have no cached sector or geographic data
**When** the investor views the allocation breakdown
**Then** an "Unclassified" or "Unknown" bucket captures the weight of positions without data
**And** a warning message indicates which symbols are missing data
**And** the chart and table still render with the available data

### Scenario: Stress test against historical scenarios
**Given** a portfolio with current positions and cached sector allocations
**When** the investor selects a historical crisis scenario from the list
**Then** they see an estimate of the portfolio's loss under that scenario
**And** the estimate is computed by applying the scenario's sector-level returns to the portfolio's sector allocation
**And** the result shows: estimated portfolio return, estimated dollar impact (based on current portfolio value), and a breakdown by sector contribution
**And** the available scenarios include:
  - **2008 Global Financial Crisis** — peak-to-trough sector returns during the GFC
  - **2000 Dot-Com Bubble** — peak-to-trough sector returns during the tech crash
  - **2020 COVID Crash** — peak-to-trough sector returns during the pandemic sell-off
  - **2022 Market Decline** — peak-to-trough sector returns during the rate-hiking bear market
  - **2025 Tariff-Driven Crash** — estimated sector returns from tariff/trade war scenarios
  - **1997 Asian Financial Crisis** — peak-to-trough sector returns
  - **2011 European Sovereign Debt Crisis** — peak-to-trough sector returns

### Scenario: Compare multiple stress scenarios
**Given** a portfolio with current positions
**When** the investor views the stress testing section
**Then** they see all available scenarios in a comparison table
**And** each row shows: scenario name, date range, estimated portfolio return, and estimated dollar impact
**And** scenarios are sorted by severity (most negative return first)

### Scenario: Stress test with no sector data
**Given** a portfolio where sector allocation cannot be computed (e.g., all positions lack sector data)
**When** the investor views the stress testing section
**Then** they see a message explaining that stress testing requires sector allocation data
**And** the section suggests refreshing symbol details for the missing data

### Scenario: View factor exposure proxy
**Given** a portfolio with ETF positions that have cached equity valuation data (P/E, P/B)
**When** the investor views the Analysis page
**Then** they see a factor exposure summary including:
  - **Value vs Growth tilt** — portfolio-weighted P/E and P/B compared to a market benchmark (e.g., MSCI World or S&P 500), shown as a position on a Value↔Growth axis
  - **Size tilt** — large-cap vs mid-cap exposure based on the market cap of underlying holdings
  - **Concentration** — Herfindahl-Hirschman Index (HHI) of the portfolio's underlying holdings, with a plain-English interpretation (e.g., "Well-diversified", "Moderately concentrated", "Highly concentrated")
  - **Top holding weight** — the single largest underlying holding as a percentage of the portfolio

### Scenario: Factor exposure with insufficient data
**Given** a portfolio where valuation data (P/E, P/B) is missing for some positions
**When** the investor views the factor exposure section
**Then** available metrics are computed from positions with data
**And** a note indicates which positions contributed to the calculation and which were excluded

### Scenario: Analysis page for a new portfolio (no positions)
**Given** a portfolio with no open positions
**When** the investor views the Analysis page
**Then** they see an empty state message for each section explaining that analysis requires open positions

### Scenario: Analysis page for a single-stock portfolio
**Given** a portfolio with only individual stock positions (no ETFs)
**When** the investor views the Analysis page
**Then** the ETF overlap section shows a message that it requires multiple ETF positions
**And** the correlation matrix shows correlations between the individual stocks
**And** sector allocation uses the individual stocks' sector classification
**And** stress testing uses the individual stocks' sector data
**And** factor exposure is computed from the available valuation data

## Edge Cases

- **All positions are individual stocks (no ETFs)**: ETF overlap section is hidden or shows a message; other sections work with stock-level data.
- **All positions are ETFs with no cached details**: Sections gracefully degrade with "data unavailable" messages; suggests refreshing symbol details.
- **Partial sector/geographic data**: Some ETFs have data, others don't — "Unknown" bucket captures missing weight.
- **ETF holdings reference symbols not in the system**: Underlying holding symbols from Yahoo may not have symbol mappings — still used for overlap/concentration calculation without requiring a mapping.
- **Nested ETFs**: An ETF holding another ETF — the top 10 holdings are used as-is without recursive expansion (noted in the UI).
- **Very few positions (1-2)**: Correlation matrix requires at least 3 positions; shows a message otherwise.
- **Correlation with identical holdings**: If two ETFs hold the exact same stocks, correlation approaches 1.0 — displayed correctly.
- **Stress test with 100% cash position**: Portfolio shows ~0% impact across all scenarios (cash doesn't participate in equity drawdowns).
- **Negative portfolio value**: Stress test dollar impact is computed correctly (negative × negative = positive recovery).
- **Symbol details fetch is stale (>7 days)**: Analysis uses cached data but shows a stale indicator; doesn't block the view.

## Constraints

- **Sector/geographic data** comes from the `symbol_details` table (Yahoo Finance `topHoldings` and `assetProfile` modules)
- **Geographic/country data** is provided by f016 (symbol_details extension); this feature consumes it via the existing symbol details infrastructure
- **Overlap analysis** is ETF-to-ETF only (not ETF-to-stock or stock-to-stock)
- **Correlation** uses daily price data from the existing `market_data` table
- **Stress test scenarios** are pre-defined historical data bundled with the application (not fetched at runtime)
- **Factor exposure** is a proxy based on available data (P/E, P/B, concentration) — not a full Fama-French regression
- **All monetary values** use `decimal.Decimal` (never `float64`)
- **Analysis is computed on-demand** — no pre-computation or caching of results
- **Period selector** for correlation matrix offers: 1Y, 3Y, 5Y, 10Y (longer than the performance page periods)
- **API response structure** — single endpoint returns all sections; optional section filter for partial responses; consistent error format `{"error": "message", "code": "ERROR_CODE"}`

## Non-Goals

- Real-time analysis updates (computed on page load)
- Custom user-defined stress scenarios (pre-defined only)
- Full Fama-French multi-factor regression (proxy-based factor exposure only)
- Historical analysis snapshots (point-in-time composition — only current portfolio)
- Recursive ETF expansion (nested ETFs not unwrapped)
- Bond/fixed-income analysis (equity-focused only)
- Tax implications of rebalancing
- Rebalancing recommendations
- Mobile-specific UI (responsive but not mobile-optimized)
- API is the single source of truth — all analysis computation lives in the service layer; web UI and any future mobile client consume the same API response

## Dependencies

- **f009 Positions** (done) — provides current open positions with market values
- **f010 Portfolio Performance** (done) — portfolio value computation and period selection pattern
- **f011 Historical Market Data Caching** (done) — provides historical price data for correlation calculation
- **f015 Symbol Details** (done) — provides cached ETF holdings, sector weightings, equity valuation data
- **f016 Symbol Details — Geographic Data** — extends symbol_details with geographic/country breakdown (fetcher update + schema migration)
