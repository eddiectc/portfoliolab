# Feature: Account CRUD

## Description
Users need to manage investment accounts — named entities that belong to a portfolio and will later hold transactions. Before any transaction recording can work, the system must support creating, viewing, updating, and deleting accounts. Each account has a unique name and belongs to exactly one portfolio. This lets investors organize accounts under portfolios to track different investment strategies.

## User Stories

### US-1: Create an Account
As an investor, I want to create a named account under a portfolio so that I can start organizing my investments.

### US-2: List All Accounts
As an investor, I want to see all my accounts at a glance so that I can navigate between them.

### US-3: List Accounts by Portfolio
As an investor, I want to see all accounts belonging to a specific portfolio so that I can review the structure of that portfolio.

### US-4: View Account Details
As an investor, I want to view details of a specific account so that I can confirm its name and portfolio assignment.

### US-5: Update an Account
As an investor, I want to rename an account or move it to a different portfolio so that I can keep my organization accurate.

### US-6: Delete an Account
As an investor, I want to remove an account I no longer need so that my workspace stays clean. Deleting an account also removes all its transactions.

### US-7: Cascade Delete Portfolio
As an investor, when I delete a portfolio, all its accounts (and their transactions) are also removed. The system warns me before this happens.

## Scenarios

### Scenario: Create an account with name and portfolio
**Given** a portfolio with ID 1 and name "Main" exists
**When** I create an account with name "IBKR" under portfolio ID 1
**Then** the account is created with a unique ID
**And** the response status is 201 Created
**And** the account name is "IBKR"
**And** the account belongs to portfolio ID 1
**And** created_at and updated_at timestamps are set

### Scenario: Create an account in a different portfolio
**Given** a portfolio with ID 2 and name "European" exists
**When** I create an account with name "Degiro" under portfolio ID 2
**Then** the account is created with a unique ID
**And** the response status is 201 Created
**And** the account name is "Degiro"
**And** the account belongs to portfolio ID 2

### Scenario: Reject creation with empty name
**Given** a portfolio with ID 1 exists
**When** I create an account with an empty name under portfolio ID 1
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_NAME"

### Scenario: Reject creation with name exceeding 100 characters
**Given** a portfolio with ID 1 exists
**When** I create an account with a name of 101 characters under portfolio ID 1
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_NAME"

### Scenario: Reject creation with duplicate name
**Given** an account named "IBKR" exists under portfolio ID 1
**When** I create an account with name "IBKR" under portfolio ID 2
**Then** the request is rejected with status 409 Conflict
**And** the error code is "ACCOUNT_NAME_EXISTS"

### Scenario: Reject creation with duplicate name (same portfolio)
**Given** an account named "IBKR" exists under portfolio ID 1
**When** I create an account with name "IBKR" under portfolio ID 1
**Then** the request is rejected with status 409 Conflict
**And** the error code is "ACCOUNT_NAME_EXISTS"

### Scenario: Reject creation with non-existent portfolio
**Given** no portfolio with ID 999 exists
**When** I create an account with name "New Account" under portfolio ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "PORTFOLIO_NOT_FOUND"

### Scenario: List all accounts
**Given** three accounts exist: "IBKR" (portfolio 1), "Degiro" (portfolio 2), "Vanguard" (portfolio 1)
**When** I request the list of all accounts
**Then** the response status is 200 OK
**And** the response contains 3 accounts
**And** each account includes its portfolio ID
**And** accounts are ordered by created_at descending (newest first)

### Scenario: List accounts with pagination limit
**Given** ten accounts exist
**When** I request the list of accounts with limit=3
**Then** the response status is 200 OK
**And** the response contains exactly 3 accounts

### Scenario: List accounts with offset
**Given** five accounts exist
**When** I request the list of accounts with limit=3 and offset=2
**Then** the response status is 200 OK
**And** the response contains exactly 3 accounts
**And** the accounts skip the first 2 entries

### Scenario: List accounts with default pagination
**Given** ten accounts exist
**When** I request the list of accounts with no pagination parameters
**Then** the response status is 200 OK
**And** the response contains all 10 accounts

### Scenario: List accounts when none exist
**Given** no accounts exist
**When** I request the list of accounts
**Then** the response status is 200 OK
**And** the response contains an empty array

### Scenario: List accounts with zero limit defaults to 50
**Given** ten accounts exist
**When** I request the list of accounts with limit=0
**Then** the response status is 200 OK
**And** the response contains all 10 accounts (default limit applied)

### Scenario: List accounts with negative offset defaults to 0
**Given** five accounts exist
**When** I request the list of accounts with limit=3 and offset=-1
**Then** the response status is 200 OK
**And** the response contains exactly 3 accounts (default offset 0 applied)

### Scenario: List accounts by portfolio
**Given** three accounts exist: "IBKR" (portfolio 1), "Degiro" (portfolio 2), "Vanguard" (portfolio 1)
**When** I request accounts filtered by portfolio ID 1
**Then** the response status is 200 OK
**And** the response contains exactly 2 accounts
**And** both accounts belong to portfolio ID 1

### Scenario: List accounts by non-existent portfolio
**Given** no portfolio with ID 999 exists
**When** I request accounts filtered by portfolio ID 999
**Then** the response status is 200 OK
**And** the response contains an empty array

### Scenario: Get an account by ID
**Given** an account with ID 5, name "IBKR", and portfolio ID 1 exists
**When** I request the account with ID 5
**Then** the response status is 200 OK
**And** the response contains the account with name "IBKR"
**And** the response includes portfolio ID 1
**And** the response includes created_at and updated_at timestamps

### Scenario: Get a non-existent account
**Given** no account with ID 999 exists
**When** I request the account with ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "ACCOUNT_NOT_FOUND"

### Scenario: Update account name
**Given** an account with ID 3 and name "Old Broker" exists
**When** I update the account with ID 3 setting name to "New Broker"
**Then** the response status is 200 OK
**And** the account name is now "New Broker"
**And** the updated_at timestamp is refreshed

### Scenario: Update account portfolio
**Given** an account with ID 3 and portfolio ID 1 exists
**And** a portfolio with ID 2 exists
**When** I update the account with ID 3 setting portfolio ID to 2
**Then** the response status is 200 OK
**And** the account belongs to portfolio ID 2
**And** the updated_at timestamp is refreshed

### Scenario: Update both name and portfolio
**Given** an account with ID 3, name "Old", and portfolio ID 1 exists
**And** a portfolio with ID 2 exists
**When** I update the account with ID 3 setting name to "New" and portfolio ID to 2
**Then** the response status is 200 OK
**And** the account name is now "New"
**And** the account belongs to portfolio ID 2

### Scenario: Update with no changes
**Given** an account with ID 3 and name "Stable" exists
**When** I update the account with ID 3 with no fields provided
**Then** the response status is 200 OK
**And** the account name remains "Stable"
**And** the updated_at timestamp is unchanged

### Scenario: Reject update with duplicate name
**Given** two accounts exist: ID 1 named "Alpha" and ID 2 named "Beta"
**When** I update account ID 2 setting name to "Alpha"
**Then** the request is rejected with status 409 Conflict
**And** the error code is "ACCOUNT_NAME_EXISTS"

### Scenario: Reject update with empty name
**Given** an account with ID 3 exists
**When** I update the account with ID 3 setting name to an empty string
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_NAME"

### Scenario: Reject update with non-existent portfolio
**Given** an account with ID 3 exists
**And** no portfolio with ID 999 exists
**When** I update the account with ID 3 setting portfolio ID to 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "PORTFOLIO_NOT_FOUND"

### Scenario: Reject creation with whitespace-only name
**Given** a portfolio with ID 1 exists
**When** I create an account with name "   " (whitespace only) under portfolio ID 1
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_NAME"

### Scenario: Reject update of non-existent account
**Given** no account with ID 999 exists
**When** I update the account with ID 999 setting name to "Ghost"
**Then** the request is rejected with status 404 Not Found
**And** the error code is "ACCOUNT_NOT_FOUND"

### Scenario: Delete an account
**Given** an account with ID 7 and name "Temporary" exists
**When** I delete the account with ID 7
**Then** the response status is 204 No Content
**And** the account no longer exists in the system

### Scenario: Delete an account cascades to its transactions
**Given** an account with ID 7 exists
**And** three transactions belong to account ID 7
**When** I delete the account with ID 7
**Then** the response status is 204 No Content
**And** the account no longer exists in the system
**And** all three transactions are also removed

> **Note:** This is a forward-looking constraint. Transactions are not yet implemented (future feature). The test for this scenario may be stubbed until the transaction feature exists.

### Scenario: Delete a non-existent account
**Given** no account with ID 999 exists
**When** I delete the account with ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "ACCOUNT_NOT_FOUND"

### Scenario: Delete a portfolio cascades to its accounts
**Given** a portfolio with ID 3 exists
**And** two accounts belong to portfolio ID 3
**When** I delete the portfolio with ID 3
**Then** the response status is 204 No Content
**And** the portfolio no longer exists in the system
**And** both accounts are also removed from the system

## Edge Cases
- Creating an account with an empty name
- Creating an account with a name exceeding 100 characters
- Creating an account with a duplicate name (globally unique, case-sensitive comparison)
- Creating an account under a non-existent portfolio
- Getting/updating/deleting a non-existent account ID
- Updating to a name that matches another existing account
- Updating portfolio to a non-existent portfolio ID
- Listing with limit=0 or negative offset (treated as defaults)
- Listing when no accounts exist (returns empty array, not error)
- Listing by a non-existent portfolio ID (returns empty array, not error)
- Submitting update with no fields to change (no-op, returns current state, timestamps unchanged)
- Deleting an account that has transactions (cascades, all removed; forward-looking — transactions not yet implemented)
- Creating/updating an account with a whitespace-only name (treated as empty → INVALID_NAME)
- Creating/updating an account with leading/trailing whitespace in the name (trimmed before validation/storage)
- Deleting a portfolio that has accounts (cascades, all removed)

## Constraints
- Name must be non-empty (after trimming leading/trailing whitespace), max 100 characters, globally unique (case-sensitive)
- Names are trimmed of leading and trailing whitespace before validation and storage
- Each account belongs to exactly one portfolio (required on creation)
- Error responses: `{"error": "message", "code": "ERROR_CODE"}`
- Error codes: `ACCOUNT_NOT_FOUND`, `ACCOUNT_NAME_EXISTS`, `INVALID_NAME`, `PORTFOLIO_NOT_FOUND`
- Deleting an account hard-deletes it and all its transactions
- Deleting a portfolio hard-deletes it, all its accounts, and all their transactions
- Cascade warnings are a UI concern: the UI should query dependent resources before delete and show a confirmation dialog. The API returns standard responses (204 No Content) without special warning payloads
- All CRUD operations complete in under 100ms for typical datasets (< 1000 accounts)

## Non-Goals
- Account balances or cash tracking
- Account-specific import formats (CSV/XML parsers)
- Account types/categories (e.g., brokerage vs. retirement)
- Account currencies (accounts inherit portfolio currency)
- Account soft-delete or archival
- Bulk account operations (import/export)
- Account permissions or sharing

## Dependencies
- **f001_portfolio-crud** (done) — accounts reference portfolios by ID
