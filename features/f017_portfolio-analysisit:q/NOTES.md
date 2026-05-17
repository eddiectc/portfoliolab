# Notes: Portfolio Analysis

## Decisions
- 2025-05-17: Task 1 complete — domain types defined in `internal/domain/analysis/analysis_types.go`. All types use `float64` for percentage/weight fields (consistent with `symbol.SymbolDetails` which stores percentages as float64). `EstimatedDollarImpact` uses `decimal.Decimal` since it represents an actual monetary amount.
- 2025-05-17: `PositionWithDetails` was already defined in Task 1 (`analysis_types.go`), so Task 2 reused it without modification.
- 2025-05-17: `roundTo2` helper uses `int(v*100+0.5)/100` for float64 rounding — simple, no math library dependency, consistent with display-only precision.

## Deviations from Plan
- Task 2: Plan said single ETF returns empty for both pairwise and concentrated stocks. Changed to still compute concentrated stocks for 1 ETF — useful to see what a single ETF is most concentrated in, even without pairwise comparison. Pairwise still requires 2+ ETFs.

## Future Improvements
- N/A

## Known Issues
- None.
