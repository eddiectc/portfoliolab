# Feature: Portfolio Comparison Engine

## Description

The **Portfolio Comparison Engine** enables side-by-side comparison of any two portfolios — model vs model, model vs real, or real vs real — across a comprehensive set of performance, risk, and distribution metrics.

The comparison engine simulates a buy-and-hold strategy for model portfolios by default: starting with a configurable base amount (default 10,000 in a selectable base currency), it allocates capital according to the portfolio weights, tracks the value over the selected historical period using available market data, and computes metrics from the resulting value series. Real portfolios use their actual transaction history and positions.

Users can optionally select a **rebalancing** strategy for model portfolios: the simulated portfolio is periodically rebalanced back to the target weights on a configurable interval (monthly, quarterly, or annual — default quarterly), with an optional drift threshold that suppresses a rebalance unless a component's weight has drifted beyond it.

Users can also select an **asymmetric rebalancing** strategy for model portfolios: the target weights depend on the portfolio's drawdown from its high-water mark. While the drawdown is below a configurable trigger (default 10%), the portfolio's normal weights are the target. Once the drawdown reaches or exceeds the trigger, the target switches to the weights of a selected **stressed model portfolio** (a separate f019 model portfolio containing exactly the same components); the portfolio stays in the stressed state until its simulated value returns to its high-water mark, at which point the target reverts to the normal weights. The interval and drift-threshold mechanics are the same as the rebalancing strategy.

## User Stories

### US-1: Compare Two Portfolios
As an investor, I want to compare any two portfolios (model vs model, model vs real, or real vs real) on performance, risk, drawdown, distribution, overlap, and correlation metrics, so that I can evaluate which allocation is better suited to my goals.

### US-2: Select Comparison Period
As an investor, I want to select the time period for the comparison using either fixed periods (1Y, 3Y, 5Y, YTD, All) or a custom date range, so that I can analyze performance over different horizons.

### US-3: Configure Comparison Simulation Parameters
As an investor, I want to select the base currency and starting value for the comparison simulation, so that I can analyze results in my preferred currency context.

### US-4: Choose Comparison Strategy
As an investor, I want to choose between buy-and-hold and periodic rebalancing as the simulation strategy for model portfolios, so that I can evaluate whether periodic rebalancing improves performance or risk characteristics relative to simply holding the initial allocation.

### US-5: Use Drawdown-Aware Asymmetric Rebalancing
As an investor, I want the simulated model portfolio to switch between its normal target weights and the target weights of a selected stressed model portfolio based on its drawdown from its high-water mark, so that I can evaluate whether defensive de-risking during drawdowns improves performance or risk characteristics relative to static periodic rebalancing.

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

### Scenario: Compare model portfolios with rebalancing strategy
**Given** a model portfolio with multiple symbols and target weights
**When** I select the rebalancing strategy for the comparison
**Then** the simulated portfolio is rebalanced back to the target weights on the first trading day on or after each interval boundary (quarter start by default)
**And** between rebalances the portfolio value tracks the price movements of the current holdings without correction
**And** all metrics (returns, risk, drawdown, correlation, overlap) are computed from the rebalanced equity curve

### Scenario: Rebalancing with drift threshold
**Given** the rebalancing strategy is selected with an interval and a drift threshold (e.g., 5%)
**When** the simulation reaches a rebalance boundary
**Then** if no component's actual weight has deviated from its target weight by at least the threshold (in percentage points), the portfolio is not rebalanced on that date
**And** if any component's deviation is at least the threshold, all components are rebalanced back to their target weights on that date

### Scenario: Compare model portfolios with the asymmetric rebalancing strategy
**Given** a model portfolio with normal weights and a stressed model portfolio (f019) containing exactly the same components
**When** I select the asymmetric rebalancing strategy for the comparison with the stressed model portfolio assigned
**Then** the simulation tracks the running maximum (high-water mark) of the simulated total portfolio value since the start of the period
**And** while the portfolio's drawdown from the high-water mark is below the drawdown trigger (default 10%), the target weights are the normal weights
**And** once the drawdown reaches or exceeds the trigger, the target weights switch to the stressed model portfolio's weights
**And** the portfolio stays in the stressed state even if the drawdown temporarily falls back below the trigger; the target reverts to the normal weights only when the simulated value returns to its high-water mark (a new high)
**And** on each interval check (the first trading day on or after the interval boundary, as with the rebalancing strategy), if any component has drifted by at least the drift threshold from the target weights in effect on that date, all components are rebalanced to those target weights
**And** all metrics (returns, risk, drawdown, correlation, overlap) are computed from the resulting equity curve

### Scenario: Stressed model portfolio with a different component set
**Given** a model portfolio is selected for a side of the comparison and the asymmetric rebalancing strategy is selected
**When** the assigned stressed model portfolio contains different components than the normal portfolio
**Then** the comparison is rejected with an error stating that the stressed portfolio must contain exactly the same components

### Scenario: Asymmetric rebalancing never entering the stressed state
**Given** the asymmetric rebalancing strategy is selected
**When** the simulated portfolio's drawdown from its high-water mark never reaches the drawdown trigger
**Then** the equity curve is identical to the plain rebalanced strategy with the same interval and drift threshold

### Scenario: Configure asymmetric rebalancing in the comparison UI
**Given** the asymmetric rebalancing strategy is selected and a model portfolio is chosen for a side of the comparison
**When** I view the comparison setup
**Then** for each side I see a selector for choosing the stressed model portfolio from the existing model portfolios
**And** once a stressed model portfolio is chosen, its components and weights are shown in a read-only table
**And** the drawdown trigger is shown with its default value (10%)
**And** changing the stressed portfolio selection, the drawdown trigger, or the interval/threshold and re-running the comparison recomputes both curves with the new values

### Scenario: Strategy applies to model portfolios only
**Given** the comparison includes a real portfolio
**When** a strategy other than buy-and-hold (rebalanced or rebalanced-asymmetric) is selected
**Then** the model portfolio is simulated with the selected strategy
**And** the real portfolio is computed from its actual transaction history and is unaffected by the strategy selection

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
- **Drift threshold suppresses all rebalances** — If no component's weight ever deviates from target by the threshold, the rebalanced equity curve is identical to the buy-and-hold curve
- **Rebalance boundary falls on a non-trading day** — The rebalance is executed on the first trading day on or after the boundary; if the period ends before any trading day on/after a boundary, no rebalance occurs
- **Asymmetric: stressed weights equal to normal weights** — If the stressed model portfolio has identical weights to the normal portfolio, the asymmetric equity curve is identical to the plain rebalanced curve with the same interval and drift threshold
- **Asymmetric: state change between check dates** — A switch between the normal and stressed target (drawdown reaching the trigger, or the value returning to its high-water mark) takes effect only at the next interval check; the equity curve between checks is unaffected
- **Asymmetric: recovery after the last check** — If the portfolio recovers toward its high-water mark after the final interval check of the period, no further rebalance occurs; the last rebalance used the target in effect on the last check date
- **Asymmetric: no new high during the period** — If the portfolio never exceeds its starting value, the high-water mark stays at the starting value and drawdown is measured against it
- **Asymmetric: stressed portfolio with a different component set** — Assigning a stressed model portfolio whose components do not exactly match the normal portfolio's is rejected with an error (400 via the API)

## Constraints

- The default simulation strategy is buy-and-hold; the rebalancing strategy is optional
- Rebalancing simulation applies to model portfolios only; real portfolios always use their actual transaction history
- Rebalance intervals: monthly, quarterly, or annual; the default interval is quarterly
- Rebalancing occurs on the first trading day on or after the interval boundary (quarter starts: Jan 1, Apr 1, Jul 1, Oct 1; month starts: 1st of each month; year start: Jan 1)
- The optional drift threshold is expressed in percentage points of weight; when set, a rebalance at a boundary happens only if at least one component's actual weight deviates from target by at least the threshold, and when it fires all components are reset to target weights
- Asymmetric rebalancing applies to model portfolios only, the same as the rebalancing strategy; each model portfolio being simulated requires a stressed model portfolio (f019) containing exactly the same components
- Stressed weights are the target weights of the selected stressed model portfolio; as with all f019 model portfolios they sum to 100
- The drawdown trigger is a percentage of the high-water mark, default 10%; the stressed state is entered when the drawdown reaches or exceeds the trigger and is left only when the simulated value returns to its high-water mark (a new high)
- The high-water mark is the maximum simulated total portfolio value since the start of the comparison period (the starting value on day 0 counts)
- The interval and drift-threshold parameters behave identically to the rebalancing strategy; the drift on a check date is measured against the target weights in effect on that date
- The stressed model portfolio selection and the drawdown trigger are comparison request parameters; the association is not persisted on either portfolio (the stressed portfolio itself is created and managed under f019)
- Rebalancing is modeled without transaction costs (no fees, spreads, or slippage), consistent with the transaction-cost non-goal
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
- **Rebalancing of real portfolios** — The rebalancing strategy affects model portfolio simulation only; real portfolios always use their actual transaction history
- **Configurable recovery level** — Leaving the stressed state is fixed to full recovery to the high-water mark (a new high); there is no lower configurable recovery threshold
- **Ad-hoc stressed weight entry in the comparison UI** — The stressed target must be an existing f019 model portfolio; its weights are managed there, not typed into the comparison setup
- **Persisting the stressed association** — The stressed portfolio selection and the drawdown trigger are per-comparison parameters, not part of the model portfolio definition (f019)
- **Transaction cost modeling** — No fees, spreads, or slippage are simulated (including in rebalancing)
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
