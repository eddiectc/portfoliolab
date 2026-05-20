# Feature: Portfolio Comparison Engine

## Description

The **Portfolio Comparison Engine** enables side-by-side comparison of any two portfolios — model vs model, model vs real, or real vs real — across a comprehensive set of performance, risk, and distribution metrics.

The comparison engine simulates a buy-and-hold strategy for model portfolios: starting with a configurable base amount (default 10,000 in a selectable base currency), it allocates capital according to the portfolio weights, tracks the value over the selected historical period using available market data, and computes metrics from the resulting value series. Real portfolios use their actual transaction history and positions.

## User Stories

### US-1: Compare Two Portfolios
As an investor, I want to compare any two portfolios (model vs model, model vs real, or real vs real) on performance, risk, drawdown, distribution, overlap, and correlation metrics, so that I can evaluate which allocation is better suited to my goals.

### US-2: Select Comparison Period
As an investor, I want to select the time period for the comparison using either fixed periods (1Y, 3Y, 5Y, YTD, All) or a custom date range, so that I can analyze performance over different horizons.

### US-3: Configure Comparison Simulation Parameters
As an investor, I want to select the base currency and starting value for the comparison simulation, so that I can analyze results in my preferred currency context.

## Scenarios

### Scenario: Compare two model portfolios — performance metrics
**Given** two model portfolios exist with sufficient historical data (at least 1 year)
**When** I select both portfolios for comparison with a 3Y period
**Then** I see side-by-side performance metrics for each:
- **Starting value** — the configured base amount (default 10,000)
- **Ending value** — the simulated portfolio value at the end of the period
- **Total return %** — (ending value - starting value) / starting value
- **CAGR** — compound annual growth rate over the period

### Scenario: Compare two model portfolios — risk metrics
**Given** two model portfolios exist with sufficient historical data
**When** I view the comparison
**Then** I see side-by-side risk metrics for each:
- **Sharpe ratio** — risk-adjusted return using daily returns and a risk-free rate
- **Volatility** — annualized standard deviation of daily returns
- **Beta** — sensitivity of portfolio returns relative to the other portfolio in the comparison
- **Alpha** — excess return not explained by beta relative to the other portfolio

### Scenario: Compare model portfolio vs real portfolio
**Given** a model portfolio and a real portfolio exist
**When** I select the model portfolio and the real portfolio for comparison
**Then** I see the same performance, risk, and drawdown metrics computed side-by-side
**And** the real portfolio metrics are computed from its actual transaction history and positions
**And** the model portfolio metrics are computed from the simulated buy-and-hold strategy

### Scenario: Compare two real portfolios
**Given** two real portfolios exist
**When** I select both real portfolios for comparison
**Then** I see the same performance, risk, and drawdown metrics computed side-by-side
**And** each portfolio's metrics are computed from its own actual transaction history

### Scenario: View drawdown comparison
**Given** two portfolios are selected for comparison
**When** I view the drawdown section
**Then** I see a line chart showing the drawdown over time for both portfolios on the same axes
**And** I see summary statistics: max drawdown %, drawdown duration (days from peak to trough), and recovery duration (days from trough to new peak, or "unrecovered")

### Scenario: View annual returns histogram
**Given** two portfolios are selected for comparison with a period spanning multiple years
**When** I view the annual returns section
**Then** I see a bar chart showing each calendar year's return for both portfolios
**And** years are labeled on the X-axis and return % on the Y-axis
**And** both portfolios' bars are shown side-by-side per year

### Scenario: View annual return frequency histogram
**Given** two portfolios are selected for comparison with a period spanning multiple years
**When** I view the annual return frequency distribution
**Then** I see a histogram showing how frequently each annual return range occurred
**And** the distribution is shown for both portfolios (e.g., overlaid or side-by-side)

### Scenario: View monthly return frequency histogram
**Given** two portfolios are selected for comparison
**When** I view the monthly return frequency distribution
**Then** I see a histogram showing how frequently each monthly return range occurred
**And** the distribution is shown for both portfolios

### Scenario: View period extremes
**Given** two portfolios are selected for comparison
**When** I view the period extremes section
**Then** I see for each portfolio:
- **Best month** — the month with the highest return and its value
- **Worst month** — the month with the lowest return and its value
- **Best year** — the calendar year with the highest return and its value
- **Worst year** — the calendar year with the lowest return and its value
- **Win rate** — the percentage of months with positive returns

### Scenario: View holdings overlap
**Given** two portfolios are selected for comparison
**And** at least one portfolio contains ETFs with holdings data available
**When** I view the holdings overlap section
**Then** I see the top 10 underlying holdings for each portfolio (aggregating ETF constituents weighted by the portfolio allocation)
**And** I see the overlap — which underlying holdings appear in both portfolios
**And** the overlap is expressed as a percentage of total exposure

### Scenario: View correlation matrix
**Given** a single portfolio (model or real) is part of the comparison
**When** I view the correlation section
**Then** I see a correlation matrix showing the pairwise correlation between all symbols within that portfolio
**And** the matrix is shown for each portfolio being compared

### Scenario: View portfolio-to-portfolio correlation
**Given** two portfolios are selected for comparison
**When** I view the correlation section
**Then** I see the overall correlation between the two portfolios' daily return series

### Scenario: Select fixed comparison period
**Given** two portfolios are selected for comparison
**When** I choose a fixed period (1Y, 3Y, 5Y, YTD, All)
**Then** the comparison metrics are computed for that period
**And** the charts and statistics update accordingly

### Scenario: Select custom comparison period
**Given** two portfolios are selected for comparison
**When** I choose a custom start and end date
**Then** the comparison metrics are computed for the specified date range
**And** the charts and statistics update accordingly

### Scenario: Base currency selection for comparison
**Given** I am setting up a portfolio comparison
**When** I select the base currency for the simulation
**Then** the starting value (10,000) is expressed in that currency
**And** all symbols' returns are computed in that currency (using FX rates where needed)
**And** the ending value and all metrics are expressed in that currency

### Scenario: Handle symbols with shorter history
**Given** a model portfolio contains symbols with different data availability windows
**When** I view the comparison
**Then** the comparison period is clipped to the earliest available data point among all symbols
**And** a warning indicates which symbols have limited history and the effective start date
**And** metrics are computed only for the overlapping period where all symbols have data

### Scenario: Comparison with insufficient data
**Given** a model portfolio contains a symbol with no market data available
**When** I attempt to view the comparison
**Then** a warning indicates which symbols are missing data
**And** the comparison is not computed until data is available
**And** a prompt offers to refresh market data for the missing symbols

### Scenario: Compare against real portfolio with no transactions
**Given** a model portfolio exists
**And** a real portfolio exists with no transactions
**When** I select both for comparison
**Then** the model portfolio metrics are computed normally
**And** the real portfolio shows zero/empty metrics
**And** a message explains that the real portfolio has no transaction history to compare against

## Edge Cases

- **Both portfolios are identical** — Comparison shows identical metrics; correlation is 1.0; overlap is 100%
- **Single symbol model portfolio** — A model portfolio with only one symbol (100% weight) computes comparison metrics normally
- **Symbol with no ETF holdings data** — When computing holdings overlap, symbols without holdings data are treated as atomic (the symbol itself is the holding)
- **Real portfolio with no transactions** — Comparing against a real portfolio with no transactions shows the real portfolio with zero/empty metrics and a message
- **Very short comparison period** — If the selected period has fewer than 30 trading days, risk metrics (Sharpe, volatility) show N/A with a message explaining insufficient data
- **Custom period with no trading days** — If the custom date range falls entirely on non-trading days or has no data, an error message is shown
- **FX rate gaps for foreign-denominated symbols** — If historical FX rates are missing, the current spot rate is used as a fallback; a warning is shown
- **Risk-free rate for Sharpe ratio** — The risk-free rate is derived from a short-term government bond proxy (e.g., 3-month Treasury) or defaults to 0% if unavailable; the rate used is displayed to the user
- **Model portfolio deleted during active comparison** — If a model portfolio used in a comparison is deleted, the comparison shows an error and prompts the user to select a replacement
- **Starting value configurable** — The default starting value is 10,000 but the user can configure a different amount

## Constraints

- The comparison simulation uses a buy-and-hold strategy — no rebalancing during the period
- Starting value defaults to 10,000 in the selected base currency; user can configure a different amount
- The Sharpe ratio uses daily returns annualized (multiplied by √252)
- The risk-free rate for Sharpe ratio defaults to 0% if no proxy is available; the actual rate used is displayed
- Beta and Alpha are computed relative to the other portfolio in the comparison (not relative to a market benchmark)
- Holdings overlap uses available ETF holdings data; symbols without this data are treated as their own holding
- Correlation is computed from daily return series over the comparison period
- The comparison uses available cached market data; newly added symbols trigger a market data refresh
- Maximum of 2 portfolios in a single comparison
- Annual returns are computed per calendar year (Jan 1 – Dec 31)
- Monthly returns are computed per calendar month (1st – last day of month)
- Fixed comparison periods: 1Y, 3Y, 5Y, YTD, All
- Custom date ranges are supported

## Non-Goals

- **Real-time paper trading** — Comparison is historical analysis only, not live tracking
- **Rebalancing simulation** — The comparison assumes buy-and-hold; no periodic rebalancing of weights
- **Transaction cost modeling** — No fees, spreads, or slippage are simulated
- **Tax lot simulation** — No tax implications are considered
- **Margin or leverage** — The simulation assumes full cash purchase
- **More than 2 portfolios** — Comparisons are limited to 2 portfolios at a time
- **Portfolio vs index benchmark** — The comparison is between 2 portfolios; for portfolio vs index benchmarking, use the existing benchmark feature (f010/f012)
- **Dividend reinvestment** — Dividends are not explicitly modeled; total return comes from price appreciation of the underlying symbols
- **Scheduling automatic comparisons** — Comparisons are computed on demand
- **Model portfolio management** — Creating, editing, and deleting model portfolios is covered by f019

## Dependencies

- **f019_model-portfolio** — Model portfolio definitions (symbols and weights)
- **f009_positions** — Real portfolio position data for real portfolio comparisons
- **f010_portfolio-performance** — Performance computation infrastructure (equity curves, return metrics)
- **f011_historical-market-data-caching** — Cached market prices and FX rates for simulation
- **f015_symbol-details** — ETF holdings data for overlap analysis
- **f012_performance-benchmark** — Benchmark data infrastructure
