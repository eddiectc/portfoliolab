# Notes: Allocation

## Decisions
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
