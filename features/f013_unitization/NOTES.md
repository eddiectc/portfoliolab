# Notes: Unitization

## Decisions
- 2026-05-13: `ComputeNavHistory` accepts `[]EquityCurvePoint` and `[]navBreakpoint` (not the plan's `snapshots, breakpoints, firstDepositIdx`). The `firstDepositIdx` parameter was eliminated — the first equity curve point is always treated as the initial deposit (fixed 10000 units), and breakpoints are only processed for points after the first. This simplifies the API and matches how the equity curve is already structured.
- 2026-05-13: `navBreakpoint` is a private type (lowercase) co-located in `unitization.go`, matching the `twrBreakpoint` pattern. It will need to be populated from the existing pre-cash-flow snapshot infrastructure when integrated in Task 3.2.

## Deviations from Plan
- None yet.

## Future Improvements
- None yet.

## Known Issues
- None yet.
