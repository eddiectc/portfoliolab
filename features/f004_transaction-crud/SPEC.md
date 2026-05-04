# Feature: Transaction CRUD

## Description
Users need to record and manage investment transactions — individual events that change the holdings or cash balance of an account. Transactions are the source of truth for all portfolio analytics (positions, P&L, cash balance, drawdowns). Each transaction belongs to exactly one account (and transitively to one portfolio through the account). This feature supports creating, viewing, updating, and deleting transactions with full filtering and pagination.

## User Stories

### US-1: Create a Transaction
As an investor, I want to record a transaction (buy, sell, deposit, etc.) for an account so that my portfolio reflects my actual activity.

### US-2: List All Transactions
As an investor, I want to see all my transactions with filtering and pagination so that I can review my activity.

### US-3: View Transaction Details
As an investor, I want to view the full details of a specific transaction so that I can verify its accuracy.

### US-4: Update a Transaction
As an investor, I want to correct a transaction's details so that my records stay accurate.

### US-5: Delete a Transaction
As an investor, I want to remove an erroneous transaction so that it no longer affects my portfolio.

## Scenarios

### Scenario: Create a buy transaction
**Given** an account with ID 3 exists under portfolio ID 1
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3, date "2025-01-15", type "buy", symbol "AAPL", quantity 10, price 150.00, currency "USD", netCash -1500.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction belongs to account ID 3
**And** created_at and updated_at timestamps are set

### Scenario: Create a sell transaction
**Given** an account with ID 3 exists under portfolio ID 1
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3, date "2025-03-20", type "sell", symbol "AAPL", quantity -5, price 175.00, currency "USD", netCash 870.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction type is "sell"
**And** the transaction quantity is -5

### Scenario: Create a deposit transaction using cash symbol
**Given** an account with ID 3 exists under portfolio ID 1
**When** I create a transaction with account ID 3, date "2025-01-01", type "deposit", symbol "$CASH-USD", quantity 10000.00, price 1, currency "USD", netCash 10000.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction symbol is "$CASH-USD"
**And** the special cash symbol "$CASH-USD" is auto-created in the symbol map if it does not already exist

### Scenario: Create a withdrawal transaction
**Given** an account with ID 3 exists under portfolio ID 1
**When** I create a transaction with account ID 3, date "2025-06-15", type "withdrawal", symbol "$CASH-USD", quantity -2000.00, price 1, currency "USD", netCash -2000.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction type is "withdrawal"

### Scenario: Create a dividend transaction
**Given** an account with ID 3 exists under portfolio ID 1
**And** a symbol "MSFT" exists in the symbol map
**When** I create a transaction with account ID 3, date "2025-03-14", type "dividend", symbol "MSFT", quantity 1, price 3.00, currency "USD", netCash 3.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction type is "dividend"

### Scenario: Create a fee transaction
**Given** an account with ID 3 exists under portfolio ID 1
**When** I create a transaction with account ID 3, date "2025-01-15", type "fee", symbol "$CASH-USD", quantity -4.95, price 1, currency "USD", netCash -4.95
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction type is "fee"

### Scenario: Create a transaction with external reference
**Given** an account with ID 3 exists under portfolio ID 1
**And** a symbol "VOO" exists in the symbol map
**When** I create a transaction with account ID 3, date "2025-02-10", type "buy", symbol "VOO", quantity 2, price 250.00, currency "USD", netCash -505.00, external_system "IBKR", external_reference "TXN-12345"
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the external_system is "IBKR"
**And** the external_reference is "TXN-12345"

### Scenario: Create a transaction with negative quantity (short position)
**Given** an account with ID 3 exists under portfolio ID 1
**And** a symbol "TSLA" exists in the symbol map
**When** I create a transaction with account ID 3, date "2025-01-15", type "buy", symbol "TSLA", quantity -100, price 200.00, currency "USD", netCash 20000.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction quantity is -100

### Scenario: Create a transaction in a non-USD currency
**Given** an account with ID 5 exists under portfolio ID 2
**And** a symbol "VOD.L" exists in the symbol map
**When** I create a transaction with account ID 5, date "2025-01-15", type "buy", symbol "VOD.L", quantity 500, price 0.75, currency "GBP", netCash -375.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction currency is "GBP"

### Scenario: Reject creation with non-existent account
**Given** no account with ID 999 exists
**When** I create a transaction with account ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "ACCOUNT_NOT_FOUND"

### Scenario: Reject creation with non-existent symbol
**Given** an account with ID 3 exists
**And** no symbol "XYZZY" exists in the symbol map
**When** I create a transaction with account ID 3 and symbol "XYZZY"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "SYMBOL_NOT_FOUND"

### Scenario: Reject creation with empty symbol
**Given** an account with ID 3 exists
**When** I create a transaction with account ID 3 and an empty symbol
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_SYMBOL"

### Scenario: Reject creation with zero price
**Given** an account with ID 3 exists
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3 and price 0
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_PRICE"

### Scenario: Reject creation with negative price
**Given** an account with ID 3 exists
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3 and price -10
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_PRICE"

### Scenario: Reject creation with invalid currency code
**Given** an account with ID 3 exists
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3 and currency "US"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_CURRENCY"

### Scenario: Reject creation with lowercase currency code
**Given** an account with ID 3 exists
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3 and currency "usd"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_CURRENCY"

### Scenario: Reject creation with invalid transaction type
**Given** an account with ID 3 exists
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3 and type "exchange"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_TYPE"

### Scenario: Reject creation with zero quantity
**Given** an account with ID 3 exists
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3 and quantity 0
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_QUANTITY"

### Scenario: Reject creation with whitespace-only symbol
**Given** an account with ID 3 exists
**When** I create a transaction with account ID 3 and symbol "   " (whitespace only)
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_SYMBOL"

### Scenario: Reject creation with invalid date format
**Given** an account with ID 3 exists
**And** a symbol "AAPL" exists in the symbol map
**When** I create a transaction with account ID 3 and date "not-a-date"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_DATE"

### Scenario: Create an interest transaction
**Given** an account with ID 3 exists under portfolio ID 1
**When** I create a transaction with account ID 3, date "2025-06-30", type "interest", symbol "$CASH-USD", quantity 25.50, price 1, currency "USD", netCash 25.50
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction type is "interest"

### Scenario: Create a tax transaction
**Given** an account with ID 3 exists under portfolio ID 1
**When** I create a transaction with account ID 3, date "2025-12-31", type "tax", symbol "$CASH-USD", quantity -150.00, price 1, currency "USD", netCash -150.00
**Then** the transaction is created with a unique ID
**And** the response status is 201 Created
**And** the transaction type is "tax"

### Scenario: List transactions filtered by start date only
**Given** five transactions exist: 2 dated in January 2025, 3 dated in March 2025
**When** I request transactions with date from "2025-03-01" (no end date)
**Then** the response status is 200 OK
**And** the response contains exactly 3 transactions
**And** all transactions have dates on or after "2025-03-01"

### Scenario: List transactions filtered by end date only
**Given** five transactions exist: 2 dated in January 2025, 3 dated in March 2025
**When** I request transactions with date until "2025-02-28" (no start date)
**Then** the response status is 200 OK
**And** the response contains exactly 2 transactions
**And** all transactions have dates on or before "2025-02-28"

### Scenario: List all transactions
**Given** five transactions exist across multiple accounts
**When** I request the list of all transactions
**Then** the response status is 200 OK
**And** the response contains 5 transactions
**And** transactions are ordered by date descending, then symbol ascending, then type ascending, then ID ascending

### Scenario: List transactions filtered by account
**Given** five transactions exist: 3 under account ID 3, 2 under account ID 5
**When** I request transactions filtered by account ID 3
**Then** the response status is 200 OK
**And** the response contains exactly 3 transactions
**And** all transactions belong to account ID 3

### Scenario: List transactions filtered by symbol
**Given** five transactions exist: 3 with symbol "AAPL", 2 with symbol "MSFT"
**When** I request transactions filtered by symbol "AAPL"
**Then** the response status is 200 OK
**And** the response contains exactly 3 transactions
**And** all transactions have symbol "AAPL"

### Scenario: List transactions filtered by date range
**Given** five transactions exist: 2 dated in January 2025, 2 dated in March 2025, 1 dated in June 2025
**When** I request transactions with date from "2025-03-01" to "2025-03-31"
**Then** the response status is 200 OK
**And** the response contains exactly 2 transactions
**And** all transactions have dates within the range (inclusive)

### Scenario: List transactions filtered by type
**Given** five transactions exist: 2 buys, 2 sells, 1 dividend
**When** I request transactions filtered by type "buy"
**Then** the response status is 200 OK
**And** the response contains exactly 2 transactions
**And** all transactions have type "buy"

### Scenario: List transactions with combined filters
**Given** ten transactions exist: 3 under account ID 3 with symbol "AAPL", 2 under account ID 3 with symbol "MSFT", 5 under account ID 5
**When** I request transactions filtered by account ID 3 and symbol "AAPL"
**Then** the response status is 200 OK
**And** the response contains exactly 3 transactions
**And** all transactions belong to account ID 3 and have symbol "AAPL"

### Scenario: List transactions with pagination limit
**Given** ten transactions exist
**When** I request the list of transactions with limit=3
**Then** the response status is 200 OK
**And** the response contains exactly 3 transactions

### Scenario: List transactions with offset
**Given** five transactions exist
**When** I request the list of transactions with limit=3 and offset=2
**Then** the response status is 200 OK
**And** the response contains exactly 3 transactions
**And** the transactions skip the first 2 entries

### Scenario: List transactions with default pagination
**Given** ten transactions exist
**When** I request the list of transactions with no pagination parameters
**Then** the response status is 200 OK
**And** the response contains all 10 transactions

### Scenario: List transactions when none exist
**Given** no transactions exist
**When** I request the list of transactions
**Then** the response status is 200 OK
**And** the response contains an empty array

### Scenario: List transactions with zero limit defaults to 50
**Given** ten transactions exist
**When** I request the list of transactions with limit=0
**Then** the response status is 200 OK
**And** the response contains all 10 transactions (limit=0 defaults to 50, which covers all items)

### Scenario: List transactions with negative offset defaults to 0
**Given** five transactions exist
**When** I request the list of transactions with limit=3 and offset=-1
**Then** the response status is 200 OK
**And** the response contains exactly 3 transactions (default offset 0 applied)

### Scenario: Get a transaction by ID
**Given** a transaction with ID 7, type "buy", symbol "AAPL", quantity 10, price 150.00 exists
**When** I request the transaction with ID 7
**Then** the response status is 200 OK
**And** the response contains the transaction with type "buy" and symbol "AAPL"
**And** the response includes account ID
**And** the response includes created_at and updated_at timestamps

### Scenario: Get a non-existent transaction
**Given** no transaction with ID 999 exists
**When** I request the transaction with ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "TRANSACTION_NOT_FOUND"

### Scenario: Update transaction date
**Given** a transaction with ID 5 and date "2025-01-15" exists
**When** I update the transaction with ID 5 setting date to "2025-01-16"
**Then** the response status is 200 OK
**And** the transaction date is now "2025-01-16"
**And** the updated_at timestamp is refreshed

### Scenario: Update transaction quantity and price
**Given** a transaction with ID 5, quantity 10, and price 150.00 exists
**When** I update the transaction with ID 5 setting quantity to 12 and price to 148.50
**Then** the response status is 200 OK
**And** the transaction quantity is now 12
**And** the transaction price is now 148.50
**And** the updated_at timestamp is refreshed

### Scenario: Update transaction netCash
**Given** a transaction with ID 5 and netCash -1500.00 exists
**When** I update the transaction with ID 5 setting netCash to -1510.00
**Then** the response status is 200 OK
**And** the transaction netCash is now -1510.00
**And** the updated_at timestamp is refreshed

### Scenario: Update transaction symbol
**Given** a transaction with ID 5 and symbol "AAPL" exists
**And** a symbol "AAPL.WS" exists in the symbol map
**When** I update the transaction with ID 5 setting symbol to "AAPL.WS"
**Then** the response status is 200 OK
**And** the transaction symbol is now "AAPL.WS"
**And** the updated_at timestamp is refreshed

### Scenario: Update transaction type
**Given** a transaction with ID 5 and type "buy" exists
**When** I update the transaction with ID 5 setting type to "sell"
**Then** the response status is 200 OK
**And** the transaction type is now "sell"
**And** the updated_at timestamp is refreshed

### Scenario: Update transaction with no changes
**Given** a transaction with ID 5 exists
**When** I update the transaction with ID 5 with no fields provided
**Then** the response status is 200 OK
**And** all transaction fields remain unchanged
**And** the updated_at timestamp is unchanged

### Scenario: Reject update of account_id (immutable)
**Given** a transaction with ID 5 and account ID 3 exists
**And** an account with ID 7 exists
**When** I update the transaction with ID 5 setting account ID to 7
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "IMMUTABLE_FIELD"

### Scenario: Reject update with non-existent symbol
**Given** a transaction with ID 5 exists
**And** no symbol "ZZZZZ" exists in the symbol map
**When** I update the transaction with ID 5 setting symbol to "ZZZZZ"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "SYMBOL_NOT_FOUND"

### Scenario: Reject update with invalid price
**Given** a transaction with ID 5 exists
**When** I update the transaction with ID 5 setting price to 0
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_PRICE"

### Scenario: Reject update with invalid currency
**Given** a transaction with ID 5 exists
**When** I update the transaction with ID 5 setting currency to "XX"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_CURRENCY"

### Scenario: Reject update with invalid type
**Given** a transaction with ID 5 exists
**When** I update the transaction with ID 5 setting type to "split"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_TYPE"

### Scenario: Reject update with zero quantity
**Given** a transaction with ID 5 exists
**When** I update the transaction with ID 5 setting quantity to 0
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_QUANTITY"

### Scenario: Reject update with invalid date
**Given** a transaction with ID 5 exists
**When** I update the transaction with ID 5 setting date to "not-a-date"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_DATE"

### Scenario: Reject update of non-existent transaction
**Given** no transaction with ID 999 exists
**When** I update the transaction with ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "TRANSACTION_NOT_FOUND"

### Scenario: Delete a transaction
**Given** a transaction with ID 7 exists
**When** I delete the transaction with ID 7
**Then** the response status is 204 No Content
**And** the transaction no longer exists in the system

### Scenario: Delete a non-existent transaction
**Given** no transaction with ID 999 exists
**When** I delete the transaction with ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "TRANSACTION_NOT_FOUND"

### Scenario: Delete an account cascades to its transactions
**Given** an account with ID 7 exists
**And** three transactions belong to account ID 7
**When** I delete the account with ID 7
**Then** the response status is 204 No Content
**And** the account no longer exists in the system
**And** all three transactions are also removed

### Scenario: Delete a portfolio cascades to its accounts and transactions
**Given** a portfolio with ID 3 exists
**And** two accounts belong to portfolio ID 3
**And** five transactions belong to those accounts
**When** I delete the portfolio with ID 3
**Then** the response status is 204 No Content
**And** the portfolio no longer exists in the system
**And** both accounts are also removed
**And** all five transactions are also removed

## Edge Cases
- Creating a transaction with a non-existent account
- Creating a transaction with a non-existent symbol (not in symbol map)
- Creating a transaction with an empty symbol
- Creating a transaction with a whitespace-only symbol (trimmed → treated as empty → INVALID_SYMBOL)
- Creating a transaction with zero quantity (rejected — INVALID_QUANTITY)
- Creating a transaction with zero or negative price
- Creating a transaction with an invalid date format (e.g., "not-a-date", "2025-13-01")
- Creating a transaction with invalid currency code (e.g., "US", "usd", "USDX")
- Creating a transaction with an invalid type (not one of the eight allowed types)
- Creating a transaction with negative quantity (short selling — allowed)
- Creating a cash transaction with the `$CASH-{currency}` symbol (auto-created if missing)
- Creating a transaction with `$CASH-{currency}` symbol where the currency in the symbol doesn't match the transaction's currency field (rejected — INVALID_CURRENCY)
- Getting/updating/deleting a non-existent transaction ID
- Attempting to change the account_id of an existing transaction (immutable — rejected)
- Updating with no fields to change (no-op, returns current state, timestamps unchanged)
- Listing with limit=0 (defaults to 50) or negative offset (defaults to 0)
- Listing when no transactions exist (returns empty array, not error)
- Listing with multiple combined filters (account + symbol, account + type, etc.)
- Listing with date range filters (start date only, end date only, or both)
- Deleting an account that has transactions (cascades, all removed)
- Deleting a portfolio cascades through accounts to all their transactions
- Creating/updating a transaction with leading/trailing whitespace in the symbol (trimmed before validation/storage)

## Constraints
- **account_id**: required on creation, immutable after creation
- **date**: date only (no time component), must be a valid calendar date, any date allowed (past, present, or future)
- **type**: one of `buy`, `sell`, `deposit`, `withdrawal`, `dividend`, `interest`, `fee`, `tax`
- **symbol**: required, trimmed of leading/trailing whitespace before validation, must be non-empty after trimming, must exist in the symbol map system; `$CASH-{currency}` symbols are auto-created if missing
- **quantity**: signed decimal, must be non-zero (positive for long direction, negative for short direction); position for a symbol = SUM(quantity) across all transactions
- **price**: positive decimal only (must be > 0)
- **currency**: ISO 4217, 3-letter uppercase (`^[A-Z]{3}$`); when using `$CASH-{currency}` symbol, the currency portion of the symbol must match the transaction's currency field
- **netCash**: signed decimal, user-provided, no validation against quantity × price
- **external_system**: optional text field, max 100 characters
- **external_reference**: optional text field, max 100 characters
- **Cash convention**: cash movements use symbol `$CASH-{currency}` (e.g., `$CASH-USD`, `$CASH-GBP`), quantity = cash value, price = 1, netCash = quantity
- **Sorting**: default order is date descending, then symbol ascending, then type ascending, then ID ascending
- **Pagination**: follows existing pattern — `limit` and `offset` query params, default limit 50, limit=0 or limit<0 defaults to 50, negative offset defaults to 0
- **Error responses**: `{"error": "message", "code": "ERROR_CODE"}`
- **Error codes**: `TRANSACTION_NOT_FOUND`, `ACCOUNT_NOT_FOUND`, `SYMBOL_NOT_FOUND`, `INVALID_SYMBOL`, `INVALID_PRICE`, `INVALID_CURRENCY`, `INVALID_TYPE`, `INVALID_QUANTITY`, `INVALID_DATE`, `IMMUTABLE_FIELD`
- All CRUD operations complete in under 100ms for typical datasets (< 10,000 transactions)

## Non-Goals
- Position tracking / P&L calculations (future feature)
- Cash balance calculations (future feature, though data is prepared for it)
- Market data fetching for current prices (future feature)
- Bulk import from broker CSV/XML files (future feature)
- Transaction splitting across lots (future feature)
- Soft-delete or archival of transactions
- Transaction permissions or multi-user access
- Transaction approval workflows
- Automatic netCash calculation from quantity × price
- Corporate action handling (splits, mergers, etc.)
- Tax lot tracking (FIFO, LIFO, specific identification)

## Dependencies

- **f002_account-crud** (done) — transactions belong to accounts; account delete cascades to transactions; portfolio delete cascades through accounts to transactions
- **f003_symbol-map** (spec) — transaction symbols must exist in the symbol map; `$CASH-{currency}` symbols are auto-created as a system convention
