# Notes: Positions

## Phase 5b Implementation Review Fixes (2026-05-09)

### Code Conventions Fixes
- **C1/D1: Raw SQL for deletes in `Recalculate`** — Replaced raw `DELETE` statements with sqlc-generated queries: `DeleteAllPositionsForAccount`, `DeleteAllLotsForAccount`, `DeleteAllLotConsumptionsForAccount`. Inserts remain as parameterized raw SQL (sqlc doesn't support bulk inserts; documented as safe).
- **C2/D2: Computed-at-read `RealizedPnlPct`** — Added `realized_pnl_pct TEXT DEFAULT NULL` column to `positions` table via migration `014`. Value is now pre-computed in `buildPosition` and persisted to DB. `toPosition()` reads from DB instead of computing at read time.
- **D7: Missing index** — Added `CREATE INDEX idx_positions_currency ON positions(currency)` in migration `014` for currency filtering queries.
- **Transaction concurrency** — Removed incorrect `sql.LevelImmediate` attempt (doesn't exist in Go stdlib). Using standard `BeginTx` with SQLite WAL mode. Concurrent recalculations get "database is locked" errors which caller can retry. Documented in code comment.

### Test Quality Additions
- **D8: `unrealizedPnlPct` computation test** — Added 3 tests: `TestUnrealizedPnlPct_PositivePnl` (20% gain), `TestUnrealizedPnlPct_NegativePnl` (-20% loss), `TestUnrealizedPnlPct_ZeroPnl` (0% flat).
- **T2: Recalculate error propagation** — Added `TestRecalculateAccount_RepositoryError` (DB error propagates) and `TestRecalculateAccount_TransactionListError` (transaction list error propagates).
- **T1: Zero-quantity lots** — Already covered by existing test `TestComputePositions_EmptyLots` in `position_computation_test.go`.
- **T3: Sell exceeds buy** — Already covered by existing tests in `fifo_matching_test.go` and `position_computation_test.go`.

### Closed Items
- **C4: `ptrDecimal` not exported** — By-design. `ptrDecimal` is an internal helper in `calculator_integration.go` used only by the calculator. Not needed outside the domain layer.
- **D3: Missing DB constraint** — Already exists. `positions` table has `NOT NULL` on `account_id`, `symbol`, `quantity`, `cost_basis`.
- **D5: Missing `ptrDecimal` in `calculator_integration.go`** — Already exists. `ptrDecimal` helper defined in `calculator_integration.go`.
- **E1/E6: Short selling** — By-design per spec. Short positions are out of scope (Non-Goals).
- **E4: Concurrent recalculation** — Documented with WAL mode explanation. SQLite handles single-writer concurrency; concurrent recalculations get "database is locked" errors.

### Review Summary
- **Spec Coverage: PASS** — All 8 user stories and 30+ scenarios implemented
- **Plan Fidelity: PASS** — All 13 tasks completed; deviations documented
- **Test Quality: PASS** — Table-driven unit tests + 15 integration tests; gaps (D8, T2) filled
- **Code Conventions: PASS** (was FAIL) — C1, C2, D1, D2, D7 fixed
- **Edge Cases: PASS** (was FAIL) — E1/E6 by-design, E4 documented
- **Scope Creep: PASS** — All additions are natural spec dependencies
- **TODOs/Debt: PASS** (was FAIL) — D1, D2, D7 fixed; remaining items closed or addressed

## FX API Refactoring (2026-05-09)

Replaced the "pair string" (`"GBP/USD"`) parameter with explicit `baseCurrency, quoteCurrency` parameters throughout the FX chain. The pair string is now only constructed at the storage layer for the DB `symbol` column.

### Interface Changes

| Interface | Before | After |
|---|---|---|
| `FxRateFetcher.FetchRate` | `(ctx, pair string)` | `(ctx, baseCurrency, quoteCurrency string)` |
| `MarketDataFetcher.FetchFxRate` | `(ctx, pair string)` | `(ctx, baseCurrency, quoteCurrency string)` |
| `FxRateProvider.GetCurrentRate` | `(ctx, pair string)` | `(ctx, baseCurrency, quoteCurrency string)` |
| `FxRateProvider.GetRateForDate` | `(ctx, pair string, date)` | `(ctx, baseCurrency, quoteCurrency string, date)` |
| `MarketDataRepository.GetCurrentFxRate` | `(ctx, pair string)` | `(ctx, baseCurrency, quoteCurrency string)` |
| `FxPairToYahooSymbol` | `(pair string)` | `(baseCurrency, quoteCurrency string)` |

### Structural Changes

- **Removed `FxRate.Pair` field** — redundant given `BaseCurrency` + `QuoteCurrency` are already present
- **Removed `market.ParseFxPair`** — no caller needed to parse pair strings anymore
- **Removed `BuildFxPair` from `fx_converter.go`** — replaced by `market.FormatFxPair(base, quote string)` in the `market` package
- **`FxRate` struct** now has `BaseCurrency`, `QuoteCurrency`, `Rate`, `FetchedAt` (no `Pair`)
- **`market.FormatFxPair`** constructs the `"BASE/QUOTE"` string only at the storage layer (DB lookups, upserts)
- **`service.go` callers** pass `p.Currency, baseCurrency` directly instead of constructing a pair string first

### Rationale

Passing the pair string through the entire call chain was lossy — every caller had to construct it from two values, and every callee had to parse it back. The explicit parameters make the API self-documenting and eliminate the parse/format round-trip.

## Batch Quote Fetching (2026-05-09)

Optimized market data fetching on the open positions page by grouping symbol lookups.

- **`FetchQuotesBatch`** added to `MarketDataFetcher` interface, implemented via `go-yfinance` `multi` package
- `multi.NewTickers(symbols)` creates tickers sharing one HTTP client — auth (cookie/crumb) handled once
- `EnrichWithMarketData` deduplicates symbols across all positions before fetching
- Partial failures tolerated: failed symbols omitted from result map, others succeed

## Regression Tests and Template Fixes (2026-05-09)

## Regression Tests and Template Fixes (2026-05-09)
- **Closed positions template crash: `.BaseCurrency` undefined on `Position` struct.** The `closed.html` template referenced `.BaseCurrency` on each position item, but only `PositionWithMarket` (open positions) has that field — `Position` (closed positions) does not. Fixed by using `$.BaseCurrency` (page-level data struct field) instead of `.BaseCurrency` (item-level).
- **`market_data.updated_at` migration failed with non-constant default.** SQLite's `ALTER TABLE ADD COLUMN` rejects expression defaults like `datetime('now')`. Migration `013` uses `DEFAULT ''` (literal empty string) instead, and `ON CONFLICT DO UPDATE SET updated_at = datetime('now')` in the `InsertMarketData` query sets the timestamp on every upsert.
- **Web rendering regression tests added.** 4 new integration tests hit the actual web pages and verify they render 200 OK with HTML content, catching template errors early:
  - `TestWeb_OpenPositionsPage_Renders200` — open positions with data
  - `TestWeb_ClosedPositionsPage_Renders200` — closed positions with data
  - `TestWeb_ClosedPositionsPage_Empty_Renders200` — empty closed positions
  - `TestWeb_OpenPositionsPage_Empty_Renders200` — empty open positions
- **Router options pattern for templates directory.** Added `RouterOption` functional options to `api.Router()` so integration tests can specify the templates directory relative to their package location (`../../templates`). Tests use `skipIfTemplatesUnavailable` guard to skip gracefully when templates aren't accessible.

## Post-Review Bug Fixes and UX Improvements (2026-05-09)

### Bug Fixes
- **Currency column showed stock symbol instead of currency.** `buildPosition` used `c.lots[0].Symbol` for the currency field. Fixed by adding `getCurrencyFromLots()` which extracts the currency from the first transaction in the first lot that has a non-empty currency. Handles both trade and cash lots uniformly.
- **Account column was empty.** `getPositions` in the service layer did not populate `AccountName`. Fixed by building an account name lookup map via `s.accountLister.GetAllAccounts()` after fetching positions.
- **Average open price displayed as negative.** `AvgOpenPrice` was computed as `costBasis / totalBuyQty` which yields a negative value (since cost basis is negative). Fixed by applying `.Abs()` to the result so it displays as a positive price. Updated 6 test assertions accordingly.
- **Numbers lacked proper formatting.** Decimal values rendered as raw strings (e.g. "1234567.8912"). Added `formatMoney` (2dp, thousands separator), `formatFX` (4dp), and `formatPct` (2dp) template helpers using `formatDecimal()` and `addThousandsSeparator()` in `renderer.go`. Updated all position templates (open, closed, lot detail) to use these helpers.
- **UK stock market data (GBp) not converted to GBP.** Yahoo Finance returns some UK stock prices in GBp (pence) instead of GBP (e.g. ARCI.L at 350 GBp = 3.50 GBP). Detection is from the market data response: if `quote.Currency == "GBp"` and `p.Currency == "GBP"`, the price is divided by 100. Not all `.L` symbols are GBp (e.g. XNAQ.L is GBP), so the currency field from the quote response is the authoritative signal.

### UX Improvements
- **Different columns for open vs closed positions.** Open positions show: Symbol, Qty, Avg Cost, Cost Basis, Mkt Price, Mkt Value, Unrealized P&L, P&L %, Currency, Account, Opened (11 columns). Removed Avg Close Price (rarely populated for open positions), Realized P&L, and Realized P&L (Base). Closed positions show: Symbol, Qty, Avg Cost, Avg Close, Cost Basis, Realized P&L, Realized P&L (Base), Currency, Account, Opened, Closed (11 columns).
- **Right-aligned numbers.** Numeric columns use `.num` CSS class with `text-align: right` and `font-variant-numeric: tabular-nums` for aligned digit columns.
- **Compact table styling.** `.table-compact` class: `0.8125rem` cell font, `0.6875rem` header font, tighter padding (`0.4rem 0.6rem` cells, `0.5rem 0.6rem` headers). Row hover highlight with `#f8f9fa` background.
- **Row-level P&L color accent.** `rowClass` template helper computes total P&L ratio = `(realized_pnl + market_value) / |cost_basis|`. Green left border (`row-profit`) for ≥ 5%, red left border (`row-loss`) for ≤ -5%, no accent otherwise. Cell-level P&L coloring (positive/negative text) preserved alongside row accent.

### Tests Added
- `TestGetCurrencyFromLots` — 5 cases covering single lot, multiple lots, empty currency, empty lots
- `TestEnrichWithMarketData_GbpConversion` — verifies GBp→GBP conversion for UK stocks
- `TestEnrichWithMarketData_GbpNotConvertedForNonGbpPosition` — verifies no conversion when position currency is not GBP
- `TestFormatDecimal_Positive`, `TestFormatDecimal_Negative`, `TestFormatDecimal_Empty`, `TestAddThousandsSeparator` — template helper coverage

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
- 2026-05-09: FX APIs take explicit `baseCurrency, quoteCurrency` parameters instead of a "BASE/QUOTE" pair string. The pair string is only constructed at the storage layer via `market.FormatFxPair()`. This eliminates the parse/format round-trip through the call chain and makes the API self-documenting. `FxRate.Pair` field and `ParseFxPair` removed.
- 2026-05-09: `FetchQuotesBatch` uses `go-yfinance` `multi` package for batch fetching. `multi.NewTickers()` creates tickers sharing one HTTP client so auth (cookie/crumb) is handled once for all symbols.
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
