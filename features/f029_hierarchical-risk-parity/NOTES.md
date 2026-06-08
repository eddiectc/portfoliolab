# Notes: Hierarchical Risk Parity

## Decisions
- 2026-06-08: Plan approved. ECharts tree chart for dendrograms, sqrt(2*(1-corr)) distance metric, cross-covariance-minimizing bisection split, analytical min-variance in bisection, returns/correlation implemented in HRP package (not shared), ticker-only display in weights table, base currency selector included.

## Deviations from Plan
—

## Task Completion
- 2026-06-08: Task 2 (Math utilities) completed. `AlignReturns` skips symbols with insufficient data silently (continue) rather than returning a per-symbol error — the service layer (Task 6) is responsible for collecting warnings per symbol. Integration test deferred to Task 11 (router wiring) where the full stack (DB → service → handler → response) is exercised.

## Future Improvements
—

## Known Issues
—
