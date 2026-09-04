# Implementation Plan: Seed Data for New Deployment

> **CONFIRMED — all decisions agreed with user.**
>
> Confirmed: **D1** (2 initial deposit transactions), **D7** (existing
> `RecalculatePortfolio` path), **D8** (dataset values, 2026-07-21 — matches the
> user's current demo DB), **D9** (no embedded symbol details, 2026-09-04 —
> mappings + source URLs only; details/prices come from the startup background
> refresh).

## Overview

Automatic startup seed that creates a fully functional sample deployment ("Sample"
GBP portfolio, 2 accounts — "Main Investment" GBP / "US Brokerage" USD, 2 initial
deposits + 8 trades across 4 real symbols, symbol mappings with `data_source_url`
for the two Vanguard ETFs, 70/30 target allocations, "Sample Model" model
portfolio with benchmark flags) — and only when no portfolio exists. No symbol
details are embedded (D9): the startup background refresh fills them in (Vanguard
extractor for URL'd symbols, Yahoo otherwise) and fetches live prices. The data
write is one atomic transaction; positions are then derived through the
**existing** recalculation path (`position.Service.RecalculatePortfolio` — the
same method used by the manual `POST /api/positions/recalculate` endpoint and run
automatically after every transaction mutation). No new API surface, no web UI
(non-goals in spec).

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

- [x] `internal/domain/seed/content.go`: types `Content` (fields:
      `Portfolio portfolio.Portfolio`; `Accounts []AccountSeed` (each:
      `account.Account` + `[]transaction.Transaction`);
      `Symbols []symbolmapping.SymbolMapping` — **mappings only, no embedded
      details (D9)**; `Model modelportfolio.ModelPortfolio`; `ModelTargets` and
      `PortfolioTargets` as `[]allocation.TargetEntry`); function
      `Content() *Content` returning the exact dataset.
- [x] Dataset values per SPEC.md §Sample Data (D8, user-confirmed 2026-07-21):
      - Portfolio "Sample", base currency **GBP** (currency lives on the
        portfolio, not the account, in the current schema).
      - Accounts: `Main Investment` (portfolio 1), `US Brokerage` (portfolio 1).
      - Symbol mappings (internal symbol = ticker, D3):
        | Internal | Market data | Benchmark | data_source_url |
        |---|---|---|---|
        | VWRP.L | VWRP.L | yes | https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-accumulating |
        | VUTA.L | VUTA.L | yes | https://www.vanguardinvestor.co.uk/investments/vanguard-usd-treasury-bond-ucits-etf-usd-accumulating |
        | GOOG | GOOG | no | — (nil) |
        | BRK-B | BRK-B | no | — (nil) |
      - Transactions (10 total — 2 deposits + 8 trades; deposits first per
        account, D1; buys/sells get lot IDs at seed time, D2/Task 2;
        sell quantity negative per the domain sign convention):
        | # | Date | Account | Type | Symbol | Quantity | Price | Currency | Net cash |
        |---|---|---|---|---|---|---|---|---|
        | D1 | 2024-07-01 | Main Investment | deposit | $CASH-GBP | 100,000.00 | 1.00 | GBP | +100,000.00 |
        | B1 | 2024-07-01 | Main Investment | buy | VWRP.L | 666 | 105.00 | GBP | −69,930.00 |
        | B2 | 2024-07-01 | Main Investment | buy | VUTA.L | 1,522 | 19.70 | GBP | −29,983.40 |
        | D2 | 2024-07-01 | US Brokerage | deposit | $CASH-USD | 30,000.00 | 1.00 | USD | +30,000.00 |
        | B3 | 2024-07-01 | US Brokerage | buy | GOOG | 40 | 148.50 | USD | −5,940.00 |
        | B4 | 2024-07-01 | US Brokerage | buy | BRK-B | 12 | 412.30 | USD | −4,947.60 |
        | S1 | 2025-07-15 | Main Investment | sell | VWRP.L | −18 | 144.60 | GBP | +2,602.80 |
        | B5 | 2025-07-15 | Main Investment | buy | VUTA.L | 108 | 19.65 | GBP | −2,122.20 |
        | B6 | 2026-03-10 | US Brokerage | buy | GOOG | 4 | 152.30 | USD | −609.20 |
        | B7 | 2026-03-10 | US Brokerage | buy | BRK-B | 1 | 418.90 | USD | −418.90 |
      - Resulting open positions: 648 VWRP.L, 1,630 VUTA.L (GBP); 44 GOOG,
        13 BRK-B (USD); cash `$CASH-GBP` = **+567.20**, `$CASH-USD` =
        **+18,084.30**.
      - `Model` = "Sample Model", entries 70% VWRP.L / 30% VUTA.L;
        `ModelTargets` 70/30; `PortfolioTargets` 70/30 (same symbols).
- [x] `content_test.go`: pin the entire dataset — assert every field of the 10
      transactions (dates, symbols, types, prices, signed quantities, **signed
      net_cash** as exact decimal strings), account names, symbol benchmark
      flags and source URLs, model entries and both target sets (each sums to
      100). Cash check via `position.ComputeCashPositions` over all seeded
      transactions: `$CASH-GBP` = +567.20, `$CASH-USD` = +18,084.30.

**Verification:** `go test ./internal/domain/seed/...` passes; dataset matches the plan table cell-for-cell.

### Task 2: Seed service — guard handling, lot IDs, recalculation, unit tests [PRIORITY: HIGH]
**Corresponds to:** Scenarios "Automatic seed on startup", "No re-seeding when a portfolio exists"; Edge Case "Seed failure"
**Description:** `internal/domain/seed/seed.go` — orchestration layer.

- [x] `Store` interface: `Seed(ctx context.Context, content *Content) (*Result, error)`
      — one call, one DB transaction for all seed data (documented).
- [x] `PositionRecalculator` interface: `RecalculatePortfolio(ctx context.Context,
      portfolioID int64) error` — structurally satisfied by `*position.Service`
      (confirmed: `internal/domain/position/service.go:453`), keeps the seed
      package decoupled (same DI style as the position/transaction services'
      narrow collaborator interfaces).
- [x] `Result` struct for the startup log: `PortfolioID`, `Accounts`,
      `Transactions`, `Mappings`, `MappingsReused`, `ModelReused`. (D9 removed
      the details-related counts.)
- [x] `ErrPortfolioExists` sentinel error.
- [x] `Service` (`NewService(store Store, recalc PositionRecalculator, logger *slog.Logger)`):
      - `SeedDemo(ctx) error`:
        1. assign lot IDs — every buy/sell transaction gets
           `transaction.GenerateLotID()` (unique per transaction); deposits keep
           `LotID` nil;
        2. `store.Seed(ctx, Content())` — on `ErrPortfolioExists`: log Info
           ("skipping sample data seed: portfolios already exist"), return nil;
        3. on store success: `recalc.RecalculatePortfolio(ctx, result.PortfolioID)`
           — the existing path (manual recalculate endpoint + automatic
           post-transaction hook both use it). If it fails: log Error, return nil
           (data is intact; the next transaction mutation or the manual endpoint
           heals positions — same failure posture as the normal flow, which
           ignores recalc errors);
        4. on store success + recalc success: log Info summary with `Result` counts;
        5. any other store error: return as-is (caller logs and continues).
- [x] Export `GenerateLotID() string` in `internal/domain/transaction` (one-line
      wrapper around the existing unexported `generateLotID()` LOT-\<ulid\>
      logic in the transaction service, so the format lives in one place).
- [x] `seed_test.go` with hand-written mock store + mock recalc (co-located,
      internal state, per AGENTS.md mock pattern):
      - skip path: store returns `ErrPortfolioExists` → `SeedDemo` returns nil,
        recalc never called;
      - lot IDs: all 8 buy/sell rows get non-nil unique `LOT-`-prefixed IDs;
        deposit rows nil;
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

- [x] Guard first (inside the tx, before any write): `SELECT 1 FROM portfolios
      LIMIT 1` → any row → rollback + return `seed.ErrPortfolioExists`.
- [x] Insert portfolio "Sample" (GBP) → capture ID.
- [x] Symbol **mappings only** (D9 — no `symbol_details` writes): in content
      order, `SELECT id FROM symbol_mappings WHERE internal_symbol = ?`; exists
      → reuse (count `MappingsReused`); else insert (internal symbol, market
      data symbol, is_benchmark, data_source_url). **Never update existing rows.**
- [x] Model portfolio: `SELECT id FROM model_portfolios WHERE name = ?`;
      exists → reuse (`ModelReused: true`), **do not** touch its entries;
      else insert model (entries as the JSON blob the repo uses) + do not write
      model targets (they live in the entries JSON).
- [x] Accounts: insert `Main Investment`, `US Brokerage` with the portfolio ID;
      capture IDs.
- [x] Transactions: for each account, insert its rows with the DB-assigned
      `account_id`, `created_at`/`updated_at` = now, and the lot IDs already
      assigned by the service (buy/sell → `lot_id`; deposits → NULL).
- [x] Portfolio targets: insert the two `PortfolioTargets` rows for the new
      portfolio (70/30 `target_allocations` rows, UNIQUE(portfolio_id, symbol)).
- [x] Commit; return `Result` with all counts.
- [x] **Implementation note:** repositories are `*sql.DB`-based with no
      transaction parameter, so the seed writes via parameterized raw SQL on its
      own `*sql.Tx` (the established pattern — `PositionRepository.Recalculate`
      does exactly this with `tx.ExecContext`). SQL statements mirror the
      existing generated queries (same columns, decimal via `.String()`).
- [x] Positions are **not** written here — they are derived after commit by
      `RecalculatePortfolio` (Task 2 / D7), the existing recalculation path.

**Verification:** compiles; `go vet ./...` clean; behavior proven by Task 4 integration tests.

### Task 4: Integration tests [PRIORITY: HIGH]
**Corresponds to:** every Scenario + Edge Case in the spec
**Description:** `tests/integration/seed_test.go` — package `integration`, fresh DB
via the existing `setupTestDB` helper; real `SeedStore`; real
`position.NewService(positionRepo, transactionRepo, accountChecker,
portfolioChecker, accountLister, portfolioCurrencyChecker)` built from the real
data-layer repositories (the same construction as `api.Router`) as the
`PositionRecalculator`.

- [x] **Fresh seed:** `SeedDemo` on empty DB → assert: portfolio "Sample" (GBP);
      2 accounts (Main Investment, US Brokerage); 10 transactions (spot-check
      signed net_cash of first/last rows); **positions exist via recalc**:
      4 open symbol positions (exact quantities 648 VWRP.L / 1,630 VUTA.L /
      44 GOOG / 13 BRK-B) + 2 cash positions (+567.20 GBP / +18,084.30 USD);
      0 closed positions; 8 open lots; 4 symbol mappings with benchmark flags on
      VWRP.L/VUTA.L and source URLs on both; **0 `symbol_details` rows** (D9 —
      no embedded details); "Sample Model" with 70/30 entries; 2 target
      allocations on the "Sample" portfolio (70/30).
- [x] **No re-seed:** call `SeedDemo` again → no error; portfolio count = 1,
      transaction count = 10, mappings = 4 (no duplicates), positions unchanged,
      still 0 symbol_details rows.
- [x] **Existing user data:** create a user portfolio via the API
      (`newTestRouter`) first → `SeedDemo` → no "Sample" portfolio, no sample
      rows, user data untouched, no error.
- [x] **Delete + re-seed reuse:** fresh seed, then delete the "Sample" portfolio
      through the normal API delete flow (`DELETE /api/portfolios/{id}`) →
      assert accounts/transactions/target allocations/positions gone, mappings
      + benchmark flags + "Sample Model" survive → `SeedDemo` again → new
      "Sample" portfolio with full data incl. recalculated positions; still
      exactly 4 mappings and 1 model portfolio (reused, not duplicated); model
      entries unchanged.
- [x] **Atomicity:** call `SeedStore.Seed` directly with a corrupted content
      fixture (duplicate `PortfolioTargets` symbol →
      `UNIQUE(portfolio_id, symbol)` violation) → error returned, zero rows in
      portfolios/accounts/transactions/symbol_mappings/model_portfolios/
      target_allocations/positions/symbol_details (full rollback, no partial
      seed, recalc never triggered).
- [x] **Symbol overlap:** pre-create a user symbol mapping with market data
      symbol GOOG under a different internal symbol (e.g. "google-2") → seed →
      user mapping untouched, sample creates its own "GOOG" mapping, no
      modification of existing rows.

**Verification:** `go test ./tests/integration/... -run TestSeed -v` passes; full `go test ./...` green.

### Task 5: Wire the seed into the app bootstrap [PRIORITY: HIGH]
**Corresponds to:** Scenario "Automatic seed on startup" (ordering); Constraint "seed completes before the market-data cache's first refresh pass"
**Description:** `internal/api/router.go` — the existing wiring location (all
services are constructed there; `marketCache.Start` is called in `main.go` only
**after** `Router` returns).

- [x] After `positionSvc.WithMarketCache(marketCache)` (router.go:193), before
      `Router` returns:
      `seedSvc := seed.NewService(data.NewSeedStore(db), positionSvc, logger)`;
      `if err := seedSvc.SeedDemo(context.Background()); err != nil {
      logger.Error("sample data seed failed; will retry on next startup", "error",
      err) }` — server continues regardless (spec Edge Case "Seed failure").
- [x] Ordering is **structural**: the seed (and its `RecalculatePortfolio`) runs
      before `main.go` starts the cache workers. Scheduling market fetches before
      `Start()` is safe by construction — `ScheduleSymbolFetch`/`ScheduleFxPairFetch`
      only enqueue into a buffered channel; the worker consumes them once started
      (drops with a warning if the buffer is full).
- [x] The seed's Info log (skip vs. summary) makes startup behavior observable.

**Verification:** `go build -o portfoliolab cmd/server/main.go` + `go test ./...`;
ordering verified by code placement; full startup exercised by the user (server
lifecycle is user-managed per AGENTS.md).

### Task 6: Docs + feature bookkeeping [PRIORITY: MEDIUM]
**Corresponds to:** workflow process requirements
**Description:**

- [x] **SPEC.md revision (D1 + D8 + D9):** full reconciliation completed
      2026-09-04 — deposits in the transaction table, D8 values, accounts
      renamed, D9 "Symbol mappings" section replaces "Symbol details",
      dependency list updated (f025 Vanguard Scraper added).
- [x] **PLAN.md revision (D1–D9):** this document, rewritten 2026-09-04.
- [x] `NOTES.md`: record D1 (why deposits are required by the schema), D7
      (existing recalc path reused; positions derived post-commit, self-healing
      via automatic post-transaction hook / manual endpoint), D8 (user-confirmed
      values), D9 (no embedded details — provider pages are live; missing
      `symbol_details` row = stale = auto-retry), the GBP deposit arithmetic
      open question, wiring location (api.Router) and why no main.go change was
      needed, no API.md change (no new API surface).
- [x] `features/README.md`: f030 status `spec` → `in-progress` (then `done`
      after review).

**Verification:** docs consistent with implemented behavior.

## Technical Decisions

| # | Decision | Choice | Reason / Alternatives |
|---|---|---|---|
| D1 | **Initial cash = 2 deposit transactions** ✅ confirmed | `$CASH-GBP` / `$CASH-USD` deposits (100,000.00 / 30,000.00 @ 1.00, 2024-07-01, net_cash positive) per account; 10 transaction rows total | Cash positions are the sum of transaction `net_cash` (no account-balance column exists); the app has no account-currency seed without cash entries. Uses the app's existing deposit convention (trading212 import: symbol `$CASH-{ccy}`, price 1, positive net_cash). |
| D2 | **Package layout** | `internal/domain/seed` (content + service + `Store`/`PositionRecalculator` interfaces) + `internal/data/seed_store.go` (implementation) | Follows the existing domain/data split; service logic unit-testable with hand-written mocks (AGENTS.md pattern); data layer covered by integration tests like other repos. The seed package never references test code or `tests/integration` helpers. Alt: single `internal/seed` package — breaks layering, no test seam for the guard. |
| D3 | **Internal symbols** | internal symbol = ticker string: `VWRP.L`, `VUTA.L`, `GOOG`, `BRK-B` | Matches the f003 examples (`AAPL` → `AAPL`); readable on positions/allocation pages; renameable later via the symbol-map UI. Alt: lowercase slugs — no functional benefit. |
| D4 | **Reuse semantics** | mappings by `internal_symbol`; model by name; never overwrite existing rows; model entries only when the model is newly created | Spec: "reuses it rather than duplicating", "never modifies existing data". Reusing a model = not touching its entries either. |
| D5 | **net_cash** | pinned explicitly per transaction in the content (buys −qty×price, sells +qty×price, deposits +amount) | App convention: net_cash is per-transaction and required (transaction service validates it; broker imports compute exactly these signs). Declarative + test-pinnable. |
| D6 | **Guard ownership** | inside `SeedStore.Seed` (data layer); `seed.Service` maps `ErrPortfolioExists` to Info log + nil | Invariant lives with the seed itself; any future caller is safe. |
| D7 | **Positions: existing recalculation path** ✅ confirmed | After the data transaction commits, call `position.Service.RecalculatePortfolio(ctx, portfolioID)` — the exact method behind `POST /api/positions/recalculate` and the automatic post-transaction hook | One source of truth for position/lot/cash math — the seed cannot drift from normal use; **zero refactoring** of the P&L core. Recalc reads committed data in its own tx, exactly like the normal flow; its failure is non-fatal and self-heals on the next transaction mutation or manual recalc. Alt (rejected): compute positions inside the seed's tx — would require extracting `Recalculate`'s body and duplicating the call path. |
| D8 | **Wiring location** | end of `api.Router` in `internal/api/router.go` (after position service + market cache wiring) | All service construction already happens there; `main.go` only gets `(router, marketCache)`. Placing the seed there structurally guarantees "seed completes before `marketCache.Start`" (Start runs in `main.go` after `Router` returns), with no main.go change and no API-signature change. Pre-Start fetch scheduling is safe (buffered channel). Alt: `main.go` — would require exposing the position service from `Router`. |
| D9 | **No embedded symbol details** ✅ confirmed 2026-09-04 | Seed writes symbol mappings only (internal symbol, market data symbol, `is_benchmark`, `data_source_url` for the two Vanguard ETFs). Details + live prices come from the existing startup background refresh: symbols with a `data_source_url` go through the extractor dispatcher (Vanguard for VWRP.L/VUTA.L), others through Yahoo. | The old embedded details (static sector/geography JSON) was maintenance overhead for data the app can fetch, and the user's real flow is "enter URL, refresh fills the rest". A missing `symbol_details` row is stale by definition (`fetched_at IS NULL`), so refresh failures retry automatically — no extra machinery. Alt (rejected): keep embedding — duplicates provider data, drifts, and breaks the single-source-of-truth for details. |
| D10 | **Dataset values** ✅ confirmed 2026-07-21 | Exact values per Task 1 table (deposits 2024-07-01; prices 105.00/19.70/148.50/412.30/144.60/19.65/152.30/418.90; quantities 666/1522/40/12/18/108/4/1) | Matches the user's current demo DB; user-confirmed. |

## Risks

- **Position math sensitivity** (AGENTS.md: "be extra careful with P&L math") — mitigated by reusing the existing recalculation path untouched (D7); Task 4 asserts exact expected position values.
- **Post-commit recalc failure** leaves seeded accounts without positions until the next transaction mutation / manual recalc — accepted (identical to the current production behavior after any transaction write); logged at Error.
- **`setupTestDB` hand-writes the schema** (not goose) — verified complete for this feature; the API delete-flow test reuses the existing `newTestRouter`.
- **Details/price dependency** (new with D9): until the startup refresh completes, seeded symbols have no details/prices — pages degrade exactly as with any not-yet-refreshed symbol (existing behavior); refresh is automatic.
- **Startup ordering** verified by code placement; the user's first manual startup is the end-to-end check.

## Open Questions

- None. (The GBP deposit amount was an open question during planning —
  resolved 2026-09-04: deposit is **£100,000.00**, leaving `$CASH-GBP` =
  **+567.20**; see D10 in NOTES.md.)
