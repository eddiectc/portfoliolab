# Implementation Plan: Model Portfolio

## Overview

Build a standalone **Model Portfolio** feature: named allocation blueprints (symbol + weight %) that can be created, managed, and applied as target allocations on real portfolios. Consists of a new domain (`modelportfolio`), one database table (entries stored as JSON), CRUD API + web UI, and integration into the Allocation page for "apply as target".

## Task Dependencies

```
Task 1 (DB migration)
    ↓
Task 2 (Domain models + validation)
    ↓
Task 3 (Repository + Service CRUD)
    ↓
Task 4 (API handlers)
    ↓
Task 5 (Web UI — list/create/edit/delete)
    ↓
Task 6 (Inline symbol creation)
    ↓
Task 7 (Apply as target allocation)
    ↓
Task 8 (Nav + integration tests)
```

Tasks 1–3 are foundational (data layer). Tasks 4–6 build the model portfolio management UI. Task 7 integrates with f018 allocation. Task 8 ties everything together.

## Tasks

### Task 1: Database migration [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundation)
**Description:** Create `model_portfolios` table with entries stored as a JSON column. Add sqlc SQL file and update schema.sql.

- [x] Create migration `020_create_model_portfolios.sql` with `model_portfolios` table (id, name UNIQUE, entries TEXT JSON, created_at, updated_at)
- [x] Create `internal/data/queries/model_portfolio.sql` with sqlc queries (CRUD: Create, GetByID, GetByName, List, Update, Delete)
- [x] Update `internal/data/queries/schema.sql` with new table
- [x] Run `sqlc generate` to produce Go types
- [x] Write migration smoke test (verifies table exists with correct columns)

**Verification:** `goose up` succeeds, `sqlc generate` produces clean Go code, migration smoke test passes.

---

### Task 2: Domain models + validation [PRIORITY: HIGH]
**Corresponds to:** Reject invalid weights, reject negative/zero weights, reject duplicate name
**Description:** Define domain types (`ModelPortfolio`, `ModelPortfolioEntry`) and validation logic (weights sum to 100%, each > 0%, unique name, name length ≤ 100).

- [x] Create `internal/domain/modelportfolio/model_portfolio.go` with:
  - `ModelPortfolio` struct (ID, Name, Entries `[]ModelPortfolioEntry`, CreatedAt, UpdatedAt)
  - `ModelPortfolioEntry` struct (Symbol, WeightPct decimal.Decimal) — marshaled to/from JSON
  - `CreateRequest` / `UpdateRequest` DTOs
  - Error variables: `ErrNotFound`, `ErrNameExists`, `ErrInvalidName`, `ErrWeightSumNot100`, `ErrInvalidWeight`, `ErrDuplicateSymbol`, `ErrEmptyEntries`
  - `ModelPortfolioError` type (Code, Message) for sum/weight errors with details
- [x] Create `internal/domain/modelportfolio/validator.go` with:
  - `ValidateCreateRequest()` — name length, non-empty
  - `ValidateEntries()` — each weight > 0, no duplicate symbols, sum == 100 (0.01% tolerance)
  - Error messages include current total and delta (matching allocation pattern)
- [x] Write unit tests: `validator_test.go` with table-driven tests for all validation rules

**Verification:** All validator unit tests pass; covers happy path, sum≠100, negative weight, zero weight, empty entries, duplicate symbols, name too long, empty name.

---

### Task 3: Repository + Service CRUD [PRIORITY: HIGH]
**Corresponds to:** Create (happy path), edit, delete, list, empty list
**Description:** Implement data access layer (sqlc-backed repository) and service layer with full CRUD. Entries stored as JSON in the `entries` column — marshaled/unmarshaled at the repo boundary.

- [ ] Create `internal/data/model_portfolio_repo.go`:
  - `ModelPortfolioRepository` struct
  - `Create`, `GetByID`, `List`, `Update`, `Delete` for model portfolios
  - `GetByName` for uniqueness check
  - JSON marshal/unmarshal of `entries` TEXT column (entries → `[]ModelPortfolioEntry` JSON)
- [ ] Create `internal/domain/modelportfolio/service.go`:
  - `Repository` interface (matches repo methods)
  - `Service` struct with repository dependency
  - `Create(ctx, CreateRequest)` — validate, check name uniqueness, marshal entries to JSON, insert
  - `Get(ctx, id)` — fetch portfolio, unmarshal entries from JSON
  - `List(ctx, limit, offset)` — list portfolios (entries included in each row)
  - `Update(ctx, id, UpdateRequest)` — update name, marshal entries to JSON, update row
  - `Delete(ctx, id)` — delete portfolio row
  - `GetAllForSelector(ctx)` — lightweight list (id, name, entry count from JSON length) for dropdowns
- [ ] Write unit tests: `service_test.go` with hand-written mock repository (maps/slices, simulates real behavior like allocation `target_test.go` pattern)

**Verification:** All service unit tests pass; covers Create/Get/List/Update/Delete CRUD cycle, name uniqueness, entry validation delegation, empty list returns `[]` not nil, JSON round-trip preserves entry data.

---

### Task 4: API handlers [PRIORITY: HIGH]
**Corresponds to:** All scenarios (API layer)
**Description:** REST API handlers for model portfolio CRUD with consistent error responses.

- [ ] Create `internal/api/handlers/model_portfolio.go`:
  - `ModelPortfolioHandler` struct with service dependency
  - `RegisterRoutes()` — mounts CRUD routes under `/api/model-portfolios`
  - `HandleList` — GET `/api/model-portfolios` (paginated)
  - `HandleCreate` — POST `/api/model-portfolios` (JSON body with name + entries)
  - `HandleGet` — GET `/api/model-portfolios/{id}`
  - `HandleUpdate` — PATCH `/api/model-portfolios/{id}`
  - `HandleDelete` — DELETE `/api/model-portfolios/{id}`
  - Error handler mapping domain errors to HTTP status codes
- [ ] Write unit tests: `model_portfolio_test.go` with `httptest.NewRecorder` + mock service
- [ ] Wire handler into `internal/api/router.go`
- [ ] Update `API.md` with model portfolio endpoint documentation

**Verification:** All API handler tests pass; routes registered on router; API.md updated.

---

### Task 5: Web UI — list, create, edit, delete [PRIORITY: HIGH]
**Corresponds to:** List model portfolios, empty list, create, edit, delete, reject duplicate name
**Description:** Server-rendered web pages for managing model portfolios.

- [ ] Create `internal/api/handlers/model_portfolio_web.go`:
  - `ModelPortfolioWebHandler` struct
  - `HandleList` — GET `/model-portfolios` (list page with empty state)
  - `HandleCreate` — GET `/model-portfolios/new` (form page)
  - `HandleCreatePost` — POST `/model-portfolios` (save, redirect with flash)
  - `HandleEdit` — GET `/model-portfolios/{id}` (edit form)
  - `HandleEditPost` — POST `/model-portfolios/{id}` (save, redirect with flash)
  - `HandleDeletePost` — POST `/model-portfolios/{id}/delete` (delete, redirect with flash)
  - Fetch symbols for autocomplete datalist
- [ ] Create `templates/model_portfolio/list.html` — table of portfolios (name, symbol count, created date, edit/delete links), empty state with "create first" prompt
- [ ] Create `templates/model_portfolio/form.html` — name input, dynamic symbol+weight rows (JS add/remove), total % calculator, symbol autocomplete datalist
- [ ] Wire web handler into `internal/api/router.go`
- [ ] Write web handler tests: `model_portfolio_web_test.go` (renders 200, POST redirects 303)

**Verification:** Pages render 200; form POST redirects 303; empty state shown when no portfolios; web handler tests pass.

---

### Task 6: Inline symbol creation [PRIORITY: MEDIUM]
**Corresponds to:** Create model portfolio with inline symbol creation
**Description:** When a ticker entered in the model portfolio form doesn't exist in the system, create it automatically before saving the model portfolio.

- [ ] Add `SymbolCreator` interface to model portfolio service (mirrors `transaction.SymbolCreator`)
- [ ] In service `Create`/`Update`: for each entry symbol, check existence via `GetByInternalSymbol`; if not found, call `SymbolCreator.CreateSymbol(ticker, ticker)` to auto-create
- [ ] After symbol creation, trigger market data fetch (non-blocking goroutine via existing `SymbolDetailsFetcher` or market cache)
- [ ] Update `router.go` to wire `symbolmapping.Service` as the symbol creator dependency
- [ ] Write unit tests covering: symbol already exists (no-op), symbol created inline, symbol creation fails (returns error)

**Verification:** Unit tests pass; inline creation works when symbol missing; no duplicate creation when symbol exists.

---

### Task 7: Populate target from model portfolio on Allocation page [PRIORITY: HIGH]
**Corresponds to:** Apply model portfolio as target allocation (with/without existing target, symbols not in real portfolio)
**Description:** Add a model portfolio selector on the Allocation page. Selecting a model portfolio pre-fills the target allocation form — user can review, adjust, save via existing "Save" button, or cancel. No new API endpoint or service changes needed.

- [ ] Update `allocationPageData` struct to include `ModelPortfolios []ModelPortfolio` for dropdown
- [ ] Wire model portfolio service into `AllocationWebHandler` (constructor + router.go)
- [ ] Update `HandleAllocation` to fetch model portfolios for dropdown
- [ ] Update `templates/allocation/list.html`:
  - Add model portfolio dropdown above the target entries section
  - Add "Load" button next to dropdown
  - Add JS handler: on click, fetch `GET /api/model-portfolios/{id}` and populate target entry fields (symbol_N, target_pct_N)
  - Clear existing entries before populating
  - Trigger `calculateTargetTotal()` after population
- [ ] Write web handler test: allocation page renders with model portfolios in data

**Verification:** Dropdown populated with model portfolios; selecting one pre-fills the target form; user can edit/save/cancel via existing controls; no new backend endpoints needed.

---

### Task 8: Navigation + integration tests [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (end-to-end verification)
**Description:** Add nav link and integration tests for the full stack.

- [ ] Add "Model Portfolios" link to `templates/partials/nav.html`
- [ ] Create `tests/integration/model_portfolio_test.go`:
  - Test: Create model portfolio via API → GET returns it
  - Test: Create with invalid weights → 400 error
  - Test: Create with inline symbol → symbol + portfolio both created
  - Test: Edit model portfolio → updated entries returned
  - Test: Delete model portfolio → 204, GET returns 404
  - Test: Apply model portfolio as target → target allocation matches model
  - Test: Web page renders 200 (list, new, edit)
- [ ] Run `go test ./...` to verify full test suite passes

**Verification:** All integration tests pass; nav link visible; `go test ./...` clean.

---

## Technical Decisions

| Decision | Choice | Reason | Alternatives Considered |
|---|---|---|---|
| **Model portfolio scope** | Global (not per-portfolio) | Spec says "named model portfolio" — blueprints are reusable across portfolios | Per-portfolio would duplicate the concept and prevent sharing |
| **Single table with JSON entries** | `model_portfolios` with `entries TEXT` (JSON array) | Simpler schema (one table, no JOINs); matches `symbol_details` pattern of storing nested data as JSON; entries are always CRUD'd as a complete set | Separate table (`model_portfolio_entries`) would match `target_allocations` pattern but adds unnecessary complexity when entries are never queried individually |
| **Inline symbol creation** | Use existing `symbolmapping.Service.Create()` | Follows existing pattern from broker imports (f007/f008); gets full symbol mapping with market data fetch | Simplified ticker-only approach would create orphaned symbols |
| **Load model portfolio on allocation page** | Web handler + JS only (no service changes) | Reuses existing target save flow; user can review/adjust before saving; no circular dependency | Direct "apply" endpoint would bypass user review and require service-layer changes |
| **Weight validation tolerance** | Exact 100% match (decimal precision) | `decimal.Decimal` has exact arithmetic — no floating-point drift; 0.01% tolerance in spec is for display only | Rounding to nearest 0.01% could mask user input errors |
| **Symbol reference integrity** | Application-level only (no FK on JSON column) | JSON column can't have FK constraints; service-layer validation ensures symbols exist before storing | DB-level FK would require a separate entries table |

## Risks

- **None** — the allocation service is not modified; model portfolio is only used in the web handler and template layer.
- **Inline symbol creation failures** — If Yahoo Finance is unavailable, the symbol is still created (the `symbolmapping.Service.Create()` doesn't block on market data). Market data fetch runs in a background goroutine.
- **Template complexity** — The model portfolio form (dynamic rows, total calculator) mirrors the existing target allocation form in the allocation page. Reuse the same JS pattern to minimize new code.
- **JSON corruption** — If malformed JSON is written to the `entries` column, reads would fail. Mitigated by always marshaling through the Go type system (never raw SQL writes) and returning explicit errors on unmarshal failure.
