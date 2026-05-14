# Notes: Unitization

## Decisions
- 2026-05-13: `ComputeNavHistory` accepts `[]EquityCurvePoint` and `[]navBreakpoint` (not the plan's `snapshots, breakpoints, firstDepositIdx`). The `firstDepositIdx` parameter was eliminated — the first equity curve point is always treated as the initial deposit (fixed 10000 units), and breakpoints are only processed for points after the first. This simplifies the API and matches how the equity curve is already structured.
- 2026-05-13: `navBreakpoint` is a private type (lowercase) co-located in `unitization.go`, matching the `twrBreakpoint` pattern. It will need to be populated from the existing pre-cash-flow snapshot infrastructure when integrated in Task 3.2.
- 2026-05-14: `ComputeRiskMetrics` uses the **daily** risk-free rate (`riskFreeRatio / 252`) for the downside deviation comparison, not the annual rate. The annual rate is used for the excess return numerator. Using the annual rate against daily returns would misclassify nearly every day as "downside" since daily returns (~0.5%) are always below an annual rate expressed as a ratio (0.045).

## Deviations from Plan
- None yet.

## Implementation Notes
- `ComputeSimpleReturn` (public, P&L-based) is distinct from `computeSimpleReturn` (private, portfolio-value-based). The private function computes `end/begin - 1` on PortfolioValue and is used internally by TWR as the no-cash-flow fallback. The public function computes the percentage change in cumulative P&L (`PortfolioValue - NetDeposit`). Both coexist — they measure different things.

## Future Improvements
- None yet.

## Known Issues
- None yet.
