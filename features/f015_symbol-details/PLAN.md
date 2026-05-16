# Implementation Plan: Symbol Details

## Overview

Build a symbol details system that fetches and caches rich Yahoo Finance metadata (name, exchange, ETF holdings, sector weightings, fund profile) for each symbol. Data is keyed by `internal_symbol`, fetched on symbol creation, and refreshed by the MarketCache periodic ticker when stale (>7 days). Exposed via an enriched `GET /api/symbols/{id}` response and a read-only web page.

## Task Dependencies

```
Task 1 (migration + repo) → Task 2 (fetcher) → Task 3 (service) → Task 4 (API enrich)
                                                                    → Task 5 (web UI)
Task 3 → Task 6 (creation hook)
Task 3 → Task 7 (background refresh)
Task 4,5,6,7 → Task 8 (router wiring)
All → Task 9 (validation)
```

## Tasks

### Task 1: Database schema + repository [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundation)
**Description:** Create the `symbol_details` table, sqlc queries, and repository layer.

- [x] Write migration `016_create_symbol_details.sql` (up + down)
- [x] Update `internal/data/queries/schema.sql` with the new table
- [x] Write sqlc queries in `symbol_details.sql`:
  - `InsertSymbolDetails` — upsert by internal_symbol (ON CONFLICT)
  - `GetSymbolDetailsByInternalSymbol` — lookup by internal_symbol
  - `ListStaleSymbolDetails` — internal_symbols + market_data_symbols where fetched_at > 7 days ago (JOIN with symbol_mappings)
- [x] Run `sqlc generate`
- [x] Create `internal/data/symbol_details_repo.go` with repository methods wrapping sqlc
- [x] Write repository tests

**Verification:** `sqlc generate` succeeds, repo compiles, tests pass.

### Task 2: Yahoo Finance symbol details fetcher [PRIORITY: HIGH]
**Corresponds to:** Fetch generic/ETF details scenarios
**Description:** Add `FetchSymbolDetails()` to `YahooFinanceFetcher` using direct HTTP to the `quoteSummary` endpoint. See `features/f015_symbol-details/RESEARCH.md` for endpoint details, auth flow, and response structure.

- [x] Define domain models in `internal/domain/symbols/symbol_details.go`:
  - `SymbolDetails` struct (shortName, longName, exchange, currency, quoteType, topHoldings, sectorWeightings, aggregatePositions, fundProfile, equityValuation, fetchedAt)
  - Sub-structs: `TopHolding`, `SectorWeighting`, `AggregatePositions`, `FundProfile`, `EquityValuation`
- [x] Add `FetchSymbolDetails(ctx, marketDataSymbol) (*SymbolDetails, error)` to `YahooFinanceFetcher` in `internal/market/quote.go`
- [x] Implement crumb/cookie auth flow (same pattern as go-yfinance's AuthManager)
- [x] Parse `quoteSummary` JSON for `topHoldings` module (holdings, sectorWeightings, aggregate positions, equityHoldings)
- [x] Parse `fundProfile` module (family, legalType, netAssets, expenseRatio, turnover)
- [x] Parse `assetProfile` module for generic info (shortName, longName, exchange, currency)
- [x] Handle partial data gracefully (store whatever fields are available)
- [x] Write unit tests for response parsing (with hardcoded JSON fixtures from RESEARCH.md)
- [x] Write integration-style test for fetcher (mock HTTP server returning known JSON)

**Verification:** Fetcher compiles, tests pass with fixtures, parses all expected fields.

### Task 3: Service layer [PRIORITY: HIGH]
**Corresponds to:** All scenarios
**Description:** Service that orchestrates fetch → store, stale detection, and retrieval.

- [x] Create `internal/domain/symbols/service.go`
- [x] Define interfaces: `SymbolDetailsRepository`, `SymbolDetailsFetcher`
- [x] Implement `FetchAndStore(ctx, internalSymbol, marketDataSymbol) error` — resolve ticker, fetch from Yahoo, upsert to DB
- [x] Implement `GetByInternalSymbol(ctx, internalSymbol) (*SymbolDetails, error)` — retrieve cached details
- [x] Implement `GetStaleSymbols(ctx) ([]StaleSymbol, error)` — find symbols needing refresh (returns internal_symbol + market_data_symbol pairs)
- [x] Implement `RefreshSymbol(ctx, internalSymbol, marketDataSymbol) error` — re-fetch and update
- [x] Write service tests with hand-written mocks (following `account/service_test.go` pattern)
- [x] Test edge cases: fetch failure (returns error, no DB change), partial data stored, stale detection threshold

**Verification:** Service compiles, tests pass, covers happy/error/partial paths.

### Task 4: Enrich symbol API with details [PRIORITY: HIGH]
**Corresponds to:** View symbol details via API (generic/ETF/no data)
**Description:** Add `symbol_details` to the response of `GET /api/symbols/{id}`. The existing symbol CRUD handlers are renamed from `symbol-mapping` to `symbols` (route rename only).

- [x] Rename API routes from `symbol-mappings` to `symbols` in handler (`internal/api/handlers/symbol_mapping.go` → `symbol.go`)
- [x] Rename handler struct from `SymbolMappingHandler` to `SymbolHandler`
- [x] Add `SymbolDetailsService` dependency to `SymbolHandler`
- [x] In `HandleGet`, after fetching the symbol mapping, enrich response with cached `symbol_details` (null if no cache)
- [x] In `HandleList`, keep response lean (no details) — callers fetch by ID for full data
- [x] Update response struct to include `SymbolDetails *SymbolDetailsResponse` field
- [x] Register renamed routes on router
- [x] Write handler tests (mock service + httptest)

**Verification:** `GET /api/symbols/{id}` returns mapping + details; `GET /api/symbols` returns mappings only; null details when no cache.

### Task 5: Web UI — symbol details page [PRIORITY: HIGH]
**Corresponds to:** View symbol details in web UI (generic/ETF/no data)
**Description:** Read-only page at `/symbols/{id}/details` showing cached details + live price.

- [x] Rename web routes from `symbol-mappings` to `symbols` in handler (`symbol_mapping_web.go` → `symbol_web.go`)
- [x] Rename handler struct from `SymbolMappingWebHandler` to `SymbolWebHandler`
- [x] Create `internal/api/handlers/symbol_details_web.go` with `SymbolDetailsWebHandler`
- [x] Implement `HandleDetailsPage` for `GET /symbols/{id}/details` — resolve symbol, fetch cached details + live price, render template
- [x] Create `templates/symbol_details/view.html` with sections:
  - Header: symbol name, exchange, live price (with currency)
  - "Last updated" timestamp (stale indicator if >7 days)
  - Top 10 holdings table (symbol, name, % allocation) — only for ETFs
  - Sector weightings table (sorted desc) — only for ETFs
  - Aggregate positions (stock %, bond %, cash %, etc.) — only for ETFs
  - Fund profile (family, legal type, net assets, expense ratio) — only for ETFs
  - "No details available" message when cache is empty
- [x] Add "Details" link to symbol list page (`templates/symbol/list.html`)
- [x] Register route on router
- [x] Write web handler tests

**Verification:** Page renders correctly for ETF/generic/no-data scenarios; live price shown; stale indicator displayed.

### Task 6: Fetch on symbol creation [PRIORITY: MEDIUM]
**Corresponds to:** Fetch generic/ETF details on symbol creation, fetch fails on creation
**Description:** Trigger non-blocking details fetch after a symbol mapping is created.

- [x] Add optional `SymbolDetailsFetcher` dependency to `symbols.Service` (the symbol CRUD service, via `ServiceOption`)
- [x] In `Service.Create()`, after successful DB save, spawn a goroutine that calls the details service to fetch and store
- [x] Goroutine logs success/failure but doesn't propagate errors
- [x] Wire the dependency in `router.go` (pass symbol details service to symbol CRUD service)
- [x] Write tests: creation succeeds when fetcher is nil, creation triggers fetch when configured, creation succeeds when fetch fails

**Verification:** Symbol creation is non-blocking; details fetch runs in background; errors logged not surfaced.

### Task 7: Background refresh integration [PRIORITY: MEDIUM]
**Corresponds to:** Background refresh scenarios (stale/fresh/failure)
**Description:** Integrate symbol details refresh into the MarketCache periodic ticker.

- [x] Add `SymbolDetailsRefreshSource` interface to `marketcache` package
- [x] Add `WithSymbolDetailsRefresh(source)` option to MarketCache
- [x] In `periodicTicker` / `doRefresh()`, after existing refresh logic, call `GetStaleSymbols()` and refresh each
- [x] Serialize fetches with ~500ms delay between symbols (per RESEARCH.md rate limiting guidance)
- [x] Preserve existing cached data on fetch failure (log warning, continue to next symbol)
- [x] Other symbols in the refresh batch are not affected by individual failures
- [x] Write tests: stale symbols refreshed, fresh symbols skipped, fetch failure handled gracefully

**Verification:** Background refresh picks up stale symbols, skips fresh ones, handles failures gracefully.

### Task 8: Router wiring + template renames [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (integration)
**Description:** Wire all new components into the router, rename templates and remaining artifacts.

- [x] Rename `templates/symbol_mapping/` → `templates/symbol/` (update template names in renderer calls)
- [x] Create symbol details repo in `router.go`
- [x] Create symbol details service with repo + fetcher
- [x] Pass service to `SymbolHandler` (API enrich), `SymbolDetailsWebHandler`, and `symbols.Service` (creation hook)
- [x] Wire details refresh into MarketCache
- [x] Register all renamed + new routes
- [x] Update nav link text from "Symbol Maps" to "Symbols" in `templates/partials/nav.html`
- [x] Verify `go build` succeeds

**Verification:** Server starts without errors; all routes registered; templates render.

### Task 9: Validation / Hardening [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (cross-cutting quality gate)
**Description:** After implementation tasks are complete, validate the feature end-to-end.

- [ ] Run all tests (`go test ./...`) — not just `-short`
- [ ] Verify each spec scenario manually or via integration test
- [ ] Check edge cases from the spec against actual behavior
- [ ] Run `go vet ./...` and linter
- [ ] Review for cross-layer consistency (data types stored match data types read)
- [ ] Verify no TODOs, FIXMEs, or temporary workarounds remain
- [ ] Update `features/README.md` feature index

**Verification:** All tests pass, all spec scenarios validated, no unresolved issues.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| DB key | `internal_symbol` | Direct lookup from API; details row lives with symbol identity, not fetch target |
| Single table | Yes, nullable JSON columns for ETF data | Simpler than two tables; stocks have NULL for ETF columns |
| Yahoo fetch | Direct HTTP in `YahooFinanceFetcher` | go-yfinance doesn't expose `topHoldings`; follows RESEARCH.md recommendation |
| API design | Enrich `GET /api/symbols/{id}` with `symbol_details` field | One call gets everything; avoids separate endpoint; backward compatible (additive) |
| Route naming | `symbols` (renamed from `symbol-mappings`) | Cleaner resource name; "mapping" is an implementation detail |
| Background refresh | Added to MarketCache periodic ticker | Reuses existing infrastructure (ticker, logger, error handling) |
| Multi-provider | Deferred (Yahoo implicit) | Spec Non-Goal; adding `source` now adds clutter with no benefit |
| Live price | Fetched at render time via existing `FetchQuote()` | Spec says "handled separately from cached symbol details" |

## Risks

- **Yahoo Finance auth changes** — crumb/cookie flow may break; mitigated by following go-yfinance's proven pattern
- **Rate limiting during bulk refresh** — mitigated by 500ms delay between symbols in background refresh
- **JSON parsing fragility** — Yahoo's response structure may change; mitigated by comprehensive fixture tests
- **Route rename impact** — renaming `symbol-mappings` to `symbols` affects all callers; mitigated by updating all internal references in the same change
