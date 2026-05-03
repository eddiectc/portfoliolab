# Feature: Portfolio CRUD

## Description
Users need to manage investment portfolios — named collections that group assets, accounts, and transactions. Before any transaction recording, position tracking, or analytics can work, the system must support creating, viewing, updating, and deleting portfolios. Each portfolio has a unique name and a base currency for reporting.

## User Stories

### US-1: Create a Portfolio
As an investor, I want to create a named portfolio with a base currency so that I can start tracking my investments.

### US-2: List All Portfolios
As an investor, I want to see all my portfolios at a glance so that I can navigate between them.

### US-3: View Portfolio Details
As an investor, I want to view details of a specific portfolio so that I can confirm its name and currency settings.

### US-4: Update a Portfolio
As an investor, I want to rename a portfolio or change its currency so that I can keep my organization accurate.

### US-5: Delete a Portfolio
As an investor, I want to remove a portfolio I no longer need so that my workspace stays clean.

## Scenarios

### Scenario: Create a portfolio with name and currency
**Given** no portfolios exist
**When** I create a portfolio with name "Tech Growth" and currency "USD"
**Then** the portfolio is created with a unique ID
**And** the response status is 201 Created
**And** the portfolio name is "Tech Growth"
**And** the portfolio currency is "USD"
**And** created_at and updated_at timestamps are set

### Scenario: Create a portfolio with default currency
**Given** no portfolios exist
**When** I create a portfolio with name "Savings" and no currency specified
**Then** the portfolio is created with a unique ID
**And** the response status is 201 Created
**And** the portfolio currency defaults to "USD"

### Scenario: Create a portfolio with a non-USD currency
**Given** no portfolios exist
**When** I create a portfolio with name "European" and currency "EUR"
**Then** the portfolio is created with a unique ID
**And** the response status is 201 Created
**And** the portfolio currency is "EUR"

### Scenario: Reject creation with empty name
**Given** no portfolios exist
**When** I create a portfolio with an empty name and currency "USD"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_NAME"

### Scenario: Reject creation with name exceeding 100 characters
**Given** no portfolios exist
**When** I create a portfolio with a name of 101 characters and currency "USD"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_NAME"

### Scenario: Reject creation with invalid currency code
**Given** no portfolios exist
**When** I create a portfolio with name "Test" and currency "US"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_CURRENCY"

### Scenario: Reject creation with lowercase currency code
**Given** no portfolios exist
**When** I create a portfolio with name "Test" and currency "usd"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_CURRENCY"

### Scenario: Reject creation with duplicate name
**Given** a portfolio named "Main" exists
**When** I create a portfolio with name "Main" and currency "EUR"
**Then** the request is rejected with status 409 Conflict
**And** the error code is "PORTFOLIO_NAME_EXISTS"

### Scenario: List all portfolios
**Given** three portfolios exist: "Main" (USD), "Savings" (EUR), "Crypto" (USD)
**When** I request the list of portfolios
**Then** the response status is 200 OK
**And** the response contains 3 portfolios
**And** portfolios are ordered by created_at descending (newest first)

### Scenario: List portfolios with pagination limit
**Given** ten portfolios exist
**When** I request the list of portfolios with limit=3
**Then** the response status is 200 OK
**And** the response contains exactly 3 portfolios

### Scenario: List portfolios with offset
**Given** five portfolios exist
**When** I request the list of portfolios with limit=3 and offset=2
**Then** the response status is 200 OK
**And** the response contains exactly 3 portfolios
**And** the portfolios skip the first 2 entries

### Scenario: List portfolios with default pagination
**Given** ten portfolios exist
**When** I request the list of portfolios with no pagination parameters
**Then** the response status is 200 OK
**And** the response contains all 10 portfolios

### Scenario: List portfolios when none exist
**Given** no portfolios exist
**When** I request the list of portfolios
**Then** the response status is 200 OK
**And** the response contains an empty array

### Scenario: List portfolios with zero limit defaults to 50
**Given** ten portfolios exist
**When** I request the list of portfolios with limit=0
**Then** the response status is 200 OK
**And** the response contains all 10 portfolios (default limit applied)

### Scenario: List portfolios with negative offset defaults to 0
**Given** five portfolios exist
**When** I request the list of portfolios with limit=3 and offset=-1
**Then** the response status is 200 OK
**And** the response contains exactly 3 portfolios (default offset 0 applied)

### Scenario: Get a portfolio by ID
**Given** a portfolio with ID 5 and name "Growth Fund" exists
**When** I request the portfolio with ID 5
**Then** the response status is 200 OK
**And** the response contains the portfolio with name "Growth Fund"
**And** the response includes created_at and updated_at timestamps

### Scenario: Get a non-existent portfolio
**Given** no portfolio with ID 999 exists
**When** I request the portfolio with ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "PORTFOLIO_NOT_FOUND"

### Scenario: Update portfolio name
**Given** a portfolio with ID 3 and name "Old Name" exists
**When** I update the portfolio with ID 3 setting name to "New Name"
**Then** the response status is 200 OK
**And** the portfolio name is now "New Name"
**And** the updated_at timestamp is refreshed

### Scenario: Update portfolio currency
**Given** a portfolio with ID 3 and currency "USD" exists
**When** I update the portfolio with ID 3 setting currency to "GBP"
**Then** the response status is 200 OK
**And** the portfolio currency is now "GBP"
**And** the updated_at timestamp is refreshed

### Scenario: Update both name and currency
**Given** a portfolio with ID 3 and name "Old" and currency "USD" exists
**When** I update the portfolio with ID 3 setting name to "New" and currency to "EUR"
**Then** the response status is 200 OK
**And** the portfolio name is now "New"
**And** the portfolio currency is now "EUR"

### Scenario: Update with no changes
**Given** a portfolio with ID 3 and name "Stable" and currency "USD" exists
**When** I update the portfolio with ID 3 with no fields provided
**Then** the response status is 200 OK
**And** the portfolio name remains "Stable"
**And** the portfolio currency remains "USD"
**And** the updated_at timestamp is unchanged

### Scenario: Reject update with duplicate name
**Given** two portfolios exist: ID 1 named "Alpha" and ID 2 named "Beta"
**When** I update portfolio ID 2 setting name to "Alpha"
**Then** the request is rejected with status 409 Conflict
**And** the error code is "PORTFOLIO_NAME_EXISTS"

### Scenario: Reject update with invalid currency
**Given** a portfolio with ID 3 exists
**When** I update the portfolio with ID 3 setting currency to "XX"
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_CURRENCY"

### Scenario: Reject update with empty name
**Given** a portfolio with ID 3 exists
**When** I update the portfolio with ID 3 setting name to an empty string
**Then** the request is rejected with status 400 Bad Request
**And** the error code is "INVALID_NAME"

### Scenario: Reject update of non-existent portfolio
**Given** no portfolio with ID 999 exists
**When** I update the portfolio with ID 999 setting name to "Ghost"
**Then** the request is rejected with status 404 Not Found
**And** the error code is "PORTFOLIO_NOT_FOUND"

### Scenario: Delete a portfolio
**Given** a portfolio with ID 7 and name "Temporary" exists
**When** I delete the portfolio with ID 7
**Then** the response status is 204 No Content
**And** the portfolio no longer exists in the system

### Scenario: Delete a non-existent portfolio
**Given** no portfolio with ID 999 exists
**When** I delete the portfolio with ID 999
**Then** the request is rejected with status 404 Not Found
**And** the error code is "PORTFOLIO_NOT_FOUND"

## Edge Cases
- Creating a portfolio with an empty name
- Creating a portfolio with a name exceeding 100 characters
- Creating a portfolio with a duplicate name (case-sensitive comparison)
- Creating a portfolio with an invalid currency code (e.g., "US", "usd", "USDX")
- Getting/updating/deleting a non-existent portfolio ID
- Updating to a name that matches another existing portfolio
- Listing with limit=0 or negative offset (treated as defaults)
- Listing when no portfolios exist (returns empty array, not error)
- Submitting update with no fields to change (no-op, returns current state, timestamps unchanged)

## Constraints
- Name must be non-empty, max 100 characters, unique (case-sensitive)
- Currency must match `^[A-Z]{3}$` (3-letter ISO 4217, default `USD`)
- Error responses: `{"error": "message", "code": "ERROR_CODE"}`
- Error codes: `PORTFOLIO_NOT_FOUND`, `PORTFOLIO_NAME_EXISTS`, `INVALID_CURRENCY`, `INVALID_NAME`
- All CRUD operations complete in under 100ms for typical datasets (< 1000 portfolios)

## Non-Goals
- Portfolio accounts and transactions (future features)
- Portfolio P&L calculations and analytics
- Portfolio sharing or multi-user access
- Portfolio archival (soft delete)
- Bulk portfolio operations (import/export)
- Portfolio templates or cloning

## Dependencies
- None (top-level entity; accounts and transactions reference portfolios in future features)
