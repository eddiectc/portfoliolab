# Notes: Positions

## Decisions
- 2026-05-08: `market_data` UNIQUE constraint on `(symbol, source, date)` — in SQLite, NULL values are distinct in UNIQUE constraints, so multiple "latest" entries (date=NULL) for the same symbol/source are allowed. This is acceptable since the app layer will handle deduplication via ON CONFLICT DO UPDATE.

## Deviations from Plan
- Task 1 required updating existing code beyond just migrations:
  - `transaction_repo.go`: `NetCash` field changed from `sql.NullString` to `string` in sqlc models (migration 008 made it NOT NULL). Updated repo to use `t.NetCash` directly and `t.NetCash.String()` for params. Removed unused `toNullDecimal` function.
  - `transaction_repo.go`: Added `LotID sql.NullString{}` to Create/BatchCreate/Update params (domain model gets LotID in Task 3).
  - `transaction.sql`: Updated CreateTransaction and UpdateTransaction queries to include `lot_id` column.
  - Integration test setup (`portfolio_test.go`): Added `lot_id TEXT` column and new tables (positions, lots, lot_consumptions, market_data) to the manual schema setup.
  - Unit test setup (`transaction_repo_test.go`): Added `lot_id TEXT` column to the manual schema setup.

## Future Improvements
- Consider using a partial unique index or trigger for `market_data` to enforce uniqueness on `(symbol, source)` when `date IS NULL`.

## Known Issues
- None
