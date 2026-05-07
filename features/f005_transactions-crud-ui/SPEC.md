# Feature: Transactions CRUD UI

## Description

Users need a server-rendered web interface to create, view, edit, and delete transactions. The transaction UI is a core feature with its own top-level navigation link and dedicated pages. The form presents all fields at once, and the currency field auto-fills from the symbol's associated details. The list page supports filtering and pagination (by account, symbol, type, date range).

### Prerequisite: netCash is Required

Before the web UI is built, `netCash` is changed from optional to **required** at the domain level. The domain model and validation are updated so that every transaction must have a net cash value. Existing transactions with a null `netCash` are migrated: `netCash` is set to `quantity × price`. This prerequisite affects the existing REST API (f004) as well as the new web UI.

## User Stories

### US-0: Enforce Net Cash as Required
As an investor, I want the system to require a net cash value for every transaction (both via the web form and the API) so that my cash flow records are always complete.

### US-1: Create a Transaction via Web Form
As an investor, I want to fill out a form to record a new transaction (buy, sell, deposit, etc.) so that my portfolio reflects my actual activity.

### US-2: List All Transactions
As an investor, I want to see a list of all my transactions with filtering and pagination so that I can review my activity.

### US-3: View Transaction Details
As an investor, I want to view the full details of a specific transaction so that I can verify its accuracy.

### US-4: Update a Transaction via Web Form
As an investor, I want to correct a transaction's details through a web form so that my records stay accurate.

### US-5: Delete a Transaction
As an investor, I want to remove an erroneous transaction from the web interface so that it no longer affects my portfolio.

## Scenarios

### Scenario: Navigate to the transactions list from the nav bar
**Given** the web application is running
**When** I click the "Transactions" link in the top-level navigation
**Then** I am taken to the transactions list page
**And** the "Transactions" nav link is active (not disabled)

### Scenario: View an empty transactions list
**Given** no transactions exist in the system
**When** I visit the transactions list page
**Then** I see an empty state message indicating no transactions yet
**And** I see a link or button to create a new transaction

### Scenario: View the transactions list with data
**Given** multiple transactions exist across different accounts
**When** I visit the transactions list page
**Then** I see a table of transactions
**And** each row shows: date, type, symbol, quantity, price, netCash, and account name
**And** transactions are ordered by date descending

### Scenario: Filter transactions by account
**Given** transactions exist under multiple accounts
**When** I select a specific account from the account filter
**Then** only transactions belonging to that account are shown

### Scenario: Filter transactions by symbol
**Given** transactions exist with different symbols (e.g., AAPL, MSFT, $CASH-USD)
**When** I enter a symbol in the symbol filter
**Then** only transactions matching that symbol are shown

### Scenario: Filter transactions by type
**Given** transactions of different types exist (buy, sell, deposit, dividend)
**When** I select a type from the type filter
**Then** only transactions of that type are shown

### Scenario: Filter transactions by date range
**Given** transactions exist across different dates
**When** I set a date range (from and/or to) in the date filter
**Then** only transactions within that date range are shown

### Scenario: Combine multiple filters
**Given** transactions exist with various accounts, symbols, types, and dates
**When** I apply filters for both account and symbol
**Then** only transactions matching all selected filters are shown

### Scenario: Filters persist across pagination
**Given** transactions exist with multiple symbols
**When** I filter by symbol "AAPL" and navigate to the next page
**Then** the symbol filter remains applied on the next page
**And** only "AAPL" transactions are shown

### Scenario: Reset filters shows all transactions
**Given** I have applied one or more filters (account, symbol, type, or date range)
**When** I clear all active filters
**Then** all transactions are shown again, unfiltered

### Scenario: Paginate through transactions
**Given** more transactions exist than fit on one page
**When** I visit the transactions list page
**Then** I see a limited number of transactions per page
**And** I can navigate to the next page of results

### Scenario: Navigate between pages
**Given** enough transactions exist to span three pages
**When** I am on page 2 of the transactions list
**Then** I can navigate to page 1 (previous) and page 3 (next)
**And** the current page is indicated in the pagination controls

### Scenario: Navigate to the new transaction form
**Given** I am on the transactions list page
**When** I click the "Add Transaction" button
**Then** I am taken to a new transaction form page
**And** the form shows fields for: account, date, type, symbol, quantity, price, currency, netCash, external system, and external reference

### Scenario: Navigate to the new transaction form from an empty list
**Given** no transactions exist
**When** I click the "Add Transaction" link in the empty state message
**Then** I am taken to the new transaction form page

### Scenario: Create a buy transaction
**Given** at least one account and one symbol mapping exist
**When** I fill out the form with account, date, type "buy", symbol "AAPL", quantity 10, price 150.00, currency "USD", and netCash -1500.00
**And** I submit the form
**Then** the transaction is created successfully
**And** a confirmation message is displayed at the top of the page
**And** I am redirected to the transactions list page
**And** the new transaction appears in the list

### Scenario: Create a sell transaction with negative quantity
**Given** at least one account and one symbol mapping exist
**When** I fill out the form with type "sell", symbol "AAPL", quantity -5, price 175.00, and submit
**Then** the transaction is created with the negative quantity
**And** a confirmation message is displayed at the top of the page

### Scenario: Create a deposit transaction with cash symbol
**Given** at least one account exists
**When** I fill out the form with type "deposit", symbol "$CASH-USD", quantity 10000, price 1, currency "USD", netCash 10000
**And** I submit the form
**Then** the transaction is created successfully
**And** a confirmation message is displayed at the top of the page
**And** the cash symbol is auto-created if it does not already exist

### Scenario: Currency auto-fills from symbol selection
**Given** a symbol mapping exists for "AAPL" with currency "USD"
**When** I select or enter "AAPL" in the symbol field
**Then** the currency field is automatically populated with "USD"

### Scenario: Currency auto-fills from a non-USD symbol
**Given** a symbol mapping exists for "VOD.L" with currency "GBP"
**When** I select or enter "VOD.L" in the symbol field
**Then** the currency field is automatically populated with "GBP"

### Scenario: User overrides the auto-filled currency
**Given** a symbol mapping exists for "AAPL" with currency "USD"
**When** I select "AAPL" in the symbol field and the currency auto-fills to "USD"
**And** I manually change the currency field to "EUR"
**And** I submit the form with all other valid fields
**Then** the transaction is created with currency "EUR"
**And** the user's manual value takes precedence over the auto-filled value

### Scenario: Reject creating a transaction with missing required fields
**Given** I am on the new transaction form
**When** I submit the form without filling in the account field
**Then** I see a validation error message indicating the account is required
**And** I remain on the form page
**And** the fields I already filled in are preserved

### Scenario: Reject creating a transaction with zero quantity
**Given** I am on the new transaction form
**When** I fill out all fields but set quantity to 0
**And** I submit the form
**Then** I see a validation error message indicating quantity cannot be zero
**And** I remain on the form page
**And** the fields I already filled in are preserved

### Scenario: Reject creating a transaction with zero or negative price
**Given** I am on the new transaction form
**When** I fill out all fields but set price to 0
**And** I submit the form
**Then** I see a validation error message indicating price must be positive
**And** I remain on the form page

### Scenario: Reject creating a transaction with invalid date format
**Given** I am on the new transaction form
**When** I fill out all fields but set date to "not-a-date"
**And** I submit the form
**Then** I see a validation error message indicating the date format is invalid
**And** I remain on the form page

### Scenario: Reject creating a transaction with invalid currency code
**Given** I am on the new transaction form
**When** I fill out all fields but set currency to "US" (2-letter code)
**And** I submit the form
**Then** I see a validation error message indicating the currency code is invalid
**And** I remain on the form page

### Scenario: Reject creating a transaction with lowercase currency code
**Given** I am on the new transaction form
**When** I fill out all fields but set currency to "usd" (lowercase)
**And** I submit the form
**Then** I see a validation error message indicating the currency code is invalid
**And** I remain on the form page

### Scenario: Reject creating a transaction with invalid type
**Given** I am on the new transaction form
**When** I fill out all fields but select an invalid transaction type
**And** I submit the form
**Then** I see a validation error message indicating the type is invalid
**And** I remain on the form page

### Scenario: Reject creating a transaction with non-existent symbol
**Given** I am on the new transaction form
**When** I fill out all fields but enter a symbol that does not exist in the symbol map
**And** I submit the form
**Then** I see a validation error message indicating the symbol was not found
**And** I remain on the form page

### Scenario: Reject creating a transaction with missing netCash (web form)
**Given** I am on the new transaction form
**When** I fill out all fields but leave netCash empty
**And** I submit the form
**Then** I see a validation error message indicating netCash is required
**And** I remain on the form page
**And** the fields I already filled in are preserved

### Scenario: Reject creating a transaction with missing netCash (API)
**Given** the transaction API is available
**When** I create a transaction with all required fields but omit netCash
**Then** the request is rejected with an error indicating netCash is required

### Scenario: View transaction details
**Given** a transaction with ID 7 exists (type "buy", symbol "AAPL", quantity 10, price 150.00)
**When** I click on the transaction from the list page
**Then** I see a detail page showing all transaction fields: ID, account, date, type, symbol, quantity, price, currency, netCash, external system, external reference, created at, and updated at

### Scenario: Navigate to the edit form from the list
**Given** I am on the transactions list page
**When** I click the "Edit" action on a transaction row
**Then** I am taken to the edit form
**And** all fields are pre-populated with the transaction's current values

### Scenario: Navigate to the edit form from the detail page
**Given** I am viewing a transaction's detail page
**When** I click the "Edit" button
**Then** I am taken to the edit form
**And** all fields are pre-populated with the transaction's current values

### Scenario: Update a transaction's date
**Given** a transaction with date "2025-01-15" exists
**When** I navigate to the edit form and change the date to "2025-01-16"
**And** I submit the form
**Then** the transaction date is updated to "2025-01-16"
**And** a confirmation message is displayed at the top of the page
**And** I am redirected to the transaction detail or list page

### Scenario: Update a transaction's quantity and price
**Given** a transaction with quantity 10 and price 150.00 exists
**When** I navigate to the edit form and change quantity to 12 and price to 148.50
**And** I submit the form
**Then** the transaction quantity is updated to 12
**And** the transaction price is updated to 148.50
**And** a confirmation message is displayed at the top of the page

### Scenario: Reject updating a transaction with invalid data
**Given** I am on the edit form for an existing transaction
**When** I change the price to 0 and submit
**Then** I see a validation error message
**And** I remain on the edit form
**And** the transaction is unchanged

### Scenario: Reject updating a transaction to clear netCash
**Given** a transaction with netCash -1500.00 exists
**When** I update the transaction setting netCash to null/empty
**Then** the update is rejected with an error indicating netCash is required
**And** the transaction is unchanged

### Scenario: Delete a transaction from the list page
**Given** I am on the transactions list page and a transaction is displayed
**When** I click the "Delete" action on the transaction row
**And** I confirm the deletion
**Then** the transaction is removed
**And** a confirmation message is displayed at the top of the page
**And** I am redirected to the transactions list page

### Scenario: Delete a transaction from the detail page
**Given** I am viewing a transaction's detail page
**When** I click the "Delete" button
**And** I confirm the deletion
**Then** the transaction is removed
**And** a confirmation message is displayed at the top of the page
**And** I am redirected to the transactions list page

### Scenario: View a non-existent transaction
**Given** no transaction with ID 999 exists
**When** I navigate to the detail page for transaction 999
**Then** I see a not found (404) page

### Scenario: Edit a non-existent transaction
**Given** no transaction with ID 999 exists
**When** I navigate to the edit form for transaction 999
**Then** I see a not found (404) page

## Edge Cases
- Creating a transaction with a cash symbol (`$CASH-{currency}`) that doesn't exist yet — the symbol should be auto-created
- Creating a transaction with optional fields (external system, external reference) left empty — these should be accepted as null
- Creating a transaction with netCash left empty — rejected at both the web form and API level with a validation error
- Updating a transaction to clear netCash — rejected at both the web form and API level
- Existing transactions with null netCash after the migration — netCash is set to `quantity × price`
- The currency portion of a `$CASH-{currency}` symbol must match the transaction's currency field (e.g., `$CASH-USD` with currency "EUR" is rejected)
- Updating a transaction with no fields changed — should be a no-op (no error, no unnecessary timestamp refresh)
- The currency field is auto-filled from the symbol but the user manually overrides it — the user's value takes precedence
- Filtering with no results — shows an empty state message, not an error
- Deleting a transaction that has already been deleted by another means (e.g., cascade from account deletion) — shows not found
- Form submission with whitespace-only symbol — should be rejected as invalid
- Form submission with leading/trailing whitespace in symbol — should be trimmed before validation
- Date field accepts YYYY-MM-DD format; other formats should be rejected
- Quantity can be negative (short selling) — this is valid and should be accepted

## Constraints
- **Form layout**: all fields on a single page (no multi-step wizard)
- **Currency auto-fill**: when a symbol is selected/entered, the currency field is populated from the symbol's associated currency; the user may override it. This is a client-side JavaScript UX enhancement (not validation — all validation is server-side).
- **Account selection**: the account dropdown lists all existing accounts; the user cannot create a new account from the transaction form
- **Symbol selection**: the symbol field draws from the existing symbol map; symbols not in the map are rejected (except `$CASH-{currency}` which is auto-created)
- **Cash symbol currency matching**: the currency portion of a `$CASH-{currency}` symbol must match the transaction's currency field (e.g., `$CASH-USD` requires currency "USD")
- **Transaction types**: limited to the eight allowed types: buy, sell, deposit, withdrawal, dividend, interest, fee, tax
- **Date format**: YYYY-MM-DD (date only, no time component)
- **List sorting**: default order is date descending, then symbol ascending, then type ascending, then ID ascending
- **Pagination**: consistent with existing pages
- **Error display**: validation errors are shown inline on the form, preserving all previously entered values
- **Success feedback**: after create/update/delete, a confirmation message is displayed
- **Delete confirmation**: deletion requires explicit user confirmation
- **netCash**: required field on both create and edit forms; required and non-zero at the API level as well (changed from optional in f004). Enforced by the application validator and by a NOT NULL constraint in the database schema.
- **External fields**: `external_system` and `external_reference` are optional, max 100 characters each
- **Migration**: existing transactions with null netCash are migrated with `netCash = quantity × price`

## Non-Goals
- Bulk import from broker files (future feature)
- Position tracking / P&L display on the transaction pages (future feature)
- Market price lookups or current price display
- Client-side form validation (all validation is server-side; client-side JavaScript is permitted for UX enhancements like currency auto-fill)
- Transaction permissions or multi-user access
- Soft-delete or archival of transactions
- Transaction splitting across lots
- Corporate action handling (splits, mergers, etc.)
- Tax lot tracking
- Export transactions to CSV/Excel
- Automatic net cash calculation from quantity × price (user must provide it explicitly)

## Dependencies
- **f004_transaction-crud** (done, modified by this feature) — domain model, service layer, repository, and REST API for transactions; this feature changes netCash from optional to required and updates existing tests accordingly
- **f002_account-crud** (done) — accounts are referenced by transactions; account list needed for the account dropdown
- **f003_symbol-map** (done) — symbol map is referenced by transactions; symbol list and currency details needed for the symbol field and currency auto-fill
- **Existing web infrastructure** — renderer, templates, flash messages, and navigation pattern already established by f001/f002/f003
