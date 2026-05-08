# Notes: Positions

## Decisions
- 2026-05-08: `market_data.date` uses `''` (empty string) as sentinel for "latest/current" instead of `NULL`. This ensures `UNIQUE(symbol, source, date)` enforces one row per symbol per source, even for current prices. `NOT NULL DEFAULT ''` on the column. Historical snapshots use `YYYY-MM-DD` format.
- 2026-05-08: `MarketDataFetcher` replaces `QuoteFetcher` as the primary interface. `Quote` struct and `QuoteFetcher` interface removed (no longer used anywhere). `YahooFinanceFetcher.FetchQuote` returns `*MarketData`. `WithQuoteFetcher` renamed to `WithMarketDataFetcher`.
- 2026-05-08: `FxPairToYahooSymbol` helper added to `market` package for converting "GBP/USD" → "GBPUSD=X".
- 2026-05-08: Position errors use a `PositionError` struct type with `Code` and `Message` fields (following the pattern of typed errors), allowing API handlers to map to specific HTTP status codes.
- 2026-05-08: `ComputePositions` groups lots by symbol before processing, since positions are per-symbol. The function delegates to `computePositionsForLots` per symbol.
- 2026-05-08: Sell lots have negative `Quantity` (from lot grouping), used directly as the delta for running quantity (no `Neg()` needed). Buy lots have positive quantity.
- 2026-05-08: `cycleState` tracks `finalQty` (runningQty at cycle end time) to correctly determine closed vs open state for each cycle independently.

## Deviations from Plan
- Task 1 required updating existing code beyond just migrations:
  - `transaction_repo.go`: `NetCash` field changed from `sql.NullString` to `string` in sqlc models (migration 008 made it NOT NULL). Updated repo to use `t.NetCash` directly and `t.NetCash.String()` for params. Removed unused `toNullDecimal` function.
  - `transaction_repo.go`: Added `LotID sql.NullString{}` to Create/BatchCreate/Update params (domain model gets LotID in Task 3).
  - `transaction.sql`: Updated CreateTransaction and UpdateTransaction queries to include `lot_id` column.
  - Integration test setup (`portfolio_test.go`): Added `lot_id TEXT` column and new tables (positions, lots, lot_consumptions, market_data) to the manual schema setup.
  - Unit test setup (`transaction_repo_test.go`): Added `lot_id TEXT` column to the manual schema setup.

## Known Issues
- None

## Task 4c Implementation Notes (2026-05-08)
- `position_computation.go` has 3 functions: `ComputePositions` (public entry, groups by symbol), `computePositionsForLots` (walks lots, detects cycles), `buildPosition` (aggregates cycle data into Position struct).
- `cycleState` struct tracks lots, quantity direction, and final quantity at cycle end.
- Zero-crossing detection: when `newQty.Sign() == 0` after adding a lot's delta, the position closes.
- Direction change detection: when sign flips (e.g., +1 → -1), current cycle closes and a new one starts.
- P&L derived directly from lots: `totalCostBasis.Add(totalSellProc)` — mathematically equivalent to summing consumption P&Ls.
- `consumptions` parameter accepted but not used (reserved for future audit trail / verification).
- Bug fixed: initial version used final `runningQty` for all cycles; fixed by storing `finalQty` per cycleState.

## Task 4b Implementation Notes (2026-05-08)
- `govalues/decimal` API: comparison is `d.Less(e)` (not `LessThan`), division is `d.Quo(e)` (not `Div`), absolute value is `d.Abs()`. These methods will be used throughout Tasks 4c-4e.
- Proportional P&L per consumption chunk uses `Quo` for ratio computation: `consumeQty.Quo(totalQty)` then `Mul` for the proportional amount. This pattern applies to all downstream calculator tasks that split amounts across lots.
- Short positions (sell exceeds buy) produce no consumption entry for the unmatched portion — the remaining quantity is implicitly zero for all buy lots. The caller (position computation) detects shorts from the sell lot's unmatched quantity.

## Task 4a Implementation Notes (2026-05-08)
- `decimal.Add()` (and other arithmetic ops) return `(Decimal, error)` — not a single value. Calculator code handles the error with `panic(fmt.Sprintf(...))` following the `Must*` convention, since overflow on validated inputs would be a programming bug.
- `SortLotsByDate` helper added as a public function for consumers that need custom ordering (e.g., if lots are re-sliced downstream).
- `buildLots` is an unexported helper that constructs LotGroups from a lot_id→transactions map, shared by both buy and sell paths.
- `toTransactionRef` converts `transaction.Transaction` → `TransactionRef` (lightweight, no timestamps/IDs beyond what's needed for drill-down).

## Task 3 Implementation Notes (2026-05-08)
- `LotChecker` is passed as `nil` to `NewService` in `router.go` for now — it will be implemented in Task 5 (position service). The service handles `nil` lotChecker gracefully: when nil, it skips cross-checking and allows any lot_id (new lots pass through; existing lots are validated only if lotChecker is non-nil).
- `generateLotID()` uses `ulid.Make()` from `github.com/oklog/ulid/v2` and produces IDs in format `LOT-<26 char ULID>` (30 chars total). ULID is lexicographically sortable by creation time. This is the project standard for string-based unique IDs.
- `LotID` field added to `Transaction`, `CreateRequest`, and `UpdateRequest` domain models.
- `LotInfo` struct and `LotChecker` interface added to `transaction.go`.
- New errors: `ErrLotNotFound`, `ErrLotSymbolMismatch`, `ErrLotAccountMismatch`, `ErrLotTypeMismatch`, `ErrInvalidLotID`.
- Web layer: lot_id field added to form template, list table (new column), and detail view.
- All existing test files updated to pass `nil` for the new `lotChecker` parameter.
- `strPtr` helper was already defined in `validator_test.go` — removed duplicate from `mock_repository.go`.
