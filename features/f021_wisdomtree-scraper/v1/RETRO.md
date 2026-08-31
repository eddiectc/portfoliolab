# Retrospective: Data Extractor Framework with WisdomTree Provider

> **SUPERSEDED.** Covers the v1 implementation (old site), rebuilt in place after the 2026-08-30
> site relaunch. The Phase 2 retrospective is pending; current state: `../NOTES.md`.

**Feature ID**: f021
**Date**: 2026-05-28
**Duration**: ~2 days (2026-05-26 to 2026-05-27)

---

## What Went Well

- **Extensible architecture delivered cleanly** — The Extractor interface + Registry + Dispatcher pattern follows existing project conventions (comparable to `MarketDataFetcher`, `SymbolDetailsFetcher`). Adding a new provider requires only a new package and one `Register()` call. No changes to routing logic, dispatcher, or DB schema needed.

- **CycleTLS resolved Cloudflare cleanly** — The initial research (RESEARCH.md) correctly identified Cloudflare as the blocker. Switching from `net/http` to CycleTLS (already a transitive dependency via `go-yfinance`) bypassed Cloudflare with zero additional dependencies. This was the riskiest technical decision and it paid off.

- **RESEARCH.md was thorough** — The upfront research phase (identifying all 10 data extraction patterns, confirming JS vs raw HTML, testing CycleTLS) prevented mid-implementation surprises. The only course correction was discovering that country allocation / market cap / characteristics were in raw HTML (not JS-rendered as initially assumed from a stale sample).

- **Cross-layer data audit (Task 9) caught nothing** — All 5 existing `data_type`-filtering queries correctly excluded NAV data. The new `data_type='nav'` sits cleanly alongside `stock` and `fx` without requiring changes to existing queries. The dedicated integration test (`TestNAV_DataTypeIsolation`) confirms this.

- **Test coverage is strong** — 2,561 lines of tests across 5 test files. Unit tests use table-driven format with sample CSV/HTML from `samples/`. Real HTML test (`real_test.go`) validates parsers against actual CycleTLS-fetched pages. Integration tests cover the full DB → service → handler pipeline. All tests pass.

- **Migration 022 handled mid-implementation discovery** — The `market_cap_breakdown` and `themes` JSON columns were identified during implementation (Task 4) rather than in the original migration 021. Migration 022 was added cleanly with full pipeline (migration → schema.sql → sqlc → types → service → repo → API → tests).

---

## What Could Be Improved

- **Spec vs implementation: "As of" date granularity** — The spec says "each extractor-sourced section shows its 'as of' date." The implementation uses a single `extractor_as_of_date` DB field shared across all sections, displayed once in the Overview card. This is a reasonable simplification (all sections come from the same fetch), but it should have been flagged as a spec deviation earlier rather than noted in NOTES.md during Task 8.

- **Spec vs implementation: Estimated P/E missing** — The spec calls for "Estimated P/E" alongside P/E in Fund Characteristics. The WisdomTree page does publish "Estimated Price/Earnings" (e.g. 34.56 vs actual 69.64), but the `FundCharacteristics` struct only has 5 fields. The parser extracts it but the field has nowhere to go. This is a minor gap — one additional field on `FundCharacteristics`, `EquityValuation`, and the display layer.

- **Spec vs implementation: "As of" date missing fallback** — The spec says "stores the fetch date as a fallback but logs a warning" when the "as of" date is missing from the provider page. The implementation fails the entire extraction instead. This is a defensible decision (avoiding silently incorrect reference dates), but it should be reflected in the spec or explicitly documented as a deviation, not just in NOTES.md.

- **Plan assumed separate create/edit files** — Task 7 assumed `create.go`/`edit.go` handlers, but the actual implementation uses a shared `symbol_web.go` and shared `symbol/form.html` template. This caused a gap: `CreateRequest` was missing `DataSourceURL` (only `UpdateRequest` had it). Caught and fixed, but the plan should have verified the existing file structure before writing tasks.

- **Optional vs required sections discovered mid-implementation** — The spec says "atomic — partial data is not persisted." However, different WisdomTree pages include different data sections (WMGT lacks `fundSectorsData`, QGRW.L lacks `fundMarketData` and `fundThemeData`). The extraction was changed from fully atomic to "required sections atomic, optional sections return nil." This is the right behavior for real-world usage, but the spec didn't account for page-level variability.

- **`float64` for extractor types** — The CONVENTIONS.md rule says "Use `decimal.Decimal` for all monetary values — never `float64`." The extractor types use `float64` throughout (AUM, weights, NAV, characteristics). This is documented in NOTES.md with a justification (display metadata, not transaction/P&L data, and matches existing `symbol` types which also use `float64`). However, the CONVENTIONS.md rule is absolute — either the rule needs a display-data exception, or the extractor types should use `decimal.Decimal`.

---

## Spec vs Reality

| Spec Item | Status | Notes |
|-----------|--------|-------|
| Provider-based routing (Story 1) | ✅ Implemented | Dispatcher routes by URL domain; Yahoo default when no URL |
| WisdomTree data extraction (Story 2) | ✅ Implemented, minor gaps | All 10 data sections parsed. Estimated P/E not stored (field missing). Optional sections return nil instead of error. |
| Symbol details page display (Story 3) | ✅ Implemented, deviation | All sections render with correct data. "As of" date shown once (Overview card) instead of per-section. "Country Allocation" header switches correctly. |
| Background refresh (Story 4) | ✅ Implemented | `refreshStaleSymbolDetails` routes by URL. `RefreshAll` includes symbol details + NAV. |
| Extensible provider architecture (Story 5) | ✅ Implemented | Registry + Dispatcher pattern. Explicit error for unregistered provider. |
| Full holdings (not limited to 10) | ✅ Implemented | Template shows top 10 with "Show all N" toggle |
| NAV history in market_data table | ✅ Implemented | `data_type='nav'`, `source='wisdomtree'`, no leakage to price queries |
| "As of" date per section | ⚠️ Deviation | Single `extractor_as_of_date` field, displayed once in Overview |
| Estimated P/E | ❌ Missing | Spec calls for it, WisdomTree page publishes it, but struct field not added |
| "As of" date missing → fallback | ⚠️ Deviation | Spec says store fetch date as fallback; implementation fails extraction |
| Atomic extraction | ⚠️ Deviation | Spec says fully atomic; implementation allows optional sections to be nil |
| Cross-layer data audit | ✅ Complete | Task 9 audit + integration test confirm no data leakage |

---

## Plan vs Reality

| Aspect | Assessment |
|--------|-----------|
| Task breakdown | **Effective** — 9 tasks + cross-layer audit covered all layers (DB → framework → extractor → service → background → API → web UI → audit). Each task was independently testable. |
| Task sizing | **Appropriate** — No task required splitting. Task 3 (extractor) was the largest but naturally subdivided into CSV parsers vs HTML table parsers (Phase 1 vs Phase 2). |
| Dependencies | **Accurate** — Task 1 (DB) → Task 2 (framework) → Task 3 (extractor) → Task 4 (service) → Task 5 (background) → Task 6 (API) → Task 7-8 (web UI) → Task 9 (audit) followed the planned order. |
| Technical decisions | **Mostly correct** — URL-based routing, NAV in existing table, JSON columns for new fields, CycleTLS, all proved sound. The only surprise was optional sections (different pages have different data). |
| Plan gaps | **Two gaps identified** — (1) `CreateRequest` missing `DataSourceURL` (plan assumed separate create/edit files), (2) `market_cap_breakdown`/`themes` columns not in migration 021 (identified during Task 4). Both caught and fixed during implementation. |
| Migration 022 | **Mid-implementation addition** — Not in the original plan. Needed because `market_cap_breakdown` and `themes` JSON columns were discovered during Task 4 service layer work. |

---

## Learnings

- **Research phase is worth the investment** — RESEARCH.md (10+ sections, 30+ attempts documented) paid off by identifying all extraction patterns, confirming Cloudflare bypass strategy, and ruling out headless browser before implementation started.

- **Always verify existing file structure before writing plan tasks** — Task 7 assumed `create.go`/`edit.go` existed. The actual codebase uses a shared `symbol_web.go` and `symbol/form.html`. This caused the `CreateRequest` gap.

- **Spec edge cases need real-world validation** — "As of" date fallback and atomic extraction were correct in theory but needed adjustment when tested against actual WisdomTree pages (different pages have different sections, and missing "as of" date is more common than expected).

- **Optional sections should be in the spec, not discovered during implementation** — The spec assumed all WisdomTree pages have the same data sections. Reality: WMGT lacks sectors, QGRW.L lacks NAV and themes. This should have been a spec edge case.

- **CONVENTIONS.md exceptions should be explicit** — The `float64` deviation for display metadata is reasonable, but the CONVENTIONS.md rule is absolute. Either add an exception clause for "display-only metadata" or accept that this feature is a documented exception.

- **Migration planning should include all new columns upfront** — Migration 021 covered `data_source_url` and `extractor_as_of_date` but missed `market_cap_breakdown` and `themes`. These were in the Technical Decisions table but not connected to the migration task. Future: cross-reference Technical Decisions against migration tasks.

- **Real HTML testing is essential** — The `real_test.go` with embedded CycleTLS-fetched HTML caught two issues that sample CSV tests missed: `unescapeJSString` not handling `\"`, and `parseTableRawValue` not handling whitespace/newlines between tag and text.

---

## Action Items

- [x] **Add `EstimatedPE` field** to `FundCharacteristics`, `EquityValuation`, and display layer. One field across 3 types + 1 parser update. Low effort, spec-compliant.
- [x] **Update CONVENTIONS.md** to clarify `float64` is acceptable for display-only metadata (non-transactional values like weights, allocations, characteristics), or document this feature as a formal exception.
- [x] **Add spec edge case** for "different provider pages have different data sections" — the atomic extraction rule should distinguish between required sections (fund info, holdings, as-of date) and optional sections (NAV, themes, sectors, country, market cap, characteristics).
- [x] **Add spec edge case** for "as-of date missing from provider page" — document the actual behavior (extraction fails) rather than the theoretical fallback.
- [ ] **Future feature: per-section "as of" dates** — If a provider publishes different reference dates for different sections (e.g., holdings as of 5/22, characteristics as of 5/15), the single `extractor_as_of_date` field is insufficient. Consider `extractor_as_of_dates JSON` for future providers.
