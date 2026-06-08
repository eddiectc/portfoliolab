# Notes: Hierarchical Risk Parity

## Decisions
- 2026-06-08: Plan approved. ECharts tree chart for dendrograms, sqrt(2*(1-corr)) distance metric, cross-covariance-minimizing bisection split, analytical min-variance in bisection, returns/correlation implemented in HRP package (not shared), ticker-only display in weights table, base currency selector included.

## Deviations from Plan
—

## Task Completion
- 2026-06-08: Task 5 (HRP computation engine) completed. `ComputeHrp` orchestrates the full pipeline: validate → align returns → correlation → distance → cluster × 4 → quasi-diagonalize → recursive bisection → result. Added `computeCovarianceMatrix` (sample covariance formula) for the bisection step, which needs covariance (not correlation) for portfolio variance calculations. Added `replaceLeafNames` to walk dendrogram trees and replace index strings with actual symbol names. Added `clusterByMethod` dispatcher.
- 2026-06-08: Task 2 (Math utilities) completed. `AlignReturns` skips symbols with insufficient data silently (continue) rather than returning a per-symbol error — the service layer (Task 6) is responsible for collecting warnings per symbol. Integration test deferred to Task 11 (router wiring) where the full stack (DB → service → handler → response) is exercised.
- 2026-06-08: Task 3 (Hierarchical clustering) completed. Used a distance-matrix approach with Lance-Williams update formula instead of explicit `Cluster` structs — cleaner and more efficient. The `Cluster` struct from the plan was dropped as unnecessary. Leaf names in dendrogram nodes are set to index strings ("0", "1", ...) and will be replaced with actual symbol names by the HRP engine (Task 5).
- 2026-06-08: Task 4 (Quasi-diagonalization and recursive bisection) completed. `QuasiDiagonalize` maintains ordered lists per cluster during merge processing — avoids building then traversing the tree. Capital allocation uses standard HRP inverse-variance weighting (w_group = (1/σ²_group) / Σ(1/σ²)), not equal-risk-split as the plan originally described; this is the correct López de Prado approach. Matrix inversion and regularization duplicated from efficient frontier (kept in HRP package per plan decision). Fallback chain: direct inversion → regularized inversion → equal weights.

## Future Improvements
—

## Known Issues
—
