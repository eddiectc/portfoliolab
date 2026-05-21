# Notes: Portfolio Comparison

## Decisions
- 2026-05-21: `SimulateEquityCurve` uses buy-and-hold logic: `value[date] = allocated * (price[date] / basePrice)` where basePrice is the first available price for each symbol. This gives dimensionally correct portfolio values that start at the starting value on day 0.
- 2026-05-21: Extracted `stats` package (`internal/domain/stats`) with `PearsonCorrelation`, `AlignSeries`, `RoundTo2`, `RoundTo4`. Both `analysis` and `comparison` import it. Replaced duplicate implementations in `analysis/correlation.go` (pearsonCorrelation), `analysis/overlap.go` (roundTo2/4), and `comparison/metrics.go` (pearsonCorr, alignDailyReturns). Also updated `allocation.go`, `factor_exposure.go`, `stress.go` and their test files.
- 2026-05-21: `metrics.go` uses `stats.AlignSeries` for date-aligned return pairing. The thin wrapper `alignDailyReturns` converts `equityCurveDailyReturn` to string-keyed maps before delegating to `stats.AlignSeries`.
- 2026-05-21: `ComputePeriodExtremes` computes yearly/monthly returns as simple first-to-last within each period. For model portfolios (no cash flows), simple == TWR. For real portfolios, the service layer (Task 3) must TWR-normalize the equity curve (reset to 1.0 at each cash flow) before calling `ComputePeriodExtremes`, so period extremes are always time-weighted and comparable across portfolio types.
- 2026-05-21: `ComputeCAGR` uses calendar days (365/days) not trading days (252/days), matching the standard CAGR convention and `performance.ComputeAnnualizedSimpleReturn`.
- 2026-05-21: `clipPeriod` checks symbol coverage against the *requested* period, not the clipped period. This catches the case where a symbol has 1Y of data but the user asked for 3Y — the symbol is flagged as limited even though it covers the full clipped period. Additionally, `PeriodClipped` bool + warning message tells the user when the effective period was truncated.
- 2026-05-21: FX conversion uses forward-fill lookup (same pattern as `performance.fx_conversion.go`) with backward-fill for dates before the first known rate. Returns (0, false) when no FX pair exists — never the unconverted value.
- 2026-05-21: Period clipping computes the intersection of available data across all symbols. Symbols with no data are listed in warnings but the curve still includes available symbols (partial curve, not empty).
- 2026-05-21: "Limited history" is detected relative to the effective (clipped) period, not the raw data range. A symbol that covers the full clipped period is NOT flagged as limited, even if its overall history is shorter than other symbols.
- 2026-05-21: `service.go` uses a `portfolioMeta` interface (`modelPortfolioMeta`, `realPortfolioMeta`) for type dispatch in `prepareCurveForExtremes`. This avoids type switches scattered across multiple methods.
- 2026-05-21: `twrNormalizeCurve` was replaced by `buildNavCurve`. The performance layer's `EquityCurvePoint` already carries `NavPerUnit` (computed via unitization, cash-flow-independent). The comparison service now preserves `NavPerUnit` through `convertPerformanceToComparisonCurve` and uses it for period extremes instead of a crude "divide by first" normalization. Model portfolios (single initial deposit) also set `NavPerUnit == PortfolioValue` in `SimulateEquityCurve`. `NavPerUnit` is always non-nil — every portfolio has at least one deposit. This makes period extremes and return distribution properly TWR-equivalent for all portfolio types.
- 2026-05-21: `ComputeCrossPortfolioOverlap` uses Jaccard similarity (|A∩B|/|A∪B| × 100) rather than weight-based overlap. This counts unique underlying symbols, not dollar exposure. ETFs with no cached holdings are treated as atomic symbols (the ETF ticker itself appears in the expanded set) with a warning.
- 2026-05-21: `ComputeIntraPortfolioCorrelation` mirrors `analysis.ComputeCorrelation` logic (period cutoff, 80% overlap threshold, short-coverage nil cells) but takes `map[string][]HistoricalPrice` directly instead of requiring a repository call. This keeps it a pure function testable without mocks.
- 2026-05-21: The `OverlapResult` type in `comparison/types.go` (Task 3) was reused for `ComputeCrossPortfolioOverlap` (Task 4) — same struct, same field names. This avoids duplication and means the API layer can return both cross-portfolio overlap and intra-portfolio overlap through the same type.

## Deviations from Plan
- None yet.

## Future Improvements
- Consider interpolating missing daily prices (e.g., forward-fill from last known) so the curve doesn't have gaps when symbols have non-overlapping trading days.

## Known Issues
- None.
