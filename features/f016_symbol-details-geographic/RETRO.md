# Retrospective: Symbol Details — Geographic Data (f016)

## What Went Well

- **Minimal, focused change set** — 19 files touched, ~970 net lines added. The feature extended all layers (migration → sqlc → repo → domain → fetcher → API → web → tests) without introducing complexity. Each layer change was a small, mechanical addition following the established f015 pattern.
- **Existing infrastructure reused completely** — No new wiring needed for creation hooks, background refresh, or stale detection. The `FetchSymbolDetails` extension and the new `geographic_allocations` column plugged into the existing flow seamlessly. This validates the f015 design decision to make the details system extensible.
- **Single-column design for stocks and ETFs** — Using one `geographic_allocations` TEXT column (stocks = single-element array, ETFs = multi-element array) simplified schema, API, and UI logic. It also makes f017 aggregation straightforward — one source, one format.
- **Country extraction from existing `assetProfile` module** — Zero additional API calls needed. The `assetProfile` module was already fetched by f015 but the `country` field wasn't consumed. This was a free data point.
- **Post-review fix caught an important convention inconsistency** — The implementation review identified that `GeographicAllocation.Percent` used 0–100 scale while `TopHolding.Percent` and `SectorWeighting.Percent` stored raw Yahoo fractions (0–1) with `* 100` at display. The fix (commit 78ff8cf) normalized all three to 0–100 at parse time, making the convention consistent and eliminating a latent bug for f017 consumers.
- **Comprehensive test coverage** — 19 new tests across 4 files (3 fetcher, 5 repo, 3 API, 3 web) covering multi-element, single-element, nil, empty, and overwrite scenarios. All internal tests pass cleanly.

## What Could Be Improved

- **Test for single-element stock "Country" row in Overview** — The web test `TestDetailsHandleDetailsPage_GeographicSingleElement` asserts that the "Geographic Allocation" card is absent for single-element, but doesn't positively assert that the "Country" row appears in the Overview section. Adding an assertion like `strings.Contains(body, "<th>Country</th>")` would make the test more complete.
- **"No geographic data available" card always shown when null** — The template shows a dedicated "Geographic Allocation" card with "No geographic data available" whenever `GeographicAllocations` is nil/empty, even if the symbol has rich details (holdings, sectors, etc.). This could feel redundant. A future improvement: only show the "no data" card if the symbol is an ETF (where geographic data is expected), or omit it entirely for stocks since the country is shown inline in Overview when available.
- **No explicit test for `toSQLNullJSON` with empty slice vs nil** — The repo tests cover nil → null and empty slice → stored, but the `toSQLNullJSON` function's behavior for empty slices (`[]GeographicAllocation{}`) vs nil is an edge case worth a dedicated unit test on the helper function itself.

## Spec vs Reality

- **All 7 spec scenarios implemented:**
  - Fetch geographic data for ETF on creation: ✅ (infrastructure ready; Yahoo doesn't provide ETF country breakdown, deferred to future feature)
  - Fetch geographic data for non-ETF (stock): ✅ (country from `assetProfile`, wrapped as single-element at 100%)
  - Geographic data in cached symbol details: ✅ (stored as JSON in `geographic_allocations` TEXT column)
  - Geographic data in web UI: ✅ (single-element → Country row in Overview; multi-element → Geographic Allocation card; null → "No geographic data available")
  - Geographic data in background refresh: ✅ (reuses existing `FetchSymbolDetails` call; no new wiring)
  - Geographic data unavailable: ✅ (stored as null; template shows "no geographic data available")
  - API response includes geographic data: ✅ (`geographic_allocations` field, sorted desc, null when unavailable)

- **Edge cases handled:**
  - Yahoo returns no geographic data: ✅ (nil stored as null)
  - Partial geographic data (doesn't sum to 100%): ✅ (stored as-is, no invented "Other" bucket)
  - Country name inconsistencies: ✅ (stored as-is from Yahoo; normalization deferred to f017)
  - Empty country field: ✅ (nil allocations)
  - Missing `assetProfile` module: ✅ (nil allocations)
  - Existing symbols before deployment: ✅ (NULL until next refresh; no backfill)

- **Non-Goals respected:**
  - No bond/fixed-income: ✅ (equity-focused)
  - No region aggregation: ✅ (individual countries only)
  - No historical snapshots: ✅
  - No manual override: ✅ (read-only)
  - No non-Yahoo providers: ✅

- **Known deviation from spec:**
  - **ETF geographic data deferred** — The spec envisioned multi-element country breakdowns for ETFs, but Yahoo's quoteSummary API doesn't provide ETF geographic allocations. The column is ready for when a data source becomes available. This is documented in PLAN.md Technical Decisions and is the primary input for f017 planning.

## Plan vs Reality

- **Task breakdown effective** — 7 tasks, each independently testable. All completed in dependency order.
- **Task sizing appropriate** — No task was too large. Task 2+3 were combined into one commit due to a hard compilation dependency (domain type needed by repo layer), which was a natural adjustment documented in NOTES.md.
- **Dependencies accurate** — The dependency graph was followed faithfully. The only adjustment was Tasks 2+3 combined, which was the right call.
- **Post-review fix not in original plan** — The Percent normalization fix (commit 78ff8cf) was identified during implementation review and required updating the fetcher, web handler, and tests. This was a necessary correction, not scope creep.

## Learnings

- **Implementation review is high-value** — The post-review process caught a `Percent` convention inconsistency that would have caused bugs in f017 (portfolio-level geographic aggregation). Without the review gate, this inconsistency would have propagated silently. Make implementation review a standard gate before retro.
- **Extending existing systems is fast** — Because f015 built an extensible foundation (uniform JSON columns, generic fetch/store flow, background refresh), f016 was essentially a "add one column, wire through layers" exercise. The investment in extensibility paid off immediately.
- **Test for positive assertions, not just absence** — The single-element stock test only asserts that the Geographic Allocation card is absent. It should also assert that the Country row IS present in Overview. Absence-only tests pass too easily and don't catch rendering regressions.
- **Convention audits at review time** — The Percent inconsistency existed because the fetcher developer used 100 (matching the struct comment) while existing fields used 0–1 fractions. A convention table in the domain type comments (explicitly stating "Percent is 0–100 percentage, not 0–1 fraction") would prevent this for future fields.
- **Deferred functionality needs clear handoff** — The ETF geographic data deferral is the bridge to f017. The RETRO.md for f017 should reference this explicitly and plan accordingly (look-through from top holdings, or alternative data source).

## Action Items

- [x] Add positive assertion to `TestDetailsHandleDetailsPage_GeographicSingleElement`: verify "Country" row exists in Overview section
- [x] Add explicit "Percent is 0–100 percentage" convention note to `TopHolding`, `SectorWeighting`, and `GeographicAllocation` struct comments in `symbol_details.go`
- [x] Suppress "No geographic data available" card for non-ETF symbols (only show when `QuoteType == "ETF"`); added `TestDetailsHandleDetailsPage_GeographicNoData_NonETF_Suppressed`
- [ ] For f017 planning: investigate look-through approach (aggregate country data from individual top holdings) as fallback when Yahoo doesn't provide ETF geographic breakdown
