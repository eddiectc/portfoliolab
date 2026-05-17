# Retrospective: Symbol Details (f015)

## What Went Well

- **Clean layering across all 4 layers** — DB schema (Task 1) → fetcher (Task 2) → service (Task 3) → API + web (Tasks 4-5) → integration (Tasks 6-8). Each layer has a clear responsibility and well-defined interfaces.
- **Type placement resolved cleanly** — The `SymbolDetails` types moved from `domain/symbols` → `market` → `types/symbol` to break import cycles. The final location (`internal/types/symbol/`) is correct: it's a shared types package consumable by `market`, `symbols`, and `data` without cycles.
- **Interface consolidation** — The plan specified two interfaces for the background refresh (`SymbolDetailsRefreshRepository` + `SymbolDetailsRefreshFetcher`), but the single `SymbolDetailsRefreshSource` interface (implemented by `symbols.Service`) proved architecturally cleaner and followed the existing `BenchmarkSymbolLister` pattern.
- **Fetcher consolidation** — `YahooFinanceFetcher` now implements both `MarketDataFetcher` and `SymbolDetailsFetcher`, with a single wiring point in router. Direct HTTP for quoteSummary (not go-yfinance) was the right call per RESEARCH.md.
- **Comprehensive test coverage** — 171 tests across 7 files (11 service, 9 repo, 12 fetcher, 39 API handler, 12 web handler, 38 marketcache, 40 symbolmapping). All pass cleanly.
- **Non-blocking creation hook** — Background goroutine with `context.Background()` ensures details fetch survives HTTP disconnect. Errors logged, not surfaced to user.
- **Graceful degradation** — Web handler accepts `nil` optional dependencies; API returns null `symbol_details` when cache is empty; live price fetch failure doesn't block page render.

## What Could Be Improved

- **NOTES.md inconsistency** — NOTES.md says `ListStaleSymbolDetails` uses an INNER JOIN, but the actual SQL (`symbol_details.sql`) uses a LEFT JOIN. The LEFT JOIN is the correct choice (picks up symbols with no details row at all), but the note should be corrected.
- **No integration tests for the full stack** — Testing is thorough at each layer (unit tests with hand-written mocks), but there's no integration test exercising: create symbol → background fetch → GET /api/symbols/{id} → enriched response. This is a general project gap, not specific to this feature.
- **`float64` for percentages** — The domain types use `float64` for holding percentages and sector weightings. The AGENTS.md convention says "use `decimal.Decimal` for all monetary values" — but percentages aren't monetary. Still, if future calculations (e.g., portfolio-level aggregate sector exposure) require precision, this could be revisited.
- **No rate-limit backoff** — The 500ms delay between symbols in background refresh is fixed. A more adaptive approach (exponential backoff on 429) would be more resilient, but the spec says "not retried immediately" so this is acceptable for now.
- **Template has an "Edit Mapping" link** — The details page includes `<a href="/symbols/{{.Symbol.ID}}/edit" class="btn btn-sm">Edit Mapping</a>` which technically violates the spec's "read-only" constraint (no edit/delete controls). However, it links to the symbol mapping edit page (a different feature), not the details themselves. This is arguably a navigation convenience, not an edit control for symbol details.

## Spec vs Reality

- **All 5 user stories implemented:**
  - US-1 (fetch on creation): ✅ Background goroutine in `Create()`
  - US-2 (API): ✅ `GET /api/symbols/{id}` enriched with `symbol_details`
  - US-3 (web UI): ✅ `/symbols/{id}/details` page with all sections
  - US-4 (background refresh): ✅ `refreshStaleSymbolDetails()` in periodic ticker
  - US-5 (live price): ✅ `FetchQuote()` at render time

- **All 12 scenarios covered:**
  - Fetch generic/ETF on creation: ✅ (service + fetcher tests)
  - Fetch fails on creation: ✅ (symbol created, error logged)
  - API generic/ETF/no-data: ✅ (handler tests with details/no-details)
  - Web generic/ETF/no-data: ✅ (6 web handler tests)
  - Background stale/fresh/failure: ✅ (8 marketcache tests)
  - Live price shown/unavailable: ✅ (web tests cover both paths)

- **Edge cases handled:**
  - Symbol not found on Yahoo: ✅ (fetcher returns error, creation succeeds)
  - ETF with no holdings data: ✅ (nil/empty fields, template shows "—")
  - Partial data: ✅ (modules parsed independently, available fields stored)
  - International holding symbols: ✅ (stored as-is, no transformation)
  - Stale data display: ✅ ("Stale" badge + "Updated Xd ago" text)
  - Concurrent fetch: ✅ (user sees cached data; background refresh independent)
  - Nested ETFs: ✅ (stored as-is without recursive expansion)
  - Variable holding count: ✅ (whatever Yahoo returns, up to 10 in template)

- **Non-Goals respected:**
  - No editing of symbol details: ✅ (read-only page, though "Edit Mapping" link navigates away)
  - No manual refresh trigger: ✅ (background job only)
  - No historical holdings snapshots: ✅
  - No portfolio-level aggregate sector exposure: ✅
  - No symbol search/discovery: ✅

## Plan vs Reality

- **Task breakdown effective** — 9 tasks, each independently testable. Tasks were executed in dependency order with no reordering needed.
- **Task sizing appropriate** — No task was too large to complete in a focused session. Task 2 (fetcher) was the most complex (crumb/cookie auth + JSON parsing), but was well-scoped by RESEARCH.md.
- **Dependencies accurate** — The dependency graph (Task 1 → 2 → 3 → 4/5/6/7 → 8 → 9) was followed faithfully.
- **One minor scope expansion** — The plan said "rename `symbol-mappings` to `symbols`" which required updating integration tests and templates referencing the old path. This was handled in Task 4 and noted in NOTES.md.

## Learnings

- **RESEARCH.md is invaluable for complex integrations** — The Yahoo Finance quoteSummary research (endpoint, auth, response structure, error codes) made Task 2 implementation straightforward. Consider this pattern for future external API integrations.
- **Type placement matters early** — Moving types three times (domain → market → types/symbol) was necessary to resolve import cycles. For future features, consider whether types need to be shared across packages before placing them in a domain package.
- **Single-interface pattern wins** — The plan's two-interface design for background refresh didn't work architecturally. The single interface (`SymbolDetailsRefreshSource`) implemented by the service layer is the right pattern — it matches how `BenchmarkSymbolLister` works. Trust the architecture over the plan.
- **Combined module fetch is efficient** — Fetching `topHoldings,fundProfile,assetProfile` in one HTTP request (instead of three separate calls) saves auth overhead and is more resilient to rate limiting.
- **TLS fingerprint matters for Yahoo** — Using standard `net/http` for quoteSummary while go-yfinance's `AuthManager` used CycleTLS caused Yahoo to return partial data (NULL metadata). Fix: share the same client end-to-end. When mixing libraries with custom TLS, always verify the fingerprint is consistent across auth + data requests.
- **Interface abstraction enables fast, deterministic tests** — The `YahooAuth` interface lets tests inject a mock that returns fake crumb/cookie and delegates HTTP to `httptest.Server`. Market tests now run in ~5ms with zero network calls, instead of hitting real Yahoo servers and risking 429s.

## Action Items

- [x] Fix NOTES.md: changed "INNER JOIN" to "LEFT JOIN" in the `ListStaleSymbolDetails` note
- [x] Added full-stack integration test (`tests/integration/symbol_details_test.go` with 3 tests: create+enrich, list excludes details, stale refresh SQL)
- [x] Removed "Edit Mapping" link from details page (strict read-only per spec); updated web handler test to assert link is absent
