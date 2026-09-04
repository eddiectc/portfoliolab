# Implementation Plan: Seed Data for New Deployment

> **CONFIRMED — all decisions agreed with user.**
>
> Confirmed: **D1 Option A** (2 initial deposit transactions), **D7** (existing
> `RecalculatePortfolio` path), SPEC.md revision via **in-place edit** with changelog note.

## Overview

Automatic startup seed that creates a fully functional sample deployment (GBP "Sample"
portfolio, 2 currency accounts, 2 initial deposits + 8 trades across 4 real symbols,
embedded symbol details, 70/30 target allocations, "Sample Model" model portfolio with
benchmark flags) — and only when no portfolio exists. The data write is one atomic
transaction; positions are then derived through the **existing** recalculation path
(`position.Service.RecalculatePortfolio` — the same method used by the manual
`POST /api/positions/recalculate` endpoint and run automatically after every
transaction mutation). No new API surface, no web UI (non-goals in spec).

## Task Dependencies

```
Task 1 (content) ─> Task 2 (service) ─> Task 3 (data layer) ─> Task 4 (integration tests) ─> Task 5 (router wiring) ─> Task 6 (docs/bookkeeping)
```
Tasks are strictly sequential (each builds on the previous).

## Tasks

### Task 1: Seed domain package — content types + fixed dataset [PRIORITY: HIGH]
**Corresponds to:** Sample Data (all sections); Edge Case "Whole-share rounding"; Constraints
**Description:** Create `internal/domain/seed/` with the fixed dataset expressed in
existing domain types (no parallel type system).

- [ ] `internal/domain/seed/content.go`: types `Content`, `AccountSeed`, `SymbolSeed`
      (fields: `Portfolio portfolio.Portfolio`; `Accounts []AccountSeed`
      (each: `account.Account` + `[]transaction.Transaction`); `Symbols []SymbolSeed`
      (each: `symbolmapping.SymbolMapping` + `symbol.SymbolDetails`);
      `Model modelportfolio.ModelPortfolio`; `ModelTargets` and `PortfolioTargets`
      as `[]allocation.TargetEntry`); function `Content() *Content` returning the exact dataset.
- [ ] Dataset values per SPEC.md §Sample Data:
      - Portfolio "Sample", base currency GBP.
      - Accounts: `sample-gbp` (GBP), `sample-usd` (USD).
      - Symbols: VWRP.L, VUTA.L (both `IsBenchmark: true`), GOOG, BRK-B (benchmark false);
        internal symbol = ticker string (D3); `MarketDataSymbol` = same ticker;
        embedded details with fixed short/long name, exchange, currency
        (VWRP.L GBP, VUTA.L GBP, GOOG USD, BRK-B USD), quote type, sector
        (GOOG: Communication Services, BRK-B: Financial Services), geographic
        allocations (both ETFs). Exact values documented in NOTES.md.
      - Per account: **2 deposits first** (D1): `$CASH-GBP` / `$CASH-USD`,
        quantity 100000, price 1, net_cash +100000, date 2024-07-15 —
        following the app's existing deposit convention (trading212 import:
        symbol `$CASH-{ccy}`, price 1).
      - Then the 8 spec trade rows verbatim with net_cash pinned: buys negative
        (−69930.00, −29983.40, −49950.00, −49830.00, −2116.80, −30400.00),
        sells positive (+2070.00, +30600.00).
- [ ] `Model` = "Sample Model", entries 70% / 30% (internal symbols of VWRP.L / VUTA.L);
      `ModelTargets` 70/30; `PortfolioTargets` 70/30 (same symbols).
- [ ] `content_test.go`: pin the entire dataset — assert every field of the 10
      transactions (dates, symbols, types, prices, quantities, **signed net_cash**),
      account names/currencies, symbol benchmark flags, model entries and both
      target sets (each sums to 100). Cash check via `position.ComputeCashPositions`:
      `$CASH-GBP` = 39.80, `$CASH-USD` = 370.00 (the spec's own remainder figures).

**Verification:** `go test ./internal/domain/seed/...` passes; dataset matches the spec table cell-for-cell.

### Task 2: Seed service — guard handling, lot IDs, recalculation, unit tests [PRIORITY: HIGH]
**Corresponds to:** Scenarios "Automatic seed on startup", "No re-seeding when a portfolio exists"; Edge Case "Seed failure"
**Description:** `internal/domain/seed/seed.go` — orchestration layer.

- [ ] `Store` interface: `Seed(ctx context.Context, content *Content) (*Result, error)`
      — one call, one DB transaction for all seed data (documented).
- [ ] `PositionRecalculator` interface: `RecalculatePortfolio(ctx context.Context,
      portfolioID int64) error` — structurally satisfied by `*position.Service`,
      keeps the seed package decoupled (same DI style as the position/transaction
      services' narrow collaborator interfaces).
- [ ] `Result` struct for the startup log: `PortfolioID`, `Accounts`,
      `Transactions`, `MappingsReused`, `DetailsReused`, `ModelReused`.
- [ ] `ErrPortfolioExists` sentinel error.
- [ ] `Service` (`NewService(store Store, recalc PositionRecalculator, logger *slog.Logger)`):
      - `SeedDemo(ctx) error`:
        1. assign lot IDs — every buy/sell transaction gets `transaction.GenerateLotID()`
           (unique per transaction); deposits keep `LotID` nil;
        2. `store.Seed(ctx, content)` — on `ErrPortfolioExists`: log Info
           ("skipping sample data seed: portfolios already exist"), return nil;
        3. on store success: `recalc.RecalculatePortfolio(ctx, result.PortfolioID)`
           — the existing path (manual recalculate endpoint + automatic
           post-transaction hook both use it). If it fails: log Error, return nil
           (data is intact; the next transaction mutation or the manual endpoint
           heals positions — same failure posture as the normal flow, which
           ignores recalc errors).
        4. on store success + recalc success: log Info summary with `Result` counts.
        5. any other store error: return as-is (caller logs and continues).
- [ ] Export `GenerateLotID() string` in `internal/domain/transaction` (one-line
      wrapper around the existing `LOT-<ulid>` logic in the transaction service, so
      the format lives in one place).
- [ ] `seed_test.go` with hand-written mock store + mock recalc (co-located,
      internal state, per AGENTS.md mock pattern):
      - skip path: store returns `ErrPortfolioExists` → `SeedDemo` returns nil,
        recalc never called;
      - lot IDs: all 8 buy/sell rows get non-nil unique `LOT-`-prefixed IDs;
        deposit rows nil; content not mutated across calls;
      - happy path: store success → recalc called with the store's `PortfolioID`;
      - recalc failure: store success + recalc error → `SeedDemo` returns nil
        (log only);
      - store error: generic store error → returned to caller.

**Verification:** `go test ./internal/domain/seed/... ./internal/domain/transaction/...` pass.

### Task 3: Seed store (data layer) [PRIORITY: HIGH]
**Corresponds to:** Scenarios "Automatic seed on startup", "Benchmark symbols are selectable", "Sample portfolio can be deleted"; Edge Cases (all); Constraints (atomic, reuse, never modify)
**Description:** `internal/data/seed_store.go` — `SeedStore`
(`NewSeedStore(db *sql.DB)`) implementing `seed.Store`. Entire data write in
**one** `*sql.Tx`; any error → rollback, nothing partial remains.

- [ ] Guard first: `SELECT 1 FROM portfolios LIMIT 1` → any row → return
      `seed.ErrPortfolioExists` before any write.
- [ ] Insert portfolio "Sample" (GBP).
- [ ] Symbols, in content order:
      - mapping: `SELECT id FROM symbol_mappings WHERE internal_symbol = ?`;
        exists → reuse (count `MappingsReused`); else insert
        (internal symbol, market data symbol, is_benchmark).
      - details: if no `symbol_details` row for the internal symbol → insert the
        embedded values (`fetched_at` = now); if it exists → leave untouched
        (count `DetailsReused`). **Never update existing rows.**
- [ ] Model portfolio: `SELECT id FROM model_portfolios WHERE name = 'Sample Model'`;
      exists → reuse (`ModelReused: true`), **do not** write its targets;
      else insert model + its `ModelTargets` rows (70/30).
- [ ] Accounts: insert `sample-gbp` (GBP), `sample-usd` (USD) with the portfolio ID.
- [ ] Transactions: for each account, insert its 5 rows with the DB-assigned
      `account_id`, `created_at`/`updated_at` = now, and the lot IDs already assigned
      by the service (buy/sell → `lot_id`; deposits → NULL).
- [ ] Portfolio targets: insert the two `PortfolioTargets` rows for the new
      portfolio (70/30).
- [ ] Commit; return `Result` with all counts.
- [ ] **Implementation note:** repositories are `*sql.DB`-based with no
      transaction parameter, so the seed writes via parameterized raw SQL on its
      own `*sql.Tx` (the established pattern — `PositionRepository.Recalculate`
      does exactly this with `tx.ExecContext`). SQL statements mirror the existing
      generated queries (same columns, decimal via `.String()`).
- [ ] Positions are **not** written here — they are derived after commit by
      `RecalculatePortfolio` (see Task 2 / D7), the existing recalculation path.

**Verification:** compiles; `go vet ./...` clean; behavior proven by Task 4 integration tests.

### Task 4: Integration tests [PRIORITY: HIGH]
**Corresponds to:** every Scenario + Edge Case in the spec
**Description:** `tests/integration/seed_test.go` — package `integration`, fresh DB
via the existing `setupTestDB` helper (verified to already create all 12 tables the
seed touches: portfolios, accounts, symbol_mappings, broker_symbol_mappings,
transactions, positions, lots, lot_consumptions, market_data, symbol_details,
target_allocations, model_portfolios — no extraction or extension needed; the
`internal/domain/seed` package itself never references test code). Real repos for
`SeedStore` and a real `position.NewService` (with its repositories; optional
market-service/cache collaborators left nil — both nil-checked) as the
`PositionRecalculator`.

- [ ] **Fresh seed:** `SeedDemo` on empty DB → assert: portfolio "Sample" (GBP);
      2 accounts; 10 transactions (spot-check signed net_cash of first/last rows);
      **positions exist via recalc**: 4 open symbol positions (exact quantities
      648 VWRP.L / 1630 VUTA.L / 100 GOOG / 180 BRK-B) + 2 cash positions
      (39.80 GBP / 370.00 USD); 0 closed positions; 8 open lots;
      4 symbol mappings with benchmark flags on VWRP.L/VUTA.L;
      4 `symbol_details` rows with `fetched_at` set; "Sample Model" with 70/30
      entries + 2 target allocations; 2 target allocations on the "Sample" portfolio.
- [ ] **No re-seed:** call `SeedDemo` again → no error; portfolio count = 1,
      transaction count = 10, mappings = 4 (no duplicates), positions unchanged.
- [ ] **Existing user data:** create a user portfolio via the API (`newTestRouter`)
      first → `SeedDemo` → no "Sample" portfolio, no sample rows, user data
      untouched, no error.
- [ ] **Delete + re-seed reuse:** fresh seed, then delete the "Sample" portfolio
      through the normal API delete flow (`DELETE /api/portfolios/{id}` — spec
      Scenario "Sample portfolio can be deleted") → assert accounts/transactions/
      target allocations/positions gone, mappings + benchmark flags + "Sample Model"
      survive → `SeedDemo` again → new "Sample" portfolio with full data incl.
      recalculated positions; still exactly 4 mappings and 1 model portfolio
      (reused, not duplicated); model targets not duplicated.
- [ ] **Atomicity:** call `SeedStore.Seed` directly with a corrupted content fixture
      (duplicate `PortfolioTargets` symbol → `UNIQUE(portfolio_id, symbol)` violation)
      → error returned, zero rows in portfolios/accounts/transactions/
      symbol_mappings/symbol_details/model_portfolios/target_allocations/positions
      (full rollback, no partial seed, recalc never triggered).
- [ ] **Symbol overlap:** pre-create a user symbol mapping for GOOG (different
      internal symbol, e.g. "google-2") → seed → user mapping untouched, sample
      creates its own mapping, no modification of existing rows.

**Verification:** `go test ./tests/integration/... -run TestSeed -v` passes; full `go test ./...` green.

### Task 5: Wire the seed into the app bootstrap [PRIORITY: HIGH]
**Corresponds to:** Scenario "Automatic seed on startup" (ordering); Constraint "seed completes before the market-data cache's first refresh pass"
**Description:** `internal/api/router.go` — the existing wiring location (all
services are constructed there; `marketCache.Start` is called in `main.go` only
**after** `Router` returns).

- [ ] After the position service + market cache are wired (post
      `positionSvc.WithMarketCache(marketCache)`), before `Router` returns:
      `seedSvc := seed.NewService(data.NewSeedStore(db), positionSvc, logger)`;
      `if err := seedSvc.SeedDemo(context.Background()); err != nil {
      logger.Error("sample data seed failed; will retry on next startup", "error",
      err) }` — server continues regardless (spec Edge Case "Seed failure").
- [ ] Ordering is **structural**: the seed (and its `RecalculatePortfolio`) runs
      before `main.go` starts the cache workers. Scheduling market fetches before
      `Start()` is safe by construction — `ScheduleSymbolFetch`/`ScheduleFxPairFetch`
      only enqueue into a buffered channel; the worker consumes them once started
      (drops with a warning if the buffer is full).
- [ ] The seed's Info log (skip vs. summary) makes startup behavior observable.

**Verification:** `go build -o portfoliolab cmd/server/main.go` + `go test ./...`;
ordering verified by code placement; full startup exercised by the user (server
lifecycle is user-managed per AGENTS.md).

### Task 6: Docs + feature bookkeeping [PRIORITY: MEDIUM]
**Corresponds to:** workflow process requirements
**Description:**

- [ ] **SPEC.md revision (per confirmed D1):** in-place edit with a changelog
      note (user-confirmed): the transaction table gains 2 initial-deposit rows;
      "exactly 8 transactions" → "8 trade transactions plus 2 initial deposits".
- [ ] `NOTES.md`: record D1 deviation (why deposits are required by the schema),
      the D7 decision (existing recalc path reused; positions derived post-commit,
      self-healing via automatic post-transaction hook / manual endpoint), exact
      embedded symbol-details values, internal-symbol naming choice, wiring
      location (api.Router) and why no main.go change was needed, no API.md change
      (no new API surface).
- [ ] `features/README.md`: f030 status `spec` → `in-progress` (then `done` after review).

**Verification:** docs consistent with implemented behavior.

## Technical Decisions

| # | Decision | Choice | Reason / Alternatives |
|---|---|---|---|
| D1 | **Initial cash = 2 deposit transactions** ✅ confirmed | `$CASH-GBP` / `$CASH-USD` deposits (100000 @ 1, 2024-07-15, net_cash +100000) per account; 10 transaction rows total | Cash positions are the sum of transaction `net_cash` (no account-balance column exists); the spec's own remainder math (£39.80 / $370.00) is only reachable with deposit rows. Uses the app's existing deposit convention (trading212 import). Requires a one-line spec revision. |
| D2 | **Package layout** | `internal/domain/seed` (content + service + `Store`/`PositionRecalculator` interfaces) + `internal/data/seed_store.go` (implementation) | Follows the existing domain/data split; service logic unit-testable with hand-written mocks (AGENTS.md pattern); data layer covered by integration tests like other repos. The seed package never references test code or `tests/integration` helpers — `setupTestDB` is consumed only by `tests/integration/seed_test.go`, which is in the same `integration` package (no extraction needed). Alt: single `internal/seed` package — breaks layering, no test seam for the guard. |
| D3 | **Internal symbols** | internal symbol = ticker string: `VWRP.L`, `VUTA.L`, `GOOG`, `BRK-B` | Matches the f003 examples (`AAPL` → `AAPL`); readable on positions/allocation pages; renameable later via the symbol-map UI. Alt: lowercase slugs — no functional benefit. |
| D4 | **Reuse semantics** | mappings by `internal_symbol`; model by name; details insert-if-missing (never overwrite); model targets only when the model is newly created | Spec: "reuses it rather than duplicating", "never modifies existing data". Reusing a model = not touching its targets either. |
| D5 | **net_cash** | pinned explicitly per transaction in the content (buys −qty×price, sells +qty×price, deposits +amount) | App convention: net_cash is per-transaction and required non-zero (transaction service validates it; trading212 import computes exactly these signs). Declarative + test-pinnable. |
| D6 | **Guard ownership** | inside `SeedStore.Seed` (data layer); `seed.Service` maps `ErrPortfolioExists` to Info log + nil | Invariant lives with the seed itself; any future caller is safe. |
| D7 | **Positions: existing recalculation path** ✅ confirmed | After the data transaction commits, call `position.Service.RecalculatePortfolio(ctx, portfolioID)` — the exact method behind `POST /api/positions/recalculate` and the automatic post-transaction hook (transaction service Create/Update/Delete already call `RecalculateAccount` automatically) | One source of truth for position/lot/cash math — the seed cannot drift from normal use; **zero refactoring** of the P&L core (no `writeCalculatedPositions` extraction). Recalc reads committed data in its own tx, exactly like the normal flow; its failure is non-fatal and self-heals on the next transaction mutation or manual recalc (same failure posture as production, which ignores recalc errors). Alt (rejected): compute positions inside the seed's tx — would require extracting `Recalculate`'s body and duplicating the call path. |
| D8 | **Wiring location** | end of `api.Router` in `internal/api/router.go` (after position service + market cache wiring) | All service construction already happens there; `main.go` only gets `(router, marketCache)`. Placing the seed there structurally guarantees "seed completes before `marketCache.Start`" (Start runs in `main.go` after `Router` returns), with no main.go change and no API-signature change. Pre-Start fetch scheduling is safe (buffered channel). Alt: `main.go` — would require exposing the position service from `Router`. |

## Risks

- **Position math sensitivity** (AGENTS.md: "be extra careful with P&L math") — mitigated by reusing the existing recalculation path untouched (D7); Task 4 asserts exact expected position values.
- **Post-commit recalc failure** leaves seeded accounts without positions until the next transaction mutation / manual recalc — accepted (identical to the current production behavior after any transaction write); logged at Error.
- **`setupTestDB` hand-writes the schema** (not goose) — verified complete for this feature; if the integration test needs the API delete flow it reuses the existing `newTestRouter`.
- **Startup ordering** verified by code placement; the user's first manual startup is the end-to-end check.
