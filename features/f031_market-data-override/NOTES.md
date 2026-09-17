# Notes — Market Data Override

## Decisions (as implemented)

### D1 — Storage: new `market_data_overrides` table
`UNIQUE(symbol, date, data_type)`, price stored as TEXT (decimal string),
mirroring `market_data`. The rejected alternative was `source='override'` rows
inside `market_data`: that table's `UNIQUE(symbol, source, date)` collides the
moment a fund has both a `stock` and a `nav` override on the same date (both
would need `source='override'`). A separate table lets provider rows and
overrides coexist (spec scenario), keeps source-keyed raw reads
(`GetBySourceAndDate`) and staleness detection provider-only, and gives a clean
upsert/delete key.

### D2 — Merge at the repository read paths
Go-side merge inside the three historical read methods of
`MarketDataRepository` (`internal/data/market_data_repo.go`):
`GetHistoricalPricesBySymbol` (stock+fx types), `GetHistoricalFxRateOnOrBefore`
(latest override with `date <= D`, provider fallback), `GetNavHistoryBySymbol`
(nav type). Verified chokepoints — every historical price read funnels through
these three:

- `GetHistoricalPricesBySymbol` ← `marketservice.Service.GetHistoricalPrices`
  ← `position/equity_curve.go:88,299`, `comparison/service.go:398,458,500`,
  `performance/twr_breakpoints.go:115`, `performance/fx_conversion.go:65,181`,
  `analysis/service.go:420`, `optimization/base_service.go:83`,
  `api/handlers/performance.go:159`, `symbol_details_web.go:299`
- `GetHistoricalFxRateOnOrBefore` ← `marketservice.Service.GetHistoricalFxRate`
  ← `position/equity_curve.go:315`, `performance/fx_conversion.go:155`
- `GetNavHistoryBySymbol` ← `symbol_details_web.go:296`

One merge point, zero consumer changes, provider SQL stays raw.

### D3 — FX forward fill: override first, provider fallback
`GetHistoricalFxRateOnOrBefore` checks overrides before the provider query, so
an override dated D is the rate used for valuations on D and afterwards.
The existing `"yahoo"` argument (`marketservice.go:150`) is untouched — it now
only scopes the provider fallback.

### D4 — Thin domain service
`internal/domain/marketdata` (`Service` over a narrow `Store` interface): all
validation (non-empty symbol, valid data type, positive price, `YYYY-MM-DD`
on or before today UTC) lives here; the repository is a dumb mapper and the
handler is a thin HTTP boundary. Sentinel errors
(`ErrInvalidSymbol`, `ErrInvalidDataType`, `ErrInvalidPrice`, `ErrInvalidDate`,
`ErrNotFound`) map 1:1 to the API's 400/404 error codes.

### D5 — Separate handler file
`internal/api/handlers/market_data_override.go` with its own
`RegisterRoutes`; `NewMarketDataHandler` and the existing market-data routes
are untouched.

### D6 — Composed repository
`MarketDataOverrideRepository` (implements `marketdata.Store`, asserted at
compile time) is composed into `MarketDataRepository` at construction; the
`NewMarketDataRepository(db)` signature is unchanged, so no call site changes.

## Plan deviations

1. **No DDL edit needed in `setupTestDB`.** The plan's Task 6/risk text assumed
   `setupTestDB` carries a hand-written schema helper that would need the
   `market_data_overrides` DDL. In reality it runs the real embedded goose
   migrations (`data.MigrateUp(db, assets.Migrations, ...)` —
   `tests/integration/portfolio_test.go:36`), so migration 025 applies
   automatically. No schema drift to maintain.
2. **Override JSON shape.** The plan did not specify wire fields. Response
   bodies use the domain `Override` shape (`symbol`, `date`, `data_type`,
   `price`, `currency`, `source`); `created_at`/`updated_at` are omitted
   (`json:"-"`) — they carry no user-facing meaning and would force timestamp
   churn on every update.

## Test coverage map (spec scenario → test)

| Spec scenario / edge case | Test |
|---|---|
| Correcting a corrupted bar (provider row stays, raw reads see it) | `TestMarketDataOverride_CorrectsCorruptedBar` |
| Override on a date the provider has no row for (union of dates) | `TestMarketDataOverride_OverrideOnlyDate` |
| Override survives a provider refresh of the same date | `TestMarketDataOverride_SurvivesRefresh` |
| FX override wins for D ≥ override date, provider before (forward fill) | `TestMarketDataOverride_FxOnOrBefore` |
| NAV override reflected in NAV history (symbol detail read path) | `TestMarketDataOverride_NavHistory` |
| Live quote (`date=''`) unaffected; no override rows in `market_data` | `TestMarketDataOverride_LiveQuoteUnaffected` |
| Update: PUT same key replaces (last write wins) | `TestMarketDataOverride_UpdateReplaces` |
| Remove: 204 → reads revert; 404 on repeat; scoped to data type | `TestMarketDataOverride_Remove` |
| Validation over HTTP: non-positive price, bad/future date, unknown type, empty symbol → 400 with code | `TestMarketDataOverride_ValidationOverHTTP` |
| List: stable order symbol/date/data_type; `?symbol=` filter | `TestMarketDataOverride_ListOrderAndFilter` |

Repository-level merge semantics (coexistence, replacement, append, sort) are
covered in `internal/data/market_data_repo_test.go`; service validation in
`internal/domain/marketdata/service_test.go`; handler mapping in
`internal/api/handlers/market_data_override_test.go`.

## No API.md change

The contract in `API.md` (§ Set/List/Remove Price Override) was locked in Phase
3 and matches the implementation field-for-field (PUT body fields, 200/400
responses, optional `?symbol=`, DELETE parameter requirements, 204/400/404).
