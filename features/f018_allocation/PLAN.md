# Implementation Plan: Allocation

## Overview

A dedicated **Allocation** page showing portfolio distribution by symbol, with target weight management and rebalancing suggestions. Aggregates positions by symbol across accounts (unlike Positions which shows per-account). Includes cash converted to base currency via FX rates.

The feature adds:
- A new `allocation` domain package with computation logic
- A `target_allocations` database table for persisting user-defined target weights
- API endpoints for allocation data, target CRUD, drift, and rebalancing
- A server-rendered web page with portfolio filter, expandable rows, and target editing
- Navigation link in the navbar

## Task Dependencies

```
Task 1 (Migration) ──→ Task 4 (Target CRUD)
                          ↓
Task 2 (Types) ──→ Task 3 (Current Allocation) ──→ Task 5 (Drift) ──→ Task 6 (Rebalancing)
                                                              ↓
                                                        Task 7 (API Handlers)
                                                              ↓
Task 8 (Web Handler + Template) ──→ Task 9 (Integration Tests) ──→ Task 10 (Router Wiring)
```

Tasks 1-6 are domain layer (can be developed largely in parallel for 3-6 once types are done). Tasks 7-10 are the integration/presentation layer.

## Tasks

### Task 1: Database migration for target allocations [PRIORITY: HIGH]
**Corresponds to:** Scenario: Create a target allocation, Scenario: Edit an existing target allocation, Scenario: Delete a target allocation
**Description:** Create the `target_allocations` table and sqlc queries for CRUD operations.

- [x] Write migration `019_create_target_allocations.sql` with `target_allocations` table:
  - `id` (INTEGER PRIMARY KEY), `portfolio_id` (INTEGER NOT NULL, FK → portfolios), `symbol` (TEXT NOT NULL), `target_pct` (TEXT NOT NULL — stores decimal as string), `created_at`, `updated_at`
  - UNIQUE constraint on `(portfolio_id, symbol)`
  - Index on `portfolio_id` for lookups
- [x] Add sqlc queries to `queries/target_allocation.sql`:
  - `GetTargetAllocationsByPortfolio` (SELECT all for a portfolio)
  - `UpsertTargetAllocation` (INSERT OR REPLACE for a portfolio+symbol)
  - `DeleteTargetAllocationBySymbol` (DELETE for portfolio+symbol)
  - `DeleteTargetAllocationsByPortfolio` (DELETE all for a portfolio)
- [x] Run `sqlc generate` to produce Go types
- [x] Write migration smoke test (verify table schema)

**Verification:** `sqlc generate` succeeds, migration runs cleanly with `goose up`, smoke test passes. ✅

---

### Task 2: Allocation domain — types and models [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundation types)
**Description:** Define the domain types for allocation, target, drift, and rebalancing.

- [x] Create `internal/domain/allocation/allocation.go` with:
  - `AllocationRow` — symbol, market_value (decimal), market_value_base (decimal ptr), allocation_pct (decimal), currency, has_market_data (bool), account_breakdown ([]AccountBreakdown)
  - `AccountBreakdown` — account_id, account_name, quantity, market_value, market_value_base, pct_of_symbol
  - `AllocationResult` — rows ([]AllocationRow), total_value_base (decimal), base_currency (string), cash_row (*AllocationRow), last_updated (time.Time), market_data_available (bool)
  - `AllocationFilter` — portfolio_ids ([]int64, empty = all portfolios)
  - `TargetAllocation` — portfolio_id, symbol, target_pct (decimal)
  - `DriftRow` — symbol, actual_pct (decimal), target_pct (decimal), drift_pct (decimal, actual - target), is_balanced (bool, |drift| ≤ 5%)
  - `DriftResult` — rows ([]DriftRow), base_currency, has_target (bool)
  - `RebalanceSuggestion` — symbol, direction ("buy"/"sell"), shares (decimal), dollar_value (decimal), drift_reduction (decimal)
  - `RebalanceResult` — suggestions ([]RebalanceSuggestion), base_currency, total_dollar_value (decimal), warnings ([]string), is_balanced (bool)
  - Error variables: `ErrNoPortfolios`, `ErrInvalidTargetPct`, `ErrTargetSumNot100`
- [x] Create `internal/domain/allocation/allocation_test.go` with basic type verification tests

**Verification:** Types compile, basic tests pass, types match spec constraints. ✅

---

### Task 3: Allocation service — current allocation computation [PRIORITY: HIGH]
**Corresponds to:** Scenario: View allocation across all portfolios, Scenario: View allocation for a single portfolio, Scenario: View allocation for multiple selected portfolios, Scenario: Drill down to per-account breakdown, Scenario: View allocation for a cash-only portfolio, Scenario: Multi-currency cash aggregation
**Description:** Compute current allocation from open positions, aggregating by symbol across accounts, including cash converted to base currency.

- [x] Create `internal/domain/allocation/service.go` with:
  - Service struct with dependencies: `PositionSource` (interface wrapping position service), `MarketDataService` (interface for FX rates), `AccountLister` (for resolving portfolio → accounts)
  - `ComputeAllocation(ctx, filter)` method:
    1. Resolve account IDs from filter (portfolio_ids → accounts, empty = all)
    2. Fetch open positions for all accounts (unbounded)
    3. Enrich with market data via position service (reuses `EnrichWithMarketData`)
    4. Group by symbol, sum market values, compute allocation % = symbol_mv_base / total_mv_base × 100
    5. Cash aggregation: all `$CASH-*` positions → single "Cash" row, converted to base currency
    6. Build account breakdown per symbol (for drill-down)
    7. Return AllocationResult
- [x] Handle edge cases:
  - Empty portfolio → empty result with message
  - Zero/negative total value → error
  - FX rate unavailable for cash → warning, exclude from total
  - Single holding → 100% allocation
- [x] Create `internal/domain/allocation/service_test.go` with:
  - Hand-written mock for `PositionSource` (returns pre-built positions)
  - Table-driven tests for: single symbol, multiple symbols, cash-only, multi-currency cash, empty portfolio, FX missing
  - Verify allocation percentages sum to ~100%
  - Verify cash aggregation across currencies

**Verification:** All unit tests pass, allocation percentages are correct against known values. ✅

---

### Task 4: Target allocation — CRUD operations [PRIORITY: HIGH]
**Corresponds to:** Scenario: Create a target allocation, Scenario: Edit an existing target allocation, Scenario: Delete a target allocation, Scenario: Reject target allocation with invalid percentages
**Description:** Persist and validate user-defined target allocations per portfolio.

- [ ] Create `internal/domain/allocation/target_repository.go` with:
  - Repository interface: `GetByPortfolio`, `Upsert`, `DeleteBySymbol`, `DeleteByPortfolio`
  - Wire to sqlc-generated queries
- [ ] Add target CRUD methods to allocation service:
  - `GetTargetAllocation(ctx, portfolioID)` → []TargetAllocation
  - `SaveTargetAllocation(ctx, portfolioID, entries []TargetEntry)` with validation:
    - Each pct ∈ [0, 100]
    - Sum of all pcts == 100 (exact)
    - Return error with current total and delta if sum ≠ 100
  - `DeleteTargetAllocation(ctx, portfolioID, symbol)` → error
  - `DeleteAllTargetAllocations(ctx, portfolioID)` → error
- [ ] Create `internal/domain/allocation/target_test.go` with:
  - Table-driven validation tests: negative pct, >100 pct, sum < 100, sum > 100, valid sum
  - Mock repository tests for CRUD operations
  - Test that save rejects with informative error message

**Verification:** Validation rejects invalid inputs with correct error messages, CRUD operations work with mock repo.

---

### Task 5: Drift computation [PRIORITY: MEDIUM]
**Corresponds to:** Scenario: Compare actual vs target allocation, Scenario: View allocation without a saved target
**Description:** Compare actual allocation against target, compute drift per symbol.

- [ ] Add `ComputeDrift(ctx, filter, portfolioID)` method to allocation service:
  1. Compute actual allocation for the portfolio
  2. Fetch target allocation for the portfolio
  3. Build unified symbol list (union of actual + target symbols)
  4. For each symbol: actual_pct (0 if not held), target_pct (0 if not in target), drift = actual - target
  5. Mark balanced if |drift| ≤ 5%
  6. Return DriftResult
- [ ] Handle edge cases:
  - No target saved → DriftResult with has_target=false, actual % only
  - Symbol in target but not held → actual = 0%, drift = -target%
  - Symbol held but not in target → target = 0%, drift = actual%
  - Cash drift included
- [ ] Create `internal/domain/allocation/drift_test.go` with:
  - Table-driven tests for drift tolerance boundary (4.9% → balanced, 5.0% → balanced, 5.1% → not balanced)
  - Tests for missing target, symbol in target only, symbol in actual only
  - Verify drift = actual - target sign convention

**Verification:** Drift values match expected calculations, tolerance boundary is correct.

---

### Task 6: Rebalancing suggestions [PRIORITY: MEDIUM]
**Corresponds to:** Scenario: Rebalancing suggestions, Scenario: Rebalancing suggestions with no drift, Scenario: Rebalancing suggestions with stale market data, Scenario: Target allocation includes symbols not yet held, Scenario: Allocation excludes symbols with zero weight in target
**Description:** Generate trade suggestions to close the gap between actual and target allocation.

- [ ] Add `ComputeRebalancingSuggestions(ctx, filter, portfolioID)` method to allocation service:
  1. Compute drift result
  2. For each symbol with |drift| > 5%:
     - If drift > 0 (overweight): suggest SELL shares_to_sell = drift_value / current_price
     - If drift < 0 (underweight): suggest BUY shares_to_buy = abs(drift_value) / current_price
     - Where drift_value = drift_pct / 100 × total_portfolio_value
  3. Sort by |drift| descending
  4. Symbols without market data → warning, excluded from suggestions
  5. If all symbols within tolerance → is_balanced = true
- [ ] Handle edge cases:
  - Symbol in target but not yet held → buy full target amount
  - Symbol with target 0% → sell full current holding
  - Stale/unavailable market data → warning entry
  - Zero total value → error
- [ ] Create `internal/domain/allocation/rebalance_test.go` with:
  - Table-driven tests with known-correct share quantities and dollar values
  - Tolerance boundary tests (4.9% drift → no suggestion, 5.1% → suggestion)
  - No-drift scenario (all within tolerance → balanced message)
  - Symbol not yet held (full buy)
  - Symbol with zero target (full sell)
  - Missing market data (warning)

**Verification:** Share quantities and dollar values match hand-calculated expected values.

---

### Task 7: API handlers [PRIORITY: HIGH]
**Corresponds to:** All scenarios (API layer)
**Description:** HTTP handlers for allocation data, target CRUD, drift, and rebalancing.

- [ ] Create `internal/api/handlers/allocation.go` with:
  - `AllocationHandler` struct with allocation service dependency
  - `NewAllocationHandler(service)` constructor
  - `RegisterRoutes(r *chi.Mux)` mounting:
    - `GET /api/allocation` → current allocation
    - `GET /api/allocation/target` → target allocation for portfolio
    - `POST /api/allocation/target` → save/update target allocation
    - `DELETE /api/allocation/target` → delete target allocation (all)
    - `GET /api/allocation/drift` → drift comparison
    - `GET /api/allocation/rebalance` → rebalancing suggestions
- [ ] Handler implementations:
  - Parse query params: `portfolio_id` (optional, single or comma-separated for multiple)
  - Delegate to allocation service methods
  - Return JSON responses with standard error format
  - Target save validates request body (array of {symbol, target_pct})
- [ ] Create `internal/api/handlers/allocation_test.go` with:
  - Hand-written mock for allocation service
  - Tests for each endpoint: happy path, error cases, validation errors
  - Verify JSON response structure matches API conventions

**Verification:** All handler tests pass, endpoints return correct JSON format, error responses follow `{"error": "...", "code": "..."}` pattern.

---

### Task 8: Web handler + template [PRIORITY: HIGH]
**Corresponds to:** All scenarios (web UI)
**Description:** Server-rendered allocation page with actual %, target %, drift columns, portfolio filter, expandable rows, and target editing form.

- [ ] Create `internal/api/handlers/allocation_web.go` with:
  - `AllocationWebHandler` struct with API handler + service dependencies
  - `RegisterRoutes(r *chi.Mux)` mounting:
    - `GET /allocation` → allocation page
    - `POST /allocation/target` → save target (redirect with flash)
    - `POST /allocation/target/delete` → delete target (redirect with flash)
  - `HandleAllocation(w, r)` — renders allocation page:
    1. Parse portfolio filter from query
    2. Call API handler for current allocation
    3. If portfolio filter is single portfolio, call API for drift + rebalance
    4. Build template data (URLs pre-built, chart data as JSON)
    5. Render template
  - `HandleSaveTarget(w, r)` — parse form, validate, save, redirect
  - `HandleDeleteTarget(w, r)` — delete, redirect
- [ ] Create `templates/allocation/list.html`:
  - Portfolio filter dropdown (all / specific)
  - Allocation table: Symbol | Market Value | Actual % | Target % | Drift | Actions
  - Expandable rows for per-account breakdown (JS toggle)
  - Target editing form (inline, per-portfolio)
  - Drift visual indicators (positive/negative CSS classes)
  - Rebalancing suggestions section (when target exists)
  - Empty state message
  - "Last updated" timestamp
- [ ] Update `templates/partials/nav.html` — add "Allocation" link
- [ ] Create `internal/api/handlers/allocation_web_test.go` with:
  - Template rendering tests (200 OK with HTML content, with data and empty)
  - Filter parameter tests
  - Flash message tests for save/delete

**Verification:** Page renders 200 OK, template has no undefined field errors, nav link appears, flash messages work.

---

### Task 9: Integration tests [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios (full-stack verification)
**Description:** End-to-end tests exercising DB → service → handler → response.

- [ ] Create `tests/integration/allocation_test.go` with:
  - Setup: create portfolio, account, symbol mappings, buy transactions, trigger position recalc
  - Test: GET /api/allocation returns 200 with correct allocation percentages
  - Test: POST /api/allocation/target saves target, GET returns saved target
  - Test: POST with invalid target (sum ≠ 100) returns 400 with error
  - Test: GET /api/allocation/drift returns drift data
  - Test: GET /api/allocation/rebalance returns suggestions
  - Test: Multi-currency cash aggregation (USD + GBP cash → correct USD total)
  - Test: Web page renders 200 OK with HTML
  - Test: Empty portfolio shows empty state
  - Test: DELETE /api/allocation/target removes target

**Verification:** `go test ./tests/integration/...` passes, all endpoints return expected responses.

---

### Task 10: Wire up in router [PRIORITY: HIGH]
**Corresponds to:** All scenarios (integration)
**Description:** Register allocation handlers in the main router.

- [ ] Update `internal/api/router.go`:
  - Import allocation domain
  - Create allocation service with dependencies (position service, market service, account lister, portfolio currency checker)
  - Create allocation API handler and register routes
  - Create allocation web handler and register routes
- [ ] Verify `go build` succeeds
- [ ] Run `go test ./...` to ensure no regressions

**Verification:** Server starts, allocation routes are accessible, no build errors.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| **Domain package location** | New `internal/domain/allocation/` package | Distinct from existing `analysis` package (which does sector/geographic allocation). Follows convention of one domain per package. |
| **Current allocation computation** | Reuse `position.Service.EnrichWithMarketData` | Follows API-first architecture: allocation service delegates to position service for enrichment, adds aggregation layer on top. No duplicated market data logic. |
| **Cash handling** | Aggregate `$CASH-*` positions to single "Cash" row | Cash positions already exist as `$CASH-{currency}` symbols. Convert each to base currency via FX, sum to single row. Matches spec: "Cash can have a target weight." |
| **Target allocation storage** | New `target_allocations` table (relational) | Follows existing pattern of relational tables. JSON column would make queries harder. Per-portfolio + per-symbol uniqueness via DB constraint. |
| **Rebalancing suggestions** | Ephemeral computation (not persisted) | Spec says "informational only — no trading execution." No need to persist advisory data. Computed fresh each request. |
| **Target percentage validation** | Exact sum = 100% | Spec constraint: "Target percentages must sum to exactly 100% — saves are rejected otherwise." Error response includes current total and delta. |
| **Drift tolerance** | 5% hard-coded constant | Spec constraint: "Drift tolerance is 5%." Not configurable per spec. |
| **Multiple portfolio selection** | `portfolio_ids` query param (comma-separated) | Consistent with existing `account_ids` filter pattern. Empty = all portfolios. |
| **Decimal storage** | `TEXT` column storing decimal string | Follows existing convention: "Use TEXT for monetary amounts; repo layer converts to/from decimal.Decimal." |
| **Drift column visual** | CSS classes `drift-positive` / `drift-negative` | Follows existing pattern of `heat-positive` / `heat-negative` CSS classes in templates. |

## Risks

- **Market data availability**: If market data cache is stale or missing for symbols, allocation percentages and rebalancing suggestions may be inaccurate. Mitigation: show "last updated" timestamp and warnings for symbols without data.
- **FX rate gaps**: If FX rates are unavailable for some cash currencies, those cash balances are excluded from totals. Mitigation: explicit warning in the result, excluded values noted.
- **Performance with many positions**: Aggregating positions across all accounts could be slow for large portfolios. Mitigation: positions are already cached and enriched by the position service; allocation adds only a grouping step.
- **Decimal precision**: Allocation percentages must sum to ~100% despite rounding. Mitigation: use high-precision decimal arithmetic, round only for display (1dp per spec).
