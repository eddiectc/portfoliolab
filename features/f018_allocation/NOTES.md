# Notes: Allocation

## Decisions
- 2026-05-19: Task 6 completed — `ComputeRebalancingSuggestions` added to allocation service. Added `GetMarketPrice(ctx, symbol)` method to `PositionSource` interface (needed for symbols in target but not yet held). Price resolution: for held symbols, derives price from allocation row (MarketValue / totalQuantity); for unheld symbols, calls `GetMarketPrice`. Shares rounded to 2 decimal places. Cash excluded from suggestions with warning. 14 tests covering: basic rebalancing, tolerance boundary (4.9/5.0/5.1%), no-drift balanced, symbol not yet held (price lookup), zero target (full sell), missing market data (warning), cash drift (warning+excluded), sort order, share rounding, market price lookup, market price unavailable, total dollar value, error propagation.
- 2026-05-19: Task 5 completed — `ComputeDrift` method added to allocation service. Builds unified symbol list from actual + target union. Drift tolerance is 5% (hard-coded `driftTolerance` var). Rows sorted by |drift| descending. 14 tests covering: basic drift, tolerance boundary (4.9/5.0/5.1%), no target, symbol in target only, symbol in actual only, cash drift, sign convention, sort order, error propagation, empty portfolio, base currency.
- 2026-05-19: Task 1 completed — migration `019_create_target_allocations.sql` created with `target_allocations` table, sqlc queries, and smoke tests.
- 2026-05-19: Task 4 completed — `TargetRepository` interface in domain layer, `TargetAllocationRepository` in `internal/data/` wired to sqlc. Service methods: `GetTargetAllocation`, `SaveTargetAllocation` (with sum=100% validation including delta in error message), `DeleteTargetAllocation`, `DeleteAllTargetAllocations`. Updated `NewService` constructor to accept `TargetRepository` parameter. Added `noopTargetRepo` stub to existing service tests.
- 2026-05-19: Task 2 completed — domain types and error variables created in `allocation.go` with basic type verification tests. Added `ErrZeroTotalValue` beyond the plan (needed for edge case: zero/negative portfolio value).
- 2026-05-19: Task 3 completed — allocation service with `ComputeAllocation` method. `AccountRef` defined as an alias for `position.AccountRef` to avoid type conversion. Cash aggregation collects all `$CASH-*` entries into a single row. Symbols with no market data are excluded from rows (not shown as zero-value entries).
- 2026-05-19: Task 3 review fixes — added `ErrMixedCurrencies` error for multi-portfolio filters with conflicting base currencies (explicit error, no silent fallback). Removed unused `baseCurrency` parameter from `buildAllocationRow`. Fixed struct field alignment (gofmt). Added tests for multiple portfolio IDs and mixed currency detection.

## Deviations from Plan
- None so far.

## Future Improvements
- None noted yet.

## Known Issues
- None.
