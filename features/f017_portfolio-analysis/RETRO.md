# Retrospective: Portfolio Analysis (f017)

## What Went Well

- **Modular architecture delivered cleanly**: The `analysis` domain package with pure computation functions (overlap, correlation, allocation, stress test, factor exposure) separated from the orchestrating service is a strong pattern. Each computation function is independently testable and has no external dependencies.
- **Comprehensive test coverage**: 181 tests across the analysis domain and 21 handler tests, all passing. Table-driven tests with sub-tests for helper functions (22 sub-tests alone for factor exposure helpers) provide thorough edge-case coverage.
- **Graceful degradation handled throughout**: Every section handles missing data with warnings, "Unknown" buckets, and empty-state messages — matching the spec's robustness requirements. The silent-failure audit in Task 7 caught 9 fallback paths that would have hidden errors from users.
- **API-first architecture maintained**: Web handler delegates to `computeResult()` on the API handler, which delegates to the service. No computation duplication between layers.
- **Schema migration handled mid-feature**: Adding the `sector` column to `symbol_details` (migration 018) was needed for individual stock sector classification. This was identified during Task 4 and resolved cleanly without blocking other tasks.
- **Feature index updated**: `features/README.md` correctly marks f017 as done with dependency on f009, f010, f011, f015, f016.

## What Could Be Improved

- **Task ordering was suboptimal**: Tasks 8 (API handler) and 9 (web handler) were completed *before* Task 7 (service). The service is the core orchestrator that both handlers depend on. This worked because the handlers use interfaces, but it means the handlers couldn't be end-to-end tested until the service was complete. Future features should complete the service layer before handlers.
- **Factor exposure scope expanded mid-implementation**: The spec and plan only described value/growth, size, concentration, and top holding. Quality, cost, momentum, and volatility factors were added during Task 6 without a spec update. While these are valuable additions, they should have been raised as a spec amendment before implementation.
- **Correlation period options expanded beyond spec**: The spec listed 1Y, 3Y, 5Y, 10Y. Implementation added 3M and 6M for symbols with limited history. This was a practical improvement but wasn't reflected in the spec.
- **Stress scenarios JSON location changed**: Plan specified `internal/config/data/stress_scenarios.json`, but `//go:embed` constraints required co-location with Go code. This was documented in NOTES.md but could have been caught during planning.
- **No integration tests for the analysis feature**: All 202 tests are unit tests. There are no integration tests exercising the full stack (DB → repo → service → handler → response) for analysis. The existing integration test suite covers migrations and other domains but not this feature.

## Spec vs Reality

| Spec Aspect | Match? | Notes |
|---|---|---|
| 18 scenarios covered | ✅ | All 18 scenarios have corresponding unit/service tests |
| 5 analytical lenses | ✅ | Overlap, correlation, sector, geographic, stress test, factor exposure |
| Section filtering | ✅ | `?section=` query param works correctly |
| Empty state handling | ✅ | All sections show messages when no data |
| Warnings for missing data | ✅ | Warnings collected from all sections and data-fetching helpers |
| Nullable sections | ✅ | Sections use `*Type` pointers with `omitempty` |
| `ComputedAt` + `PortfolioID` metadata | ✅ | Present in `AnalysisResult` |
| ETF overlap pairwise matrix | ✅ | Symmetric matrix, overlapping count + combined weight |
| Top 10 concentrated stocks | ✅ | Sorted by total weight descending |
| Correlation with lookback period | ✅ | 6 period options (3M, 6M, 1Y, 3Y, 5Y, 10Y) |
| Correlation insufficient data warning | ✅ | 80% adaptive threshold (spec said fixed 60 days) |
| Sector allocation ETF look-through | ✅ | Weighted by ETF portfolio weight × sector % |
| Geographic allocation ETF look-through | ✅ | Same pattern as sector |
| "Unknown" bucket for missing data | ✅ | Both sector and geographic |
| 6 stress test scenarios | ✅ | 2008 GFC, 2000 Dot-Com, 2020 COVID, 2022, 1997 Asian, 2011 European |
| Stress test sorted by severity | ✅ | Most negative return first |
| Factor exposure proxy | ✅ | Value/growth, size, concentration, top holding |
| Factor exposure with insufficient data | ✅ | Partial computation with warnings |
| Single-stock portfolio handling | ✅ | Overlap shows message, other sections work |
| Background refresh for stale data | ✅ | Fire-and-forget goroutine, 7-day threshold |
| Performance constraint (5s for ≤50 positions) | ⚠️ | Not measured/verified — no benchmark test |

**Spec gaps (implementation went beyond spec):**
- **4 additional factor metrics** (quality, cost, momentum, volatility) not in the original spec
- **2 additional correlation periods** (3M, 6M) not in the original spec
- **Cash position filtering** — spec didn't mention cash positions; implementation filters `$`-prefixed symbols
- **Short coverage → null cells** — post-completion fix that improves data quality beyond spec

## Plan vs Reality

| Plan Aspect | Match? | Notes |
|---|---|---|
| 11 tasks | ✅ | All 11 tasks completed and checked |
| Task dependency ordering | ⚠️ | Tasks 8-9 completed before Task 7 (service). Interfaces made this work but reversed the logical dependency |
| Pure computation functions | ✅ | overlap.go, correlation.go, allocation.go, stress.go, factor_exposure.go are all pure |
| Service orchestrates data fetching | ✅ | 7 interfaces, nil-tolerant, background refresh |
| API handler with section/period validation | ✅ | 21 tests, validates inputs with helpful error messages |
| Web handler + template with ECharts | ✅ | Full page with all 6 sections, charts, tables, empty states |
| Router wiring + nav link | ✅ | Analysis link in nav, routes registered |
| Validation (Task 11) | ✅ | All tests pass, go vet clean, no TODOs |
| float64 for correlation | ✅ | Documented justification in plan and NOTES.md |
| JSON file for stress scenarios | ✅ | Co-located with Go code (plan said config/data) |
| Hand-written mocks | ✅ | Service and handler tests use hand-written mocks |

**Deviations from plan (documented in NOTES.md):**
1. **Single ETF concentrated stocks**: Plan said single ETF returns empty for both pairwise and concentrated stocks. Changed to still compute concentrated stocks for 1 ETF — useful UX improvement.
2. **Correlation threshold**: Plan specified fixed `< 60 days`. Changed to 80% of expected period — scales correctly with period length.
3. **Stress scenarios JSON location**: Moved from `internal/config/data/` to co-located with Go code due to `//go:embed` constraints.
4. **Factor exposure signature**: Plan specified `portfolioValue` parameter; omitted as unused.
5. **No response DTO**: Plan mentioned separate DTO; `AnalysisResult` serves as both domain type and response DTO.
6. **Interface on handler**: Plan specified `*analysis.Service`; changed to `analysisService` interface for mockable tests.

## Learnings

- **Service-before-handlers ordering matters**: Even with interfaces, completing handlers before the service prevents end-to-end testing. The service should be Task N and handlers should be Task N+1.
- **Spec scope for factor-based features is easy to underestimate**: The original spec listed 4 factor metrics; implementation delivered 8. This is a positive outcome but should be captured in the spec before implementation to avoid "did we deliver what was specified?" questions.
- **`//go:embed` has directory constraints**: Files must be in the same directory or a subdirectory of the Go source. Plan this during the planning phase, not during implementation.
- **Silent failure audit is worth the effort**: The 9 silent-failure fixes in Task 7 significantly improved observability. Consider making this a standard checklist item in future features.
- **Schema prerequisites should be identified earlier**: The `sector` column addition (migration 018) was needed mid-Task 4. This should have been identified during Task 1 (types) or as a prerequisite task.
- **float64 rounding for negative values needs `math.Round`**: The initial `int(v*100+0.5)/100` formula truncated toward zero for negatives. `math.Round` handles both correctly. This is a subtle bug that the correlation tests caught.

## Action Items

- [ ] Add a benchmark test for analysis computation time (spec constraint: 5s for ≤50 positions)
- [x] Add at least one integration test for the analysis feature (full stack: DB → service → handler → response) — `tests/integration/analysis_test.go`, 8 tests
- [x] Add "silent failure audit" as a standard checklist item in future feature plans — added to DoD.md
- [ ] Identify schema/data prerequisites during the planning phase, not mid-implementation
- [ ] Update spec before expanding scope (factor metrics, correlation periods) — or document as a spec amendment in NOTES.md
- [ ] Consider service-before-handlers task ordering as a default convention
