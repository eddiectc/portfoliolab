# Notes: Unitization

## Decisions
- 2026-05-13: `ComputeNavHistory` accepts `[]EquityCurvePoint` and `[]navBreakpoint` (not the plan's `snapshots, breakpoints, firstDepositIdx`). The `firstDepositIdx` parameter was eliminated — the first equity curve point is always treated as the initial deposit (fixed 10000 units), and breakpoints are only processed for points after the first. This simplifies the API and matches how the equity curve is already structured.
- 2026-05-13: `navBreakpoint` is a private type (lowercase) co-located in `unitization.go`, matching the `twrBreakpoint` pattern. It will need to be populated from the existing pre-cash-flow snapshot infrastructure when integrated in Task 3.2.
- 2026-05-14: `ComputeRiskMetrics` uses the **daily** risk-free rate (`riskFreeRatio / 252`) for the downside deviation comparison, not the annual rate. The annual rate is used for the excess return numerator. Using the annual rate against daily returns would misclassify nearly every day as "downside" since daily returns (~0.5%) are always below an annual rate expressed as a ratio (0.045).

## Deviations from Plan
- None yet.

## Implementation Notes
- 2026-05-14 (Task 3.1): `RiskMetrics` and `DrawdownAnalysis` were **not re-declared** in `performance_types.go` — they already exist in `risk_metrics.go` and `drawdown.go` respectively (from Tasks 2.3 and 2.1). `PerformanceResult` references them directly. `YearlyPerformance` is a type alias (`[]YearlyReturn`) rather than a new struct.
- Private `computeValueReturn` (formerly `computeSimpleReturn`) computes `end/begin - 1` on PortfolioValue — used as the TWR no-cash-flow fallback. Public `ComputeSimpleReturn` computes P&L-based return. Renamed to avoid confusion.
- 2026-05-14 (Task 3.2): **Deposits-only fallback in `ComputeNavHistory`**: For portfolios with only deposits (no positions), pre-cash-flow snapshots yield zero portfolio value because there are no positions to value and cash hasn't been deposited yet. Added a fallback: when `bp.value` is zero, use the previous equity curve point's `PortfolioValue` as the pre-cash-flow value. This correctly computes NAV for deposits-only portfolios.
- 2026-05-14 (Task 3.2): **Breakpoint computation moved earlier**: `computePreCashFlowValues` is now called before interpolation (step 10 instead of step 11) so breakpoints are available for NAV history computation. The same breakpoints are reused for both NAV and TWR, avoiding duplicate computation.
- 2026-05-14 (Task 3.2): **NAV fields in interpolation**: `interpolateDaily` now carries forward `NavPerUnit` and `Units` for non-transaction days, ensuring NAV data is present on all equity curve points.

## Future Improvements
- None yet.

## Known Issues
- None yet.
