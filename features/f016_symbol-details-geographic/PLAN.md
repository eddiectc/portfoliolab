# Implementation Plan: Symbol Details — Geographic Data

## Overview

Extend the existing symbol details system (f015) to capture geographic/country data. For individual stocks, the country from `assetProfile` (already fetched by f015) is stored as a single-element array. For ETFs, the schema supports a multi-element country breakdown, but populating it is deferred to a future feature — Yahoo Finance's quoteSummary API does not provide ETF geographic allocations. A single `geographic_allocations` JSON column serves both cases uniformly. Data is included in API responses and displayed on the web symbol details page. Background refresh and creation-hook fetch are inherited from the existing flow — no new wiring needed.

## Task Dependencies

```
Task 1 (migration + schema) → Task 2 (sqlc queries + repo) → Task 3 (domain model)
                                                              → Task 4 (fetcher)
Task 3,4 → Task 5 (API response enrich)
Task 3,4 → Task 6 (web UI)
Task 5,6 → Task 7 (tests + validation)
```

## Tasks

### Task 1: Database migration [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundation)
**Description:** Add a new column to the `symbol_details` table for geographic data.

- [ ] Write migration `017_add_geographic_to_symbol_details.sql` (up + down):
  - Add `geographic_allocations TEXT` column (JSON array; stocks have single element, ETFs have multiple)
- [ ] Update `internal/data/queries/schema.sql` with the new column

**Verification:** Migration runs cleanly (up and down) against an in-memory SQLite database.

### Task 2: sqlc queries + repository updates [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundation)
**Description:** Update sqlc queries and repository to handle the new geographic column.

- [ ] Update `symbol_details.sql` — add `geographic_allocations` to `InsertSymbolDetails` (INSERT + ON CONFLICT UPDATE)
- [ ] Run `sqlc generate` to regenerate types and query functions
- [ ] Update `InsertSymbolDetailsParams` usage in `symbol_details_repo.go` Upsert to include new field
- [ ] Update `toSymbolDetail()` in repo to deserialize `geographic_allocations` JSON
- [ ] Write repository tests: upsert with geographic data, upsert without (null), retrieval round-trip

**Verification:** `sqlc generate` succeeds, repo compiles, tests pass.

### Task 3: Domain model updates [PRIORITY: HIGH]
**Corresponds to:** All scenarios
**Description:** Add geographic field to the `SymbolDetails` domain type.

- [ ] Add `GeographicAllocations []GeographicAllocation` field to `SymbolDetails` in `internal/types/symbol/symbol_details.go`
- [ ] Define `GeographicAllocation` struct: `Country string`, `Percent float64`

**Verification:** Types compile, no breaking changes to existing consumers.

### Task 4: Fetcher — extract stock country from assetProfile [PRIORITY: HIGH]
**Corresponds to:** Fetch geographic data for non-ETF scenarios, geographic data unavailable
**Description:** Extend `FetchSymbolDetails` to extract the country field from the `assetProfile` module (already fetched but not currently used) and wrap it as a single-element geographic allocation. ETF geographic allocations are deferred to a future feature.

- [ ] Extract `country` from `assetProfileModule` (field exists in Yahoo response but not currently consumed)
- [ ] In `FetchSymbolDetails()`, if country is non-empty, populate `details.GeographicAllocations` as `[{Country: country, Percent: 100}]`
- [ ] If country is empty, leave `details.GeographicAllocations` as nil
- [ ] Write unit tests with JSON fixtures:
  - Individual stock with country (e.g., AAPL — assetProfile contains country)
  - Individual stock with empty country (edge case → nil allocations)
  - ETF with no country in assetProfile (expected → nil allocations)

**Verification:** Fetcher compiles, tests pass with fixtures, single-element allocation populated for stocks.

### Task 5: API response enrichment [PRIORITY: HIGH]
**Corresponds to:** Symbol details API response includes geographic data
**Description:** Include geographic allocations in the `SymbolDetailsResponse` for `GET /api/symbols/{id}`.

- [ ] Add `GeographicAllocations []symbol.GeographicAllocation` to `SymbolDetailsResponse` in `symbol.go`
- [ ] In `toSymbolDetailsResponse()`, copy geographic allocations (sorted by percent descending)
- [ ] Write handler tests: response includes geographic allocations (sorted), response includes single-element for stocks, response has null when no data

**Verification:** API response includes geographic data, sorted correctly; tests pass.

### Task 6: Web UI — geographic section [PRIORITY: HIGH]
**Corresponds to:** Geographic data included in web UI symbol details page
**Description:** Add a geographic/country allocation section to the symbol details page.

- [ ] Add `GeographicAllocations []displayGeographicAllocation` field to `symbolDetailsDisplay` struct in `symbol_details_web.go`
- [ ] Define `displayGeographicAllocation` struct: `Country string`, `Percent string` (pre-formatted, e.g., "45.20%")
- [ ] In `toDisplayDetails()`, convert `GeographicAllocations` to `[]displayGeographicAllocation` (sorted desc)
- [ ] Update `templates/symbol_details/view.html`:
  - Add "Geographic Allocation" card (table: Country, Weight) — shown when allocations exist
  - For single-element (stocks), render as a simple "Country" row in the Overview section instead of a table
  - Show "No geographic data available" when allocations are empty/null
- [ ] Write web handler tests: page shows geographic table for multi-element, page shows country row for single-element, page handles missing data

**Verification:** Page renders correctly for multi-element/single-element/no-data scenarios; tests pass.

### Task 7: Validation / Hardening [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (cross-cutting quality gate)
**Description:** After implementation tasks are complete, validate the feature end-to-end.

- [ ] Run all tests (`go test ./...`) — not just `-short`
- [ ] Verify each spec scenario against actual behavior
- [ ] Check edge cases from the spec (partial data, no data, name inconsistencies stored as-is)
- [ ] Run `go vet ./...` and linter
- [ ] Review for cross-layer consistency (data types stored match data types read)
- [ ] Verify no TODOs, FIXMEs, or temporary workarounds remain
- [ ] Update `features/README.md` feature index

**Verification:** All tests pass, all spec scenarios validated, no unresolved issues.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Single column | `geographic_allocations` (TEXT/JSON) | Stocks and ETFs share one uniform field — stocks have single-element array, ETFs have multi-element; simplifies schema, API, and f017 aggregation |
| Country for stocks | Extract from existing `assetProfile` module, wrapped as single-element array | `assetProfile` is already fetched; `country` field exists in the response but was not previously used — zero additional API calls |
| Geographic for ETFs | Deferred to future feature | Yahoo Finance quoteSummary API does not provide ETF geographic allocations (topHoldings has individual holdings + sectors, no country breakdown); column is ready for when a data source becomes available |
| API field name | `geographic_allocations` (array) | Single uniform field; f017 aggregates from one source |
| Sorting | Sort by percent descending in API response | Spec requires it; computed at response time (not stored sorted) so DB stays simple |
| Background refresh | No changes needed | Existing `FetchSymbolDetails` call is extended; stale refresh and creation hook already wired |
| Migration approach | ALTER TABLE ADD COLUMN (nullable) | Non-destructive; existing rows get NULL for new columns; no data loss |

## Risks

- **Country name inconsistencies** — Yahoo may use different names for the same country across symbols; spec says store as-is, normalization deferred to f017
- **Migration on production** — ALTER TABLE is safe (adds nullable columns), but existing rows will have NULL geographic data until next refresh
