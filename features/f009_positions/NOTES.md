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

## Task 3 Implementation Notes (2026-05-08)
- `LotChecker` is passed as `nil` to `NewService` in `router.go` for now — it will be implemented in Task 5 (position service). The service handles `nil` lotChecker gracefully: when nil, it skips cross-checking and allows any lot_id (new lots pass through; existing lots are validated only if lotChecker is non-nil).
- `generateLotID()` uses `ulid.Make()` from `github.com/oklog/ulid/v2` and produces IDs in format `LOT-<26 char ULID>` (30 chars total). ULID is lexicographically sortable by creation time. This is the project standard for string-based unique IDs.
- `LotID` field added to `Transaction`, `CreateRequest`, and `UpdateRequest` domain models.
- `LotInfo` struct and `LotChecker` interface added to `transaction.go`.
- New errors: `ErrLotNotFound`, `ErrLotSymbolMismatch`, `ErrLotAccountMismatch`, `ErrLotTypeMismatch`, `ErrInvalidLotID`.
- Web layer: lot_id field added to form template, list table (new column), and detail view.
- All existing test files updated to pass `nil` for the new `lotChecker` parameter.
- `strPtr` helper was already defined in `validator_test.go` — removed duplicate from `mock_repository.go`.
