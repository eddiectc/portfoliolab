# Retrospective: Market Data Override

**Date:** 2026-09-18
**Scope:** Phase 5 retrospective for f031 — spec, plan, NOTES.md, and the
implementation (commits `28a0dc3`…`290788f` + remediation `3e9466b`).

## What Went Well

- **The single merge point (D2) paid off.** Every historical price read funnels
  through three `MarketDataRepository` methods, so merging overrides there (Go,
  not SQL) meant zero consumer changes: equity curve, position P&L, FX
  conversion, TWR breakpoints, comparison, analysis, optimization, and the
  symbol-detail NAV page all inherited the feature. The verified chokepoints
  are documented in NOTES.md with file:line references, and provider SQL stayed
  raw.
- **The motivating incident closed on day 1.** The four corrupted 2025-10-24
  Yahoo bars (XNAQ.L, QGRP.L, ARCI.L, WOEE.L) were overridden through the new
  API immediately after implementation; the equity curve corrected live,
  verified via `GET /api/performance`. The overrides survive full refreshes
  by design, so the fix is durable.
- **The plan held.** Six strictly sequential tasks → six commits, no splits, no
  reordering. All checkboxes ticked; full suite green (3,764 tests) at the
  final task.
- **The API contract was locked before implementation.** The override
  endpoints were finalized in API.md during Phase 3; implementation required
  zero API.md changes (confirmed field-for-field in NOTES.md).
- **Deviation handling worked.** Two spec edge cases (SPEC lines 165–166)
  drifted from the implementation behavior. The Phase 4b review surfaced both,
  the user made an explicit accept decision, and NOTES.md documents them as
  *accepted deviations* with rationale and code references — no silent drift,
  and no code churn to match idealized spec wording.
- **Testing is layered and green.** 46 focused tests: domain validation
  (table-driven, hand-written mock `Store`), repository merge semantics
  (in-memory SQLite), handler HTTP mapping (httptest), plus 16 full-stack
  integration tests covering every spec scenario end to end.

## What Could Be Improved

- **Spec edge cases were written before the storage/read design (D1/D2).** The
  "stored but inert" and "mismatched type has no effect" wording assumed
  provider-only resolution; the merge design naturally surfaces any stored
  override, and the stock/fx price path matches cross-type overrides. The
  mismatch only surfaced at the Phase 4b review. Edge cases whose behavior
  depends on the chosen architecture should be reconciled against the
  confirmed plan decisions (Phase 3), not discovered at review.
- **A stale risk item in PLAN.md.** The risks section assumed `setupTestDB`
  hand-writes the schema (and needed the new DDL, "same caveat as f030"); in
  reality it runs the real goose migrations, so migration 025 applied
  automatically. It turned out favorably (no schema drift to maintain), but the
  risk was written without checking the harness — a one-line read of
  `portfolio_test.go:36` would have caught it.
- **Override lookup is list-and-filter, not key lookup.** `rangeOverrides` /
  `latestFxOverride` list all overrides for the symbol and filter by
  date/type in Go. Negligible at single-user scale (the spec constraint is
  "must not *meaningfully* degrade"), but it is not the "single key lookup per
  symbol/date/data type" the constraint describes; the `UNIQUE(symbol, date,
  data_type)` index is not fully exploited on reads.
- **Integration test coupled to the merge design, without a comment.** The
  remove test must use a *different* symbol for its FX override (an FX
  override on the stock symbol would add a union date via the cross-type
  merge). Harmless, but the "why" is not commented and cost a debug cycle.

## Spec vs Reality

| Spec item | Status | Notes |
|---|---|---|
| All 12 scenarios | ✅ Complete | Each covered by an integration test; mapping table in NOTES.md |
| Precedence: override wins within the read | ✅ Complete | `mergePriceOverrides`; provider row untouched; `GetBySourceAndDate` still returns the raw value (coexist scenario) |
| Survives manual full refresh | ✅ Complete | Separate table; upserts only touch provider rows |
| Survives periodic (gap-fill) refreshes | ✅ Complete | Gap-fill only re-fetches the tail; override rows never re-fetched |
| Live current quote unaffected | ✅ Complete | `date=''` read paths unchanged; tested |
| Staleness/gap-fill detection stays provider-only | ✅ Complete | `GetLatestPriceDatePerSymbol` unchanged (rejected "override extends latest date" alternative, per SPEC notes) |
| Override on a date with no provider row | ✅ Complete | Union of dates; forward-fill from it verified |
| FX override forward-fill semantics | ✅ Complete | Override first (`date <= D`, latest wins), provider fallback; unchanged fill semantics otherwise |
| Validation: price / date / data type (3 scenarios) | ✅ Complete | Domain service, distinct sentinel errors → 400 with codes |
| Remove: 204 / 404 / three-key match | ✅ Complete | Mismatched data type → 404 even when another type exists |
| List: stable order + `?symbol=` filter | ✅ Complete | `ORDER BY symbol, date, data_type` in SQL |
| Edge: override for a symbol with no fetched data | ⚠️ Accepted deviation | Surfaced instead of "inert" (NOTES.md, user-accepted 2026-09-17) |
| Edge: data type mismatched with symbol kind | ⚠️ Accepted deviation | Cross-type match on the stock/fx path (NOTES.md, user-accepted 2026-09-17); NAV and FX paths filter their own type |
| Edge: currency stored as given; reads see override's currency | ✅ Complete | Tested at repo and integration level |
| Read-performance constraint ("single key lookup") | ⚠️ Partial | Per-symbol list + Go filter; fine at single-user scale, see Action Items |

## Plan vs Reality

| Plan item | Status | Notes |
|---|---|---|
| 6 tasks, all completed | ✅ Complete | 6 commits, one per task, strictly sequential as planned |
| Task ordering / dependencies | ✅ Accurate | No reordering; the linear chain (migration → domain → repo/merge → handler → wiring → integration) held |
| Task sizing | ✅ Appropriate | No task needed splitting; each done in one focused session |
| Task 6: add override DDL to `setupTestDB` | ⚠️ Deviation | Plan assumed a hand-written schema helper; `setupTestDB` actually runs real goose migrations, so no DDL edit was needed. Documented in NOTES.md (deviation 1). |
| Wire JSON shape | ⚠️ Minor gap | Plan did not specify response fields; resolved to the domain `Override` shape with `created_at`/`updated_at` omitted (`json:"-"`). Documented in NOTES.md (deviation 2); consistent with the locked API.md contract. |
| Risks | ⚠️ Partially accurate | One stale risk (setupTestDB schema — see above); the read-fan-out, FX forward-fill, and read-performance risks were real and mitigated as planned. |

## Learnings

- **Chokepoint merges are the right pattern for cross-cutting read changes.**
  Verifying and documenting the funnel (which consumers call which repo
  methods) up front — the plan's consumer table did this — made Task 3
  mechanical and the "no consumer changes" claim auditable.
- **Design decisions can change what a spec edge case *means*.** When a
  storage/read design (D1/D2) makes an edge case behave differently than its
  wording assumes, reconcile the spec wording in Phase 3 (or mark the edge
  case design-dependent). Otherwise the review gate spends its budget
  arbitrating wording-vs-behavior drift instead of finding real defects.
- **The Phase 4b gate earned its keep.** Both edge-case drifts were caught
  before "done", and *accept-as-is + document in NOTES.md* (with an explicit
  user decision) is a clean resolution when the implemented behavior is the
  better one.
- **Keep first-real-data out of tests.** The plan correctly noted the four
  incident overrides would be recorded by the user via the API, not by test
  fixtures — real data belongs in the system, tests pin behavior with
  synthetic data.
- **Check harness assumptions against code before writing plan risks.**
  `setupTestDB`'s behavior (real goose migrations, not hand-written DDL) was
  already established in f030's history; one read of the harness would have
  corrected the risk item.

## Action Items

- [ ] (process) In Phase 3, reconcile spec edge cases whose behavior depends
      on the chosen storage/read design against the confirmed plan decisions.
- [ ] (process) Verify PLAN.md risk items against the actual test harness/code
      before the plan is confirmed.
- [ ] (low, f031) Replace list-and-filter override lookups with direct sqlc
      key/range queries (`date` bounds + `data_type`) if the override count
      ever grows; current behavior is fine at single-user scale.
- [ ] (low, f031) Comment the integration remove test explaining why the FX
      override uses a different symbol (cross-type merge union point).
- [ ] (future feature) Web UI to view/edit overrides — already flagged as a
      follow-up candidate in SPEC.md → Out of Scope.
