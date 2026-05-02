---
feature: portfolio-crud
created: 2026-05-02
status: draft
---
# Feature: Portfolio CRUD

## Problem Statement

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

## Functional Requirements

### FR-1: Create Portfolio
- **Description**: Create a new portfolio with a name and base currency
- **Input**: Portfolio name (string), currency code (3-letter ISO 4217 code)
- **Output**: Created portfolio with system-generated ID, timestamps
- **Constraints**:
  - Name must be non-empty
  - Name must be at most 100 characters
  - Name must be unique across all portfolios (case-sensitive comparison)
  - Currency must match the pattern `^[A-Z]{3}$` (e.g., USD, EUR, GBP)
  - Default currency is `USD` if not provided

### FR-2: List Portfolios
- **Description**: Retrieve all portfolios
- **Input**: Optional pagination parameters (limit, offset)
- **Output**: Ordered list of portfolios (id, name, currency, created_at, updated_at)
- **Constraints**:
  - Default limit is 50, maximum limit is 100
  - Default offset is 0
  - Results ordered by created_at descending (newest first)

### FR-3: Get Portfolio
- **Description**: Retrieve a single portfolio by its ID
- **Input**: Portfolio ID
- **Output**: Full portfolio record (id, name, currency, created_at, updated_at)
- **Constraints**:
  - Returns not-found error if ID does not exist

### FR-4: Update Portfolio
- **Description**: Update name and/or currency of an existing portfolio
- **Input**: Portfolio ID, new name (optional), new currency (optional)
- **Output**: Updated portfolio record
- **Constraints**:
  - Same validation rules as creation (uniqueness, length, currency format)
  - updated_at timestamp is refreshed on every change
  - Returns not-found error if ID does not exist
  - Returns conflict error if new name duplicates another portfolio
  - If no fields provided, returns current state without updating timestamps

### FR-5: Delete Portfolio
- **Description**: Permanently remove a portfolio
- **Input**: Portfolio ID
- **Output**: Confirmation of deletion
- **Constraints**:
  - Returns not-found error if ID does not exist

## Non-Functional Requirements

### NFR-1: Performance
- All CRUD operations complete in under 100ms for typical datasets (< 1000 portfolios)
- List endpoint returns first page in under 200ms

### NFR-2: Consistency
- Error responses use a consistent JSON format: `{"error": "message", "code": "ERROR_CODE"}`
- Error codes: `PORTFOLIO_NOT_FOUND`, `PORTFOLIO_NAME_EXISTS`, `INVALID_CURRENCY`, `INVALID_NAME`

### NFR-3: Reliability
- Duplicate name conflicts are detected before persistence
- Database operations are atomic (single-row inserts/updates/deletes)
- System starts with database schema automatically migrated to latest version

## Data Model (Implementation-Agnostic)

### Entities

**Portfolio**
| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| id | integer | yes | Unique identifier | system-generated, auto-increment |
| name | string | yes | Portfolio display name | non-empty, max 100 chars, unique |
| currency | string | yes | Base currency code | 3-letter ISO 4217, default `USD` |
| created_at | timestamp | yes | Creation time | auto-populated on insert |
| updated_at | timestamp | yes | Last modification time | auto-populated on insert and update |

### Relationships
- None (portfolios are a top-level entity; accounts and transactions reference portfolios via foreign key in future features)

## API Contract (Implementation-Agnostic)

### Create Portfolio
- **Purpose**: Create a new portfolio
- **Input**: JSON body with `name` and optional `currency`
- **Output**: 201 Created with portfolio JSON
- **Error Cases**: 400 (invalid name/currency), 409 (duplicate name)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| name | string | yes | Portfolio name (1-100 chars) |
| currency | string | no | 3-letter ISO 4217 code (default: USD) |

### List Portfolios
- **Purpose**: Retrieve all portfolios with pagination
- **Input**: Optional query parameters `limit` and `offset`
- **Output**: 200 OK with JSON array of portfolios
- **Error Cases**: None (returns empty array if no portfolios)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| limit | integer | no | Max results (default 50, max 100) |
| offset | integer | no | Skip count (default 0) |

### Get Portfolio
- **Purpose**: Retrieve a single portfolio by ID
- **Input**: Portfolio ID (path parameter)
- **Output**: 200 OK with portfolio JSON
- **Error Cases**: 404 (not found)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| id | integer | yes | Portfolio ID |

### Update Portfolio
- **Purpose**: Update name and/or currency of an existing portfolio
- **Input**: Portfolio ID (path), JSON body with `name` and/or `currency`
- **Output**: 200 OK with updated portfolio JSON
- **Error Cases**: 400 (invalid input), 404 (not found), 409 (duplicate name)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| id | integer | yes | Portfolio ID (path) |
| name | string | no | New name (1-100 chars, unique) |
| currency | string | no | New 3-letter ISO 4217 code |

### Delete Portfolio
- **Purpose**: Permanently remove a portfolio
- **Input**: Portfolio ID (path parameter)
- **Output**: 204 No Content
- **Error Cases**: 404 (not found)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| id | integer | yes | Portfolio ID |

## Edge Cases
- Creating a portfolio with an empty name
- Creating a portfolio with a name exceeding 100 characters
- Creating a portfolio with a duplicate name (case-sensitive comparison)
- Creating a portfolio with an invalid currency code (e.g., "US", "usd", "USDX")
- Getting/updating/deleting a non-existent portfolio ID
- Updating to a name that matches another existing portfolio
- Listing with limit=0 or negative offset (treated as defaults: limit=50, offset=0)
- Listing when no portfolios exist (returns empty array, not error)
- Submitting update with no fields to change (no-op, returns current state, timestamps unchanged)

## Out of Scope
- Portfolio accounts and transactions (future features)
- Portfolio P&L calculations and analytics
- Portfolio sharing or multi-user access
- Portfolio archival (soft delete)
- Bulk portfolio operations (import/export)
- Portfolio templates or cloning
