# Notes: Positions

## Decisions
- 2026-05-08: `market_data.date` uses `''` (empty string) as sentinel for "latest/current" instead of `NULL`. This ensures `UNIQUE(symbol, source, date)` enforces one row per symbol per source, even for current prices. `NOT NULL DEFAULT ''` on the column. Historical snapshots use `YYYY-MM-DD` format.
- 2026-05-08: `MarketDataFetcher` replaces `QuoteFetcher` as the primary interface. `Quote` struct and `QuoteFetcher` interface removed (no longer used anywhere). `YahooFinanceFetcher.FetchQuote` returns `*MarketData`. `WithQuoteFetcher` renamed to `WithMarketDataFetcher`.
- 2026-05-08: `FxPairToYahooSymbol` helper added to `market` package for converting "GBP/USD" → "GBPUSD=X".
- 2026-05-08: Position errors use a `PositionError` struct type with `Code` and `Message` fields (following the pattern of typed errors), allowing API handlers to map to specific HTTP status codes.

## Deviations from Plan
- Task 1 required updating existing code beyond just migrations:
  - `transaction_repo.go`: `NetCash` field changed from `sql.NullString` to `string` in sqlc models (migration 008 made it NOT NULL). Updated repo to use `t.NetCash` directly and `t.NetCash.String()` for params. Removed unused `toNullDecimal` function.
  - `transaction_repo.go`: Added `LotID sql.NullString{}` to Create/BatchCreate/Update params (domain model gets LotID in Task 3).
  - `transaction.sql`: Updated CreateTransaction and UpdateTransaction queries to include `lot_id` column.
  - Integration test setup (`portfolio_test.go`): Added `lot_id TEXT` column and new tables (positions, lots, lot_consumptions, market_data) to the manual schema setup.
  - Unit test setup (`transaction_repo_test.go`): Added `lot_id TEXT` column to the manual schema setup.

## Known Issues
- None
