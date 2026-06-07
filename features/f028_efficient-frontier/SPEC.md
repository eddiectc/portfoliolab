# Feature: Efficient Frontier

## Description
Portfolio optimization via the efficient frontier method. The user selects a set of candidate symbols, configures optimization parameters (time period, risk-free rate), and the system computes the efficient frontier — the set of allocations that maximize expected return for a given level of risk. Key portfolios (maximum Sharpe ratio, minimum variance, highest return, maximum Sortino ratio, minimum drawdown) are highlighted. A correlation matrix heatmap shows pairwise relationships between candidate assets. The user can save any optimized allocation as a new model portfolio for later comparison.

This helps investors answer: "Given these assets, what is the best risk/return allocation?"

## User Stories

- **As an investor**, I want to select candidate symbols for optimization (via autocomplete or by copying from an existing portfolio/model portfolio), so that I can define the asset universe for my analysis.
- **As an investor**, I want to compute the efficient frontier for my candidate symbols over a chosen historical period, so that I can see the risk/return trade-offs across all possible allocations.
- **As an investor**, I want to see key portfolios (maximum Sharpe ratio, minimum variance, highest return, maximum Sortino ratio, minimum drawdown) highlighted on the frontier, so that I can quickly identify the most attractive allocations.
- **As an investor**, I want to see a correlation matrix heatmap of my candidate symbols, so that I can understand diversification benefits and concentration risk.
- **As an investor**, I want to view the target allocation weights for any portfolio on the frontier, so that I know how to construct the portfolio.
- **As an investor**, I want to save an optimized allocation as a new model portfolio, so that I can track and compare it against my actual portfolios over time.

## Scenarios

### Scenario: Select candidate symbols
**Given** I am on the efficient frontier page

**When** I type a symbol name into the symbol input
**Then** I see autocomplete suggestions from the symbol map (f003)
**And** I can select a symbol to add it to my candidate set

**When** I choose to copy symbols from an existing portfolio
**Then** the candidate set is pre-filled with the distinct symbols from that portfolio
**And** I can still add or remove individual symbols from the set

**When** I choose to copy symbols from a model portfolio
**Then** the candidate set is pre-filled with the distinct symbols from that model portfolio
**And** I can still add or remove individual symbols from the set

### Scenario: Compute efficient frontier with defaults
**Given** I have selected at least two candidate symbols
**When** I request the optimization with default settings
**Then** the system computes the efficient frontier using a default historical period (3 years), a default risk-free rate, and long-only fully-invested constraints (weights sum to 100%, no negative weights)
**And** the frontier is displayed as a risk/return chart
**And** key portfolios (maximum Sharpe ratio, minimum variance, highest return, maximum Sortino ratio, minimum drawdown) are highlighted
**And** a correlation matrix heatmap is displayed for the candidate symbols

### Scenario: Compute efficient frontier with custom period
**Given** I have selected candidate symbols
**When** I choose a predefined period (1Y, 3Y, or 5Y) and request the optimization
**Then** the system computes the frontier using historical data for the selected period

### Scenario: Compute efficient frontier with custom risk-free rate
**Given** I have selected candidate symbols
**When** I enter a custom risk-free rate and request the optimization
**Then** the system uses my specified rate for Sharpe ratio calculations

### Scenario: View allocation weights for any frontier point
**Given** the efficient frontier has been computed and displayed
**When** I select any point on the frontier curve
**Then** I see a table of target allocation weights (as percentages) for each candidate symbol
**And** the weights sum to 100%
**And** I see risk metrics including Sharpe ratio, Sortino ratio, and maximum drawdown

### Scenario: Save optimized allocation as a model portfolio
**Given** I have computed the efficient frontier and selected a portfolio
**When** I choose to save the allocation as a model portfolio
**Then** a new model portfolio is created with the target allocation weights
**And** I can give it a name
**And** the model portfolio appears in my list of model portfolios

### Scenario: Error — empty candidate set
**Given** I have not selected any candidate symbols
**When** I request the optimization
**Then** I see an error message explaining that at least two symbols are required

### Scenario: Error — numerical failure
**Given** I have selected candidate symbols with sufficient historical data
**When** the optimization fails due to numerical issues (e.g., singular covariance matrix)
**Then** I see an error message explaining that the optimization could not be computed
**And** the message indicates likely causes (e.g., perfectly correlated assets)

### Scenario: Error — some symbols missing historical data
**Given** I have selected candidate symbols
**When** one or more symbols lack sufficient historical data for the selected period
**Then** I see a warning indicating which symbols have insufficient data
**And** the optimization proceeds with the symbols that have adequate data

### Scenario: Error — all symbols missing historical data
**Given** I have selected candidate symbols
**When** all symbols lack sufficient historical data for the selected period
**Then** I see an error message indicating that no usable data is available

## Edge Cases
- **Single candidate symbol**: The frontier degenerates to a single point — show an error requiring at least two symbols.
- **Highly correlated symbols**: The frontier may be very narrow or nearly linear — still compute and display, but the result may not be useful.
- **Symbols with no overlapping history**: If two symbols have no overlapping price history, covariance cannot be computed — exclude and warn.
- **Very short time period**: If the selected period has very few data points (e.g., 1Y with limited trading days), results may be unreliable — still compute but note the limited data.
- **Risk-free rate equals or exceeds portfolio return**: Sharpe ratio becomes zero or negative — still display; the minimum variance portfolio remains valid.
- **Duplicate symbols in candidate set**: Silently deduplicate when computing.
- **Symbols in different currencies**: If candidate symbols are denominated in different currencies, the system must handle currency conversion for consistent return/covariance computation.
- **Delisted symbols**: A symbol may have historical prices but no recent data — treat as insufficient data and warn the user.

## Constraints
- **Long-only**: No short selling; all weights are non-negative.
- **Fully invested**: Weights sum to 100%.
- **No per-asset min/max constraints** in this version (kept simple).
- **Historical data**: Uses existing market data from f011 for price history.
- **Minimum two symbols** required for optimization.

## Non-Goals
- Live rebalancing or trade execution.
- Transaction cost or tax-loss harvesting modeling.
- Multi-period / dynamic optimization.
- Short selling.
- External optimization libraries.
- Per-asset weight constraints (min/max per symbol).
- Custom / continuous date range selection (predefined periods only: 1Y, 3Y, 5Y).

## Dependencies
- **f003 (Symbol Map)**: Symbol lookup and autocomplete.
- **f011 (Historical Market Data Caching)**: Historical price data.
- **f019 (Model Portfolio)**: Saving optimized allocation as a model portfolio; copying symbols from model portfolios.
- **f001 (Portfolio CRUD)**: Copying symbols from existing portfolios.
