# Plan: Unitization

## Overview

Introduce mutual-fund-style unitization (units + NAV per unit) for cash-flow-independent performance measurement, alongside a comprehensive metrics summary table and NAV-mode chart with benchmark comparison.

**Key design decision**: Unitization is **computed on-the-fly** from existing transaction and market data — no new database tables. NAV history is derived during the equity curve computation by walking transactions and applying unit buy/sell logic at each cash flow.

## Technical Decisions

### 1. Storage: Computed vs persisted

**Decision**: Compute units and NAV on-the-fly (no new DB tables).

| Option | Pros | Cons |
|--------|------|------|
| **Computed** (chosen) | No migration needed, always consistent with transactions, full recalc on edit/delete is automatic | Slightly more CPU per request (mitigated by existing equity curve already being O(n×m)) |
| Persisted | Faster reads | Requires migration, cache invalidation complexity, consistency risk on transaction edit/delete |

**Rationale**: Unitization is a derived view of existing data. The equity curve computation already walks all transactions and values positions — adding unit tracking to this walk is incremental cost. Persisting would add complexity for no functional benefit given typical portfolio sizes (< 10K transactions).

### 2. NAV computation integration point

**Decision**: Extend the existing `walkTransactions` → `buildEquityCurvePoints` pipeline in `equity_curve.go` to produce NAV data alongside equity curve points.

| Option | Pros | Cons |
|--------|------|------|
| **Extend equity curve** (chosen) | Single pass over transactions, shares price/FX lookup infrastructure, pre-cash-flow snapshots already available | Couples unitization to equity curve module |
| Separate function | Cleaner separation | Must re-walk transactions or accept equity curve as input (loses pre-cash-flow granularity) |

**Rationale**: The equity curve already captures pre-cash-flow snapshots needed for correct NAV computation. Extending it avoids duplicating the transaction walk and price lookup logic.

### 3. Chart mode switching

**Decision**: Tab selector on the performance page ("NAV Mode" / "Total Return Mode"), controlled by a `mode` query parameter.

| Option | Pros | Cons |
|--------|------|------|
| **Tab selector** (chosen) | Clear visual distinction, preserves existing chart as default, matches spec | Requires template refactoring |
| Single chart with toggle | Simpler UI | Ambiguous state, harder to compare modes side-by-side |

**Rationale**: Two distinct views serve different analytical purposes. Tabs make the mode explicit and allow easy switching.

### 4. Risk-free rate for Sharpe/Sortino

**Decision**: Emit a warning when risk-free rate is unavailable. Sharpe and Sortino are `nil` (not computed) rather than silently defaulting to 0%.

**Rationale**: A 0% risk-free rate silently inflates Sharpe/Sortino ratios, which can mislead the user. Better to surface the issue explicitly so the user knows the metric is unavailable. Treasury data fetching is out of scope; we can wire it in later.

## Task List

### Phase 1: Domain — Unitization core

- [x] **Task 1.1**: Create `internal/domain/position/unitization.go` with `NavPoint` type and `ComputeNavHistory` pure function
  - `NavPoint`: `Date`, `NavPerUnit`, `Units`, `PortfolioValue`
  - `ComputeNavHistory(snapshots, breakpoints, firstDepositIdx) → []NavPoint`
  - Fixed initial units = 10000
  - Deposit: `newUnits = amount / currentNav`, `totalUnits += newUnits`
  - Withdrawal: `redeemedUnits = amount / currentNav`, `totalUnits -= redeemedUnits` (cap at 0)
  - NAV = portfolioValue / units (unchanged by deposit/withdrawal themselves)
  - **Tests**: `unitization_test.go` — table-driven tests for initial deposit, subsequent deposits, withdrawals, zero-value portfolio, withdrawal cap, fractional units, multiple transactions same day, non-deposit first transaction

- [x] **Task 1.2**: Create `internal/domain/position/daily_returns.go` with daily return computation helpers
  - `ComputeDailyReturns(points) → []DailyReturn` where `DailyReturn = {Date, ReturnPct}`
  - Used by risk metrics, drawdown, and yearly performance
  - **Tests**: `daily_returns_test.go` — basic returns, zero-value handling, single point

### Phase 2: Domain — Additional metrics

- [x] **Task 2.1**: Create `internal/domain/position/drawdown.go` with drawdown analysis
  - `ComputeDrawdownAnalysis(navPoints) → DrawdownAnalysis`
  - `DrawdownAnalysis`: `MaxDrawdownPct`, `CurrentDrawdownPct`, `DrawdownDurationDays`
  - Running peak tracking, current drawdown from peak
  - **Tests**: `drawdown_test.go` — no drawdown, full drawdown, recovering drawdown, current drawdown duration

- [x] **Task 2.2**: Create `internal/domain/position/yearly_performance.go` with calendar-year return breakdown
  - `ComputeYearlyPerformance(navPoints) → []YearlyReturn`
  - `YearlyReturn`: `Year`, `ReturnPct`
  - Group by calendar year, first/last NAV per year
  - **Tests**: `yearly_performance_test.go` — single year, multiple years, partial years, cross-year boundary

- [x] **Task 2.3**: Create `internal/domain/position/risk_metrics.go` with volatility, Sharpe, Sortino
  - `ComputeRiskMetrics(dailyReturns, riskFreeRatePct) → RiskMetrics`
  - `RiskMetrics`: `AnnualizedVolatilityPct`, `SharpeRatio`, `SortinoRatio`
  - Annualized vol = std-dev of daily returns × sqrt(252)
  - Sharpe = (mean daily return × 252 - riskFreeRate) / annualizedVol
  - Sortino = (mean daily return × 252 - riskFreeRate) / downsideDeviation × sqrt(252)
  - **Tests**: `risk_metrics_test.go` — zero volatility, positive/negative returns, nil risk-free rate (returns nil Sharpe/Sortino), valid risk-free rate

- [x] **Task 2.4**: Add `SimpleReturn` computation to `return_metrics.go`
  - `ComputeSimpleReturn(first, last EquityCurvePoint) → *decimal.Decimal`
  - Formula: `(endingTotalReturn - beginningTotalReturn) / beginningTotalReturn`
  - Where `TotalReturn = PortfolioValue - NetDeposit`
  - Also add annualized simple return
  - **Tests**: Add to `return_metrics_test.go`

### Phase 3: Types & equity curve integration

- [x] **Task 3.1**: Extend `performance_types.go` with new types and fields
  - Add `NavPerUnit`, `Units` fields to `EquityCurvePoint` (optional — nil when unitization not applicable)
  - Add `NavSummary` struct: `NavPerUnit`, `TotalUnits`, `TotalValue`, `InceptionDate`
  - Extend `ReturnMetrics` with: `SimpleReturnPct`, `AnnualizedSimpleReturnPct`
  - Add `RiskMetrics` struct: `AnnualizedVolatilityPct`, `SharpeRatio *decimal.Decimal`, `SortinoRatio *decimal.Decimal` (pointers — nil when risk-free rate unavailable)
  - Add `DrawdownAnalysis` struct: `MaxDrawdownPct`, `CurrentDrawdownPct`, `DrawdownDurationDays`
  - Add `YearlyPerformance` type alias for `[]YearlyReturn`
  - Extend `PerformanceResult` with: `NavSummary`, `RiskMetrics`, `DrawdownAnalysis`, `YearlyPerformance`, `SimpleReturnPct`, `AnnualizedSimpleReturnPct`
  - Extend `PerformanceFilters` with `Mode` field ("equity" or "nav")

- [x] **Task 3.2**: Integrate unitization into `ComputeEquityCurve` in `equity_curve.go`
  - After building equity curve points, call `ComputeNavHistory` using pre-cash-flow values
  - Populate `NavPerUnit` and `Units` on each `EquityCurvePoint`
  - Compute `NavSummary` from final state
  - Handle empty state (no deposits → nil NAV fields)
  - **Tests**: Add integration test cases to `equity_curve_test.go` or new `nav_test.go`

- [ ] **Task 3.3**: Compute additional metrics in `ComputeEquityCurve`
  - After NAV history is available, compute risk metrics, drawdown, yearly performance
  - Populate new fields on `PerformanceResult`
  - Add simple return to return metrics computation
  - **Tests**: Verify metrics are populated correctly in existing test scenarios

### Phase 4: API layer

- [ ] **Task 4.1**: Update `performance.go` handler
  - Parse `mode` query parameter in `parsePerformanceFilters`
  - Pass mode to `PerformanceFilters`
  - Ensure new fields serialize correctly in JSON response
  - **Tests**: Update `performance_test.go` with mode parameter tests

- [ ] **Task 4.2**: Update `performance_web.go` handler
  - Parse `mode` query parameter
  - Add `SelectedMode` to `performancePageData`
  - Compute NAV-mode chart data (normalized to 100% at inception)
  - Compute benchmark data normalized to same inception date
  - Build `ModeURLs` map for tab navigation
  - Update `buildPeriodURLs`, `buildBenchmarkURLs`, `buildRefreshURL` to preserve mode param
  - Add `computeNavChartData` function for NAV-mode ECharts data
  - **Tests**: Update `performance_web_test.go`

### Phase 5: Web UI

- [ ] **Task 5.1**: Redesign `templates/performance/index.html`
  - Add mode tab selector ("NAV Mode" / "Total Return Mode")
  - Add summary table with grouped metrics:
    - NAV summary: NAV per unit, total units, total value
    - TWR: TWR, annualized TWR
    - MWR: MWR, annualized MWR (holding period)
    - Simple return: simple return %, annualized
    - Risk metrics: volatility, Sharpe, Sortino
    - Drawdown: max drawdown, current drawdown, duration
    - Yearly performance: calendar-year rows
  - NAV mode chart: cumulative return % normalized to 100% at inception, with benchmark lines
  - Total return mode: preserve existing chart (portfolio value + net deposit + P&L)
  - **No separate test needed** — template changes verified via manual testing

- [ ] **Task 5.2**: Add CSS for summary table and mode tabs
  - Style for `metrics-table` with grouped sections
  - Tab selector styling consistent with period buttons
  - **No separate test needed** — CSS verified via manual testing

## Dependencies Between Tasks

```
Task 1.1 ──→ Task 3.1 ──→ Task 3.2 ──→ Task 3.3
Task 1.2 ──→ Task 2.1
         ──→ Task 2.2
         ──→ Task 2.3 ──→ Task 3.3
Task 2.4 ──→ Task 3.1
Task 3.3 ──→ Task 4.1 ──→ Task 4.2 ──→ Task 5.1
                         ──→ Task 5.2
```

## Execution Order

1. **Task 1.1** — Unitization core (foundation for everything)
2. **Task 1.2** — Daily returns helper (needed by risk/drawdown/yearly)
3. **Task 2.1** — Drawdown analysis
4. **Task 2.2** — Yearly performance
5. **Task 2.3** — Risk metrics
6. **Task 2.4** — Simple return
7. **Task 3.1** — Extend types
8. **Task 3.2** — Integrate unitization into equity curve
9. **Task 3.3** — Compute additional metrics
10. **Task 4.1** — API handler update
11. **Task 4.2** — Web handler update
12. **Task 5.1** — Template redesign
13. **Task 5.2** — CSS additions

## Files Created

| File | Purpose |
|------|---------|
| `internal/domain/position/unitization.go` | NavPoint type, ComputeNavHistory function |
| `internal/domain/position/unitization_test.go` | Unit tests for unitization logic |
| `internal/domain/position/daily_returns.go` | Daily return computation helpers |
| `internal/domain/position/daily_returns_test.go` | Tests for daily returns |
| `internal/domain/position/drawdown.go` | Drawdown analysis computation |
| `internal/domain/position/drawdown_test.go` | Tests for drawdown |
| `internal/domain/position/yearly_performance.go` | Calendar-year return breakdown |
| `internal/domain/position/yearly_performance_test.go` | Tests for yearly performance |
| `internal/domain/position/risk_metrics.go` | Volatility, Sharpe, Sortino |
| `internal/domain/position/risk_metrics_test.go` | Tests for risk metrics |

## Files Modified

| File | Changes |
|------|---------|
| `internal/domain/position/performance_types.go` | Add NAV fields, risk/drawdown/yearly types, mode filter |
| `internal/domain/position/equity_curve.go` | Integrate ComputeNavHistory, populate NAV fields |
| `internal/domain/position/return_metrics.go` | Add simple return computation |
| `internal/domain/position/return_metrics_test.go` | Tests for simple return |
| `internal/api/handlers/performance.go` | Parse mode param, pass to filters |
| `internal/api/handlers/performance_test.go` | Tests for mode parameter |
| `internal/api/handlers/performance_web.go` | Mode-aware page data, NAV chart computation, URL builders |
| `internal/api/handlers/performance_web_test.go` | Tests for mode handling |
| `templates/performance/index.html` | Mode tabs, summary table, NAV chart |
| `internal/web/static/style.css` | Summary table and tab styles |

## Non-Goals (per spec)

- No new database tables or migrations
- No new transaction types
- No subscription/redemption fees
- No tax lot tracking
- No user-selectable inception date
- No multiple unit classes
- No Treasury data fetching (Sharpe/Sortino are nil with a warning when risk-free rate unavailable)
