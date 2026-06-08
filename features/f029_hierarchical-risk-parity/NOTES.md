# Notes: Hierarchical Risk Parity

## Decisions
- 2026-06-08: Plan approved. ECharts tree chart for dendrograms, sqrt(2*(1-corr)) distance metric, cross-covariance-minimizing bisection split, analytical min-variance in bisection, returns/correlation implemented in HRP package (not shared), ticker-only display in weights table, base currency selector included.

## Deviations from Plan
—

## Post-Review Fixes (Task 6)
- 2026-06-08: `computeDataSpan` now emits per-symbol warnings when data span is shorter than ~80% of expected trading days for the requested period (spec scenario: "Warning — some symbols with insufficient historical data").
- 2026-06-08: `engineReq.Symbols` now preserves the original request order (filtered to symbols with data) instead of iterating a map.
- 2026-06-08: Added `expectedTradingDays` helper and table-driven test.
- 2026-06-08: `GetSymbolsFromModelPortfolio` now returns empty slice when source is nil, consistent with `GetCandidateSymbols` and `GetSymbolsFromPortfolio`.
- 2026-06-08: Confirmed `HistoricalPrice` has only `Close` (no Open/High/Low) — FX conversion of all price fields is already complete.

## Task Completion
- 2026-06-08: Task 8 (API handler) completed. Created `internal/api/handlers/hrp.go` with five endpoints: `POST /api/hrp/compute` (2-20 symbols, default period 3Y, optional base_currency), `GET /api/hrp/symbols`, `GET /api/hrp/portfolio/{id}/symbols`, `GET /api/hrp/model-portfolio/{id}/symbols`, `POST /api/hrp/save`. Follows efficient frontier handler pattern exactly — same `modelPortfolioCreator` interface, same weight conversion (fraction→percentage), same error handling. Handler tests (22 tests) cover success, validation, service errors, empty states, and route registration.
- 2026-06-08: Task 7 (Data adapters) completed. Created `internal/data/hrp_adapters.go` with four adapter types: `HrpSymbolListerImpl`, `HrpPortfolioSymbolSourceImpl`, `HrpModelPortfolioSourceImpl`, `HrpFxRateSourceImpl`. Design decision: the HRP adapters delegate to the existing efficient frontier adapters rather than duplicating the underlying queries — `HrpSymbolListerImpl` wraps `SymbolListerImpl`, `HrpPortfolioSymbolSourceImpl` wraps `PortfolioSymbolSourceImpl`, and the two type-specific adapters (`HrpModelPortfolioSourceImpl`, `HrpFxRateSourceImpl`) convert between `efficientfrontier.ModelPortfolioRef`/`FxRate` and the HRP equivalents. The HRP interfaces for `MarketDataHistorySource` and `MarketDataSymbolResolver` are satisfied directly by `marketservice.Service` and `data.MarketDataSymbolResolverImpl` respectively (identical signatures), so no HRP-specific wrapper is needed for those. Added compile-time interface checks and full build verified.
- 2026-06-08: Task 6 (Service layer) completed. `ComputeHrp` service mirrors the efficient frontier pattern: resolve symbols → fetch prices → FX conversion → delegate to engine. Key difference: engine errors (insufficient symbols, insufficient data, numerical failure) are converted to empty-state `ServiceResult` with a `Message` field instead of propagated as errors — this lets the UI display user-friendly messages rather than error pages. Default period is `3Y` (vs efficient frontier's `1Y`).
- 2026-06-08: Task 5 (HRP computation engine) completed. `ComputeHrp` orchestrates the full pipeline: validate → align returns → correlation → distance → cluster × 4 → quasi-diagonalize → recursive bisection → result. Added `computeCovarianceMatrix` (sample covariance formula) for the bisection step, which needs covariance (not correlation) for portfolio variance calculations. Added `replaceLeafNames` to walk dendrogram trees and replace index strings with actual symbol names. Added `clusterByMethod` dispatcher.
- 2026-06-08: Task 2 (Math utilities) completed. `AlignReturns` skips symbols with insufficient data silently (continue) rather than returning a per-symbol error — the service layer (Task 6) is responsible for collecting warnings per symbol. Integration test deferred to Task 11 (router wiring) where the full stack (DB → service → handler → response) is exercised.
- 2026-06-08: Task 3 (Hierarchical clustering) completed. Used a distance-matrix approach with Lance-Williams update formula instead of explicit `Cluster` structs — cleaner and more efficient. The `Cluster` struct from the plan was dropped as unnecessary. Leaf names in dendrogram nodes are set to index strings ("0", "1", ...) and will be replaced with actual symbol names by the HRP engine (Task 5).
- 2026-06-08: Task 4 (Quasi-diagonalization and recursive bisection) completed. `QuasiDiagonalize` maintains ordered lists per cluster during merge processing — avoids building then traversing the tree. Capital allocation uses standard HRP inverse-variance weighting (w_group = (1/σ²_group) / Σ(1/σ²)), not equal-risk-split as the plan originally described; this is the correct López de Prado approach. Matrix inversion and regularization duplicated from efficient frontier (kept in HRP package per plan decision). Fallback chain: direct inversion → regularized inversion → equal weights.

## Future Improvements
—

## Known Issues
—
