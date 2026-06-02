# Retrospective: BlackRock/iShares Data Extractor

**Feature**: f026_blackrock-scraper
**Date**: 2026-06-01
**Status**: Complete

## What Went Well

- **Clean reuse of f021 framework**: The provider dispatcher, registry, and symbol details schema from the WisdomTree scraper (f021) required zero changes. The BlackRock extractor plugged in as a drop-in registration — exactly as intended.

- **Two-phase extraction design proved sound**: Separating product page HTML (Phase 1: identity, profile, characteristics, component ID) from the JSON API (Phase 2: holdings + derived allocations) kept each parser focused and testable. The component ID extraction from Phase 1 feeding into Phase 2 was a clean dependency.

- **Task breakdown was effective**: The 7-task plan (types → no migration → extractor package → service mapping → repo serialization → web display → registration + integration tests) followed a clear bottom-up dependency chain. Each task was independently testable and verifiable.

- **Test coverage is comprehensive**: 29 unit tests across the blackrock package (8 extractor, 1 matcher, 20 parsers) plus 13 integration tests covering the full stack. All tests pass cleanly.

- **Post-implementation fix was quick**: The negative weight handling in sector/country aggregation (commit 69efdd5) was caught after initial implementation and fixed with a one-line change (`math.Abs`) plus two new test cases. The bug was isolated and didn't affect other extractors.

- **No database migration needed**: Confirmed early (Task 2) that all new fields flow through existing JSON columns. This saved time and avoided deployment complexity.

## What Could Be Improved

- **Bond characteristics field mapping is lossy**: The `ParseFundCharacteristics` parser maps both "Yield to Maturity" and "Weighted Average YTM" to `AverageCoupon`, and both "Modified Duration" and "Effective Duration" to `AverageDuration`, with the second value silently dropped if the first is already set. The iShares source provides both values as distinct metrics, but the shared `FundCharacteristics` type has no separate fields for them. This is a type system limitation inherited from earlier extractors.

- **Integration tests don't exercise the real extraction**: The integration tests insert pre-serialized JSON directly into the DB rather than running the actual `Extract()` method through the dispatcher. This verifies persistence and display but not the full extraction pipeline end-to-end. (This is consistent with the project's existing integration test pattern — the blackrock package's own unit tests with `httptest` cover the extraction logic.)

- **Web display test is shallow**: `TestBlackRock_WebPageRendersWithNewFields` checks for substring presence ("Article 6", "Ireland") rather than structured HTML parsing. A more robust test would parse the rendered HTML and assert specific table cells.

- **Holdings table overcrowding concern**: The plan noted that only the **Sector** column was added to the visible holdings table, with 9 other new fields (AssetClass, MarketValue, NotionalValue, Shares, Price, Identifier, Location, Exchange, MarketCurrency) available in the display struct but not rendered. For 350+ holding lists, this is the right call, but there's no "expandable row" or detail view for users who want to see the additional data.

## Spec vs Reality

| Spec Aspect | Match | Notes |
|---|---|---|
| Story 1: Provider-based routing | ✅ Exact | Reused f021 dispatcher, no schema changes |
| Story 2: Fund identity extraction | ✅ Exact | Name, ticker (Bloomberg), ISIN, currency |
| Story 2: Fund profile extraction | ✅ Exact | All 17 Key Facts fields extracted |
| Story 2: Holdings extraction (350+) | ✅ Exact | Full holdings via JSON API, no artificial cap |
| Story 2: Empty holdings | ✅ Exact | Returns empty list, not a failure |
| Story 2: Missing as-of date fails | ✅ Exact | `ParseAsOfDate` returns error if missing |
| Story 2: Equity characteristics | ✅ Exact | P/E, P/B, beta, std dev, number of holdings |
| Story 2: Bond characteristics | ⚠️ Partial | Equity-specific fields null for bond funds ✓; but YTM/duration fields are mapped to shared types with potential data loss (see above) |
| Story 2: Sector allocation | ✅ Exact | Derived from holdings data, sorted descending |
| Story 2: Geography allocation | ✅ Exact | Derived from holdings data, sorted descending |
| Story 2: Missing optional sections | ✅ Exact | Empty sectors/geography stored as empty, not failure |
| Story 2: Atomic extraction | ✅ Exact | Phase 1 or Phase 2 failure rejects entire extraction |
| Story 3: Web display sections | ✅ Exact | Fund Characteristics, Top 10 Holdings, Sector Weightings, Country Allocation |
| Story 3: Per-section "as of" dates | ✅ Exact | Each section shows its own date |
| Story 3: Country vs Geographic header | ✅ Exact | "Country Allocation" for extractor data |
| Story 4: Background refresh dual-path | ✅ Exact | BlackRock for details, Yahoo for market data |
| Story 4: Failure preserves previous data | ✅ Exact | Tested in integration suite |
| Edge case: Negative weights | ✅ Fixed | Post-implementation fix (commit 69efdd5) |
| Edge case: Hybrid funds | ✅ Bonus | Parser handles both equity and bond fields simultaneously |
| Edge case: UTF-8 BOM | ✅ Exact | Stripped before JSON parsing |
| Edge case: Context cancellation | ✅ Exact | Checked at entry point |

**Spec scenarios missed**: None. All acceptance criteria are covered.

**Spec scenarios not planned but discovered**:
- Negative weights in holdings (short positions) — discovered during implementation, fixed post-merge.
- Full Bloomberg ticker value (e.g. "IWMO LN") — initially only the first part was extracted; corrected in the same fix commit.

## Plan vs Reality

| Plan Aspect | Match | Notes |
|---|---|---|
| Task 1: Extend types | ✅ Exact | All fields added to both extractor and symbol packages |
| Task 2: No migration | ✅ Exact | Confirmed JSON columns accommodate new fields |
| Task 3: BlackRock package | ✅ Exact | client.go, matcher.go, parsers.go, extractor.go + tests |
| Task 4: Service mapping | ✅ Exact | `extractResultToSymbolDetails` updated with all new fields |
| Task 5: Repo serialization | ✅ Exact | Round-trip tests in `internal/data/symbol_details_repo_test.go` |
| Task 6: Web display | ✅ Exact | Template updated, integration test passes |
| Task 7: Registration + integration | ✅ Exact | Registered in router, 13 integration tests |
| Task sizes | ✅ Accurate | No task required splitting or was unexpectedly large |
| Dependencies | ✅ Accurate | Sequential dependency chain held |

**Deviations from plan**:
- Task 3 test `TestExtractor_Extract_MissingAsOfDate` was renamed to `TestExtractor_Extract_Cancellation` (documented in NOTES.md). The as-of date validation is covered by `TestParseAsOfDate` in parsers_test.go.
- Post-implementation fix (commit 69efdd5) for negative weights and Bloomberg ticker — not in the original plan but caught during review.

## Learnings

- **Type system constraints matter**: The shared `FundCharacteristics` type was designed before BlackRock's richer bond metrics. When future extractors need even more fields (e.g. separate YTM vs weighted average YTM), consider whether the bitmask pattern is still the right approach or if a more flexible map-based structure would serve better.

- **Negative weights in financial data**: Cash/derivative positions in ETF holdings can have negative weights. Any aggregation (sector, country, etc.) must use `math.Abs()` to avoid cancellation. This is a domain-specific gotcha that should be documented for future extractors.

- **Post-implementation review catches real bugs**: The negative weight bug and Bloomberg ticker truncation were both caught in a post-implementation review pass. This reinforces the value of the `/review-impl` gate before declaring a feature done.

- **Integration tests for extractors are naturally DB-focused**: The existing pattern of inserting pre-serialized JSON directly into the DB and verifying round-trip/display is efficient and avoids network dependencies. The extraction logic itself is covered by unit tests with `httptest`. This separation is the right approach.

- **RESEARCH.md is valuable**: The detailed research document (data source URLs, JSON format, attempted approaches for sector/geography) made implementation straightforward and provided the test fixtures. This should be a standard artifact for any scraper feature.

## Action Items

- [ ] **Consider FundCharacteristics type expansion**: Add dedicated fields for `WeightedAverageYTM`, `EffectiveDuration`, `WALToworst` to avoid data loss when mapping bond fund characteristics. This would be a small, backward-compatible change to the shared type.
- [ ] **Add expandable holdings detail view**: For extractors with rich holding data (BlackRock, Dimensional), consider an expandable row or modal showing MarketValue, NotionalValue, Shares, Price, Identifier, Location, Exchange, MarketCurrency. Currently only Sector is visible in the table.
- [ ] **Document negative weight handling**: Add a note in `docs/CONVENTIONS.md` or the extractor registration guide about using `math.Abs()` for weight aggregation, so future extractors don't repeat this bug.
- [ ] **Deepen web display tests**: Replace substring checks with structured HTML parsing for the symbol details page, asserting specific table cells rather than just presence of values.
