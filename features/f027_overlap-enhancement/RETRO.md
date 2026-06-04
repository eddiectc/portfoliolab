# Retrospective: Overlap Enhancement

## What Went Well

- **Comprehensive spec-to-implementation coverage**: All 13 spec scenarios (6 user stories + 7 edge cases) are implemented and tested. The sector allocation, country allocation, merged holdings, and overweight/underweight/neutral tables are all present in the template.
- **Strong test coverage**: 81 sub-tasks across 10 planned tasks, all checked off. Tests span domain computation (`allocation_test.go`, `overlap_enhanced_test.go`), service wiring (`service_test.go`), and chart serialization (`comparison_web_test.go`). Edge cases (empty portfolios, identical portfolios, missing data, zero limits) are covered.
- **Clean domain/web separation**: Computation lives in the comparison domain (`allocation.go`, `overlap_enhanced.go`), serialization in the web handler (`comparison_web.go`), display in the template. No computation logic leaked into templates.
- **NOTES.md is thorough**: Every deviation, decision, and implementation detail is documented — type placement rationale, test consolidation reasoning, dead code removal, template function additions, `roundTo2` negative number handling.
- **Graceful degradation**: Missing sector/country data accumulates into "Unknown" bucket with per-symbol warnings. Empty portfolios show 0% columns. Identical portfolios produce zero drift and all-neutral holdings. All handled without panics.
- **Builds and tests cleanly**: `go build ./...` and `go test ./...` pass without errors.

## What Could Be Improved

- **`normalizeSector` is a no-op**: The function body is `return sector` (pass-through), but the godoc says "Handles common Yahoo Finance sector name variations." Either implement the normalization or update the docstring. This was likely a placeholder from the initial implementation.
- **Template table repetition**: The sector and country allocation tables share identical structure (iterate A's keys, then B's unique keys, then Unknown row). A template partial could reduce duplication, but the current approach is clear and maintainable for the two use cases.
- **`normalizeName` not used in allocation context**: The `normalizeName` function in `overlap.go` handles name matching for holdings overlap, but sector names from different ETFs could have variations (e.g., "Information Technology" vs "Technology") that would produce separate rows. This is mitigated by the fact that Yahoo Finance is a single data source, but worth noting.
- **No integration test with real HTTP stack**: The service-level tests (`TestComputeComparison_EnhancedOverlap_*`) exercise the full domain pipeline with mock repositories, but there's no `httptest.NewRecorder` integration test verifying the rendered page includes the new sections. The template compiles and the handler tests cover serialization, but a full-stack test would catch template rendering issues.

## Spec vs Reality

| Spec Scenario | Status | Notes |
|---|---|---|
| View sector allocation for both portfolios | ✅ Implemented | Side-by-side table with portfolio names; sorted by combined weight desc |
| View sector drift chart | ✅ Implemented | Diverging bar chart (ECharts), sorted by abs diff desc |
| View country allocation for both portfolios | ✅ Implemented | Same pattern as sector |
| View country drift chart | ✅ Implemented | Top 15 limit with truncation note |
| View merged holdings table | ✅ Implemented | Shared first (overlap % desc), then unique (weight desc, interleaved) |
| View overweight holdings table | ✅ Implemented | Top 10, sorted by difference desc |
| View underweight holdings table | ✅ Implemented | Top 10, sorted by abs(difference) desc |
| View neutral holdings table | ✅ Implemented | Top 10, sorted by weight desc |
| Sector allocation with missing data | ✅ Implemented | Unknown bucket + warning per symbol |
| Country allocation with missing data | ✅ Implemented | Same pattern as sector |
| Holdings analysis with no underlying data | ✅ Implemented | Direct positions treated as their own holdings |
| Portfolio names used throughout | ✅ Implemented | All headers/labels use actual portfolio names |
| View complete enhanced overlap page | ✅ Implemented | All sections in single scrollable view |
| One portfolio is empty | ✅ Implemented | 0% for empty side, message shown, drift = full allocation |
| Both portfolios identical | ✅ Implemented | Zero drift, empty overweight/underweight, all neutral |

**No spec scenarios were missed.** All edge cases from the spec are handled.

## Plan vs Reality

| Aspect | Assessment | Notes |
|---|---|---|
| Task breakdown | ✅ Effective | 10 tasks, each independently testable. Tasks 2+3 combined in practice (same file, same pattern) — documented in NOTES.md. |
| Task sizing | ✅ Appropriate | No task was too large to complete in one focused session. The largest (Task 9: template) was well-scoped. |
| Dependencies | ✅ Accurate | The dependency graph (1 → 2-5 → 6-7 → 8-9 → 10) held up. Tasks 2-5 were done in parallel after Task 1. |
| Technical decisions | ✅ Sound | TD-1 (duplicate allocation logic): correct — keeps comparison self-contained. TD-2 (enrich PortfolioHolding): minimal change. TD-3 (extend OverlapResult): clean single access point. TD-4 (new types): properly separated. TD-5 (ECharts): consistent with existing charts. |
| Deviations | ✅ Documented | Type placement (allocation.go vs types.go), Sector field addition, test consolidation, dead code removal — all in NOTES.md. |

**Plan fidelity was high.** The only notable deviation was combining Tasks 2+3, which was a natural optimization since both functions shared the same file and test file.

## Learnings

- **Combine related tasks early**: When two tasks share a file, test file, and result-type pattern, combine them into one implementation pass. Tasks 2+3 (sector + country allocation) are structurally identical — one pass was more efficient than two.
- **Pre-compute in domain, serialize for templates**: The pattern of computing domain types (`SectorAllocationResult`, `MergedHolding`) and then serializing to JSON for chart libraries works well. It keeps templates declarative and testable.
- **Template helper functions are a necessary addition**: `weightPct` (decimal fraction → percentage) and `mapKeys` (iterate map keys) were needed beyond the existing FuncMap. Document these additions for future template work.
- **Test consolidation saves effort**: Task 10's "full pipeline" and "identical portfolios" tests were already covered by Task 7's service-level tests. Rather than duplicating, reference the existing tests. This keeps the test suite lean.
- **`roundTo2` for negative numbers**: Go's `int()` truncates toward zero, so `int(-5499.5) = -5499` (not `-5500`). The sign-extraction pattern (`sign * float64(int(v*100+0.5)) / 100`) is correct but non-obvious. Consider adding a comment or using `math.Round`.
- **Allocation breakdown values are fractions, not percentages**: `SectorAllocationResult.Breakdown` uses 0.0-1.0 fractions, but the template displays them as percentages (multiplied by 100 in `printf "%.2f"`). This is correct but could be confusing — the field name `WeightPct` on `AllocationEntry` suggests percentage but the value is a fraction.

## Action Items

- [x] Fix `normalizeSector` — updated godoc to match analysis domain's honest comment (pass-through with rationale)
- [x] Added `TestComparison_EnhancedOverlap_FullStackWithData` — seeds symbol_details with sector/geographic data, creates model portfolios, hits the web page, verifies sector names, country names, merged holdings, and overweight/underweight sections render with actual data
- [x] Rename `AllocationEntry.WeightPct` → `Weight` with inline comment clarifying it's a fraction 0.0-1.0
- [x] Add comment to `roundTo2` explaining the sign-extraction pattern for negative numbers
- [x] Evaluated: sector and country tables share identical structure but differ in header label, data path, and chart div. Go templates don't support parameterized partials cleanly (would need wrapper struct + handler wiring). Two instances is below the threshold where abstraction pays off — kept as-is.
