# Feature: Positions

## Description
Users need to see their current investment positions — what they own, at what cost, and how it's performing. A **position** is the current holding of a symbol in an account, computed from all buy/sell/cash transactions. Positions are automatically recalculated when transactions change, and can also be recalculated manually on demand.

Each position breaks down into **lots** — groups of related transactions. Both buy and sell transactions are grouped into lots. Users can optionally assign a lot ID when creating any transaction to group related buys or sells together (e.g., dollar-cost averaging buys into one lot, or grouping related sells). When no lot ID is specified, each buy or sell gets its own auto-generated lot. During recalculation, sell lots are matched against buy lots using **FIFO** (first-in, first-out) to determine realized P&L.

Positions include market data (current prices), P&L in both transaction currency and portfolio base currency, and support drill-down from position → lots → transactions. Cash is tracked as a position using the existing `$CASH-{currency}` symbol convention.

## User Stories

### US-1: View Open Positions
As an investor, I want to see my open positions (current holdings) with key metrics (quantity, cost basis, market value, P&L) so that I can assess what I own at a glance.

### US-1b: View Closed Positions
As an investor, I want to see my closed positions (fully sold) with realized P&L so that I can review my trade history and performance.

### US-2: Filter Positions by Portfolio and/or Account
As an investor with multiple portfolios and accounts, I want to filter positions by portfolio and/or account so that I can focus on specific parts of my investment activity.

### US-3: Drill Down into Lots
As an investor, I want to drill into a position to see its constituent lots so that I understand my cost basis and holding history for each acquisition batch.

### US-4: Drill Down into Lot Transactions
As an investor, I want to see all transactions that make up a lot so that I can trace the origin of my holdings.

### US-5: Assign Lot IDs on Transaction Creation
As an investor, I want to optionally group transactions into lots when creating them so that I can organize related buys or sells together.

### US-6: See P&L in Transaction Currency and Base Currency
As an investor with multi-currency accounts, I want to see P&L both in the transaction currency and converted to my portfolio's base currency so that I can evaluate performance consistently.

### US-7: Manually Recalculate Positions
As an investor, I want to manually trigger a position recalculation so that I can refresh computed values after bulk data changes.

### US-8: Browse Positions in the Web UI
As an investor, I want to browse my open and closed positions through a web interface with drill-down into lots and transactions, so that I can review my portfolio holdings visually.

## Scenarios

### Scenario: View open positions for a single account (open positions view)
**Given** account ID 3 has transactions: buy 10 AAPL @ $150, buy 5 AAPL @ $160, sell 3 AAPL @ $170
**When** I view positions for account ID 3
**Then** I see one open position for AAPL with quantity 12
**And** the average cost basis reflects the FIFO-weighted cost of remaining shares
**And** the position includes realized P&L (stored) and market value, unrealized P&L (computed from current market price at read time)
**And** the position shows the open date as the date of the first transaction
**And** the position includes P&L percentage

### Scenario: View open positions filtered by portfolio
**Given** portfolio ID 1 has accounts 3 and 5
**And** account 3 has an open AAPL position, account 5 has an open MSFT position
**When** I view open positions filtered by portfolio ID 1
**Then** I see open positions from both accounts
**And** each position indicates which account it belongs to
**And** closed positions from those accounts are not shown

### Scenario: View open positions filtered by specific accounts
**Given** accounts 3, 5, and 7 exist with various open and closed positions
**When** I view open positions filtered by accounts 3 and 5
**Then** I see open positions only from accounts 3 and 5
**And** no positions from account 7
**And** closed positions from accounts 3 and 5 are not shown

### Scenario: View all open positions across all portfolios
**Given** multiple portfolios with multiple accounts and open/closed positions exist
**When** I view open positions with no filter
**Then** I see all open positions from all accounts
**And** each position indicates its account and portfolio
**And** closed positions are not shown

### Scenario: View open positions when account has no transactions
**Given** account ID 9 exists but has no transactions
**When** I view open positions for account ID 9
**Then** I see an empty positions list

### Scenario: View closed positions for an account
**Given** account ID 3 previously held 100 TSLA shares that were fully sold in two sells (40 + 60)
**And** account ID 3 currently has open AAPL positions
**When** I view closed positions for account ID 3
**Then** the TSLA closed position is shown with quantity 100 (the total quantity that was held), open date, and close date
**And** the realized P&L reflects the total gain/loss from the trade
**And** the open AAPL position is NOT shown in this view

### Scenario: View closed positions filtered by portfolio
**Given** portfolio ID 1 has accounts 3 and 5
**And** account 3 has a closed TSLA position (100 shares), account 5 has a closed MSFT position (50 shares)
**When** I view closed positions filtered by portfolio ID 1
**Then** I see closed positions from both accounts showing their closed quantities
**And** each position indicates which account it belongs to
**And** open positions from those accounts are not shown

### Scenario: View closed positions when none exist
**Given** account ID 3 has only open positions (no fully sold positions)
**When** I view closed positions for account ID 3
**Then** I see an empty closed positions list

### Scenario: View cash position in open positions
**Given** account ID 3 has: deposit $10,000 USD, buy 10 AAPL for net cash -$1,500, sell 3 AAPL for net cash $510
**When** I view open positions for account ID 3
**Then** I see a cash position for `$CASH-USD` with balance $9,010
**And** the cash position reflects the running net cash from all transactions in that currency
**And** the cash position is always shown in the open positions view (cash is never "closed")

### Scenario: View open position with short quantity
**Given** account ID 3 has a short position: sell 100 TSLA @ $200 (short sale), buy 30 TSLA @ $180 (cover)
**When** I view open positions for account ID 3
**Then** I see a TSLA position with quantity -70 (negative)
**And** the P&L reflects the short position economics

### Scenario: Drill down from open position to lots
**Given** account ID 3 has an open AAPL position with 2 buy lots and 2 sell lots
**When** I drill into the AAPL position from the open positions view
**Then** I see all 4 lots (2 buy lots and 2 sell lots)
**And** each buy lot shows its quantity, cost basis, open date, and remaining shares after sell consumption
**And** each sell lot shows its quantity, sell price, date, and which buy lot(s) it consumed from (FIFO matching)
**And** I can further drill into each lot to see its transactions

### Scenario: Drill down from closed position to lots
**Given** account ID 3 has a closed TSLA position that had 2 buy lots and 3 sell lots (all buy lots fully consumed)
**When** I drill into the TSLA position from the closed positions view
**Then** I see all 5 lots (2 buy lots and 3 sell lots)
**And** each buy lot shows its original quantity, cost basis, open date, and close date
**And** each sell lot shows its quantity, sell price, date, and which buy lot(s) it consumed from
**And** I see the realized P&L for each lot
**And** I can further drill into each lot to see its transactions

### Scenario: Drill down from buy lot to transactions
**Given** buy lot "LOT-001" contains 2 buy transactions
**When** I view the details of lot "LOT-001"
**Then** I see both buy transactions with their dates, quantities, prices, and currencies
**And** the lot shows the average cost basis across its transactions

### Scenario: Drill down from sell lot to transactions
**Given** sell lot "LOT-002" contains 1 sell transaction that consumed from buy lots L1 and L2
**When** I view the details of lot "LOT-002"
**Then** I see the sell transaction with its date, quantity, price, and currency
**And** I see which buy lots it consumed from and how many shares from each (FIFO matching)
**And** the realized P&L is shown

### Scenario: Create a buy transaction with auto-generated lot
**Given** account ID 3 exists
**And** symbol "AAPL" exists
**When** I create a buy transaction for AAPL with no lot_id specified
**Then** the transaction is created successfully
**And** a new buy lot is automatically created with a system-generated lot_id
**And** the transaction is associated with that lot

### Scenario: Create a buy transaction with user-specified lot ID (existing lot)
**Given** account ID 3 exists
**And** symbol "AAPL" exists
**And** buy lot "LOT-001" already exists with 10 AAPL shares in account 3
**When** I create a buy transaction for AAPL with lot_id "LOT-001" and quantity 5
**Then** the transaction is created successfully
**And** it is grouped into the existing lot "LOT-001"
**And** the lot now has 15 shares with updated average cost basis

### Scenario: Create a buy transaction with user-specified lot ID (new lot)
**Given** account ID 3 exists
**And** symbol "AAPL" exists
**And** no lot "LOT-002" exists
**When** I create a buy transaction for AAPL with lot_id "LOT-002" and quantity 10
**Then** the transaction is created successfully
**And** a new buy lot "LOT-002" is created
**And** the transaction is associated with that lot

### Scenario: Create a sell transaction with auto-generated lot
**Given** account ID 3 exists
**And** symbol "AAPL" exists
**And** account ID 3 has an open AAPL position
**When** I create a sell transaction for AAPL with no lot_id specified
**Then** the transaction is created successfully
**And** a new sell lot is automatically created with a system-generated lot_id
**And** the transaction is associated with that sell lot
**And** during recalculation the sell lot is matched against buy lots via FIFO

### Scenario: Create a sell transaction with user-specified lot ID
**Given** account ID 3 exists
**And** symbol "AAPL" exists
**And** account ID 3 has an open AAPL position
**When** I create a sell transaction for AAPL with lot_id "LOT-SELL-001" and quantity 30
**Then** the transaction is created successfully
**And** a new sell lot "LOT-SELL-001" is created
**And** the transaction is associated with that sell lot
**And** during recalculation the sell lot is matched against buy lots via FIFO

### Scenario: Reject transaction with lot ID belonging to a different symbol
**Given** account ID 3 exists
**And** lot "LOT-001" exists for symbol "AAPL"
**When** I create a buy transaction for "MSFT" with lot_id "LOT-001"
**Then** the request is rejected with an error
**And** the error indicates the lot_id belongs to a different symbol

### Scenario: Reject transaction with lot ID belonging to a different account
**Given** account ID 3 and account ID 5 exist
**And** lot "LOT-001" exists for account ID 5
**When** I create a buy transaction for account ID 3 with lot_id "LOT-001"
**Then** the request is rejected with an error
**And** the error indicates the lot_id belongs to a different account

### Scenario: Reject sell transaction with lot ID belonging to a buy lot
**Given** account ID 3 exists
**And** buy lot "LOT-001" exists for symbol "AAPL" in account ID 3
**When** I create a sell transaction for AAPL with lot_id "LOT-001"
**Then** the request is rejected with an error
**And** the error indicates the lot_id belongs to a lot of a different type (buy lot cannot be used for sell transactions)

### Scenario: Multiple open-to-close cycles create separate closed positions
**Given** account ID 3 has the following AAPL transactions: buy 100 @ $150 (Jan), sell 100 @ $170 (Feb), buy 50 @ $180 (Mar), sell 50 @ $190 (Apr)
**When** I view closed positions for account ID 3
**Then** I see two separate closed AAPL positions
**And** the first closed position shows quantity 100 with its own open date, close date, and realized P&L
**And** the second closed position shows quantity 50 with its own open date, close date, and realized P&L

### Scenario: View P&L in transaction currency and base currency (open position)
**Given** portfolio ID 1 has base currency USD
**And** account ID 5 (under portfolio 1) has GBP transactions: buy 100 VOD.L @ £0.75, sell 50 VOD.L @ £0.80
**And** FX rate GBP/USD was 1.27 on the buy date and 1.28 on the sell date
**And** current FX rate GBP/USD is 1.29
**When** I view the open VOD.L position for account ID 5
**Then** the realized P&L in GBP is shown (£2.50)
**And** the realized P&L in USD uses the sell-date FX rate (£2.50 × 1.28 = $3.20)
**And** the unrealized P&L in GBP is shown
**And** the unrealized P&L in USD uses the current FX rate
**And** the market value in GBP is shown
**And** the market value in USD uses the current FX rate
**And** the cost basis in USD uses the current FX rate
**And** the P&L percentage is shown for both realized and unrealized P&L
**And** a summary panel shows aggregated base-currency totals: Cost Basis (USD), Mkt Value (USD), Unrealized P&L (USD), Unrealized P&L %

### Scenario: View P&L in transaction currency and base currency (closed position)
**Given** portfolio ID 1 has base currency USD
**And** account ID 5 has a fully closed GBP position (100 shares) with realized P&L of £2.50
**And** FX rate GBP/USD was 1.28 on the sell date
**When** I view the closed position
**Then** the position shows quantity 100 (the closed quantity)
**And** the realized P&L in GBP is shown (£2.50)
**And** the realized P&L in USD uses the sell-date FX rate (£2.50 × 1.28 = $3.20)
**And** the P&L percentage is shown (realized P&L as % of cost basis)
**And** the FX rate used for conversion is shown
**And** no unrealized P&L is shown (position is closed)

### Scenario: FX rate unavailable for historical conversion (open position)
**Given** a sell transaction occurred on a date where no FX rate is available for the currency pair
**And** the position is still open
**When** I view the position with P&L converted to base currency
**Then** the realized P&L in transaction currency is shown normally
**And** the realized P&L in base currency falls back to the current spot rate
**And** an indicator shows that the conversion used a fallback rate

### Scenario: Market data unavailable for a symbol (open position)
**Given** account ID 3 has an open position for symbol "XYZ"
**And** no market quote is available for "XYZ"
**When** I view open positions for account ID 3
**Then** the position is shown with quantity and cost basis
**And** the market value, unrealized P&L, and P&L percentage are shown as unavailable
**And** the position is not hidden or errored

### Scenario: Auto-recalculate on transaction create (open position)
**Given** account ID 3 has an open AAPL position
**When** I create a new buy transaction for AAPL in account ID 3
**Then** the AAPL position is automatically recalculated
**And** the updated position reflects the new transaction
**And** the cash position is also updated

### Scenario: Auto-recalculate on transaction update (open position)
**Given** account ID 3 has an open AAPL position
**And** a buy transaction for AAPL exists
**When** I update the quantity or price of that transaction
**Then** the AAPL position is automatically recalculated
**And** the updated position reflects the changed values
**And** the cash position is also updated

### Scenario: Auto-recalculate on transaction delete (closes a position)
**Given** account ID 3 has an open AAPL position with 10 shares
**And** the only remaining AAPL transaction is a buy of 10 shares
**When** I delete that buy transaction
**Then** the AAPL position is automatically recalculated
**And** the position quantity becomes 0 and is moved to closed positions
**And** the cash position is also updated

### Scenario: Manual recalculate for an account (open and closed)
**Given** account ID 3 has open and closed positions
**When** I trigger a manual recalculation for account ID 3
**Then** all open and closed positions for account ID 3 are recalculated
**And** the updated positions are returned

### Scenario: Manual recalculate for a portfolio (open and closed)
**Given** portfolio ID 1 has accounts 3 and 5 with open and closed positions
**When** I trigger a manual recalculation for portfolio ID 1
**Then** all open and closed positions for accounts 3 and 5 are recalculated
**And** the updated positions are returned

### Scenario: Manual recalculate for all (open and closed)
**Given** multiple portfolios with multiple accounts and open/closed positions exist
**When** I trigger a manual recalculation for all
**Then** all open and closed positions across all accounts are recalculated
**And** the updated positions are returned

### Scenario: FIFO lot matching on sell
**Given** account ID 3 has two AAPL buy lots: L1 (100 shares @ $150, opened Jan) and L2 (50 shares @ $160, opened Feb)
**When** I create a sell transaction for 30 AAPL @ $170
**Then** a new sell lot is created for the 30 shares
**And** the sell lot is matched against buy lot L1 first (FIFO)
**And** L1 has 70 shares remaining
**And** L2 is untouched (50 shares)
**And** the realized P&L uses L1's cost basis ($150)

### Scenario: FIFO lot matching consuming entire buy lot
**Given** account ID 3 has two AAPL buy lots: L1 (100 shares @ $150, opened Jan) and L2 (50 shares @ $160, opened Feb)
**When** I create a sell transaction for 120 AAPL @ $170
**Then** a new sell lot is created for the 120 shares
**And** the sell lot consumes all 100 shares from buy lot L1 (L1 fully consumed, close date set)
**And** the sell lot consumes 20 shares from buy lot L2 (30 remaining)
**And** the realized P&L is calculated proportionally from L1 and L2 cost bases

### Scenario: Deposit and withdrawal affect cash position
**Given** account ID 3 has a `$CASH-USD` position with balance $10,000
**When** I create a withdrawal transaction for $2,000
**Then** the cash position is recalculated
**And** the new cash balance is $8,000

### Scenario: Dividend affects cash position
**Given** account ID 3 has a `$CASH-USD` position with balance $10,000
**And** an AAPL position with 100 shares
**When** I create a dividend transaction for AAPL with quantity 1, price $3.00, net_cash $3.00
**Then** the AAPL position is unchanged (still 100 shares)
**And** the cash position increases by $3.00 to $10,003

### Scenario: Position with multiple currencies in same account
**Given** account ID 5 has USD transactions (AAPL) and GBP transactions (VOD.L)
**When** I view positions for account ID 5
**Then** I see separate positions for AAPL (USD) and VOD.L (GBP)
**And** each position shows P&L in its own currency
**And** each position shows P&L converted to the portfolio base currency

### Scenario: View open positions includes market data when available
**Given** account ID 3 has an open AAPL position and current market quote for AAPL is available
**When** I view open positions for account ID 3
**Then** the AAPL position includes the current market price (fetched at read time, not stored)
**And** the market value is computed as quantity × current price
**And** the unrealized P&L is computed as market value − total cost
**And** these values are not persisted — a subsequent view may show different values if the market price changed

### Scenario: Closed positions view does not fetch market data
**Given** account ID 3 has a closed AAPL position (was 100 shares, fully sold)
**When** I view closed positions for account ID 3
**Then** the position shows quantity 100 (the closed quantity), cost basis, and realized P&L
**And** no market price, market value, or unrealized P&L is shown or fetched

## Edge Cases
- Account with no transactions (empty open and closed positions lists)
- Position transitions from open to closed when current quantity reaches 0 (moves between views; closed quantity = total quantity held)
- Position transitions from closed to open when new buys are added after full close (reopens as a new open position)
- Multiple open-to-close cycles for the same symbol in an account create separate closed position entries (e.g., buy 100, sell 100, buy 50, sell 50 → two closed positions with quantities 100 and 50)
- Cash position is always open (never appears in closed positions view)
- Cash positions show no P&L (neither realized nor unrealized)
- Short positions — negative quantity from short selling
- Multi-currency account — positions in different currencies, each with own P&L currency and base currency conversion
- FX rate unavailable for historical date — falls back to current spot rate with indicator
- Market data unavailable for a symbol — position shown without market value/unrealized P&L
- Lot with partial sells — lot remains open with reduced quantity
- Cash position with negative balance (overdraft — withdrawals exceed deposits)
- Dividend transaction on a symbol where the user has no open position (cash still affected)
- Creating a buy with a lot_id that belongs to a different symbol (rejected)
- Creating a buy with a lot_id that belongs to a different account (rejected)
- Sell transaction that exceeds total open quantity (allowed — creates short position)
- Position with only non-trade transactions (e.g., dividends without any buy/sell — no position created for that symbol)
- Manual recalculate when positions are already up-to-date (idempotent — no change)
- Lot ID is case-sensitive (e.g., "LOT-001" ≠ "lot-001")
- Sell lot matched against buy lots with different currencies within the same account (each currency tracked separately)
- Empty lot ID string treated as "no lot ID" (auto-generate)

## Constraints
- **Lot ID on transactions:** a `lot_id` field is added to the transaction model; it is optional — if omitted on creation, a system-generated lot_id is assigned; once set, it is immutable; must match the account and symbol of the lot it references
- **Lot ID on imports:** import parsers (IBKR Flex XML, Trading 212 CSV) extract lot/grouping information from source data and populate `lot_id` on imported transactions; if the source has no natural grouping, each transaction gets its own auto-generated lot_id
- **Lot assignment:** every buy and sell transaction gets a lot (user-specified or auto-generated); deposit/withdrawal/dividend/interest/fee/tax transactions do not create lots — they affect the cash position directly
- **Lot type:** lots are typed as buy or sell based on the transaction type(s) they contain; a lot cannot mix buy and sell transactions
- **FIFO matching:** sell lots are matched against open buy lots in chronological order (oldest buy lot first); a single sell lot can consume from multiple buy lots
- **Position scope:** positions are computed per account per symbol; filtering by portfolio aggregates across all accounts in that portfolio
- **Open positions view:** positions with non-zero current quantity (including cash); sorted by symbol ascending, then open date ascending
- **Closed positions view:** positions that are fully closed (current quantity = 0, close date set, excluding cash); the displayed quantity is the total quantity held during that open-to-close cycle; each open-to-close cycle creates a separate closed position entry; sorted by symbol ascending, then open date ascending
- **Cash position:** tracked using a cash symbol per currency (e.g., `$CASH-USD`); cash balance is the running net cash from all transactions in that currency for the account; market value equals balance; no P&L on cash positions (neither realized nor unrealized); cash position is always open (never appears in closed positions view)
- **P&L in base currency:** realized P&L uses the FX rate on the transaction date; unrealized P&L uses the current spot FX rate; if historical rate unavailable, falls back to current spot with indicator
- **P&L percentage:** shown for both open and closed positions; computed as P&L / |CostBasis| × 100; displayed with 2 decimal places
- **Base currency on open positions:** market value and unrealized P&L are converted to portfolio base currency using current spot FX rate; shown alongside native currency values
- **Summary panel:** open positions page includes a summary panel with aggregated base-currency totals: Cost Basis (Base), Mkt Value (Base), Unrealized P&L (Base), Unrealized P&L %. Native currency values not shown in summary.
- **FX rate display:** closed positions show the FX rate used for base-currency conversion
- **Market data:** market price, market value, and unrealized P&L are computed when viewing positions using current market data; they are not persisted; positions are shown even when market data is unavailable (market value/unrealized P&L marked as unavailable)
- **Recalculation:** positions are updated immediately when transactions are created, updated, or deleted; also available as a manual trigger for a single account, a portfolio, or all accounts
- **Stored vs. computed data:** positions store quantity, cost basis, realized P&L, and dates; market price, market value, and unrealized P&L are computed at read time using current market data and are not persisted
- **Sorting:** positions sorted by symbol ascending, then open date ascending
- **Error responses:** `{"error": "message", "code": "ERROR_CODE"}`

## Non-Goals
- Tax lot reporting (FIFO/LIFO/specific identification selection)
- Historical position snapshots (point-in-time position state)
- Position alerts or notifications
- Background/asynchronous recalculation jobs
- Soft-delete or archival of positions/lots
- Position permissions or multi-user access
- Corporate action handling (splits, mergers) affecting lots
- Automatic FX rate fetching schedule (rates fetched on-demand during recalculation)
- Bulk import of lot IDs
- Position-level editing (positions are derived from transactions; edit transactions instead)

## Dependencies
- **f002_account-crud** (done) — positions are per account; filtering by portfolio requires account→portfolio relationship
- **f003_symbol-map** (done) — positions reference symbols
- **f004_transaction-crud** (done) — positions are computed from transactions; lot_id added to transactions
- **f005_transactions-crud-ui** (done) — web UI patterns followed for position pages; lot_id field added to transaction create/edit forms
- **f007_import-ibkr-flex-xml** (done) — parser updated to extract lot/grouping info from source data and populate `lot_id` on imported transactions
- **f008_import-trading212-csv** (done) — parser updated to extract lot/grouping info from source data and populate `lot_id` on imported transactions
- **Market data (quote fetching)** — already exists in `internal/market/`; used for current prices in positions
