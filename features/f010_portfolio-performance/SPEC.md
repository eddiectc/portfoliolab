# Feature: Portfolio Performance

## Description

Portfolio-level performance analytics that let an investor understand how their portfolio has performed over time relative to their capital invested (net deposits) and against market benchmarks. Provides time-series equity curves, return metrics, drawdown analysis, and benchmark comparison — all currency-adjusted to the portfolio base currency.

This feature answers the core question: *"How much money did I make, and how does that compare to just holding an index?"*

## User Stories

- As an investor, I want to see my portfolio's equity curve over time alongside my net deposits, so I can visually track how much value I've created beyond what I put in.
- As an investor, I want to see summary return metrics (total return, annualized return, CAGR), so I can quickly assess overall performance at a glance.
- As an investor, I want to see my maximum and current drawdown with duration, so I understand my worst-case losses and recovery time.
- As an investor, I want to compare my portfolio's performance against market benchmarks (S&P 500, NASDAQ, or a custom index), so I can evaluate whether my investments outperform or underperform the market.
- As an investor with multi-currency accounts, I want all performance metrics expressed in my portfolio's base currency, so I have a consistent view without manual conversion.

## Scenarios

### Scenario: View portfolio equity curve (happy path)
**Given** multiple portfolios with accounts containing deposits, withdrawals, buys, and sells across several months
**When** the investor views the performance page with no portfolio selected (all portfolios) and the "All Time" period selected
**Then** they see a time-series chart with two lines: portfolio market value and cumulative net deposit
**And** the portfolio value line reflects the sum of all open position market values plus cash balances across all portfolios, converted to a common display currency
**And** the net deposit line reflects the running sum of deposits minus withdrawals across all portfolios, converted to a common display currency
**And** hovering over any point on the chart shows a tooltip with the date, portfolio value, and net deposit for that day


### Scenario: View portfolio equity curve with period selector
**Given** a portfolio with at least one year of transaction history
**When** the investor selects different periods (1W, 1M, 3M, 1Y, 3Y, 5Y, YTD, All Time)
**Then** the equity curve chart updates to show data only within the selected period
**And** the data points are daily granularity for all periods

### Scenario: View summary return metrics
**Given** a portfolio with transactions spanning at least 30 days
**When** the investor views the performance page
**Then** they see total return as a percentage (current value minus net deposit, divided by net deposit)
**And** they see annualized return (CAGR) calculated from the first transaction date to today
**And** if the portfolio has less than one year of history, annualized return is still shown (annualized from the actual period)

### Scenario: View drawdown metrics
**Given** a portfolio that has experienced at least one period of decline from a peak within the selected period
**When** the investor views the performance page with a period selected (e.g., 1M)
**Then** they see the maximum drawdown as a percentage with its peak date and trough date
**And** they see the current drawdown as a percentage with its peak date (or zero if at a new peak)
**And** they see the duration of the maximum drawdown (number of days from peak to trough)
**And** the performance chart visually marks the maximum drawdown region (e.g., shaded area between peak and trough)
**And** when the investor changes the period, the drawdown metrics and chart markers update to reflect the new period

### Scenario: Compare portfolio against a benchmark
**Given** a portfolio with historical data and a selected benchmark (e.g., ^GSPC for S&P 500)
**When** the investor enables benchmark comparison
**Then** they see the benchmark's equity curve overlaid on or alongside the portfolio equity curve
**And** both series are normalized to start at the same value (e.g., 100) for easy comparison
**And** they see the benchmark's total return and annualized return alongside the portfolio's

### Scenario: Benchmark comparison with no historical data available
**Given** a portfolio with transactions but no benchmark price history cached
**When** the investor enables benchmark comparison
**Then** the system attempts to fetch historical benchmark data
**And** if fetch succeeds, the comparison renders normally
**And** if fetch fails, the benchmark line is hidden and a message indicates that benchmark data is unavailable

### Scenario: Multi-currency portfolio performance
**Given** a portfolio with base currency USD that contains accounts in GBP and EUR
**When** the investor views the performance page filtered to that portfolio
**Then** all portfolio values are converted to USD using historical FX rates for each date
**And** net deposits in foreign currencies are converted to USD using historical FX rates
**And** if a historical FX rate is unavailable for a date, the current spot rate is used as a fallback

### Scenario: All portfolios with matching base currencies
**Given** multiple portfolios that share the same base currency (e.g., all USD)
**When** the investor views the performance page with no portfolio selected (all portfolios)
**Then** all values are shown in that shared base currency with no conversion needed

### Scenario: All portfolios with different base currencies
**Given** multiple portfolios with different base currencies (e.g., one in USD, one in GBP)
**When** the investor views the performance page with no portfolio selected (all portfolios)
**Then** an error message is shown explaining that cross-currency aggregation is not supported
**And** the investor is prompted to select a specific portfolio to view

### Scenario: Portfolio with only deposits (no investments yet)
**Given** a portfolio that has only deposit transactions and no buys/sells
**When** the investor views the performance page
**Then** the equity curve shows only the net deposit line (portfolio value equals net deposit)
**And** return metrics show 0% return
**And** drawdown metrics show 0% drawdown

### Scenario: Portfolio with negative net deposit (withdrawals exceed deposits)
**Given** a portfolio where total withdrawals exceed total deposits (net deposit is negative)
**When** the investor views the performance page
**Then** return metrics handle the negative denominator gracefully (e.g., show N/A or a meaningful message)
**And** the equity curve still renders correctly with the net deposit line below zero

### Scenario: Performance page for a new portfolio (no transactions)
**Given** a portfolio with no transactions at all
**When** the investor views the performance page
**Then** they see an empty state message indicating no performance data is available yet

## Edge Cases

- **No market data for a position**: If a symbol has no cached or fetchable price, that position is excluded from the market value calculation for that date, and a warning is shown.
- **FX rate gaps**: If historical FX rates are missing for certain dates, the current spot rate is used as a fallback. The UI indicates when fallback rates are in use.
- **Weekends and holidays**: Portfolio value is only computed on trading days. Non-trading days are interpolated (carried forward from the last trading day) for continuous chart rendering.
- **Very short time periods**: If the selected period (e.g., 1W on a new portfolio) has fewer than 2 data points, return metrics show N/A and a message explains the insufficient data.
- **Benchmark symbol not found**: If the user selects a custom benchmark symbol that doesn't exist or can't be fetched, an error message is shown and the portfolio data still renders.
- **Single-currency portfolio**: FX conversion is skipped entirely when all accounts share the portfolio base currency.
- **Large portfolios (100+ positions)**: Historical price fetching is batched and cached to avoid excessive API calls. Stale data is served if fetching is still in progress.
- **Date alignment**: Portfolio daily values and benchmark daily values are aligned to the same set of dates before computing comparisons. Dates where one series has no data are excluded from the calculation.

## Constraints

- All monetary values use `decimal.Decimal` (never `float64`)
- Performance data is computed from historical transactions and cached market data — no real-time streaming
- Historical price data must be fetched and cached in the `market_data` table before performance can be computed
- FX rates for currency conversion are fetched from the same market data source (Yahoo Finance)
- Performance defaults to aggregated across all portfolios; a portfolio selector lets the investor narrow to a single portfolio
- When no portfolio is selected, all accounts across all portfolios are included in the calculation
- When aggregating across portfolios with different base currencies, an error is shown and the investor must select a single portfolio

## Non-Goals

- Tax-loss harvesting analysis
- Sector/industry breakdown
- Dividend tracking and dividend yield analysis
- Correlation analysis (portfolio↔benchmark, asset↔benchmark, asset↔asset)
- Risk metrics beyond drawdown (e.g., Sharpe ratio, Sortino ratio, beta, alpha, volatility)
- Per-account performance breakdown (portfolio-level only)
- Real-time performance updates (computed on-demand or pre-computed and cached)
- Performance attribution (which specific trades contributed most to returns)
- Mobile-specific UI (the web UI is responsive but not mobile-optimized)

## Dependencies

- **f009 Positions** (done) — provides position data, market value enrichment, and FX conversion infrastructure
- **Market data table** (migration 010) — existing `market_data` table stores cached prices and FX rates
- **go-yfinance** — existing market data fetcher for pulling historical prices and FX rates
- **go-values/decimal** — existing decimal library for all monetary calculations
