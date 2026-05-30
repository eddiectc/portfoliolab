# Retrospective: iM Global Partner (iMGP) Scraper

**Feature ID**: f024
**Date**: 2026-05-30
**Status**: Draft

## What Went Well

- **Clean reuse of existing patterns**: The extractor dispatcher (`FindByURL`), `data_source_url` column, CycleTLS client, and `goose` + `sqlc` migration workflow were all reused without modification. Zero changes to core routing logic — exactly as planned.
- **Silent fallback audit caught a real bug**: The `FieldsPresent` bitmask on `RiskMeasures` and the `extractShareClass` error return were added during a proactive audit. Without this, parsed `Isin`, `ShareClassName`, and `OngoingCharges` fields would have been silently dropped in `extractResultToSymbolDetails`. This is a concrete quality win.
- **PDF parsing is resilient**: The text-based regex approach handles PDF extraction artefacts (split labels like "Other" + "DM FX", multi-line region labels like "Cash\n&\nOthers", missing percentages) gracefully. The `mergeSplitLabels` and `extractRegionLabelsAndPercentages` helpers are well-designed for the messy reality of PDF text extraction.
- **Comprehensive test coverage**: 20+ table-driven test cases across parsers, matcher, client, and extractor. Missing-section and count-mismatch edge cases are explicitly tested. The fixture-based approach (extracted PDF text saved as `testdata/`) is fast and deterministic.
- **Atomic extraction implemented correctly**: Required sections (Fund Facts + Reference Date) fail the entire extraction; optional sections (risk, allocations) return partial data with warnings. This matches the spec exactly.

## What Could Be Improved

- **Missing repo round-trip tests for new fields**: Task 6 planned "Update `symbol_details_repo_test.go` with tests for new fields (serialization round-trip)" but no explicit test was added for the four new JSON columns (`risk_measures`, `asset_class_allocation`, `equity_derivatives_by_region`, `currency_derivatives_allocation`). The `symbol_details_test.go` in the `symbol` package has JSON serialization tests, but the repo layer (which does the actual `json.Marshal`/`json.Unmarshal`) is not tested for these fields. Add a round-trip test.
- **No end-to-end integration test**: Task 8 planned "Add a real integration test (`real_test.go`)" but the file only tests parsers against the embedded fixture text — it doesn't exercise the full stack (DB → repo → service → handler). Add an integration test using `httptest` + in-memory SQLite that verifies the full flow from extraction through display.
- **English-only PDF pattern**: The `extractPDFURL` regex matches `FACTSHEETS_EN.pdf` only. The spec mentions "PDF language variants" as an edge case, but the implementation doesn't support non-English factsheets. This is acceptable since the spec says the URL determines language, but the regex is hardcoded to `_EN.pdf`. Consider making the language suffix configurable or matching any `_FACTSHEETS_*.pdf`.
- **`extractPDFURL` uses regex, not HTML parsing**: The plan says "try multiple selectors (href patterns, data attributes)" as mitigation for HTML structure changes, but the implementation uses a single regex. If iMGP changes their link format (e.g., data attribute, JavaScript-rendered link), extraction breaks. Consider adding a fallback pattern.

## Spec vs Reality

| Spec Item | Status | Notes |
|---|---|---|
| Story 1: iMGP data extraction | ✅ Implemented | All data sections (Fund Facts, Risk Measures, 3 portfolio tiers) extracted correctly |
| Story 1: Atomic extraction | ✅ Implemented | Fund Facts + Reference Date required; optional sections may be absent |
| Story 1: Sample PDF test | ✅ Implemented | Fixture-based tests verify all parsers against real PDF text |
| Story 2: Dispatcher registration | ✅ Implemented | Registered via `extractorReg.Register(imgp.NewExtractor())` in `router.go` |
| Story 2: No core routing changes | ✅ Implemented | Reuses existing `FindByURL` mechanism |
| Story 2: Storage extensions | ✅ Implemented | 4 new TEXT columns in `symbol_details` |
| Story 3: UI display — Fund Facts | ✅ Implemented | Shows AUM, inception date, fees, reference date |
| Story 3: UI display — Risk Measures | ✅ Implemented | Table with all 6 metrics, `—` for absent fields |
| Story 3: UI display — Asset Class | ✅ Implemented | Table (not pie chart), negative values supported |
| Story 3: UI display — Equity Derivatives | ✅ Implemented | Table with regions, reference date |
| Story 3: UI display — Currency Derivatives | ✅ Implemented | Table with currencies, reference date |
| Edge case: Factsheet 404 | ✅ Handled | Explicit error from client |
| Edge case: Missing optional data | ✅ Handled | Returns `nil, nil` for absent sections |
| Edge case: Empty required data | ✅ Handled | Returns explicit error |
| Edge case: PDF format changes | ✅ Handled | Text-based regex; explicit error on parse failure |
| Edge case: Different fund types | ✅ Handled | Optional sections return nil gracefully |
| Edge case: Rate limiting | ✅ Handled | 1s minimum delay between requests |
| Edge case: Bot detection | ✅ Handled | CycleTLS with browser fingerprint |
| Edge case: Multiple share classes | ✅ Handled | Each ISIN configured as separate symbol |
| Edge case: PDF language variants | ⚠️ Partial | Regex hardcoded to `_EN.pdf` only |
| Edge case: Corrupted PDF | ✅ Handled | Explicit error from PDF parser |
| Constraint: No browser automation | ✅ Met | Direct HTTP + CycleTLS |
| Constraint: No CGO | ✅ Met | `ledongthuc/pdf` is pure Go |
| Constraint: Manual source URL | ✅ Met | Reuses existing `data_source_url` mechanism |

**Spec gaps discovered during implementation:**
- The spec didn't mention that `FundInfo` (symbol + fund name) comes from the HTML page, not the PDF. The extractor wiring (`parseFundInfoFromHTML`) handles this correctly, but it was a deviation from the original parser signature.
- The spec didn't account for the silent fallback problem where `float64` zero values are indistinguishable from missing values. The `FieldsPresent` bitmask was added to solve this.

## Plan vs Reality

| Task | Status | Notes |
|---|---|---|
| Task 1: PDF parsing + types | ✅ Complete | Dependency stabilized in Task 3 via `go mod tidy` (minor deviation) |
| Task 2: matcher + client | ✅ Complete | All subtasks done as planned |
| Task 3: PDF parsers | ✅ Complete | `ParseFundFacts` signature changed (FundInfo from HTML, not PDF); `FieldsPresent` bitmask added |
| Task 4: Extractor wiring | ✅ Complete | Optional section errors silently discarded (matches plan) |
| Task 5: DB schema migration | ✅ Complete | All subtasks done as planned |
| Task 6: Repository + service | ✅ Complete | Silent fallback fix for `Isin`/`ShareClassName`/`OngoingCharges` fields |
| Task 7: UI display | ✅ Complete | All 4 new card sections rendered; reference date shown per section |
| Task 8: Registration + integration | ✅ Complete | Extractor registered; `real_test.go` is parser-level only, not full-stack |

**Task sizing**: All tasks were appropriately sized. No task required splitting. The parsers (Task 3) were the most complex due to PDF text extraction quirks, but stayed within a single focused session.

**Dependencies**: The dependency graph was accurate. Tasks 1–3 were developed in parallel with Task 5 as planned. No unexpected blocking dependencies.

## Learnings

- **Silent fallback audit is critical for new fields**: Whenever new fields are added to shared types (`extractor.FundProfile`, `symbol.FundProfile`), the service mapping layer must be audited to ensure every new field is mapped. A checklist item for "cross-layer field audit" should be added to the plan template for future features.
- **PDF text extraction is inherently fragile**: Even with a stable PDF library, the extracted text is an unstructured mess (split labels, missing values, axis numbers interleaved with data). The parser design (phase-based extraction, known-header boundaries, label merging) is the right approach, but it requires careful testing against real PDFs.
- **`float64` zero values need a presence indicator**: For optional numeric fields, a bitmask or similar mechanism is needed to distinguish "field = 0" from "field absent in source". This should be a standard pattern for future extractors that produce optional metrics.
- **Fixture-based testing is the right approach for PDFs**: Extracting PDF text once and saving as a text fixture gives fast, deterministic tests. The `real_test.go` with embedded fixture is a good pattern for regression testing.
- **Type duplication between `extractor` and `symbol` packages is intentional and correct**: Following the existing pattern (`FundProfile`, `MarketCapBreakdown`), types are duplicated with identical field names. This makes the service mapping straightforward but requires discipline to keep both copies in sync.

## Action Items

- [x] Repo round-trip tests — already present in `symbol_details_repo_test.go` (added during implementation)
- [x] End-to-end integration test — already present in `tests/integration/imgp_details_test.go` (added during implementation)
- [x] `extractPDFURL` language — user confirmed English-only (`_EN.pdf`) is acceptable
- [ ] Add "cross-layer field audit" step to PLAN.md template for future features with new shared types
- [ ] Add "presence indicator" pattern (bitmask or similar) to project conventions for optional numeric fields
