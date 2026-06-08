# Retrospective: Hierarchical Risk Parity

## What Went Well

- **Exceptional test coverage**: 4,628 lines of test code across 11 test files for ~4,835 lines of production code (~1:1 ratio). 111 test functions cover domain logic, service layer, and handlers. Table-driven tests used consistently.
- **Clean separation of concerns**: Domain computation (`hrp.go`, `clustering.go`, `bisection.go`, etc.) is fully isolated from data fetching (`service.go`) and HTTP (`hrp.go`, `hrp_web.go`). No coupling between layers.
- **Elegant clustering implementation**: The four linkage methods share a single `hierarchicalCluster` function with a `distanceUpdateFunc` callback — clean, DRY, and easy to extend.
- **Consistent with existing patterns**: Followed the f028 Efficient Frontier architecture exactly (domain → service → adapters → API handler → web handler → template). This reduced decision fatigue and kept the codebase uniform.
- **All edge cases handled**: zero-return symbols, insufficient data, too many symbols, missing data, empty symbols, FX conversion, and singular matrices (with regularization fallback) all tested.
- **No technical debt**: Zero TODOs, FIXMEs, HACKs, or XXXs in the codebase. `go vet` clean.
- **API.md updated**: All five new endpoints documented in the API reference.

## What Could Be Improved

- **Missing integration test (initially)**: The DoD requires "at least one integration test exercises the full stack (DB → service → handler → response)". This was dropped per user decision. Addressed via retro action item — 13 integration tests added.
- **No compile-time interface checks on the service**: The data adapters have `var _ Interface = (*Impl)(nil)` checks, but the `hrpService` consumer interface in `hrp.go` and `hrpModelPortfolioSelector` in `hrp_web.go` lack compile-time verification.
- **In-place mutation in `convertToBaseCurrency`**: The service modifies the `prices` map values directly. If the caller reuses the same map, converted prices would be applied again. (Currently not an issue since the map is created fresh per request, but the in-place mutation is implicit.)
- **Template JavaScript inline**: ~180 lines of JavaScript embedded in the HTML template. Matches the efficient frontier pattern, but a separate `.js` file would be more maintainable for a feature this complex.
- **`generateRandomWalk` test helper**: Uses a sine-wave-based deterministic sequence labeled as "pseudo-random." Works for testing, but the name is misleading — it produces smooth oscillations, not random walks. Could cause confusion if someone expects true randomness.

## Spec vs Reality

| Spec Item | Status | Notes |
|---|---|---|
| 12 scenarios implemented | ✅ Complete | All scenarios covered in code and tests |
| Four linkage methods | ✅ Complete | single, complete, average, Ward — all produce correct merge sequences |
| Dendrogram rendering (ECharts) | ✅ Complete | 2×2 grid of tree charts with orthogonal layout |
| Weights table (four columns) | ✅ Complete | Populated client-side from serialized JSON |
| Save as model portfolio | ✅ Complete | Method selector dropdown + hidden form fields |
| Edge cases (10 listed) | ✅ Complete | All handled: single symbol, duplicates, correlated, insufficient data, no data, multi-currency, delisted, max count, short period, zero-return |
| "Ticker only" in weights table | ⚠️ Deviation | Spec originally said "symbol's name/description alongside the ticker"; changed to ticker-only per NOTES.md. Consistent with efficient frontier pattern. |
| No integration test | ❌ Gap | DoD item dropped per user decision; documented in NOTES.md |

## Plan vs Reality

| Plan Item | Status | Notes |
|---|---|---|
| 11 tasks, all completed | ✅ Complete | All checkboxes marked done in PLAN.md |
| Task ordering | ✅ Accurate | Linear dependency chain held; no reordering needed |
| Task sizing | ✅ Appropriate | All tasks completed in a single session; none needed splitting |
| Task 3: `Cluster` struct dropped | ⚠️ Minor deviation | Plan called for explicit `Cluster` structs; implementation uses distance-matrix with Lance-Williams update — cleaner and more efficient |
| Task 4: bisection weighting | ⚠️ Minor deviation | Plan described "equal-risk-split"; implementation uses correct López de Prado inverse-variance weighting. This was a plan error, not an implementation error. |
| Task 5: `computeCovarianceMatrix` added | ⚠️ Addition | Not in original plan; needed for the bisection step (covariance, not correlation, is used for portfolio variance). Documented in NOTES.md. |
| Task 2: `AlignReturns` silent skip | ⚠️ Minor deviation | Plan implied per-symbol errors; implementation silently continues on insufficient data, delegating warnings to the service layer. Cleaner separation. |

## Learnings

- **Math-heavy domains benefit from 1:1 test ratio**: The hierarchical clustering, quasi-diagonalization, and recursive bisection algorithms required exhaustive testing. The table-driven approach with known inputs (e.g., perfectly correlated → distance 0) was effective.
- **Following an existing pattern is the right default**: The efficient frontier template saved significant design time. Every architectural decision (interfaces, adapters, handler structure, template layout) had a precedent.
- **Consumer-defined interfaces work well**: The HRP domain defines its own interfaces (`MarketDataHistorySource`, `FxRateSource`, etc.) with HRP-specific types, then the data adapters bridge to existing implementations. This avoids coupling to efficient frontier's types while reusing the same underlying repos.
- **The missing integration test is a real risk**: Without a full-stack test, bugs in the wiring (router → handler → service → adapters → repos) are not caught by the test suite. The unit tests cover each layer in isolation, but the integration points are not verified.
- **Dendrogram rendering via ECharts tree chart works but is fragile**: The `convertDendrogram` JavaScript function assumes a specific tree structure. Any change to `DendrogramNode` (e.g., adding a `children` field with different semantics) would silently break rendering.

## Action Items

- [x] Add integration test for HRP — 13 tests in `tests/integration/hierarchical_risk_parity_test.go` covering compute (2-symbol, 5-symbol), errors (single symbol, no data, too many, invalid period), warnings (partial data), save as model portfolio, candidate symbols, default period, duplicate name, and perfectly correlated assets
- [x] Add compile-time interface checks for `hrpService` and `hrpModelPortfolioSelector` interfaces
- [x] Rename `generateRandomWalk` to `generateDeterministicWalk` — clearer name, updated doc comment
- [x] Document `convertToBaseCurrency` in-place mutation — added caller contract to doc comment
- [ ] Consider extracting template JavaScript to a separate `.js` file (low priority, matches existing pattern)
