# Retrospective: Vanguard Data Extractor (f025)

**Date**: 2026-06-01
**Feature**: f025_vanguard-scraper
**Status**: Complete

---

## What Went Well

- **All 7 tasks completed** with checkboxes checked off in PLAN.md. Implementation covers the full stack: types, migration, extractor, service mapping, repo serialization, web display, and registration + integration tests.

- **Strong test coverage**: 42+ unit tests across the vanguard package (URL matcher, 7 parsers, slug extraction, 8 extractor scenarios) plus 13 integration tests covering routing, API response, web rendering, NAV data, provider switching, and background refresh. All pass cleanly.

- **Clean architecture**: The vanguard package has a clear separation — `client.go` (HTTP + rate limiting), `matcher.go` (URL matching), `parsers.go` (JSON parsing for both REST and GraphQL), `extractor.go` (orchestration). Follows the WisdomTree/DWS pattern while adding GraphQL capability.

- **GraphQL as first**: This is the first extractor to use GraphQL (POST with structured queries). The approach of defining query strings as constants and using typed response structs is clean and maintainable.

- **Pagination handled transparently**: Holdings pagination (1500/page, `lastItemKey` as JSON string) is handled internally by the extractor. The caller sees a flat list — no pagination awareness required upstream.

- **Zero TODOs/FIXMEs**: The codebase is clean with no unresolved todos or temporary workarounds.

- **NOTES.md is thorough**: Documents all design decisions, task-by-task completion notes, cross-layer audits, and post-completion fixes. Excellent reference for future maintainers.

- **Backward compatibility maintained**: Existing WisdomTree/DWS/iMGP extractors unaffected. Old-format JSON deserializes correctly with new fields empty/nil.

---

## What Could Be Improved

- **Post-completion fixes indicate brittle API assumptions**: 5 fix commits after the feature was declared done:
  1. `borHoldings` is an array in the GraphQL response (not a single object)
  2. `annualNAVReturns` type changed from `[]interface{}` to `interface{}`
  3. Characteristics query requires sub-fields on each code (bare code names return HTTP 500)
  4. Country allocation needed `FTCTYATPCS` filtering (duplicates and subtotals were displayed)
  5. HTTP error messages enhanced to include response body

  These were all structural differences between the API research assumptions and actual responses. **Lesson**: When reverse-engineering an undocumented API, validate response shapes with real data before declaring the feature done, rather than relying on sample JSON from research.

- **Test gap — `TestExtractor_Extract_MissingEffectiveDate`**: The plan called for a dedicated end-to-end test for "holdings without effectiveDate → fails". The scenario is covered at the parser level (`TestParseHoldings/missing_effectiveDate`) and in the `fetchAllHoldings` code path, but there is no dedicated `extractor_test.go` case. The `TestExtractor_Extract_EmptyHoldings` test covers empty holdings (zero items, no effectiveDate), which is a related but distinct scenario. **Low risk** — the code path is exercised, just not as a named test case.

- **Integration tests don't exercise real extraction**: The integration tests mock the data (insert directly into DB) rather than exercising the actual extraction pipeline against the live API. This is pragmatic (network dependency) but means the "full stack round-trip" test is really a "data persistence" test. Consider adding a CI job that runs the real extraction against a known fund (e.g. VWRL) periodically to catch API changes.

- **Hardcoded NAV date range**: The NAV history query uses `startDate: "2020-01-01"` hardcoded in `extractor.go`. This works for the current use case but is not configurable. If a user wants a different range, the code must be changed.

- **Spec still marked "Draft"**: The SPEC.md status field says "Draft" rather than "Approved" or "Final". Minor but worth noting for process hygiene.

---

## Spec vs Reality

| Spec Element | Status | Notes |
|---|---|---|
| Story 1: Fund identity/profile | ✅ Implemented | REST Phase 1 extracts all fields |
| Story 2: Holdings extraction | ✅ Implemented | Full pagination, optional bond fields |
| Story 3: Sector + country with benchmark | ⚠️ Partial | Benchmark % parsed but **excluded from types** (documented decision) |
| Story 4: Fund characteristics | ✅ Implemented | Equity + bond metrics, presence bitmask |
| Story 5: NAV + price history | ⚠️ Partial | NAV only — market prices excluded (documented decision) |
| Story 6: Error handling | ✅ Implemented | Atomic two-phase, explicit errors |
| Story 7: Extractor framework integration | ✅ Implemented | Registered in dispatcher, no core changes needed |
| Story 8: Web display | ✅ Implemented | All new fields rendered, conditional bond section |

**Key deviations** (all documented in SPEC.md "Implementation Decisions and Deviations" and NOTES.md):

1. **Benchmark comparison excluded from types**: The spec requires `benchmarkPercent` on sectors and `benchmarkMktPercent` on countries. These are parsed from the GraphQL response but deliberately excluded from type definitions. Rationale: storing benchmark alongside fund data adds complexity without clear display benefit.

2. **Market price history excluded**: The spec requires market prices per exchange listing. Excluded — market prices continue to come from Yahoo Finance. NAV flows through `market_data` with `data_type='nav'`.

3. **Country allocation filtering**: The spec says "all countries are extracted and stored". The implementation filters to `FTCTYATPCS` (FTSE Country of Risk) only, excluding `MSCTYATPCS` (Market of Domicile duplicates) and `SASTTYPPC` (region subtotals). This was discovered during implementation — without filtering, duplicate countries appeared with slightly different weights.

4. **Holdings section header**: Spec called for "Full Holdings". Implementation keeps "Top 10 Holdings" with expand link — consistent regardless of data source.

5. **Country allocation sort**: Spec implied region grouping. Implementation sorts by percent descending with a Region column for identification.

---

## Plan vs Reality

| Plan Aspect | Assessment |
|---|---|
| **Task breakdown** | ✅ Effective — 7 sequential tasks, each independently testable |
| **Task sizing** | ✅ Appropriate — Task 3 (extractor package) was the largest but manageable |
| **Dependencies** | ✅ Accurate — Task 1 → Task 3 dependency was correct; Task 2 (migration) was lightweight |
| **Task 5 (repo serialization)** | ⚠️ Over-estimated — the repo uses `json.Marshal` on whole structs, so all new fields automatically flow through. No field-by-field mapping was needed. The plan accounted for this but the task still required round-trip tests. |
| **Technical decisions** | ✅ All 7 decisions documented and applied correctly |
| **Post-completion fixes** | ❌ 5 fix commits after completion — the plan's "Verification" criteria for Task 3 didn't catch API response shape mismatches |

**Task count**: 7 tasks planned, 7 tasks completed. No tasks added or removed during implementation.

---

## Learnings

1. **Reverse-engineered API validation**: When the source is an undocumented API, validate response shapes with real data (not just sample JSON from research) before declaring a task done. The 5 post-completion fixes were all structural mismatches between expected and actual response shapes.

2. **GraphQL query specificity**: GraphQL queries must specify sub-fields for every code (e.g. `PERATIO { analyticValue effectiveDate __typename }`). A bare code name returns HTTP 500. This is a GraphQL-specific gotcha not present in REST scraping.

3. **JSON column flexibility confirmed**: The existing `json.Marshal` / `json.Unmarshal` approach on whole structs means new fields on existing types automatically flow through the repo layer without field-by-field mapping. Task 5 was essentially a validation exercise rather than an implementation task.

4. **Presence bitmask pattern**: The `FieldsPresent` bitmask on `FundCharacteristics` (introduced in f024) proved essential for distinguishing "field not applicable" (null) from "field zero" (zero value). The conditional `EquityValuation` / `BondCharacteristics` mapping in the service layer uses this pattern effectively.

5. **Country allocation stat codes**: The `holdingStatCode` field in market allocation responses requires filtering. Without it, duplicate countries and region subtotals pollute the display. This was a research gap — the RESEARCH.md documented the codes but the initial implementation didn't filter.

6. **Pagination key as JSON string**: The `lastItemKey` in holdings pagination is a JSON string (not an object), which required careful handling. The `checkHasMore` helper function cleanly encapsulates this.

---

## Action Items

- [x] **Make NAV date range configurable** — Done. Extracted `2020-01-01` hardcoded date to `DefaultNavHistoryDays` constant (730 days / 2 years) with `WithNavHistoryDays(int)` option function. Start date is now computed relative to today.
- [ ] **Add API smoke test**: Not needed for now.
- [ ] **Add `TestExtractor_Extract_MissingEffectiveDate`**: Named end-to-end test for the missing effectiveDate scenario in `extractor_test.go` (currently covered at parser level only).
- [ ] **Update SPEC.md status**: Change from "Draft" to "Complete" or "Approved".
- [ ] **Document benchmark comparison as future feature**: Not needed.
