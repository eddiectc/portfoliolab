# Feature: Hierarchical Risk Parity

## Description
Portfolio optimization via the Hierarchical Risk Parity (HRP) method. The user selects a set of candidate symbols, configures the historical period, and the system computes four HRP allocations (one per linkage method: single, complete, average, Ward) — risk-parity portfolios constructed through hierarchical clustering and recursive bisection rather than mean-variance optimization. The result is four target allocations (table of weights, one column per method) along with four clustering dendrograms showing how the assets group together under each method. Comparing the four allocations lets the user assess robustness — if all four are similar, the result is stable; if they diverge, the allocation is sensitive to the clustering choice.

Unlike the Efficient Frontier (f028), which requires numerical optimization and can be unstable with correlated assets, HRP is a purely analytical method that is more robust and produces allocations without needing to search a frontier. The user can save any of the four HRP allocations as a new model portfolio for later comparison.

This helps investors answer: "Given these assets, what does a hierarchical risk-parity allocation look like under different clustering assumptions, and how robust is the result?"

## User Stories

- **As an investor**, I want to select candidate symbols for HRP analysis, so that I can define the asset universe for my analysis.
- **As an investor**, I want to compute the HRP allocations for my candidate symbols over a chosen historical period, so that I can see the four risk-parity target weights (one per linkage method).
- **As an investor**, I want to see a clustering dendrogram for each linkage method, so that I can understand how the correlation structure differs across methods.
- **As an investor**, I want to view the four target allocation weights side-by-side in a table, so that I can assess how robust the allocation is to the clustering choice.
- **As an investor**, I want to save any of the four HRP allocations as a new model portfolio, so that I can track and compare it against my actual portfolios over time.

## Scenarios

### Scenario: Select candidate symbols via autocomplete
**Given** I am on the HRP page

**When** I type a symbol name into the symbol input
**Then** I see autocomplete suggestions from the symbol map (f003)
**And** I can select a symbol to add it to my candidate set

### Scenario: Copy symbols from an existing portfolio
**Given** I am on the HRP page
**When** I choose to copy symbols from an existing portfolio
**Then** the candidate set is pre-filled with the distinct symbols from that portfolio
**And** I can still add or remove individual symbols from the set

### Scenario: Remove a symbol from the candidate set
**Given** I have symbols in my candidate set
**When** I remove one of the symbols
**Then** it is no longer included in the candidate set
**And** it will be excluded from the next HRP computation

### Scenario: Copy symbols from a model portfolio
**Given** I am on the HRP page
**When** I choose to copy symbols from a model portfolio
**Then** the candidate set is pre-filled with the distinct symbols from that model portfolio
**And** I can still add or remove individual symbols from the set

### Scenario: Compute HRP allocations with default period
**Given** I have selected at least two candidate symbols
**When** I request the HRP computation with default settings
**Then** the system computes four HRP allocations (single, complete, average, Ward linkage) using a default historical period (3 years)
**And** a table of target allocation weights (as percentages) is displayed for each candidate symbol, with one column per linkage method
**And** each column's weights sum to 100%
**And** four clustering dendrograms are displayed, one per linkage method, showing how the symbols are grouped

### Scenario: Compute HRP allocations with custom period
**Given** I have selected candidate symbols
**When** I choose a predefined period (1Y, 3Y, or 5Y) and request the computation
**Then** the system computes the four HRP allocations using historical data for the selected period
**And** the allocation weights and dendrograms are updated

### Scenario: View allocation weights table
**Given** the HRP allocations have been computed and displayed
**When** I view the results
**Then** I see a table listing each candidate symbol with four target weight columns (single, complete, average, Ward)
**And** each column's weights sum to 100%
**And** the table includes the symbol's name/description alongside the ticker

### Scenario: View clustering dendrograms
**Given** the HRP allocations have been computed and displayed
**When** I view the results
**Then** I see four dendrograms (tree diagrams), one per linkage method, showing the hierarchical clustering of the candidate symbols
**And** symbols that are more closely related (higher correlation) appear closer together in each tree
**And** each dendrogram is labeled with the linkage method used and the symbol tickers at the leaves

### Scenario: Save an HRP allocation as a model portfolio
**Given** I have computed the four HRP allocations
**When** I choose to save one of the allocations (e.g., the "average" linkage result) as a model portfolio
**Then** a new model portfolio is created with the selected linkage method's target allocation weights
**And** I can give it a name
**And** I see a confirmation that the model portfolio was saved
**And** the model portfolio appears in my list of model portfolios

### Scenario: Error — insufficient candidate symbols
**Given** I have fewer than two candidate symbols selected
**When** I request the computation
**Then** I see an error message explaining that at least two symbols are required

### Scenario: Warning — some symbols with insufficient historical data
**Given** I have selected candidate symbols
**When** one or more symbols have less historical data than the selected period (e.g., 100 days of data when 1Y was requested)
**Then** I see a warning for each affected symbol indicating how much data is available vs. expected
**And** the computation includes all symbols using whatever overlapping data is available

### Scenario: Error — symbol with no historical data at all
**Given** I have selected candidate symbols
**When** one or more symbols have no historical price data at all
**Then** I see an error indicating which symbols have no data
**And** the computation cannot proceed until those symbols are removed

### Scenario: Error — all symbols missing historical data
**Given** I have selected candidate symbols
**When** all symbols lack sufficient historical data for the selected period
**Then** I see an error message indicating that no usable data is available

## Edge Cases
- **Single candidate symbol**: HRP requires at least two symbols — show an error.
- **Identical/duplicate symbols**: If the candidate set contains the same symbol twice, silently deduplicate.
- **Highly correlated symbols**: HRP handles this gracefully (unlike mean-variance) — still compute and display. The dendrogram will show the tight clustering.
- **Symbols with insufficient but non-zero data**: If a symbol has some historical data but less than the requested period, include it in computation using available overlapping data and show a warning (e.g., "expected 365 days, got 100 days for AAPL").
- **Symbols with no data at all**: If a symbol has zero historical price data, it cannot be included — show an error and require removal before computation.
- **Symbols in different currencies**: If candidate symbols are denominated in different currencies, the user selects a base currency and the system converts all prices using cached FX rates (same approach as f028 Efficient Frontier).
- **Delisted symbols**: A symbol may have historical prices but no recent data — treat as insufficient data and warn the user.
- **Maximum symbol count**: More than 20 symbols — show an error blocking submission; the candidate set exceeds the limit.
- **Very short time period**: If the selected period has very few data points, results may be unreliable — still compute but note the limited data.
- **Zero-return symbol**: A symbol with a flat price series (no price movement) has zero volatility — the algorithm should handle this gracefully (near-zero weight or warning).

## Constraints
- **Long-only**: All weights are non-negative.
- **Fully invested**: Weights sum to 100%.
- **Maximum 20 symbols** in the candidate set.
- **Minimum 2 symbols** required for computation.
- **Predefined periods only**: 1Y, 3Y, 5Y (no custom date ranges).
- **Historical data**: Uses existing cached market data for price history.
- **No external optimization/ML libraries**: Standard library and existing Go dependencies only.
- **Four allocations**: HRP produces four allocations (one per linkage method: single, complete, average, Ward), not a frontier of allocations.

## Non-Goals
- Live rebalancing or trade execution.
- Transaction cost or tax-loss harvesting modeling.
- Backtesting HRP allocations over time.
- Short selling.
- Per-asset weight constraints (min/max per symbol).
- Custom / continuous date range selection.
- Multiple optimization objectives (the four linkage-method allocations are the fixed set).
- Exporting or downloading the dendrogram as an image.
- Comparative analysis between HRP and other methods on the same page (use f020 Portfolio Comparison for that).
- Custom linkage methods or distance metrics (the four standard linkage methods are the fixed set).

## Dependencies
- **f003 (Symbol Map)**: Symbol lookup and autocomplete.
- **f011 (Historical Market Data Caching)**: Historical price data.
- **f019 (Model Portfolio)**: Saving HRP allocation as a model portfolio; copying symbols from model portfolios.
- **f001 (Portfolio CRUD)**: Copying symbols from existing portfolios.
