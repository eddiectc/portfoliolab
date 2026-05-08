# Notes: Positions

## Task 13 Implementation Notes (2026-05-08)
- `RealizedPnlPct` computed during `buildPosition` in `position_computation.go` as `RealizedPnL / Abs(CostBasis) × 100`. Uses `decimal.MustNew(10000, 2)` for the × 100 multiplier.
- `EnrichWithMarketData` now accepts a `baseCurrency` parameter. The web handler resolves it from the filtered portfolio (or first portfolio if no filter). The API handler passes empty string.
- `convertValuesToBase` is a standalone function (not a method) for easy testing. Uses `FxRateProvider.GetCurrentRate` to fetch current spot FX rate. Returns nil pointers if conversion not possible.
- Cash positions: `CostBasis = Quantity` (the balance is the cost), so P&L% is computed naturally as `0 / |Qty| × 100 = 0` with no special-casing. `CostBasisBase = MarketValueBase` (same FX rate).
- `CostBasisBase` added to `PositionWithMarket` — cost basis converted to base currency using same FX rate as market value. For same-currency positions, `CostBasisBase = Abs(CostBasis)`.
- `positionSummary` struct in web handler aggregates totals across all positions in the current page (not across all pages — pagination means only visible positions are summed).
- Summary panel shows only base-currency values: Cost Basis (Base), Mkt Value (Base), Unrealized P&L (Base), Unrealized P&L %. Native currency columns not needed in summary.
- Open positions table includes "Cost Basis (Base)" column alongside native cost basis.
- `sign` template function returns "positive", "negative", or "" for decimal strings. Handles "0", "0.00", and empty string as neutral.
- Closed positions template uses dynamic header `P&L ({BaseCurrency})` via Go template interpolation.
- FX Rate column on closed positions shows the rate used during recalculation (`FxRateUsed` field), not the current spot rate.
- Summary panel CSS uses CSS Grid with `auto-fit` for responsive layout. Cards have white background with subtle shadow.
- `sign` function added to template funcMap for use in summary panel P&L coloring.

## Task 12 Implementation Notes (2026-05-08)
- **Bug fix: `toPosition()` and `toLot()` in `position_repo.go` crashed on empty-string nullable fields.** The `avg_close_price`, `realized_pnl_base`, `close_date`, and `sell_price` fields are stored as empty strings `""` (not NULL) for open positions/lots. When `sql.NullString` has `Valid=true` but `String=""`, `decimal.Parse("")` fails with "invalid decimal: no coefficient". Fixed by adding `&& p.Field.String != ""` guards before parsing. Also fixed nil-pointer dereference for `avg_open_price` (non-pointer field in domain model) by defaulting to `decimal.Zero` when nil.
- Integration tests cover: full recalc flow, FIFO with real SQL, cash tracking, position transitions, multiple cycles, recalculate endpoint, idempotency, account filtering, closed positions, lot detail, cascade delete, and dividend with no open position.
- Unit tests added: `TestGroupTransactionsIntoLots_LotIDCaseSensitive` (lot IDs are case-sensitive) and `TestCalculatePositions_IdempotentRecalc` (recalc produces identical results on same input).
- Cash position quantity is the raw `net_cash` sum (in cents), not converted to dollars. Tests updated accordingly.

## Task 11 Implementation Notes (2026-05-08)
- `EnrichWithMarketData` is a method on `position.Service` (not in the API handler) so both the API and web handlers share the same enrichment logic.
- `WithMarketDataFetcher` is a separate method (not a constructor parameter) to keep the existing `NewService` signature backward compatible — all existing callers (transaction service, import services) continue to work without changes.
- `EnrichWithMarketData` is nil-safe: if `marketFetcher` or `marketDataRepo` is nil, returns positions with `MarketDataAvailable=false` and zero market values.
- Market data fetch errors are logged at DEBUG level (not WARN) to avoid noise when Yahoo Finance is temporarily unavailable.
- Upsert cache errors are also logged at DEBUG level — the position is still returned with market data even if caching fails.
- Cash positions (`$CASH-*`) skip the market fetch entirely: `MarketValue = quantity` (balance), `UnrealizedPnL = 0`, `MarketDataAvailable = true`.
- `UnrealizedPnL = MarketValue + CostBasis` (since CostBasis is negative, adding it is equivalent to subtracting the absolute total cost).
- `UnrealizedPnlPct = UnrealizedPnL / Abs(CostBasis) × 100`, stored at scale 2 (e.g. 123.45 = 123.45%).
- API handler `HandleListOpen` returns `[]PositionWithMarket` (breaking change from `[]Position`, but feature is still in-progress).
- API handler `HandleListClosed` unchanged — returns `[]Position` without market data enrichment.
- Web handler `HandleOpenPositions` uses new `openPositionListPageData` struct with `[]PositionWithMarket`.
- Template `open.html` adds 4 new columns: Market Price, Market Value, Unrealized P&L, P&L %.
- Template uses `—` for unavailable market data; P&L columns styled with `positive`/`negative` CSS classes.
- `router.go` wires `yahooFetcher` and `marketDataRepo` into position service via `WithMarketDataFetcher`.

## Task 10 Implementation Notes (2026-05-08)
- FX conversion happens in the **service layer** (not inside the calculator), keeping `CalculatePositions` pure and testable without I/O dependencies.
- `FxConverter` implements `FxRateProvider` interface with three-tier fallback: DB historical → on-demand fetch + cache → current spot rate.
- `FxConverter` handles nil logger gracefully via `logWarn()` helper (nil-safe).
- `ConvertPnlToBase` is a standalone function (not a method) for easy unit testing. It returns `(convertedPnL, rateUsed, isFallback)`.
- `BuildFxPair` constructs "BASE/QUOTE" pair from position currency and base currency; returns empty string when currencies match.
- `PortfolioCurrencyChecker` interface added to position domain; `PortfolioCurrencyCheckerImpl` in data layer uses existing `GetPortfolio` sqlc query.
- `AccountRef` extended with `PortfolioCurrency` field; new sqlc queries `ListAllAccountsWithPortfolioCurrency` and `GetAccountsByPortfolioWithCurrency` join accounts with portfolios.
- `AccountListerImpl` updated to use the new join queries, populating `PortfolioCurrency`.
- Cash positions (`$CASH-*`) are skipped during FX conversion — they are already in their own currency.
- For closed positions, FX rate date is the close date; for open positions, current spot rate is preferred.
- `decimal.Mul` combines scales (pnl scale 2 × rate scale 4 = result scale 6), which is preserved in the stored `realized_pnl_base`.
- Templates show `—` when no base-currency P&L is available (same-currency positions), and `⚠` when fallback rate was used.
- Service constructor accepts `nil` for both `PortfolioCurrencyChecker` and `FxRateProvider` — FX conversion is silently skipped when either is nil (backward compatible).

## Task 9 Implementation Notes (2026-05-08)
- `MarketDataRepository` in `internal/data/market_data_repo.go` delegates to sqlc-generated queries from `market_data.sql` (created in Task 1).
- Repository methods return `nil` (not error) when no data found, following the "no data is not an error" pattern for optional lookups.
- `toMarketDatum` converts sqlc `MarketDatum` to domain `*market.MarketData`, parsing price from string to decimal and fetched_at from RFC3339 to time.Time.
- `Upsert` uses the existing sqlc `InsertMarketData` query which has `ON CONFLICT(symbol, source, date) DO UPDATE SET`.
- `FxRate` struct and `FxRateFetcher` interface added to `internal/market/fx.go`.
- `YahooFinanceFetcher` implements both `MarketDataFetcher` and `FxRateFetcher` interfaces.
- `FetchRate` on `YahooFinanceFetcher` delegates to `FetchFxRate` and wraps the result as `*FxRate`.
- `ParseFxPair` helper splits "BASE/QUOTE" into components with validation.
- `FxError` typed error for FX-specific errors.
- `market_data_repo` created in `router.go` but not yet wired to services (reserved for Tasks 10-11).
- Existing `market_data.sql` queries were already generated by sqlc in Task 1; no regeneration needed.
- Repo tests use manual schema setup (in-memory SQLite) following the `symbol_mapping_repo_test.go` pattern.

## Task 8 Implementation Notes (2026-05-08)
- `PositionWebHandler` takes `*position.Service`, `*account.Service`, `*portfolio.Service`, and `*web.Renderer` (following the TransactionWebHandler pattern).
- `PositionFilter` struct with `AccountID` and `PortfolioID` string fields, `QueryParams()` and `PaginationQuery(page int)` methods (following TransactionFilter pattern).
- Three templates: `open.html` (open positions with recalculate button and link to closed), `closed.html` (closed positions with link to open), `lot_detail.html` (lot summary, consumptions table, source transactions table).
- Cash positions highlighted with `cash-row` CSS class and `cash-symbol` span using the `contains` template function.
- P&L values styled with `positive`/`negative` CSS classes using `IsPos`/`IsNeg` decimal methods.
- Recalculate handler redirects to `/positions` with flash message indicating scope (account, portfolio, or all).
- Lot detail page shows consumptions (only populated for sell lots via `GetConsumptionsBySellLot`), and source transactions from `LotWithDetails.Transactions`.
- Nav link updated from `#`/`disabled` to `/positions` (active link).
- Tests cover PositionFilter serialization, parsing, and query encoding.

## Task 7 Implementation Notes (2026-05-08)
- `PositionHandler` takes `*position.Service` (concrete type, following the existing pattern of TransactionHandler).
- Filter resolution (`account_id` → single ID, `portfolio_id` → accounts in portfolio, `account_ids` → explicit list, none → all accounts) is handled by two new service methods: `GetOpenPositionsFiltered` and `GetClosedPositionsFiltered`, which internally call `resolveAccountIDs`. This keeps the handler thin.
- `resolveAccountIDs` is a private method on the service that resolves `ListFilters` to `[]int64` account IDs using the `AccountLister`.
- Recalculate endpoint accepts `account_id`, `portfolio_id`, or neither (→ all) as query params.
- Error responses use `PositionError` typed errors: `ErrLotNotFound` → 404 LOT_NOT_FOUND, `ErrAccountNotFound` → 404 ACCOUNT_NOT_FOUND, `ErrPortfolioNotFound` → 404 PORTFOLIO_NOT_FOUND.
- Tests use real `position.Service` with mock repos (following the transaction handler test pattern), not a mock service interface.
- Mock types prefixed with `mockPos` (e.g., `mockPosRepo`, `mockPosAccountChecker`) to avoid name collisions with other test files in the same package.
- Lot tests use chi router for URL param extraction (`chi.URLParam` needs chi's routing context).

## Task 6 Implementation Notes (2026-05-08)
- Both import services use functional options pattern (`ServiceOption`) for optional dependencies (`WithPositionRecalculator`, `WithLogger`), keeping the `NewService` signature backward compatible.
- IBKR: lot_id format is `LOT-IBKR-<ibOrderID>` so partial fills of the same order share one lot. Only buy/sell trades get lot_ids; cash transactions (dividends, interest, fees) and transfers do not.
- Trading 212: lot_id format is `LOT-<ulid>` (auto-generated per trade) since no order ID exists in broker data. Only buy/sell trades get lot_ids; deposits, withdrawals, and interest do not.
- Recalculation is fire-and-forget: errors are logged but don't fail the import (same semantics as transaction CRUD recalc).
- Both services accept `*slog.Logger` via `WithLogger` for conditional warning logs on recalc failures.

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

## Task 4e Implementation Notes (2026-05-08)
- `calculator_integration.go` has one public function: `CalculatePositions` and two unexported helpers: `lotsToDomain` and `ptrDecimal`.
- `CalculatePositions` accepts `context.Context` and `accountID` but doesn't use the context (no I/O — pure computation). Context accepted for API consistency with downstream service layer.
- `lotsToDomain` converts `LotGroup` (calculator intermediate) to `Lot` (domain model). DB fields (ID, CreatedAt, UpdatedAt) left at zero — populated by service layer on persist.
- Sell lots in `LotGroup` have negative `Quantity`; `lotsToDomain` takes `Abs()` for the domain model `Lot.Quantity` (always positive).
- Sell lot's `SellProceeds` mapped to domain model's `SellPrice` field.
- Account ID injected into all positions after computation (calculator doesn't know the account).
- Position-level `RealizedPnL` = total cost basis + total sell proceeds (net cash flow), NOT the FIFO consumption P&L. This is by design in `ComputePositions`.
- Floating point tolerance needed in one test: `Quo(20, 120) = 1/6` (repeating decimal) introduces tiny precision error in proportional P&L. Used `Abs().Less(tolerance)` for comparison.

## Task 4d Implementation Notes (2026-05-08)
- `cash_position.go` has one public function: `ComputeCashPositions` and one unexported helper: `cashSymbol`.
- Cash-affecting transaction types are defined as a `map[string]bool` constant: deposit, withdrawal, dividend, interest, fee, tax, buy, sell.
- Transactions with `net_cash == 0` are skipped (no effect on cash balance).
- The `net_cash` field already carries the correct sign per transaction type, so no manual sign flipping is needed — just sum directly.
- Open date is the earliest transaction date across all cash-affecting transactions in that currency.
- Output is sorted by symbol (`$CASH-GBP` before `$CASH-USD`) for deterministic output.

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

## Task 5 Implementation Notes (2026-05-08)
- `PositionRepository` in `internal/data/position_repo.go` delegates to sqlc-generated queries and handles domain ↔ sqlc type mapping (timestamps as RFC3339 strings, decimal as text).
- `Recalculate` method on `PositionRepository` handles the full delete-then-insert within a single DB transaction, called by the service layer. This keeps the service testable with mocks.
- `PositionRepository` interface includes `Recalculate(ctx, accountID, result *CalculateResult) error` as a single method rather than exposing raw DB access.
- `Service` in `internal/domain/position/service.go` depends on: `PositionRepository`, `TransactionRepository` (custom interface for `ListAllTransactionsByAccount`), `AccountChecker`, `PortfolioChecker`, `AccountLister`.
- `AccountLister` interface (in position domain) returns `AccountRef` (minimal: ID, Name, PortfolioID). Implemented by `AccountListerImpl` in `internal/data/account_lister.go`.
- `GetLotInfo` returns `*transaction.LotInfo` (not `*position.LotInfo`) to satisfy the `transaction.LotChecker` interface without circular imports.
- `PositionRecalculator` interface added to transaction domain: `RecalculateAccount(ctx, accountID) error`. Wired into transaction service as optional dependency (nil-safe).
- Transaction service calls `RecalculateAccount` after Create/Update/Delete. Errors from recalc are logged but don't fail the transaction mutation (fire-and-forget semantics).
- New sqlc queries added: `ListAllTransactionsByAccount` (no pagination, date ASC), `ListAllAccounts`, `GetAllAccountsByPortfolio`.
- New file: `internal/data/account_lister.go` — adapter for listing accounts as `position.AccountRef`.
- Service tests use hand-written mocks for all dependencies. `mockPositionRepository.Recalculate` simulates real behavior: deletes old data, inserts new lots/consumptions/positions with generated IDs.
- Test helpers `makeBuyTxn`/`makeSellTxn` generate sequential lot IDs via `nextLotID()` to satisfy the calculator's lot grouping requirement.

## Task 3 Implementation Notes (2026-05-08)
- `LotChecker` is wired to the position service in `router.go` (Task 5). The service handles `nil` lotChecker gracefully for backward compatibility: when nil, it skips cross-checking and allows any lot_id (new lots pass through; existing lots are validated only if lotChecker is non-nil).
- `generateLotID()` uses `ulid.Make()` from `github.com/oklog/ulid/v2` and produces IDs in format `LOT-<26 char ULID>` (30 chars total). ULID is lexicographically sortable by creation time. This is the project standard for string-based unique IDs.
- `LotID` field added to `Transaction`, `CreateRequest`, and `UpdateRequest` domain models.
- `LotInfo` struct and `LotChecker` interface added to `transaction.go`.
- New errors: `ErrLotNotFound`, `ErrLotSymbolMismatch`, `ErrLotAccountMismatch`, `ErrLotTypeMismatch`, `ErrInvalidLotID`.
- Web layer: lot_id field added to form template, list table (new column), and detail view.
- All existing test files updated to pass `nil` for the new `lotChecker` parameter.
- `strPtr` helper was already defined in `validator_test.go` — removed duplicate from `mock_repository.go`.
