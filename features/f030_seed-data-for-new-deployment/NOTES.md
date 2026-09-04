# Notes — Seed Data for New Deployment

## Decisions (as implemented)

### D1 — Initial cash as deposit transactions
Cash positions are derived as the sum of transaction `net_cash` (there is no
account-balance column). Two deposit transactions (`$CASH-GBP` +100,000.00,
`$CASH-USD` +30,000.00 @ 1.00, 2024-07-01) follow the app's existing deposit
convention (trading212 import: symbol `$CASH-{ccy}`, price 1, positive
net_cash).

### D7 — Positions via the existing recalculation path
The seed writes only source data in one atomic `*sql.Tx`. After commit it calls
`position.Service.RecalculatePortfolio(ctx, portfolioID)` — the same method
behind `POST /api/positions/recalculate` and the automatic post-transaction
hook. Consequences:

- One source of truth for position/lot/cash math; the seed cannot drift.
- Recalc reads committed data in its own transaction, exactly like the normal
  flow. If it fails, the seed logs Error and returns nil — the next
  transaction mutation or the manual recalculate endpoint heals positions
  (identical failure posture to normal use).
- Recalc is pure math — no market fetching. (Verified: `RecalculatePortfolio`
  → `RecalculateAccount` only.)

### D8 — Wiring location: `api.Router`
The seed runs at the end of `Router()` (after all service wiring, before
`return r, marketCache`). `main.go` is untouched. Ordering is structural:
`main.go` calls `marketCache.Start()` only after `Router` returns, so the seed
and its recalculation always complete before the first refresh pass. Fetch
scheduling before `Start()` is safe by construction (buffered channel; observed
in `TestRouterSeedsSampleDataByDefault` — four "symbol fetch queued" logs at
router construction time, no worker yet started).

### D9 — No embedded symbol details
The seed writes symbol **mappings only** (internal symbol, market data symbol,
`is_benchmark`, `data_source_url` for the two Vanguard ETFs). Details and live
prices come from the startup background refresh: symbols with a
`data_source_url` go through the extractor dispatcher (Vanguard), others
through Yahoo. A missing `symbol_details` row is stale by definition
(`fetched_at IS NULL`), so refresh failures retry automatically — no extra
machinery needed.

## Plan deviations

1. **`WithoutSampleSeed()` router option (Task 5).** `api.Router` is also the
   construction path for every API integration test (`newTestRouter`, 45+ call
   sites in 19 files). Seeding unconditionally inside `Router` would inject
   sample data into all of them. The seed therefore defaults to **on** (so
   `main.go` stays unchanged, per D8) and `newTestRouter` opts out with
   `api.WithoutSampleSeed()`. This is a construction-time option, not API
   surface — the spec's "no new API surface" non-goal is unaffected.
2. **Type name is `Dataset`, not `Content`.** The plan's Task 1 text says
   `Content`; the implemented type is `seed.Dataset` (same shape: portfolio,
   accounts+transactions, symbol mappings, model, both target sets).
3. **Realized P&L on partial sells is 0 by design.** The plan's Task 4 sketch
   assumed the rebalance sell would surface realized P&L (712.80) on the open
   VWRP.L position. `position_computation.go` deliberately zeroes realized P&L
   on positions that are not fully closed (partial-sell proceeds already live
   in the cash balance; counting them as realized would double-count against
   the equity curve). The integration test pins the actual behavior: open
   VWRP.L position has `realized_pnl = 0`.
4. **modernc/sqlite: `LastInsertId` via `ExecContext`.** `QueryRow.Scan` on an
   INSERT does not return the rowid with the modernc driver (the existing
   repos use `ExecContext` + `LastInsertId` for the same reason); the seed
   store follows that pattern for portfolio/account IDs.

## Test coverage map (spec → test)

| Spec scenario / edge case | Test |
|---|---|
| Automatic seed on startup (fresh DB) | `TestRouterSeedsSampleDataByDefault` (wiring), `TestSeedServiceFullPathWithRecalculation`, `TestSeedStoreWritesDataset` |
| Seed completes before market cache first pass | `TestRouterSeedsSampleDataByDefault` (seed runs synchronously inside `Router`, which returns before `main.go`'s `marketCache.Start()`) |
| No re-seeding when a portfolio exists | second `SeedDemo` in `TestSeedServiceFullPathWithRecalculation`; second `Router` in `TestRouterSeedsSampleDataByDefault` |
| Seed failure (non-fatal) | service unit tests (`seed_test.go` in the seed package: recalc failure → nil; store error → returned, caller logs) |
| Benchmark symbols selectable | `TestSeedStoreWritesDataset` (benchmark flags on VWRP.L/VUTA.L) |
| Sample portfolio can be deleted | `TestSeedStoreReusesMappingsAndModel` (`DELETE /api/portfolios/1` via the real API, then re-seed) |
| Existing user data untouched | `TestSeedSkipsWhenUserPortfolioExists` |
| Atomicity (no partial seed) | `TestSeedStoreAtomicity` (duplicate target symbol → UNIQUE violation → zero rows in all 8 tables) |
| Symbol overlap (never modify existing) | `TestSeedStoreSymbolOverlap` (user mapping `google-2`→GOOG survives; sample creates its own GOOG mapping) |
| D9 (no embedded details) | `TestSeedServiceFreshSeedLeavesNoDetailsOrClosedPositions` + `symbol_details` count assertions in the fresh/no-reseed tests |
| Exact position math | `TestSeedServiceFullPathWithRecalculation` (648 VWRP.L @ 125.8128428…, 1,630 VUTA.L @ 19.6966871, 44 GOOG, 13 BRK-B; cash +567.20 GBP / +18,084.30 USD) |

## D10 — GBP deposit amount (resolved 2026-09-04)

**£100,000.00** initial GBP deposit (user-confirmed). The 2024 GBP buys
(£99,913.40) nearly exhaust it; with the 2025 rebalance (sell 18 VWRP.L
+£2,602.80, buy 108 VUTA.L −£2,122.20) the resulting `$CASH-GBP` is
**+567.20** — a realistic small residual balance rather than a negative cash
position. All pinning assertions (`content_test.go`,
`tests/integration/seed_test.go`) pin the +567.20 value.

## No API.md change

No new API surface: the seed is startup-only (Router wiring) and exposes no
HTTP endpoints. The "Sample" portfolio is managed through the existing
portfolio APIs (create/delete/etc.).
