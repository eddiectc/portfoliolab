# Feature: Unitization

## Description

Unitize the portfolio like a mutual fund / ETF by introducing the concept of "units" and "NAV per unit". This enables cash-flow-independent performance measurement — deposits and withdrawals no longer distort the performance picture because they are treated as buying or selling units at the current NAV, not as changes to portfolio value.

This also allows direct comparison of portfolio performance against benchmarks (S&P 500, NASDAQ, custom ETFs) on a percentage basis: both the portfolio NAV and each benchmark are normalized to 100% at portfolio inception and displayed as cumulative return %, enabling apples-to-apples comparison regardless of absolute price levels.

## User Stories

- **As an investor**, I want my portfolio to be unitized with a NAV per unit, so that my performance is not distorted by deposits and withdrawals.
- **As an investor**, I want to see my NAV per unit, number of units, and total value (NAV &times; units) on the performance page, so that I have a clear summary of my position.
- **As an investor**, I want to see my portfolio NAV compared against benchmarks on a percentage-return chart (both normalized to 100% at inception), so that I can directly compare my portfolio performance to other ETFs or indices.
- **As an investor**, I want comprehensive performance metrics (TWR, MWR, simple return, risk, drawdown, yearly performance) grouped in a summary table, so that I can analyze my portfolio from multiple angles.
- **As an investor**, I want a total return chart mode showing total value, net deposits, and P&L, so that I can still see the absolute portfolio picture.

## Scenarios

### Scenario: Initial unit creation at portfolio inception
**Given** a portfolio with no units yet
**When** the first deposit transaction occurs
**Then** the portfolio is unitized with 10000 initial units
**And** the initial NAV per unit equals the deposit amount divided by 10000

### Scenario: Non-deposit transaction before unitization
**Given** a portfolio with no units yet (no deposit has occurred)
**When** a buy, sell, or withdrawal transaction occurs
**Then** the transaction is processed normally (holdings/cash updated)
**And** the portfolio remains un-unitized (no units created)
**And** performance metrics show an empty state until the first deposit

### Scenario: Deposit buys new units at current NAV
**Given** a portfolio with existing units and a current NAV per unit
**When** a deposit transaction occurs
**Then** new units are created equal to deposit amount divided by current NAV per unit
**And** total units increase by the new units
**And** NAV per unit remains unchanged by the deposit itself

### Scenario: Withdrawal sells units at current NAV
**Given** a portfolio with existing units and a current NAV per unit
**When** a withdrawal transaction occurs
**Then** units are redeemed equal to withdrawal amount divided by current NAV per unit
**And** total units decrease by the redeemed units
**And** NAV per unit remains unchanged by the withdrawal itself

### Scenario: NAV per unit changes with market movements
**Given** a portfolio with units holding securities whose market prices change
**When** market prices are updated
**Then** NAV per unit is recalculated as total portfolio value divided by total units outstanding
**And** NAV increases if holdings appreciate and decreases if they depreciate

### Scenario: NAV mode chart with benchmark comparison
**Given** a portfolio with unitized NAV history over time
**When** the user selects the NAV mode tab on the performance page and selects one or more benchmarks
**Then** the chart displays portfolio NAV as a cumulative return % line, normalized to 100% at the inception date
**And** the chart displays each selected benchmark as a cumulative return % line, normalized to 100% at the same inception date
**And** all lines share the same Y-axis (percentage scale starting at 100%)
**And** no total value or deposit lines are shown

### Scenario: Total return chart mode
**Given** a portfolio with transaction history and market data
**When** the user selects the total return mode tab on the performance page
**Then** the upper panel displays total portfolio value over time
**And** the upper panel displays a line for cumulative net deposits
**And** the lower panel displays P&L over time
**And** benchmark comparison is not available in this mode

### Scenario: Summary table with grouped metrics
**Given** a portfolio with unitized performance data
**When** the user views the performance page
**Then** a summary table is displayed with the following groups:
- **NAV summary**: NAV per unit, total units, total value (NAV &times; units)
- **Time-Weighted Return**: TWR, annualized TWR
- **Money-Weighted Return**: MWR (IRR-based), annualized MWR
- **Simple Return**: simple return percentage, annualized simple return
  - Formula: `Simple Return = (Ending Total Return - Beginning Total Return) / Beginning Total Return`
  - Where `Total Return = Total Portfolio Value - Net Deposits` (P&L-adjusted value)
- **Risk metrics**: annualized volatility, Sharpe ratio, Sortino ratio
  - Risk-free rate sourced from US 3-month Treasury; defaults to 0% if unavailable
- **Drawdown analysis**: maximum drawdown, current drawdown, drawdown duration
- **Yearly performance**: calendar-year return breakdown (e.g. 2024: +12.3%, 2025: +8.1%)

### Scenario: Dividend reinvestment increases NAV without changing units
**Given** a portfolio with existing units
**When** a dividend is received and reinvested (or a cash dividend increases portfolio cash)
**Then** the total portfolio value increases
**And** the number of units remains unchanged
**And** NAV per unit increases accordingly

### Scenario: Buy/sell of securities does not directly change units
**Given** a portfolio with existing units holding securities
**When** the user buys or sells a security (not a deposit or withdrawal)
**Then** the number of units remains unchanged
**And** NAV per unit adjusts based on the change in total portfolio value

## Edge Cases

- **Withdrawal exceeds portfolio value**: The system must not allow redemption of more units than exist; reject or cap the withdrawal.
- **Zero-value portfolio with units outstanding**: NAV per unit is zero; chart and metrics should handle this gracefully (no division by zero).
- **Multiple transactions on the same day**: Each transaction is processed sequentially; units and NAV are updated after each one.
- **First transaction is a deposit**: Initial 10000 units are created; NAV per unit equals deposit amount / 10000.
- **First transaction is not a deposit**: Buy/sell/withdrawal before any deposit does not trigger unitization; portfolio remains un-unitized until the first deposit.
- **Transaction edit/delete invalidating NAV history**: Editing or deleting a transaction triggers full recalculation of NAV history from inception through the present.
- **Currency differences**: If the portfolio spans multiple currencies, NAV must be expressed in a single base currency (consistent with existing portfolio value calculations).
- **No transactions yet**: Unitization has not started; performance page shows appropriate empty state.
- **No deposits yet (only buy/sell)**: Unitization has not started; performance page shows appropriate empty state.
- **Dividend received as cash (not reinvested)**: Cash balance increases, raising total portfolio value and thus NAV per unit, without changing units.
- **Fractional units**: Unit arithmetic may produce fractional units (e.g. deposit of $500 at NAV $0.75321 = 663.84 units). Units are stored with full decimal precision internally; display rounds to 2 decimal places.

## Constraints

- Unitization uses existing transaction types (`deposit`, `withdrawal`) — no new transaction types are introduced.
- Cash is tracked via the existing cash-tracking mechanism.
- Buy/sell transactions affect holdings and cash but do not directly create or redeem units.
- NAV per unit is always expressed as a positive decimal value.
- NAV per unit is stored with 6 decimal places of precision; fractional units are stored with full decimal precision internally.
- Initial unit count is fixed at 10000 (not user-configurable).
- The system must support full recalculation of NAV history from inception (e.g., if a transaction is edited or deleted).
- Chart mode switching is via a tab selector on the performance page ("NAV Mode" / "Total Return Mode").

## Non-Goals

- **User-selectable inception date**: Unitization starts automatically at the first deposit; no manual configuration.
- **Multiple unit classes**: Only one class of units per portfolio.
- **Subscription/redemption fees**: No fee structure on unit creation or redemption.
- **Tax lot tracking**: Unitization is for performance measurement only, not tax reporting.
- **API-only endpoints without web UI**: All functionality includes both API and web UI per project conventions.

## Dependencies

- **f004 Transaction CRUD**: Requires existing `deposit` and `withdrawal` transaction types.
- **f009 Positions**: Requires position and portfolio value calculations.
- **f010 Portfolio Performance**: Adds NAV mode alongside the existing performance chart/metrics (existing equity curve mode is preserved, not replaced).
- **f012 Performance Benchmark**: Requires existing benchmark data for comparison.
