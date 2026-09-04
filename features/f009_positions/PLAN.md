# Implementation Plan: Positions

## Overview

Build a position tracking system that computes open/closed positions from transactions, with lot management (buy/sell lots), FIFO lot matching for realized P&L, cash positions, multi-currency P&L with FX conversion, and a web UI for browsing positions with drill-down into lots and transactions.

The system stores pre-computed positions and lots in the database, recalculating them whenever transactions change (auto-recalc on CRUD) or on manual trigger.

## Task Dependencies

```
Task 1 (schema) → Task 2 (domain models) → Task 3 (lot_id on transactions) → Task 4a (lot grouping)
                                                    ↓
                                              Task 4b (FIFO matching)
                                                    ↓
                                              Task 4c (position computation)
                                                    ↓
                                              Task 4d (cash positions)
                                                    ↓
                                              Task 4e (calculator integration)
                                                    ↓
Task 5 (position service + recalc)
                                                    ↓
Task 6 (import handler recalc)
                                                    ↓
Task 7 (position API) → Task 8 (position web UI)
                                                    ↓
Task 9 (market_data table + fetcher) → Task 10 (multi-currency P&L)
                                                    ↓
Task 11 (market data integration for open positions)
                                                    ↓
Task 12 (polish + integration tests)
                                                    ↓
Task 13 (position page enhancements)
```

Tasks 1-2 are foundations. Task 3 adds lot_id to transactions (needed by calculator). Tasks 4a-4e are the calculator (sequential, each independently testable). Tasks 5-6 wire recalc into the system. Tasks 7-8 are the user-facing layers. Tasks 9-11 add market data and FX. Task 12 is final polish.

## Tasks

### Task 1: Database schema — positions, lots, lot_consumptions, market_data tables [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundational)

**Description:** Add four new tables and one column alteration to support position tracking and market data storage.

- [x] Write migration `009_create_positions.sql` with:
  - `positions` table (id, account_id, symbol, currency, quantity TEXT, cost_basis TEXT, avg_open_price TEXT, avg_close_price TEXT, realized_pnl TEXT, realized_pnl_base TEXT, open_date TEXT, close_date TEXT, is_closed INTEGER DEFAULT 0, created_at TEXT, updated_at TEXT)
  - `lots` table (id, lot_id TEXT UNIQUE, account_id, symbol, lot_type TEXT ('buy'/'sell'), quantity TEXT, cost_basis TEXT, sell_price TEXT, realized_pnl TEXT, open_date TEXT, close_date TEXT, created_at TEXT, updated_at TEXT)
  - `lot_consumptions` table (id, sell_lot_id, buy_lot_id, quantity_consumed TEXT, cost_basis_consumed TEXT, realized_pnl TEXT, created_at TEXT)
  - `ALTER TABLE transactions ADD COLUMN lot_id TEXT` (nullable)
  - Indexes: `idx_positions_account_symbol`, `idx_lots_account_symbol`, `idx_lots_lot_id`, `idx_transactions_lot_id`, `idx_lot_consumptions_sell_lot`, `idx_lot_consumptions_buy_lot`
  - Foreign keys with `ON DELETE CASCADE`
- [x] Write migration `010_create_market_data.sql` with:
  - `market_data` table: id, symbol TEXT, price TEXT, currency TEXT, data_type TEXT ('stock'/'fx'), source TEXT (provider identifier, e.g. 'yahoo'), date TEXT (NULL = latest/current, YYYY-MM-DD = historical snapshot), fetched_at TEXT, created_at TEXT
  - `UNIQUE(symbol, source, date)` constraint (one entry per symbol per source per date)
  - Indexes: `idx_market_data_symbol`, `idx_market_data_symbol_date`
  - Supports both stock quotes and FX rates in one table; `source` column enables multiple providers; historical data for future analytics features
- [x] Write `sqlc` queries in `position.sql` and `market_data.sql`
- [x] Run `sqlc generate`
- [x] Verify migrations run cleanly in `tests/integration/migration_smoke_test.go`

**Verification:** Both migrations run cleanly on empty DB; `sqlc generate` produces Go code for all new tables; smoke test passes.

---

### Task 2: Domain models — Position, Lot, LotConsumption, MarketData [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundational)

**Description:** Define domain models and DTOs in `internal/domain/position/` and extend `internal/market/`.

- [x] Create `internal/domain/position/position.go` with:
  - `Position` struct (ID, AccountID, AccountName, Symbol, Currency, Quantity, CostBasis, AvgOpenPrice, AvgClosePrice, RealizedPnL, RealizedPnlBase, OpenDate, CloseDate, IsClosed)
    - CostBasis, AvgOpenPrice, AvgClosePrice all derived from `net_cash` (includes commission/fee), not quantity × price
  - `PositionWithMarket` struct (Position + MarketPrice, MarketValue, UnrealizedPnL, UnrealizedPnlPct, MarketDataAvailable)
  - `Lot` struct (ID, LotID, AccountID, Symbol, LotType, Quantity, CostBasis, SellPrice, RealizedPnL, OpenDate, CloseDate)
  - `LotConsumption` struct (ID, SellLotID, BuyLotID, QuantityConsumed, CostBasisConsumed, RealizedPnL)
  - `LotWithDetails` struct (Lot + Consumptions []LotConsumption, Transactions []Transaction)
  - `ListFilters` struct (AccountID, PortfolioID, AccountIDs) with `QueryParams()` method implementing `web.FilterEncoder`
- [x] Define position service errors: `ErrPositionNotFound`, `ErrLotNotFound`, `ErrLotSymbolMismatch`, `ErrLotAccountMismatch`, `ErrLotTypeMismatch`, `ErrInvalidLotID`
- [x] Define calculator interfaces in `internal/domain/position/calculator.go` (stub — logic in Tasks 3a-3d):
  - `LotGroup` struct (LotID, AccountID, Symbol, LotType, Transactions []Transaction, OpenDate)
  - `MatchResult` struct (SellLot, Consumptions []LotConsumption, RealizedPnL)
  - `CalculateResult` struct (OpenPositions []Position, ClosedPositions []Position, Lots []Lot, Consumptions []LotConsumption)
- [x] Extend `internal/market/quote.go`:
  - Add `MarketData` struct (Symbol, Price, Currency, DataType, Source, Date, FetchedAt)
  - Add `MarketDataFetcher` interface with `FetchQuote(ctx, symbol string) (*MarketData, error)`, `FetchFxRate(ctx, baseCurrency, quoteCurrency string) (*MarketData, error)`, and `FetchQuotesBatch(ctx, symbols []string) map[string]*MarketData`
  - Rename existing `QuoteFetcher` → embed into `MarketDataFetcher`
  - Update `YahooFinanceFetcher` to implement `MarketDataFetcher`

**Verification:** Models compile; no logic yet, just types, interfaces, and constants. ✅ All tests pass.

---

### Task 3: Add lot_id to transaction CRUD — validation, auto-generation, lot_id field [PRIORITY: HIGH]

**Corresponds to:** "Create a buy transaction with auto-generated lot", "Create a buy transaction with user-specified lot ID (existing lot)", "Create a buy transaction with user-specified lot ID (new lot)", "Create a sell transaction with auto-generated lot", "Create a sell transaction with user-specified lot ID", "Reject transaction with lot ID belonging to a different symbol", "Reject transaction with lot ID belonging to a different account", "Reject sell transaction with lot ID belonging to a buy lot"

**Description:** Add `lot_id` field to transaction create/update, with validation and auto-generation.

- [x] Add `LotID *string` to `transaction.CreateRequest` and `transaction.UpdateRequest` in `transaction.go`
- [x] Update sqlc queries in `transaction.sql` to include `lot_id` in Create/Update/List (done in Task 1)
- [x] Add `LotID sql.NullString` to `queries.Transaction` model; run `sqlc generate` (done in Task 1)
- [x] Add `LotID` field to `toTransaction()` converter in `transaction_repo.go`
- [x] Add lot_id validation in `transaction/validator.go`:
  - Empty string treated as nil (auto-generate)
  - Must be at most 100 chars
  - Case-sensitive
- [x] Add `LotChecker` interface to transaction domain:
  - `GetLotInfo(ctx, lotID string) (*LotInfo, error)` — returns (account_id, symbol, lot_type) or not found
- [x] In `transaction/service.go`, add lot_id handling during Create:
  - If lot_id provided: verify via LotChecker that it belongs to same account, same symbol, same type
  - If lot_id not provided or empty: auto-generate UUID-based lot_id (format: `LOT-<uuid-short>`)
  - Only for buy/sell transactions (deposit/withdrawal/dividend/interest/fee/tax don't get lots)
- [x] In `transaction/service.go`, add lot_id handling during Update:
  - lot_id is immutable once set (reject changes)
- [x] Write unit tests for lot_id validation in `service_test.go`
- [x] Update `transaction_web.go` to include lot_id field in create/edit forms
- [x] Update transaction form template (`templates/transaction/form.html`) to show lot_id field

**Verification:** Creating transactions with/without lot_id works; validation rejects mismatched lot_ids; auto-generation produces unique lot_ids; lot_id immutable on update.

---

### Task 4a: Lot grouping — group transactions into buy/sell lots [PRIORITY: HIGH]

**Corresponds to:** "Create a buy transaction with auto-generated lot", "Create a buy transaction with user-specified lot ID (existing lot)", "Create a sell transaction with auto-generated lot"

**Description:** Group transactions by lot_id into buy lots and sell lots.

- [x] Implement `GroupTransactionsIntoLots(transactions []Transaction) ([]LotGroup, []LotGroup)` in `calculator.go`:
  - Separate buy transactions from sell transactions
  - Group by lot_id (transactions with same lot_id → same lot)
  - Compute lot open_date (earliest transaction date in the lot)
  - Compute lot quantity (sum of quantities)
  - For buy lots: compute cost_basis (sum of net_cash, which is negative for buys — includes commission/fee) and avg_open_price (cost_basis / total quantity, will be negative reflecting cash outflow)
  - For sell lots: compute sell_proceeds (sum of net_cash, which is positive for sells — includes commission/fee) and avg_close_price (sell_proceeds / total quantity, will be positive reflecting cash inflow)
  - Sort lots chronologically by open_date
- [x] Write unit tests in `lot_grouping_test.go` (table-driven):
  - Single buy transaction → one buy lot
  - Multiple buys with same lot_id → one buy lot with summed quantity
  - Multiple buys with different lot_ids → separate buy lots
  - Mixed buy/sell → separate buy and sell lot lists
  - Empty transactions → empty lots
  - Chronological sorting of lots

**Verification:** All lot grouping tests pass; correctly groups by lot_id and computes lot aggregates. ✅ 10/10 tests pass.

---

### Task 4b: FIFO matching — match sell lots against buy lots [PRIORITY: HIGH]

**Corresponds to:** "FIFO lot matching on sell", "FIFO lot matching consuming entire buy lot"

**Description:** Match sell lots against buy lots using FIFO (first-in, first-out) ordering.

- [x] Implement `MatchSellLotsAgainstBuys(buyLots []LotGroup, sellLots []LotGroup) ([]LotConsumption, map[string]decimal.Decimal)` in `fifo_matching.go`:
  - Process sell lots in chronological order
  - For each sell lot, consume from oldest buy lot first
  - Track partial consumption (sell lot may span multiple buy lots)
  - Compute realized P&L per consumption: sell net_cash portion (positive) + buy net_cash portion (negative) for the consumed shares — net_cash is signed, so P&L = sell_inflow + buy_outflow; result is positive for gain, negative for loss
  - Return consumptions and remaining quantity per buy lot
  - Handle short positions (sell without matching buy → negative remaining)
- [x] Write unit tests in `fifo_matching_test.go` (table-driven):
  - Simple FIFO: one buy lot, one sell lot (partial consume)
  - Full consume: sell equals buy quantity
  - Multi-buy consume: one sell lot consumes from two buy lots
  - Multiple sell lots against one buy lot
  - Short position: sell exceeds total buy quantity
  - Multiple open-to-close cycles
  - Empty buy lots → all sells unmatched
  - Empty sell lots → no consumptions

**Verification:** All FIFO matching tests pass; correct P&L per consumption; handles partial and full consumption.

---

### Task 4c: Position computation — compute open/closed positions from matched lots [PRIORITY: HIGH]

**Corresponds to:** "View open positions for a single account", "View closed positions for an account", "Multiple open-to-close cycles create separate closed positions", "View open position with short quantity"

**Description:** Compute open and closed positions from the FIFO-matched lots, tracking quantity over time to detect open-to-close transitions.

- [x] Implement `ComputePositions(buyLots []LotGroup, sellLots []LotGroup, consumptions []LotConsumption) ([]Position, []Position)` in `position_computation.go`:
  - Walk through all lots chronologically (buys and sells interleaved)
  - Track running quantity per symbol
  - When quantity transitions from non-zero to zero → create closed position entry
  - When quantity is non-zero at end → create open position entry
  - For closed positions: quantity = total held during that cycle, close_date = date of final sell
  - For open positions: quantity = current remaining, cost_basis from unmatched buy lots (negative), avg_open_price = sum of buy net_cash / total buy quantity (negative), avg_close_price = sum of sell net_cash / total sell quantity (positive; nil if no sells yet)
  - For closed positions: avg_open_price = sum of buy net_cash / total buy quantity (negative), avg_close_price = sum of sell net_cash / total sell quantity (positive)
  - Handle multiple open-to-close cycles (buy 100, sell 100, buy 50, sell 50 → two closed positions)
  - Handle short positions (negative quantity)
- [x] Write unit tests in `position_computation_test.go` (table-driven):
  - Simple buy → one open position
  - Buy + partial sell → one open position with reduced quantity
  - Buy + full sell → one closed position
  - Buy, sell all, buy again → one closed + one open
  - Two full cycles → two closed positions
  - Short position → open position with negative quantity
  - Mixed symbols → separate positions per symbol

**Verification:** All position computation tests pass; correct open/closed state; multiple cycles handled. ✅ 10/10 tests pass.

---

### Task 4d: Cash position computation [PRIORITY: HIGH]

**Corresponds to:** "View cash position in open positions", "Deposit and withdrawal affect cash position", "Dividend affects cash position"

**Description:** Compute cash positions from deposit, withdrawal, dividend, interest, fee, tax, and trade net_cash transactions.

- [x] Implement `ComputeCashPositions(transactions []Transaction) ([]Position)` in `cash_position.go`:
  - Filter transactions affecting cash: deposit, withdrawal, dividend, interest, fee, tax, buy, sell
  - Group by currency (via symbol `$CASH-{currency}` or transaction currency)
  - Compute running net_cash sum per currency
  - Create cash position with quantity = balance, cost_basis = 0, realized_pnl = 0
  - Cash positions are always open (never closed)
  - No P&L on cash positions
- [x] Write unit tests in `cash_position_test.go` (table-driven):
  - Single deposit → positive cash balance
  - Deposit + withdrawal → reduced balance
  - Multiple currencies → separate cash positions
  - Dividend adds to cash without affecting share position
  - Negative balance (overdraft) → valid cash position
  - Empty transactions → no cash positions
  - Buy and sell affect cash (net_cash flows)
  - Fee and tax reduce cash
  - Open date is earliest transaction date

**Verification:** All cash position tests pass; correct balance computation; multi-currency handled. ✅ 10/10 tests pass.

---

### Task 4e: Calculator integration — combine all calculator sub-components [PRIORITY: HIGH]

**Corresponds to:** All calculator scenarios

**Description:** Wire the calculator sub-components into a single `CalculatePositions` function.

- [x] Implement `CalculatePositions(ctx context.Context, accountID int64, transactions []Transaction) (*CalculateResult, error)`:
  - Call `GroupTransactionsIntoLots`
  - Call `MatchSellLotsAgainstBuys`
  - Call `ComputePositions`
  - Call `ComputeCashPositions`
  - Merge results into `CalculateResult`
- [x] Write integration-style unit tests in `calculator_test.go`:
  - Full flow: deposits + buys + sells → open positions + cash position
  - Full flow: buy + sell all → closed position + cash position
  - Multiple symbols in same account → separate positions
  - End-to-end FIFO matching with P&L

**Verification:** Calculator produces correct results for complete transaction histories. ✅ 12/12 tests pass.

---

### Task 5: Position service, repository, recalculation trigger [PRIORITY: HIGH]

**Corresponds to:** "Auto-recalculate on transaction create", "Auto-recalculate on transaction update", "Auto-recalculate on transaction delete", "Manual recalculate for an account", "Manual recalculate for a portfolio", "Manual recalculate for all"

**Description:** Build the position service layer that orchestrates calculation + persistence, and wire it into the transaction lifecycle.

- [x] Create `internal/data/position_repo.go`:
  - `PositionRepository` with CRUD for positions, lots, lot_consumptions
  - `DeleteAllForAccount(ctx, accountID)` — clears all position data for an account (within DB transaction)
  - `CreatePosition(ctx, p *Position)`, `CreateLot(ctx, l *Lot)`, `CreateConsumption(ctx, c *LotConsumption)`
  - `GetOpenPositions(ctx, accountIDs []int64, limit, offset int)` — list open positions
  - `GetClosedPositions(ctx, accountIDs []int64, limit, offset int)` — list closed positions
  - `GetLotByLotID(ctx, lotID string)` — get lot + consumptions
  - sqlc queries in `position.sql`
- [x] Run `sqlc generate`
- [x] Create `internal/domain/position/service.go`:
  - `Service` struct with position repo, transaction repo, calculator, account checker, portfolio checker, lot checker
  - `RecalculateAccount(ctx, accountID)` — fetch all txns for account, run calculator, delete old positions/lots/consumptions, insert new (all in one DB transaction)
  - `RecalculatePortfolio(ctx, portfolioID)` — get accounts for portfolio, recalc each
  - `RecalculateAll(ctx)` — get all accounts, recalc each
  - `GetOpenPositions(ctx, filters, limit, offset)` — resolve account IDs from filters, query repo
  - `GetClosedPositions(ctx, filters, limit, offset)` — same pattern
  - `GetLotDetails(ctx, lotID)` — get lot + consumptions + source transactions
  - `GetLotInfo(ctx, lotID)` — for LotChecker interface
- [x] Create `internal/domain/position/service_test.go` with unit tests (hand-written mocks):
  - RecalculateAccount with known transactions → expected positions/lots/consumptions
  - RecalculateAccount with empty transactions → empty positions
  - RecalculatePortfolio → recalc all accounts in portfolio
  - RecalculateAll → recalc all accounts
  - GetOpenPositions filters by account IDs
  - GetClosedPositions filters by account IDs
  - GetLotDetails returns lot with consumptions and transactions
- [x] Wire recalculation into transaction service:
  - Add `PositionRecalculator` interface to transaction domain: `RecalculateAccount(ctx, accountID) error`
  - After Create/Update/Delete in transaction service, call `RecalculateAccount(accountID)`
  - Make it an optional dependency (nil-safe — if not set, skip recalc)
- [x] Update `router.go` to wire position service into transaction service

**Verification:** Creating/updating/deleting a transaction triggers position recalc; manual recalc works; unit tests pass with hand-written mocks.

---

### Task 6: Wire recalculation into import handlers [PRIORITY: HIGH]

**Corresponds to:** "Auto-recalculate on transaction create" (import-created transactions)

**Description:** Ensure imported transactions get lot_ids and trigger position recalculation.

- [x] Update `internal/domain/ibkrimport/service.go`:
  - During `ConfirmImport`, populate `lot_id` from broker `IbOrderID` for each buy/sell transaction before `BatchCreate` (groups partial fills of the same order into one lot)
  - Add `PositionRecalculator` dependency
  - After batch-creating transactions, collect affected account IDs
  - Call `RecalculateAccount` for each affected account
- [x] Update `internal/domain/trading212import/service.go`:
  - During `ConfirmImport`, auto-generate `lot_id` (`LOT-<ulid>`) for each buy/sell transaction (no order ID in broker data)
  - Add `PositionRecalculator` dependency
  - After batch-creating transactions, collect affected account IDs
  - Call `RecalculateAccount` for each affected account
- [x] Update `router.go` to pass position recalculator to import services
- [x] Update import service tests to include mock recalculator

**Verification:** Importing transactions triggers position recalc; existing import tests still pass. ✅ All tests pass.

---

### Task 7: Position API endpoints [PRIORITY: HIGH]

**Corresponds to:** "View open positions for a single account", "View open positions filtered by portfolio", "View open positions filtered by specific accounts", "View all open positions across all portfolios", "View open positions when account has no transactions", "View closed positions for an account", "View closed positions filtered by portfolio", "View closed positions when none exist", "Drill down from open position to lots", "Drill down from closed position to lots", "Drill down from buy lot to transactions", "Drill down from sell lot to transactions", "Manual recalculate for an account/portfolio/all"

**Description:** Build REST API endpoints for positions, lots, and recalculation.

- [x] Create `internal/api/handlers/position.go`:
  - `PositionHandler` struct with position service
  - `HandleListOpen` — GET /api/positions (open positions, filtered by account_id/portfolio_id/account_ids)
  - `HandleListClosed` — GET /api/positions/closed (closed positions, same filters)
  - `HandleGetLot` — GET /api/lots/{lot_id} (lot details with consumptions and transactions)
  - `HandleRecalculate` — POST /api/positions/recalculate (account_id/portfolio_id/all)
  - `RegisterRoutes(r *chi.Mux)`
- [x] Create `internal/api/handlers/position_test.go` with unit tests:
  - List open positions returns correct JSON
  - List closed positions returns correct JSON
  - List with account_id filter
  - List with portfolio_id filter (resolves to account IDs)
  - Get lot returns lot with consumptions
  - Recalculate returns updated positions
  - Error cases: invalid ID, lot not found, account not found
- [x] Update `router.go` to register position handler

**Verification:** All API endpoints return correct JSON; error responses follow `{"error": "...", "code": "..."}` format; handlers tested with mock services. ✅ 18/18 tests pass.

---

### Task 8: Position web UI — list pages, lot detail page [PRIORITY: MEDIUM]

**Corresponds to:** "Browse Positions in the Web UI", "View open positions for a single account", "View closed positions for an account", "Drill down from open position to lots", "Drill down from buy lot to transactions", "Drill down from sell lot to transactions"

**Description:** Build server-rendered web pages for browsing positions with drill-down.

- [x] Create `internal/api/handlers/position_web.go`:
  - `PositionWebHandler` with position service, account service, portfolio service, renderer
  - `HandleOpenPositions` — GET /positions (open positions list with filters and pagination)
  - `HandleClosedPositions` — GET /positions/closed (closed positions list with filters and pagination)
  - `HandleLotDetail` — GET /lots/{lot_id} (lot detail with consumptions and transactions)
  - `HandleRecalculate` — POST /positions/recalculate (manual recalc trigger with flash message)
  - `RegisterRoutes(r *chi.Mux)` — specific routes before catch-all
- [x] Create templates:
  - `templates/position/open.html` — table of open positions (symbol, quantity, avg open price, avg close price, cost basis, realized P&L, account name); links to lot detail; filter bar with account/portfolio dropdowns
  - `templates/position/closed.html` — table of closed positions (symbol, quantity, avg open price, avg close price, cost basis, realized P&L, open/close dates); links to lot detail; same filter bar
  - `templates/position/lot_detail.html` — lot details (type, quantity, cost basis or sell price, date); consumptions table (which buy lots consumed, quantities, P&L); transactions table (source transactions with links)
- [x] Add `PositionFilter` struct with `QueryParams()` and `PaginationQuery()` methods (following `TransactionFilter` pattern)
- [x] Update `templates/partials/nav.html` — change "Positions" link from `#` to `/positions`, remove `disabled` class
- [x] Create `internal/api/handlers/position_web_test.go` with basic handler tests
- [x] Update `router.go` to register position web handler

**Verification:** Web pages render correctly; navigation works; filters preserve across pagination; drill-down links work. ✅ All tests pass; go vet clean.

---

### Task 9: Market data storage and fetching [PRIORITY: MEDIUM]

**Corresponds to:** "View P&L in transaction currency and base currency", "FX rate unavailable for historical conversion", "View open positions includes market data when available"

**Description:** Add market data fetching and caching for both stock quotes and FX rates in the unified `market_data` table.

- [x] Create `internal/data/market_data_repo.go`:
  - `MarketDataRepository` with `GetLatest(ctx, symbol string)`, `GetBySourceAndDate(ctx, symbol, source, date string)`, `Upsert(ctx, *MarketData)`, `GetCurrentFxRate(ctx, baseCurrency, quoteCurrency string)`
  - sqlc queries in `market_data.sql` (queries parameterized by source; default source used when unspecified)
  - Run `sqlc generate`
- [x] Extend `internal/market/quote.go`:
  - `YahooFinanceFetcher.FetchQuote(ctx, symbol)` → returns `*MarketData` (stock)
  - `FetchFxRate(ctx, baseCurrency, quoteCurrency string)` → converts to "GBPUSD=X", fetches via go-yfinance, returns `*MarketData` with data_type='fx'
  - `FetchQuotesBatch(ctx, symbols []string)` → batch fetch via `go-yfinance` `multi` package (shared client)
  - Symbol conversion helper: `FxPairToYahooSymbol(base, quote string) string`
- [x] Create `internal/market/fx.go`:
  - `FxRate` struct (BaseCurrency, QuoteCurrency, Rate, FetchedAt)
  - `FxRateFetcher` interface with `FetchRate(ctx, baseCurrency, quoteCurrency string) (*FxRate, error)`
  - `FormatFxPair(base, quote string) string` for DB symbol construction
  - Wire `YahooFinanceFetcher` to implement `FxRateFetcher`
- [x] Create `internal/market/market_data_test.go` with unit tests:
  - FX pair to Yahoo symbol conversion
  - FormatFxPair
  - Market data struct serialization
- [x] Create `internal/data/market_data_repo_test.go` with basic repo tests
- [x] Update `router.go` to create market_data repo and pass to services

**Verification:** Stock quotes and FX rates can be fetched and stored; symbol conversion works; unit tests pass. ✅ 9/9 repo tests pass, 8/8 market tests pass.

---

### Task 10: Multi-currency P&L with FX conversion [PRIORITY: MEDIUM]

**Corresponds to:** "View P&L in transaction currency and base currency (open position)", "View P&L in transaction currency and base currency (closed position)", "FX rate unavailable for historical conversion", "Position with multiple currencies in same account"

**Description:** Integrate FX rates into position calculation for P&L in base currency.

- [x] Add to position domain model:
  - `RealizedPnlBase` field (P&L converted to portfolio base currency)
  - `FxRateUsed` field (the rate used for conversion, nullable)
  - `FxRateFallback` boolean (true if current spot rate was used as fallback)
- [x] Create `internal/domain/position/fx_converter.go`:
  - `FxRateProvider` interface: `GetRateForDate(ctx, baseCurrency, quoteCurrency string, date time.Time) (*FxRate, bool)`, `GetCurrentRate(ctx, baseCurrency, quoteCurrency string) (*FxRate, bool)`
  - `FxConverter` struct wraps MarketDataRepository + FxRateFetcher + logger
  - `GetRateForDate()` — checks DB for historical rate, falls back to fetching + caching, falls back to current spot
  - Fetches missing rates on-demand during recalculation
  - Stores fetched rates in `market_data` table
  - `ConvertPnlToBase()` — converts P&L from position currency to base currency
  - Pair string constructed only at storage layer via `market.FormatFxPair()`
- [x] Update position service to use FX converter during recalculation:
  - Added `PortfolioCurrencyChecker` interface for looking up portfolio base currency
  - Added `PortfolioCurrencyCheckerImpl` in data layer
  - Updated `AccountRef` to include `PortfolioCurrency`
  - Added new sqlc queries: `ListAllAccountsWithPortfolioCurrency`, `GetAccountsByPortfolioWithCurrency`
  - `RecalculateAccount` now converts P&L after calculation (not inside calculator — keeps it pure)
- [x] Update position API responses to include `realized_pnl_base`, `fx_rate_used`, `fx_rate_fallback`
- [x] Update position web templates to show P&L in both currencies with fallback indicator (⚠ icon)
- [x] Write unit tests for FX conversion logic in `fx_converter_test.go`

**Verification:** P&L shown in both transaction currency and base currency; fallback indicator shown when historical rate unavailable; unit tests cover happy path and fallback. ✅ All tests pass; go vet clean.

---

### Task 11: Market data integration for open positions [PRIORITY: MEDIUM]

**Corresponds to:** "View open positions includes market data when available", "Market data unavailable for a symbol", "Closed positions view does not fetch market data", "View open position includes market data when available"

**Description:** Fetch current market prices for open positions at read time (not stored).

- [x] Update position service to accept `market.MarketDataFetcher` dependency
- [x] In `HandleListOpen` (API handler), after fetching positions:
  - For each non-cash position, fetch current quote via `MarketDataFetcher`
  - Store fetched quotes in `market_data` table (latest, date=NULL) for caching
  - Compute `MarketValue = quantity × current_price`
  - Compute `UnrealizedPnL = market_value - total_cost`
  - Compute `UnrealizedPnlPct = unrealized_pnl / total_cost × 100`
  - If quote fetch fails, mark `MarketDataAvailable = false` (position still shown)
  - Cash positions: `MarketValue = balance`, no P&L, no market data fetch
- [x] In `HandleListClosed` (API handler), skip market data fetch entirely
- [x] Update position web templates to show market data columns (price, market value, unrealized P&L, P&L %) with "—" when unavailable
- [x] Update `router.go` to pass `YahooFinanceFetcher` to position handler

**Verification:** Open positions show current market data; closed positions don't; unavailable market data shows gracefully with "—". ✅ All tests pass; go vet clean.

---

### Task 12: Polish, edge cases, integration tests [PRIORITY: LOW]

**Corresponds to:** All remaining edge cases from spec

**Description:** Final polish, edge case handling, and integration tests.

- [x] Handle remaining edge cases:
  - Empty lot ID string treated as "no lot ID" (auto-generate) — verified in existing tests
  - Cash position with negative balance (overdraft) — already handled by calculator
  - Position with only non-trade transactions (no position created) — verified in calculator
  - Manual recalculate when already up-to-date (idempotent) — verified via unit + integration test
  - Lot ID case-sensitivity — verified via unit test (`TestGroupTransactionsIntoLots_LotIDCaseSensitive`)
  - Dividend on symbol with no open position (cash still affected) — verified via integration test
  - Sell exceeding total open quantity (creates short) — already handled
- [x] Write integration tests in `tests/integration/position_test.go`:
  - Full recalculation flow: create txns → positions computed → verify via API
  - FIFO matching with real SQL
  - Cash position tracking through deposits/withdrawals/trades
  - Position transitions (open → closed → open again)
  - Multiple open-to-close cycles
  - Recalculate endpoint + idempotency
  - Filter by account
  - Closed positions endpoint
  - Lot detail endpoint
  - Cascade delete (account → positions)
  - Dividend with no open position
- [x] Update `features/README.md` to mark f009 as "in-progress"
- [x] Run `go test ./...` and verify all tests pass
- [x] Run `go vet ./...` and fix any issues

**Bug fix during Task 12:** Fixed `toPosition()` and `toLot()` in `position_repo.go` to handle empty strings in nullable `sql.NullString` fields (`avg_close_price`, `realized_pnl_base`, `close_date`, `sell_price`). These fields were stored as empty strings `""` (not NULL) for open positions, causing `decimal.Parse("")` to fail with "invalid decimal: no coefficient". Added `&& p.Field.String != ""` checks and safe nil handling for `avg_open_price`.

**Verification:** All edge cases handled; integration tests pass; full test suite passes; no vet issues.

---

### Task 13: Position page enhancements — base currency, P&L%, summary panel [PRIORITY: MEDIUM]

**Corresponds to:** "View P&L in transaction currency and base currency (open position)", "View P&L in transaction currency and base currency (closed position)"

**Description:** Enhance position pages with base-currency columns, P&L percentage, FX rate display, and a summary panel.

- [x] Add `RealizedPnlPct *decimal.Decimal` to `Position` domain model
- [x] Compute `RealizedPnlPct` in `buildPosition` during recalculation (P&L / |CostBasis| × 100)
- [x] Update `EnrichWithMarketData` to accept `baseCurrency` parameter for open position base-currency conversion
- [x] Add `convertValuesToBase` helper to convert market value and unrealized P&L to base currency using current spot FX rate
- [x] Add `MarketValueBase` and `UnrealizedPnLBase` to `PositionWithMarket` struct
- [x] Add `BaseCurrency` field to web page data structs
- [x] Add `resolveBaseCurrency()` helper in web handler to determine portfolio currency from filter or first portfolio
- [x] Add `CostBasisBase *decimal.Decimal` to `PositionWithMarket` struct
- [x] Compute `CostBasisBase` in `EnrichWithMarketData` using same FX rate as market value
- [x] Add `positionSummary` struct for aggregated base-currency totals
- [x] Add `computePositionSummary()` to aggregate cost basis (base), market value (base), unrealized P&L (base), and P&L% across positions
- [x] Summary panel shows only base-currency values: Cost Basis (Base), Mkt Value (Base), Unrealized P&L (Base), Unrealized P&L %
- [x] Update `closed.html` template: dynamic "P&L ({BaseCurrency})" header, FX Rate column, P&L% column
- [x] Update `open.html` template: summary panel with 5 cards, base currency columns in table
- [x] Add CSS for `.position-summary`, `.summary-card`, `.summary-label`, `.summary-value`
- [x] Add `sign` template function for positive/negative/zero classification of decimal strings
- [x] Update API handler to pass empty baseCurrency (no portfolio context)
- [x] Update unit tests to pass baseCurrency parameter

**Verification:** Position pages show base-currency values with dynamic headers; P&L% computed correctly; summary panel aggregates totals; all tests pass.

## Phase R1: Sell-based closed positions (revision — spec commit 29500f8)

**Context:** Revision R1 changes the closed-position model: every sell lot with
FIFO-matched consumption produces a closed position row (matched shares only),
and open positions keep only the remaining shares with their FIFO cost basis.
Previously closed rows were whole-cycle (full buy quantity, full cost basis,
P&L only when fully closed), so partial sells were invisible on the closed page.

**No schema change needed** — `positions` has no unique constraint on
(account_id, symbol) and `Recalculate` is delete-all + insert, so any number of
closed rows per symbol is already representable.

**Behavior changes** (flagged in spec/NOTES):
1. Partial sells now produce closed rows (the point of the revision).
2. Open position quantity/cost basis shrink by the amount sold.
3. A full close spanning multiple sell lots now yields one row per sell lot
   (same total P&L, own dates/prices per sale).
4. Performance page Realized P&L increases for accounts with partial sells
   (it reads the closed-position summary; previously missing gains now count).
5. Short open positions show signed negative quantity (was `.Abs()` — market
   value looked like an asset, not a liability). A short round-trip (sell
   before covering buy) no longer yields a closed row — previously it yielded
   one shaped like a long trade (cover price shown as "open", short price as
   "close"); its P&L now lives only in the cash balance.
6. Latent bug fixed: direction changes (long→short→long) previously could
   produce multiple open rows for the same account+symbol; now exactly one
   open row per account+symbol when net qty ≠ 0.

### Task R1-1: Date-aware FIFO matching [PRIORITY: HIGH]

**Corresponds to:** edge case "A short sell (sell with no prior buy) matches no buy lots — no closed position row is created for it; the short quantity appears only as a negative open position"

**Description:** `MatchSellLotsAgainstBuys` currently ignores dates — a sell lot
before any buy still consumes a later buy lot (a short sale matched against
shares not yet held), which would create closed rows with impossible open
dates. Make matching date-aware.

- [x] In `fifo_matching.go`: a sell lot consumes only buy lots with
      `OpenDate <= sellLot.OpenDate` (same-day buys count as available —
      convention: buys precede sells within the same day)
- [x] Return signature unchanged: `([]LotConsumption, map[buyLotID]remainingQty)`
- [x] Unit tests: sell before any buy → zero consumptions, full buy remaining;
      same-day sell consumes same-day buy; well-ordered data unchanged (regression)
- [x] Note: persisted `lot_consumptions` become date-correct after next recalc

**Verification:** `go test ./internal/domain/position/ -run FIFO` passes; no
behavior change for chronological data.

### Task R1-2: Sell-lot closed rows + remaining-cost open positions [PRIORITY: HIGH]

**Corresponds to:** scenario "Partial sale creates a closed position and a reduced open position"; edge cases "Each sell creates a closed position row for the matched shares; the open position keeps the remaining shares…", "Multiple partial sells of the same cycle — each sell lot produces its own closed position row", "A buy lot partially consumed by sells — the lot keeps its remaining shares, which contribute to the open position's cost basis at the lot's cost", "Position transitions from closed to open when new buys are added after full close"

**Description:** Rewrite position computation to derive closed rows from FIFO
consumptions (one per sell lot) and open rows from the net remaining quantity
with FIFO cost. Replaces the cycle-walk model in `computePositionsForLots`.

- [ ] `ComputePositions` signature: add `consumptions []LotConsumption` and
      `remaining map[string]decimal.Decimal` parameters; `CalculatePositions`
      (calculator_integration.go) passes its existing matching results through
- [ ] Closed rows — group consumptions by `SellLotID` (one row per sell lot):
      - Quantity = Σ QuantityConsumed for that sell lot
      - CostBasis = Σ CostBasisConsumed (negative)
      - RealizedPnL = Σ consumption RealizedPnL; RealizedPnlPct = P&L / |CostBasis| × 100
      - AvgOpenPrice = |CostBasis| / Quantity (cost per matched share)
      - AvgClosePrice = sellLot.SellProceeds / |sellLot.Quantity| (net sale price per share —
        equals the per-consumption proportional price)
      - OpenDate = earliest OpenDate among the buy lots consumed by this sell lot
      - CloseDate = sell lot's date; Currency/AccountID from the sell lot; IsClosed = true
- [ ] Open row — one per symbol when net running quantity ≠ 0:
      - Quantity = signed net (positive long, negative short)
      - CostBasis (net > 0): walk buy lots chronologically, take
        `min(remaining[buyLotID], needed)` shares each, sum proportional cost;
        (net < 0): zero
      - AvgOpenPrice = |CostBasis| / Quantity (long); zero for short
      - OpenDate: net > 0 → oldest buy lot with remaining > 0; net < 0 → first
        lot after the last point where running quantity was zero
      - RealizedPnL = 0, no close date
- [ ] Remove: zero-crossing cycle closed rows, "P&L only if fully closed" gate,
      direction-change cycle splits (source of the multi-open-row bug)
- [ ] Cash positions: untouched
- [ ] Unit tests (position_computation_test.go):
      - VWRP scenario: buy 666 @ 105.00 (2024-07-01), sell 18 @ 144.60 (2025-07-15)
        → open (648, cost −68,040.00, avg 105.00) + closed (18, cost −1,890.00,
        P&L +712.80, +37.72%, avg open 105.00, avg close 144.60, dates 2024-07-01/2025-07-15)
      - Full close via two sells (100 bought, 40 + 60 sold) → two closed rows;
        total P&L equals the old single-row P&L
      - Sale spanning two buy lots → one row per sell lot, OpenDate = oldest buy consumed
      - Full close + reopen → new open row dated at the re-opening buy
      - Sell-only (short) → negative open row, zero closed rows
      - Update existing tests that pin the old model (partial sell → open only;
        full-cycle closed row shape)

**Verification:** `go test ./internal/domain/position/` passes; VWRP scenario
asserted end-to-end at the calculator level.

### Task R1-3: Close-date sort tie-break [PRIORITY: LOW]

**Corresponds to:** constraint "Closed positions view: … Sorted by symbol ascending, then open date ascending, then close date ascending"

- [ ] `sortPositions` in service.go: after symbol ASC, open_date ASC, add
      close_date ASC (closed rows; open rows have no close date — stable no-op)
- [ ] Unit test for the tie-break

**Verification:** `go test ./internal/domain/position/ -run Service` passes.

### Task R1-4: Integration tests + regression sweep [PRIORITY: HIGH]

**Corresponds to:** full-path verification per SPEC scenarios

- [ ] Integration test (tests/integration): account with the VWRP scenario →
      `RecalculateAccount` → assert `positions` table contains open row
      (648 / −68,040.00) and closed row (18 / +712.80 / 2024-07-01 → 2025-07-15);
      assert cash position unchanged (proceeds +2,602.80 flow to cash exactly
      as before — nothing double counted)
- [ ] Web integration: GET /positions/closed renders 200 and includes the
      partial-sell row; GET /positions/open shows the reduced open position
- [ ] Regression sweep: `go test ./...`; audit tests asserting the old model
      (grep `IsClosed`, `GetClosedPositions`, partial-sell fixtures in
      performance_test.go / equity_curve tests) and update expectations
- [ ] Note the expected performance-page shift: Realized P&L total rises for
      accounts with partial sells (intended, per spec)
- [ ] Delete `internal/domain/position/tmp_scenario_test.go` (temporary scenario file)

**Verification:** `go test ./...` green; VWRP numbers visible through API + web.

### Task R1-5: Documentation [PRIORITY: LOW]

- [ ] API.md: closed-position list behavior — one row per matched sell lot;
      open-date semantics (oldest consumed buy lot); partial sells included
- [ ] NOTES.md: check off R1 tasks; retro entry (fixes: direction-change
      multi-open-row bug, misleading long-shaped closed row for short
      round-trips; intended change: performance-page realized P&L)

**Verification:** docs match implemented behavior.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
|---|---|---|
| Position storage | Pre-computed and stored in DB | Spec says "positions store quantity, cost basis, realized P&L, and dates"; fast reads; recalc on write |
| Lot storage | Separate `lots` and `lot_consumptions` tables | Lots are independently queryable (drill-down); consumptions need many-to-many mapping |
| Lot ID generation | UUID-based auto-generation (`LOT-<uuid-short>`) | Unique, no coordination needed; user can override with meaningful IDs |
| Recalculation trigger | Synchronous after transaction CRUD | Spec says "positions are updated immediately"; simpler than async; acceptable for single-user workload |
| Market data table | Unified `market_data` table for stocks + FX + historical | Single table supports both data types; `source` column enables multiple providers; `date` column (NULL = latest) enables historical snapshots for future analytics |
| FX rates | On-demand fetch + DB cache in `market_data` | Spec says "rates fetched on-demand during recalculation"; avoids background jobs |
| Market data for positions | Fetched at read time, cached in `market_data` | Spec says computed at read time; caching avoids redundant fetches within same request |
| Multi-currency P&L | Stored `realized_pnl_base` + `fx_rate_used` + `fx_rate_fallback` | Allows showing both currencies; fallback indicator for transparency |
| Cash positions | Tracked as positions with `$CASH-{currency}` symbol | Consistent with existing convention; always open, no P&L |
| sqlc for queries | Yes, follows existing pattern | Type-safe queries; consistent with rest of codebase |
| Calculator split | 4 sub-tasks + integration | Each sub-task is independently testable; follows single-responsibility principle |
| P&L% computation | During recalculation in `buildPosition` | Pure computation, no I/O; stored with position for efficient display |
| Base currency for open positions | Current spot FX rate via `EnrichWithMarketData` | Consistent with unrealized P&L; computed at read time |
| Summary panel scope | All positions (not just current page) | Summary must reflect total portfolio state regardless of pagination; uses unbounded fetch (10k limit) |
| `sign` template function | Returns "positive"/"negative"/"" for decimal strings | Avoids complex conditional logic in templates; handles zero edge cases |
| Web rendering regression tests | Integration tests hit actual web pages and verify 200 OK | Catches template errors (undefined fields, nil pointers) before they reach production |
| **R1: Closed row granularity** | One row per FIFO-matched sell lot | Each sale keeps its own dates/prices/P&L; matches "separate cycles → separate rows"; FIFO data already gives this |
| **R1: Open position source of truth** | Net running quantity + FIFO remaining map | Net qty is economically correct (handles short round-trips); remaining map attributes the right cost to held shares |
| **R1: FIFO date-awareness** | Sell consumes only buys with OpenDate ≤ sell date | True FIFO availability; prevents short sales matching later buys (impossible open dates, wrong economics) |
| **R1: Closed-row sort** | symbol ASC, open_date ASC, close_date ASC | Deterministic order for rows sharing symbol+open date (multiple sell lots) |
| Router options pattern | Functional options (`RouterOption`) for configurable templates dir | Allows integration tests to specify templates path relative to package location |

## Risks

- **Recalculation performance:** If an account has thousands of transactions, recalc on every CRUD could be slow. Mitigation: single-user workload; SQLite is fast for this scale. Can optimize later if needed.
- **FX rate fetching:** Yahoo Finance may rate-limit or be unavailable. Mitigation: graceful fallback to cached/spot rate; log warnings.
- **Position state consistency:** If recalc fails mid-way, positions could be stale. Mitigation: use DB transactions for the upsert; recalc is idempotent (can be retried).
- **Complex calculator logic:** FIFO matching with multiple lots and currencies is error-prone. Mitigation: comprehensive unit tests with table-driven approach per sub-component.
- **Market data table growth:** Historical data could grow large. Mitigation: for now, only store latest + rates fetched during recalc. Future features can add cleanup/purging.
