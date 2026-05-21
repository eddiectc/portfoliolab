# Notes: Portfolio Comparison

## Decisions
- 2026-05-21: `SimulateEquityCurve` uses buy-and-hold logic: `value[date] = allocated * (price[date] / basePrice)` where basePrice is the first available price for each symbol. This gives dimensionally correct portfolio values that start at the starting value on day 0.
- 2026-05-21: `clipPeriod` checks symbol coverage against the *requested* period, not the clipped period. This catches the case where a symbol has 1Y of data but the user asked for 3Y — the symbol is flagged as limited even though it covers the full clipped period. Additionally, `PeriodClipped` bool + warning message tells the user when the effective period was truncated.
- 2026-05-21: FX conversion uses forward-fill lookup (same pattern as `performance.fx_conversion.go`) with backward-fill for dates before the first known rate. Returns (0, false) when no FX pair exists — never the unconverted value.
- 2026-05-21: Period clipping computes the intersection of available data across all symbols. Symbols with no data are listed in warnings but the curve still includes available symbols (partial curve, not empty).
- 2026-05-21: "Limited history" is detected relative to the effective (clipped) period, not the raw data range. A symbol that covers the full clipped period is NOT flagged as limited, even if its overall history is shorter than other symbols.

## Deviations from Plan
- None yet.

## Future Improvements
- Consider interpolating missing daily prices (e.g., forward-fill from last known) so the curve doesn't have gaps when symbols have non-overlapping trading days.

## Known Issues
- None.
