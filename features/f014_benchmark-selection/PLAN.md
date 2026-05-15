# Implementation Plan: Benchmark Selection

## Overview

Replace the hardcoded list of five predefined benchmarks with user-defined benchmarks. Any symbol in the user's symbol mappings can be marked as a benchmark via a checkbox on the create/edit forms. Benchmark data is populated by the market cache's existing refresh cycle (manual "Refresh All" + background gap-fill). The performance comparison UI uses a text input with autocomplete suggestions drawn from user-defined benchmarks.

## Task Dependencies

```
Task 1 (DB + sqlc + models)
    ├── Task 2 (service layer — persist flag only)
    ├── Task 3 (API handler)
    ├── Task 4 (web handler + templates — symbol mapping)
    └── Task 5 (market cache)
Task 5 ──► Task 6 (performance API handler)
Task 4 ──► Task 7 (performance web handler)
Task 5, 6, 7 ──► Task 8 (router wiring)
All ──► Task 9 (delete comparison.Predefined + cleanup)
All ──► Task 10 (validation / hardening)
```

Tasks 2-5 can be implemented in parallel after Task 1. Tasks 6-7 depend on their prerequisites. Task 8 wires everything together. Task 9 removes the hardcoded list. Task 10 is the final quality gate.

## Tasks

### Task 1: Database migration, sqlc queries, and model updates [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundation)
**Description:** Add `is_benchmark` boolean column to the `symbol_mappings` table, update sqlc queries, regenerate types, and add the field to domain models and DTOs.

- [x] Create migration `015_add_is_benchmark_to_symbol_mappings.sql` (ADD COLUMN `is_benchmark BOOLEAN NOT NULL DEFAULT 0`; down migration drops it)
- [x] Update `internal/data/queries/symbol_mapping.sql`:
  - `CreateSymbolMapping`: add `is_benchmark` to INSERT columns and VALUES
  - `UpdateSymbolMapping`: add `is_benchmark` to SET clause
  - Add new query `ListBenchmarkSymbols`: `SELECT * FROM symbol_mappings WHERE is_benchmark = 1 ORDER BY internal_symbol`
- [x] Run `sqlc generate` to regenerate Go types
- [x] Update `internal/domain/symbolmapping/symbol_mapping.go`:
  - Add `IsBenchmark bool` to `SymbolMapping` struct
  - Add `IsBenchmark bool` to `CreateRequest` struct
  - Add `IsBenchmark *bool` to `UpdateRequest` struct (pointer for omit-on-nil semantics)
- [x] Update `internal/data/symbol_mapping_repo.go`:
  - `toSymbolMapping()`: map `IsBenchmark` from sqlc model
  - `Create()`: pass `IsBenchmark` in params
  - `Update()`: pass `IsBenchmark` in params
  - Add `ListBenchmarks()` method using the new sqlc query

**Verification:** `sqlc generate` succeeds, `go build ./...` compiles, migration runs up/down cleanly.

### Task 2: Symbol mapping service — persist benchmark flag [PRIORITY: HIGH]
**Corresponds to:** Scenario: Create symbol with benchmark flag, Toggle benchmark flag (enable/disable)
**Description:** Update the symbol mapping service to persist the benchmark flag. No background fetch triggered — the market cache handles data population via its existing refresh cycle.

- [x] Update `Create()`: pass `req.IsBenchmark` to repo.Create (already handled via SymbolMapping struct)
- [x] Update `Update()`: handle `IsBenchmark` pointer — if non-nil, set on symbol mapping before calling repo.Update
- [x] Write unit tests:
  - `TestServiceCreate_WithBenchmark_PersistsFlag` — IsBenchmark=true saved correctly
  - `TestServiceCreate_WithoutBenchmark_DefaultsFalse` — IsBenchmark=false saved correctly
  - `TestServiceUpdate_EnableBenchmark` — toggle from false → true persisted
  - `TestServiceUpdate_DisableBenchmark` — toggle from true → false persisted
  - `TestServiceUpdate_IsBenchmarkNil_NoChange` — nil pointer leaves flag unchanged

**Verification:** All tests pass, service compiles, flag persisted correctly with no side effects.

### Task 3: API handler — include is_benchmark in CRUD [PRIORITY: HIGH]
**Corresponds to:** Scenario: Create symbol with benchmark flag, Toggle benchmark flag
**Description:** Update the symbol mapping API handler to accept and return the `is_benchmark` field.

- [x] `HandleCreate`: pass `req.IsBenchmark` through to service (already handled by CreateRequest)
- [x] `HandleUpdate`: pass `req.IsBenchmark` through to service (already handled by UpdateRequest)
- [x] `HandleGet`, `HandleList`: `IsBenchmark` is already on the SymbolMapping struct — serializes automatically via json tags
- [x] Write tests:
  - `TestHandleCreate_WithBenchmark_ReturnsTrue` — POST with `is_benchmark: true` returns it in response
  - `TestHandleCreate_WithoutBenchmark_ReturnsFalse` — POST without flag returns `false`
  - `TestHandleUpdate_EnableBenchmark` — PATCH with `is_benchmark: true` updates correctly
  - `TestHandleUpdate_DisableBenchmark` — PATCH with `is_benchmark: false` updates correctly

**Verification:** API tests pass, JSON serialization/deserialization works correctly.

### Task 4: Web handler + templates — benchmark checkbox [PRIORITY: HIGH]
**Corresponds to:** Scenario: Create symbol with benchmark flag, Toggle benchmark flag, Multiple benchmarks selected
**Description:** Add a "Use as benchmark" checkbox to the symbol mapping create/edit forms and display benchmark status in the list view. Checkbox is a regular form field — saves on submit alongside all other fields.

- [x] Update `symbolMappingFormPageData` struct: add `IsBenchmark bool`
- [x] Update `HandleNewPage`: initialize `IsBenchmark` to false
- [x] Update `HandleCreatePage`: read `r.FormValue("is_benchmark")` (checkbox sends "on" or nothing), set `req.IsBenchmark` accordingly
- [x] Update `HandleEditPage`: populate `IsBenchmark` from existing symbol mapping
- [x] Update `HandleUpdatePage`: read checkbox value, compare with current, set `req.IsBenchmark` (pointer — only send if changed)
- [x] Update `HandleListPage`: pass benchmark info to template
- [x] Update `templates/symbol_mapping/form.html`:
  - Add checkbox: `<input type="checkbox" id="is_benchmark" name="is_benchmark" {{if .IsBenchmark}}checked{{end}}> Use as benchmark`
  - Add help text: "Enables full historical price caching for portfolio performance comparison"
- [x] Update `templates/symbol_mapping/list.html`:
  - Add "Benchmark" column with a badge/indicator for benchmark symbols
- [x] Write web tests:
  - `TestHandleCreatePage_WithBenchmark` — form submission with checkbox checked creates benchmark
  - `TestHandleCreatePage_WithoutBenchmark` — form submission without checkbox creates non-benchmark
  - `TestHandleEditPage_LoadsBenchmark` — edit page shows checkbox checked for benchmark symbol
  - `TestHandleUpdatePage_ToggleBenchmark` — form submission toggles benchmark flag

**Verification:** Form renders correctly, checkbox works for create and edit, list shows benchmark indicator.

### Task 5: Market cache — use user-defined benchmarks [PRIORITY: HIGH]
**Corresponds to:** Scenario: Manual refresh all includes benchmark symbols, Background refresh includes benchmark symbols
**Description:** Replace the hardcoded `comparison.GetPredefined()` with a query against symbol mappings where `is_benchmark=1`. Add `FetchBenchmarkHistorical` method for on-demand full fetch.

- [x] Add `BenchmarkSymbolLister` interface to `marketcache` package
- [x] Add `WithBenchmarkLister(lister BenchmarkSymbolLister) *MarketCache` option on `MarketCache`
- [x] Add `FetchBenchmarkHistorical(symbol string)` method to `MarketCache` — fetches full history from 2000 to now, uses internal context with timeout, runs synchronously
- [x] Update `RefreshPredefinedBenchmarks()` → rename to `RefreshBenchmarks()`, query `benchmarkLister.ListBenchmarks()` instead of `comparison.GetPredefined()`, iterate over user-defined symbols (use `market_data_symbol` for fetching)
- [x] Update `gapFillBenchmarks()` → query `benchmarkLister.ListBenchmarks()` instead of `comparison.GetPredefined()`, same gap-fill logic
- [x] Update `doRefreshAll()` → call `RefreshBenchmarks()` (renamed) instead of `RefreshPredefinedBenchmarks()`
- [x] Write tests:
  - `TestFetchBenchmarkHistorical_FetchesFullHistory` — verifies full fetch from 2000
  - `TestRefreshBenchmarks_AllFetched` — refresh uses user-defined benchmarks
  - `TestRefreshBenchmarks_PartialFailure` — partial failure handled correctly
  - `TestRefreshAll_IncludesBenchmarks` — full refresh includes user benchmarks
  - `TestRefreshBenchmarks_NoLister_Skips` — graceful degradation when no lister configured
  - `TestGapFillBenchmarks_NoLister_Skips` — gap-fill skips when no lister
  - `TestRefreshBenchmarks_ListError_LogsAndContinues` — list error handled gracefully
  - `TestRefreshAll_NoBenchmarks` — refresh-all works with no benchmarks
  - Updated existing tests to use mock benchmark lister instead of hardcoded 5 benchmarks

**Verification:** Market cache compiles, all 32 tests pass, benchmark refresh uses user-defined symbols.

### Task 6: Performance API handler — accept user-defined benchmarks [PRIORITY: HIGH]
**Corresponds to:** Scenario: Multiple benchmarks selected, Benchmark historical data unavailable
**Description:** Remove the `comparison.IsValidPredefined` validation. Validate benchmark against user-defined benchmarks from symbol mappings.

- [x] Add optional `BenchmarkValidator` interface to `PerformanceHandler`:
  ```go
  type benchmarkValidator interface {
      IsBenchmark(ctx context.Context, marketDataSymbol string) bool
  }
  ```
- [x] Add `WithBenchmarkValidator(v benchmarkValidator) *PerformanceHandler` option
- [x] Replace `comparison.IsValidPredefined` check in `HandlePerformance` with `h.validator.IsBenchmark(ctx, filters.Benchmark)` (if validator nil, reject with error)
- [x] Write tests:
  - `TestHandlePerformance_ValidBenchmark` — benchmark from symbol mappings accepted
  - `TestHandlePerformance_InvalidBenchmark` — symbol not marked as benchmark rejected
  - `TestHandlePerformance_NoBenchmark` — empty benchmark works (no comparison)

**Verification:** API validates against user-defined benchmarks, rejects non-benchmark symbols.

### Task 7: Performance web handler — text input with autocomplete [PRIORITY: HIGH]
**Corresponds to:** Scenario: Multiple benchmarks selected, No benchmarks configured, Switch benchmark on performance page
**Description:** Replace the benchmark `<select>` dropdown with a text input that provides autocomplete suggestions from user-defined benchmarks. Server validates the submitted symbol.

- [x] Add `BenchmarkSymbolLister` dependency to `PerformanceWebHandler`:
  ```go
  type benchmarkSymbolLister interface {
      ListBenchmarks(ctx context.Context) ([]symbolmapping.SymbolMapping, error)
  }
  ```
- [x] Update `NewPerformanceWebHandler` to accept lister as required param
- [x] Update `HandlePerformance`:
  - Query `h.benchmarkLister.ListBenchmarks(ctx)` to build `BenchmarkNames` map (market_data_symbol → display name using internal_symbol)
  - Validate selected benchmark against the list (silently drop if not found)
  - If no benchmarks configured, set empty map and show message
- [x] Update `buildBenchmarkURLs`: build URLs from user-defined benchmark list
- [x] Update `templates/performance/index.html`:
  - Replace `<select>` with `<input type="text" id="benchmark" name="benchmark" list="benchmark-list" value="{{.SelectedBenchmark}}" placeholder="Select a benchmark...">`
  - Add `<datalist id="benchmark-list">` with `<option>` entries for each user-defined benchmark
  - Add JS `handleBenchmarkChange()` to navigate on selection
  - Show "No benchmarks configured. <a href="/symbol-mappings">Create one</a>." when list is empty
- [x] Write tests:
  - `TestLoadBenchmarkNames` — loads from lister, handles nil/empty/error
  - `TestPerformanceTemplate_NoBenchmarks` — shows "no benchmarks" message
  - `TestPerformanceTemplate_WithBenchmarkInput` — text input + datalist rendered
  - Updated `TestBuildBenchmarkURLs` — URLs built from user benchmarks
  - Updated `TestPerformanceTemplate_BenchmarkSelectorURLs` — datalist options
  - Updated `tests/integration/setupPerf` — creates ^GSPC as benchmark symbol

**Verification:** Performance page shows text input with autocomplete, switching works, empty state handled. All tests pass.

### Task 8: Router wiring — connect all new dependencies [PRIORITY: HIGH]
**Corresponds to:** All scenarios (cross-cutting integration)
**Description:** Wire the new dependencies in `router.go` so all components are connected.

- [x] Pass `symbolMappingRepo` to `marketCache` via `WithBenchmarkLister`:
  ```go
  marketCache.WithBenchmarkLister(symbolMappingRepo)
  ```
- [x] Pass `symbolMappingRepo` to `performanceHandler` via `WithBenchmarkValidator`
- [x] Pass `symbolMappingRepo` to `performanceWebHandler` via new constructor param
- [x] Verify `symbolMappingRepo` is created before consumers (it is — created early in router)
- [x] Verify `go build ./...` compiles cleanly
- [x] Verify no circular imports

**Verification:** Full build succeeds, no circular dependencies, all handlers wired correctly.

### Task 9: Delete comparison.Predefined + cleanup [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (remove legacy code)
**Description:** Remove the hardcoded predefined benchmark list and all references to it.

- [x] Delete `Predefined` map, `IsValidPredefined`, `GetPredefined` from `internal/domain/comparison/comparison.go`
- [x] Remove `comparison` import from `performance.go` (API handler)
- [x] Remove `comparison` import from `performance_web.go` (web handler)
- [x] Remove `comparison` import from `marketcache.go`
- [x] Delete or update tests in `comparison/comparison_test.go`
- [x] Check for any remaining references to `comparison.Predefined`, `comparison.GetPredefined`, `comparison.IsValidPredefined` across the codebase
- [x] If `comparison` package has other useful code (e.g., `ComputeMWRForPeriod`), keep the package but remove benchmark-specific code
- [x] Update any remaining test files that reference predefined benchmarks

**Verification:** `go build ./...` compiles, no references to `comparison.Predefined` remain, all tests pass.

### Task 10: Validation / Hardening [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (cross-cutting quality gate)
**Description:** After implementation tasks are complete, validate the feature end-to-end before declaring it done.

- [ ] Run all tests (`go test ./...`) — not just `-short`
- [ ] Verify each spec scenario manually or via integration test
- [ ] Check edge cases from the spec against actual behavior:
  - Symbol marked as benchmark but not found on Yahoo — warning shown on performance page
  - Benchmark symbol deleted — no longer available as benchmark
  - Provider symbol changed on active benchmark — data refreshed on next "Refresh All"
  - Re-submitting with benchmark already enabled — no side effects
  - No benchmarks configured — performance page shows "no benchmarks" message
- [ ] Run `go vet ./...` and linter
- [ ] Review for cross-layer consistency (data types stored match data types read)
- [ ] Verify `is_benchmark` field appears in all read paths (API list, API get, web list, web edit)
- [ ] Verify no TODOs, FIXMEs, or temporary workarounds remain
- [ ] Verify autocomplete UX works (datalist suggestions appear, selection works)

**Verification:** All tests pass, all spec scenarios validated, no unresolved issues.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| `is_benchmark` column type | `BOOLEAN NOT NULL DEFAULT 0` | SQLite boolean, explicit default prevents ambiguity |
| `UpdateRequest.IsBenchmark` type | `*bool` (pointer) | Matches existing pattern for optional fields; nil = no change |
| Benchmark data population | Market cache refresh cycle only | Uses existing infrastructure; no immediate fetch on save |
| Benchmark input UX | Text input + `<datalist>` autocomplete | Flexible — user can type or pick from suggestions |
| `comparison.Predefined` | Deleted entirely | No fallback; user-defined only |
| Performance API validation | `BenchmarkValidator` interface on handler | Validates against DB; rejects non-benchmark symbols |
| Full historical fetch start date | 2000-01-01 | Consistent with existing benchmark behavior |
| `FetchBenchmarkHistorical` on MarketCache | Public method, synchronous | Reusable by any caller; caller handles goroutine if needed |

## Risks

- **No benchmarks configured**: Performance page shows empty state with link to create one. No crash or error.
- **Long initial fetch for very old benchmarks**: Handled by market cache's existing refresh mechanism (manual "Refresh All" or background gap-fill). User sees "no cached data" warning until fetch completes.
- **Existing tests referencing `comparison.GetPredefined`**: All updated to use user-defined benchmarks or deleted.
- **Migration on existing data**: `is_benchmark` defaults to `false` (0), so existing symbol mappings are unaffected.
- **Datalist autocomplete browser support**: `<datalist>` is widely supported (Chrome, Firefox, Safari, Edge). Falls back to plain text input on unsupported browsers.
