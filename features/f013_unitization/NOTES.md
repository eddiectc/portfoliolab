# Notes: Unitization

## Decisions
- 2026-05-13: `ComputeNavHistory` accepts `[]EquityCurvePoint` and `[]navBreakpoint` (not the plan's `snapshots, breakpoints, firstDepositIdx`). The `firstDepositIdx` parameter was eliminated — the first equity curve point is always treated as the initial deposit (fixed 10000 units), and breakpoints are only processed for points after the first. This simplifies the API and matches how the equity curve is already structured.
- 2026-05-13: `navBreakpoint` is a private type (lowercase) co-located in `unitization.go`, matching the `twrBreakpoint` pattern. It will need to be populated from the existing pre-cash-flow snapshot infrastructure when integrated in Task 3.2.
- 2026-05-14: `ComputeRiskMetrics` uses the **daily** risk-free rate (`riskFreeRatio / 252`) for the downside deviation comparison, not the annual rate. The annual rate is used for the excess return numerator. Using the annual rate against daily returns would misclassify nearly every day as "downside" since daily returns (~0.5%) are always below an annual rate expressed as a ratio (0.045).
- 2026-05-14: `ComputeDailyReturns` uses `NavPerUnit` instead of `PortfolioValue` when available, falling back to `PortfolioValue` when NAV is nil (no unitization). This isolates daily returns from cash flow effects — a deposit that doubles portfolio value doesn't create a spurious +100% "daily return". `ComputeDrawdownAnalysis` similarly uses `NavPerUnit` instead of `PortfolioValue`, so drawdown reflects actual market losses, not the impact of deposits or withdrawals. Both changes are consistent with TWR: they measure investment performance independent of cash flow timing.

## Deviations from Plan
- 2026-05-14 (Task 3.3): **Simple return uses value-based, not P&L-based**. The plan specified `ComputeSimpleReturn` (P&L-based: `(endTotalReturn - beginTotalReturn) / beginTotalReturn` where `TotalReturn = PortfolioValue - NetDeposit`). At portfolio inception, `PortfolioValue == NetDeposit` so `TotalReturn = 0` and the function returns nil for every portfolio viewed from inception. Switched to `computeValueReturn` (value-based: `end/begin - 1` on PortfolioValue), which always yields a meaningful percentage and matches what the summary table needs to display.

## Implementation Notes
- 2026-05-14 (Task 4.2): `computeNavChartData` normalizes PortfolioValue to 100% at inception using `(val / startValue) * 100` with rounding to 2 decimal places to avoid floating point artifacts (e.g., `110.00000000000001`). Mode parameter is preserved across all URL builders (`buildRefreshURL`, `buildPeriodURLs`, `buildBenchmarkURLs`) and omitted from URLs when mode is "equity" (the default). `buildModeURLs` generates URLs for both "equity" and "nav" modes.
- 2026-05-14 (Task 3.1): `RiskMetrics` and `DrawdownAnalysis` were **not re-declared** in `performance_types.go` — they already exist in `risk_metrics.go` and `drawdown.go` respectively (from Tasks 2.3 and 2.1). `PerformanceResult` references them directly. `YearlyPerformance` is a type alias (`[]YearlyReturn`) rather than a new struct.
- Private `computeValueReturn` (formerly `computeSimpleReturn`) computes `end/begin - 1` on PortfolioValue — used as the TWR no-cash-flow fallback. Public `ComputeSimpleReturn` computes P&L-based return. Renamed to avoid confusion.
- 2026-05-14 (Task 3.2): **Deposits-only fallback in `ComputeNavHistory`**: For portfolios with only deposits (no positions), pre-cash-flow snapshots yield zero portfolio value because there are no positions to value and cash hasn't been deposited yet. Added a fallback: when `bp.value` is zero, use the previous equity curve point's `PortfolioValue` as the pre-cash-flow value. This correctly computes NAV for deposits-only portfolios.
- 2026-05-14 (Task 3.2): **Breakpoint computation moved earlier**: `computePreCashFlowValues` is now called before interpolation (step 10 instead of step 11) so breakpoints are available for NAV history computation. The same breakpoints are reused for both NAV and TWR, avoiding duplicate computation.
- 2026-05-14 (Task 3.2): **NAV fields in interpolation**: `interpolateDaily` now carries forward `NavPerUnit` and `Units` for non-transaction days, ensuring NAV data is present on all equity curve points.
- 2026-05-14 (Task 3.3): **Metrics computed on sliced curve**. Risk metrics, drawdown, and yearly performance are computed from the period-filtered (sliced) equity curve, not the full history. This keeps metrics aligned with the selected period — e.g., "1Y" volatility reflects only the last year. A `convertToNavPoints` helper converts `[]EquityCurvePoint` to `[]NavPoint` for the drawdown and yearly functions.

## Future Improvements
- None yet.

## Known Issues
- None yet.
