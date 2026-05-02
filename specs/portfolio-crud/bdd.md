---
feature: portfolio-crud
---
# BDD: Portfolio CRUD

## Scenario Group: Create a Portfolio (US-1, FR-1)

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

## Scenario Group: List Portfolios (US-2, FR-2)

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

## Scenario Group: View Portfolio Details (US-3, FR-3)

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

## Scenario Group: Update a Portfolio (US-4, FR-4)

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

## Scenario Group: Delete a Portfolio (US-5, FR-5)

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

## Coverage Checklist

| Requirement | Happy Path | Invalid Input | Not Found | Conflict | Boundary |
|-------------|-----------|---------------|-----------|----------|----------|
| FR-1 Create | Scenario 1, 2, 3 | Scenario 4, 5, 6, 7 | — | Scenario 8 | — |
| FR-2 List | Scenario 9, 10, 11, 12 | — | — | — | Scenario 13, 14, 15 |
| FR-3 Get | Scenario 16 | — | Scenario 17 | — | — |
| FR-4 Update | Scenario 18, 19, 20, 21 | Scenario 24, 25 | Scenario 26 | Scenario 23 | — |
| FR-5 Delete | Scenario 27 | — | Scenario 28 | — | — |
