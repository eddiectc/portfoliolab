# Implementation Plan: Market Data Override

> **CONFIRMED — all decisions agreed with user (2026-09-17).**
>
> Confirmed: **D1** (new `market_data_overrides` table), **D2** (merge at the
> repository read paths), **D4** (thin domain service in
> `internal/domain/marketdata`), **D5** (new handler file
> `market_data_override.go`), **D6** (separate `MarketDataOverrideRepository`).
> **D3** (FX forward-fill order) follows from D2 — no independent choice.

## Overview

User-set historical price corrections stored in a dedicated
`market_data_overrides` table (`UNIQUE(symbol, date, data_type)`), managed via
`PUT/GET/DELETE /api/market-data/overrides` (contract finalized in API.md,
Phase 3 — this plan does not alter it). Overrides win over provider rows
**inside historical reads**; provider rows are never deleted or modified, so
provider rows and overrides coexist for the same date. Every historical price
read funnels through three `MarketDataRepository` methods, which merge
overrides in Go (D2):

| Read method | Merges data types | Consumed by |
|---|---|---|
| `GetHistoricalPricesBySymbol` (market_data_repo.go:145) | `stock` + `fx` | position P&L (position/equity_curve.go:295), performance equity curve + FX conversion (performance/fx_conversion.go) |
| `GetHistoricalFxRateOnOrBefore` (market_data_repo.go:85) | `fx` | single-date historical FX forward fill |
| `GetNavHistoryBySymbol` (market_data_repo.go:259) | `nav` | symbol detail page NAV chart (symbol_details_web.go:296) |

Unaffected, per spec: live quotes (`date=''` — `GetLatestQuote`/
`GetCurrentFxRate`), staleness/gap-fill detection
(`GetLatestPriceDatePerSymbol`), source-keyed raw reads
(`GetBySourceAndDate`), refresh write paths (`Upsert`). No web UI (out of
scope). Prices as `decimal.Decimal` end to end (never `float64`).

## Task Dependencies

```
Task 1 (migration + sqlc) ─> Task 2 (domain service) ─> Task 3 (repository + merge) ─> Task 4 (handler) ─> Task 5 (wiring) ─> Task 6 (integration tests + bookkeeping)
```
Tasks are strictly sequential (each builds on the previous).

## Tasks

### Task 1: Migration + sqlc queries [PRIORITY: HIGH]
**Corresponds to:** Constraints (single key lookup per symbol/date/data type); storage substrate for all scenarios
**Description:** New table + generated queries.

- [ ] `internal/assets/migrations/025_create_market_data_overrides.sql`
      (goose Up/Down with `StatementBegin/End`), table:
      `id INTEGER PRIMARY KEY AUTOINCREMENT, symbol TEXT NOT NULL,
      date TEXT NOT NULL, data_type TEXT NOT NULL, price TEXT NOT NULL,
      currency TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT
      (datetime('now')), updated_at TEXT NOT NULL DEFAULT
      (datetime('now')), UNIQUE(symbol, date, data_type)` + index
      `idx_market_data_overrides_symbol`. Price stored as a decimal string —
      same convention as `market_data.price` (TEXT).
- [ ] `internal/data/queries/market_data_overrides.sql` (new sqlc file, same
      `-- name: X :one/:many/:execrows` style as `market_data.sql`):
      `UpsertMarketDataOverride :one` (INSERT … `ON CONFLICT(symbol, date,
      data_type) DO UPDATE SET price = excluded.price, currency =
      excluded.currency, updated_at = datetime('now')` RETURNING *),
      `GetMarketDataOverride :one`, `ListMarketDataOverrides :many` (ORDER BY
      symbol, date, data_type), `ListMarketDataOverridesBySymbol :many` (same
      order), `DeleteMarketDataOverride :execrows`.
- [ ] Run `sqlc generate`; generated code compiles.

**Verification:** `GOCACHE=/tmp/go-cache GOPATH=/tmp/go-path go build ./...` + `go vet ./internal/data/...`.

### Task 2: Domain package `internal/domain/marketdata` [PRIORITY: HIGH]
**Corresponds to:** Behavior → Setting an override; Scenarios "Validation rejects a non-positive price", "Validation rejects an invalid date", "Validation rejects an unknown data type"
**Description:** Types, validation, and the thin orchestration service (D4).

- [ ] `override.go`: `DataType` string type + constants `stock`/`fx`/`nav` +
      `Valid()`; `Override` struct (Symbol, Date string, DataType, Price
      `decimal.Decimal`, Currency string, Source — constant `"override"`,
      CreatedAt/UpdatedAt); `SetRequest` struct with JSON tags `symbol`,
      `date`, `price`, `currency`, `data_type` (direct `decimal.Decimal` JSON
      field — same pattern as `transaction.CreateRequest`); sentinel
      `ErrNotFound`.
- [ ] `service.go`: `Store` interface (narrow, DI pattern): `Upsert(ctx,
      *Override) error`; `Delete(ctx, symbol, date string, dt DataType)
      (bool, error)`; `List(ctx, symbol *string) ([]Override, error)`.
      `Service` (`NewService(store Store)`):
      - `Set(ctx, SetRequest) (*Override, error)` — validate, then upsert:
        symbol non-empty; date parses as `2006-01-02` and is ≤ today (UTC);
        price > 0; data_type ∈ {stock, fx, nav}. A clear error per rule
        (validation errors are distinct, mappable to 400).
      - `Remove(ctx, symbol, date string, dt DataType) error` — validate
        data_type; `store.Delete`; `deleted == false` → `ErrNotFound`.
      - `List(ctx, symbol *string) ([]Override, error)` — pass-through (nil
        symbol = list all).
- [ ] `service_test.go`: table-driven, hand-written mock Store (co-located,
      internal state, asserts which method/args were called): valid Set
      (exact field mapping, Source `"override"`); each validation failure
      (empty symbol; price 0 / negative; malformed dates `2025-13-40`,
      `01/01/2025`; future date; unknown data type); Remove → `ErrNotFound` on
      `deleted=false`; Remove validates data_type before touching the store;
      List with nil vs non-nil symbol.

**Verification:** `go test ./internal/domain/marketdata/...` passes; no DB, no network.

### Task 3: Override repository + merge in the historical read paths [PRIORITY: HIGH]
**Corresponds to:** Behavior → Precedence / Where overrides take effect / Surviving refreshes; Edge Cases (all read-side)
**Description:** D6 (separate repository) + D2/D3 (merge inside the three read methods).

- [ ] `internal/data/market_data_override.go`: `MarketDataOverrideRepository`
      (`NewMarketDataOverrideRepository(db *sql.DB)`) implementing
      `marketdata.Store` (compile-time assertion); decimal ↔ string
      conversion per repo convention; `Delete` maps `:execrows`
      RowsAffected → bool.
- [ ] `internal/data/market_data_repo.go` — `MarketDataRepository` composes a
      `*MarketDataOverrideRepository` (constructor signature unchanged):
      - `GetHistoricalPricesBySymbol` (:145): after the provider rows, load
        overrides for the symbol with `data_type IN ('stock','fx')` in the
        same [from, to] range; the override wins per (date, data_type);
        dates present only as overrides are appended (price + currency from
        the override); result sorted by date ASC.
      - `GetHistoricalFxRateOnOrBefore` (:85): check overrides first
        (`data_type='fx'`, `date <= D`, latest date wins); fall back to the
        existing provider query. The `"yahoo"` argument at
        marketservice.go:150 is untouched — it now only scopes the fallback.
      - `GetNavHistoryBySymbol` (:259): merge `nav` overrides the same way
        (override wins per date; override-only dates appended).
      - **Unchanged:** `GetLatestQuote`, `GetCurrentFxRate`,
        `GetBySourceAndDate` (raw — "Provider row and override coexist"),
        `GetLatestPriceDatePerSymbol` (staleness stays provider-only),
        `Upsert`, `DeleteStaleMarketData`.
- [ ] `internal/data/market_data_override_test.go` (in-memory SQLite,
      `file::memory:?cache=shared`, raw DDL for `market_data` +
      `market_data_overrides` — pattern from market_data_repo_test.go:15-45):
      - CRUD: upsert insert; upsert replace (price/currency change, still one
        row); get; list all in stable order (symbol, date, data_type); list
        by symbol excludes other symbols; delete → true / false.
      - Merge: provider row + override on the same date → override wins
        (price **and** currency); override-only date → included; no override →
        provider values unchanged; FX on-or-before: provider 2024-01-05,
        override 2024-01-10 → D=01-12 returns override, D=01-08 returns
        provider; nav merge analog.

**Verification:** `go test ./internal/data/...` passes.

### Task 4: REST handler [PRIORITY: HIGH]
**Corresponds to:** API (API.md → Market Data → Price Overrides); Scenarios "Listing overrides", "Removing an override…", "Removing a non-existent override…", Validation *
**Description:** D5 — new file; `NewMarketDataHandler` stays as-is.

- [ ] `internal/api/handlers/market_data_override.go`:
      `MarketDataOverrideHandler` (`NewMarketDataOverrideHandler(svc
      *marketdata.Service)`); `RegisterRoutes(r *chi.Mux)`: `PUT/GET/DELETE
      /api/market-data/overrides`. Pattern from transaction.go (decode body
      into the domain request struct, delegate, map service errors with
      `errors.Is`):
      - PUT: `200` the stored override (JSON: `symbol`, `date`, `data_type`,
        `price`, `currency`, `source: "override"` — `decimal.Decimal`
        marshals as a JSON number, precedent symbol.go:210) | `400`
        `INVALID_*` per validation error.
      - GET: optional `symbol` query param → `200` list (empty array if none).
      - DELETE: `symbol`/`date`/`data_type` query params all required →
        `204` | `400` missing parameter | `404` `OVERRIDE_NOT_FOUND` (from
        `ErrNotFound`).
- [ ] `market_data_override_test.go`: httptest + chi router + hand-written
      mock service: PUT happy path (decimal parses exactly, e.g. 44.02); PUT
      400 for each validation error; GET all / GET by symbol / GET empty;
      DELETE 204 / 400 (each missing parameter) / 404.

**Verification:** `go test ./internal/api/handlers/...` passes.

### Task 5: Router wiring [PRIORITY: HIGH]
**Corresponds to:** API (endpoint availability)
**Description:** `internal/api/router.go`, after the `marketDataHandler`
block (~:239).

- [ ] `overrideSvc := marketdata.NewService(data.NewMarketDataOverrideRepository(db))`;
      `marketDataOverrideHandler := handlers.NewMarketDataOverrideHandler(overrideSvc)`;
      `marketDataOverrideHandler.RegisterRoutes(r)`.
- [ ] No other wiring is needed — consumers already read through
      `MarketDataRepository`, which merges in Task 3:
      `symbolDetailsSvc.WithMarketDataRepo(marketDataRepo)` (:187),
      `NewSymbolDetailsWebHandler(..., marketDataRepo, ...)` (:278 — its
      navSource is the repository itself), position/performance via
      `marketSvc` (:210).

**Verification:** `go build -o portfoliolab cmd/server/main.go` + `go test ./...`.

### Task 6: Integration tests + bookkeeping [PRIORITY: HIGH]
**Corresponds to:** every Scenario + Edge Case (full stack)
**Description:** `tests/integration/market_data_override_test.go` — package
`integration`, `setupTestDB` (add `market_data_overrides` DDL to the
hand-written schema helper), real repository + real service, full stack via
`newTestRouter` (DB → service → handler → response).

- [ ] **Correcting a corrupted bar:** provider row XNAQ.L 2025-10-24 (wrong)
      → PUT override 44.02 → `GetHistoricalPricesBySymbol` returns 44.02; the
      provider row still exists and `GetBySourceAndDate` returns the raw
      value (coexist scenario).
- [ ] **Override on a date the provider has no row for:** override-only date
      appears in the price history (union of dates).
- [ ] **Survives a refresh:** `repo.Upsert` of a provider row for the same
      date (manual full-refresh semantics) → reads still resolve to the
      override; the override row is unmodified.
- [ ] **FX override:** provider GBP/USD rate + a later-dated override →
      `GetHistoricalFxRateOnOrBefore` returns the override for D ≥ override
      date, the provider rate before it.
- [ ] **NAV override:** nav provider row + nav override →
      `GetNavHistoryBySymbol` reflects the override (symbol detail page read
      path).
- [ ] **Live quote unaffected:** a `date=''` row exists → `GetLatestQuote`
      returns the provider value regardless of overrides.
- [ ] **Update:** PUT the same key with a new price → list shows the
      replacement (last write wins).
- [ ] **Remove:** DELETE → `204` → reads revert to the provider price;
      DELETE again → `404`; DELETE with a non-matching data type → `404` even
      when an override exists under another data type.
- [ ] **Validation over HTTP:** price ≤ 0, invalid date, future date,
      unknown data type → `400` each.
- [ ] **List:** several overrides (different symbols/dates/types) → stable
      order symbol, date, data_type; `?symbol=` filter.
- [ ] **Bookkeeping:** `NOTES.md` (D1–D6 rationale — incl. the
      `UNIQUE(symbol, source, date)` collision argument and the verified
      merge chokepoints); `features/README.md` f031 status `draft` →
      `in-progress` (→ `done` after review); confirm API.md needs no change
      (contract locked in Phase 3); PLAN.md checkboxes reflect completion.

**Verification:** `go test ./tests/integration/... -run TestMarketDataOverride -v` + full `go test ./...` green.

## Technical Decisions

| # | Decision | Choice | Reason / Alternatives |
|---|---|---|---|
| D1 | **Storage: new `market_data_overrides` table** ✅ confirmed | `UNIQUE(symbol, date, data_type)`; price TEXT (decimal string) as in `market_data` | `market_data.UNIQUE(symbol, source, date)` collides as soon as a fund has both a `stock` and a `nav` override on the same date (both would need `source='override'`). A separate table lets provider rows and overrides coexist (spec scenario), keeps source-keyed raw reads and staleness detection provider-only, and gives a clean upsert/delete key. Alt: `source='override'` rows in `market_data` — rejected (unique-constraint collision + raw reads polluted). |
| D2 | **Merge at the repository read paths** ✅ confirmed | Go-side merge inside `MarketDataRepository.GetHistoricalPricesBySymbol` / `GetHistoricalFxRateOnOrBefore` / `GetNavHistoryBySymbol` | All historical price reads funnel through these three methods (verified: position/equity_curve.go:295, performance/fx_conversion.go, symbol_details_web.go:296 — all via `*MarketDataRepository`). One merge point, no consumer changes, provider SQL stays raw. Alt: SQL LEFT JOIN in the provider queries — rejected (would leak overrides into `GetBySourceAndDate` and staleness queries). |
| D3 | **FX forward fill: override first, provider fallback** (consequence of D2) | `GetHistoricalFxRateOnOrBefore` checks overrides (`date <= D`, latest wins) before the provider query | Matches the spec: an FX override for date D is the rate used for valuations on D and afterwards. The existing `"yahoo"` argument (marketservice.go:150) is untouched — it now only scopes the provider fallback. |
| D4 | **Thin domain service in a new `internal/domain/marketdata` package** ✅ confirmed | `Override`/`DataType` types + `Service` with a narrow `Store` interface; all validation lives here | DI pattern from position/transaction/seed; unit-testable with hand-written mocks (no DB); `internal/data` importing domain packages is an established pattern (e.g. seed_store.go → domain/seed). Alt: validate in the handler — rejected (validation bypassed by future callers, no test seam). |
| D5 | **New handler file `market_data_override.go`** ✅ confirmed | Separate `MarketDataOverrideHandler` for `PUT/GET/DELETE /api/market-data/overrides` (API.md contract) | `NewMarketDataHandler` keeps its current routes and signature; overrides are a distinct concern with its own test file. Alt: extend market_data.go — rejected (file focus). |
| D6 | **Separate `MarketDataOverrideRepository`** ✅ confirmed | `internal/data/market_data_override.go` + own sqlc file; composed by `MarketDataRepository` (constructor signature unchanged) | Override CRUD is self-contained; the existing repository keeps one concern (provider rows) — only its three read methods call the composed override repository. Alt: add override methods to `MarketDataRepository` — rejected (mixes provider + override writes in one type). |

## Risks

- **Wide read fan-out** (performance, positions, NAV web page) — mitigated by
  the single verified merge point, Task 3 unit tests pinning merge semantics,
  and Task 6 integration coverage of each consumer read method.
- **`setupTestDB` hand-writes the schema** (not goose) — Task 6 must add the
  `market_data_overrides` DDL there (same caveat as f030); goose migrations
  are covered separately by `migration_smoke_test.go`.
- **FX forward-fill semantics** (override earlier than any provider rate):
  the override serves D ≥ override date even with no prior provider history —
  accepted, matches the spec wording.
- **Read performance** (spec constraint: single key lookup) — covered by the
  `UNIQUE(symbol, date, data_type)` index.
- **Server lifecycle is user-managed** (AGENTS.md) — Task 5 verification is
  build + tests; the user performs the first manual smoke test.

## Open Questions

- None. (Motivating incident per SPEC → Notes for Review: the four 2025-10-24
  bars XNAQ.L / QGRP.L / ARCI.L / WOEE.L are the first real data entry,
  recorded by the user after implementation — not by tests.)
