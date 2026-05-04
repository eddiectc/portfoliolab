# Implementation Plan: transaction-crud

## Overview

Build the transaction CRUD infrastructure: database schema, domain layer with validation, repository, REST API with filtering/pagination, and cascade delete from accounts. Transactions are the source of truth for all future portfolio analytics (positions, P&L, cash balance).

This feature depends on **f002_account-crud** (done) for account existence checking and **f003_symbol-map** (must be implemented first) for symbol existence checking and `$CASH-{currency}` auto-creation. f004 is blocked on f003 completion — no stub or mock implementations for symbol dependencies.

## Data Model

### Relationships

```
portfolios (1) ──── (N) accounts (1) ──── (N) transactions
                                  │
                                  └── account_id (FK, ON DELETE CASCADE)
```

Transactions reference accounts (not portfolios directly). Portfolio delete cascades through accounts to transactions.

### Database Schema

```sql
CREATE TABLE transactions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL,
    date                TEXT    NOT NULL,
    type                TEXT    NOT NULL,
    symbol              TEXT    NOT NULL,
    quantity            TEXT    NOT NULL,
    price               TEXT    NOT NULL,
    currency            TEXT    NOT NULL,
    net_cash            TEXT,
    external_system     TEXT,
    external_reference  TEXT,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_transactions_account_id ON transactions(account_id);
CREATE INDEX idx_transactions_date ON transactions(date DESC);
CREATE INDEX idx_transactions_symbol ON transactions(symbol);
CREATE INDEX idx_transactions_type ON transactions(type);
```

**Key constraints:**
- `account_id` FK with `ON DELETE CASCADE` — account/portfolio delete removes transactions
- No application-level UNIQUE constraints (multiple identical transactions allowed)
- Indexes on commonly filtered columns (account_id, date, symbol, type)
- `date` stored as TEXT (SQLite date format: `YYYY-MM-DD`), mapped to `time.Time` in Go (date-only, midnight UTC)
- `quantity`, `price`, `net_cash` stored as TEXT (decimal.Decimal string representation); repo layer handles decimal ↔ string conversion
- `quantity` is signed (positive = long direction, negative = short direction)
- `price` is always positive (validated at application layer)
- `net_cash`, `external_system`, `external_reference` are nullable

### Go Domain Model

```go
// transaction/transaction.go

import "github.com/govalues/decimal"

type Transaction struct {
    ID                int64           `json:"id"`
    AccountID         int64           `json:"account_id"`
    Date              time.Time       `json:"date"`             // date-only (midnight UTC, serialized as YYYY-MM-DD)
    Type              string          `json:"type"`             // buy, sell, deposit, etc.
    Symbol            string          `json:"symbol"`
    Quantity          decimal.Decimal `json:"quantity"`
    Price             decimal.Decimal `json:"price"`
    Currency          string          `json:"currency"`         // ISO 4217
    NetCash           *decimal.Decimal `json:"net_cash,omitempty"`
    ExternalSystem    *string         `json:"external_system,omitempty"`
    ExternalReference *string         `json:"external_reference,omitempty"`
    CreatedAt         time.Time       `json:"created_at"`
    UpdatedAt         time.Time       `json:"updated_at"`
}

type CreateRequest struct {
    AccountID         int64            `json:"account_id"`
    Date              string           `json:"date"`             // YYYY-MM-DD (parsed to time.Time in service)
    Type              string           `json:"type"`
    Symbol            string           `json:"symbol"`
    Quantity          decimal.Decimal  `json:"quantity"`
    Price             decimal.Decimal  `json:"price"`
    Currency          string           `json:"currency"`
    NetCash           *decimal.Decimal `json:"net_cash,omitempty"`
    ExternalSystem    *string          `json:"external_system,omitempty"`
    ExternalReference *string          `json:"external_reference,omitempty"`
}

type UpdateRequest struct {
    Date              *string          `json:"date,omitempty"`        // YYYY-MM-DD
    Type              *string          `json:"type,omitempty"`
    Symbol            *string          `json:"symbol,omitempty"`
    Quantity          *decimal.Decimal `json:"quantity,omitempty"`
    Price             *decimal.Decimal `json:"price,omitempty"`
    Currency          *string          `json:"currency,omitempty"`
    NetCash           *decimal.Decimal `json:"net_cash,omitempty"`
    ExternalSystem    *string          `json:"external_system,omitempty"`
    ExternalReference *string          `json:"external_reference,omitempty"`
}

type ListFilters struct {
    AccountID *int64
    Symbol    *string
    Type      *string
    DateFrom  *time.Time
    DateTo    *time.Time
}
```

## Task Dependencies

```
f003_symbol-map (must be complete first)
    ↓
Task 1 (Migration + sqlc)
    ↓
Task 2 (Domain Model + Validation) ← parallel with Task 3
Task 3 (Repository)               ← parallel with Task 2
    ↓
Task 4 (Service Layer)
    ↓
Task 5 (HTTP Handler)
    ↓
Task 6 (Router Wiring)
    ↓
Task 7 (Integration Tests)
    ↓
Task 8 (Cascade Delete Verification)
```

f003 must be fully implemented before starting f004. Tasks 2 and 3 can be done in parallel after Task 1. Tasks 4–8 are sequential.

## Tasks

### Task 1: Database migration and sqlc queries [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundation)

**Description:** Create the transactions table migration and sqlc query definitions. Multiple specialized queries for list filtering (one per filter combination) instead of a single dynamic query.

- [ ] Create migration `003_create_transactions.sql` with:
  - `transactions` table (all columns, FK to accounts with ON DELETE CASCADE; monetary fields as TEXT)
  - Indexes: `idx_transactions_account_id`, `idx_transactions_date`, `idx_transactions_symbol`, `idx_transactions_type`
- [ ] Create `internal/data/queries/transaction.sql` with sqlc queries:
  - `CreateTransaction` (:one, INSERT RETURNING *)
  - `GetTransaction` (:one, SELECT by id)
  - `UpdateTransaction` (:one, UPDATE SET ... RETURNING *)
  - `DeleteTransaction` (:execrows, DELETE by id)
  - Specialized list queries (each :many, with ORDER BY + LIMIT/OFFSET):
    - `ListTransactions` (no filters)
    - `ListTransactionsByAccount` (WHERE account_id = ?)
    - `ListTransactionsBySymbol` (WHERE symbol = ?)
    - `ListTransactionsByType` (WHERE type = ?)
    - `ListTransactionsByDateRange` (WHERE date >= ? AND date <= ?)
    - `ListTransactionsByAccountAndSymbol` (WHERE account_id = ? AND symbol = ?)
    - `ListTransactionsByAccountAndType` (WHERE account_id = ? AND type = ?)
    - `ListTransactionsByAccountAndDateRange` (WHERE account_id = ? AND date >= ? AND date <= ?)
    - `ListTransactionsBySymbolAndType` (WHERE symbol = ? AND type = ?)
    - `ListTransactionsBySymbolAndDateRange` (WHERE symbol = ? AND date >= ? AND date <= ?)
    - `ListTransactionsByTypeAndDateRange` (WHERE type = ? AND date >= ? AND date <= ?)
    - `ListTransactionsByAccountSymbolType` (WHERE account_id = ? AND symbol = ? AND type = ?)
    - `ListTransactionsByAccountSymbolDateRange` (WHERE account_id = ? AND symbol = ? AND date >= ? AND date <= ?)
    - `ListTransactionsByAccountTypeDateRange` (WHERE account_id = ? AND type = ? AND date >= ? AND date <= ?)
    - `ListTransactionsBySymbolTypeDateRange` (WHERE symbol = ? AND type = ? AND date >= ? AND date <= ?)
    - `ListTransactionsByAllFilters` (WHERE account_id = ? AND symbol = ? AND type = ? AND date >= ? AND date <= ?)
- [ ] Run `sqlc generate` from `internal/data/queries/` (or hand-write `.sql.go` following existing pattern if sqlc unavailable)
- [ ] Update `tests/integration/portfolio_test.go` setupTestDB to include the transactions table schema (for integration tests)

**Verification:** `goose sqlite3 data/portfoliolab.db up` runs cleanly; migration creates table and indexes; sqlc generates types.

### Task 2: Domain model and validation [PRIORITY: HIGH]

**Corresponds to:** All create/update validation scenarios

**Description:** Define the transaction domain model, DTOs, and validation functions. Uses `github.com/govalues/decimal` for monetary values and `time.Time` for dates (date-only, midnight UTC).

- [ ] Create `internal/domain/transaction/transaction.go` with:
  - `Transaction` struct (all fields, json tags, pointer fields for optionals; `decimal.Decimal` for quantity/price/netCash, `time.Time` for Date/CreatedAt/UpdatedAt)
  - `CreateRequest`, `UpdateRequest` DTOs (Date as `string` in JSON, parsed to `time.Time` in service)
  - `ListFilters` struct (AccountID *int64, Symbol *string, Type *string, DateFrom *time.Time, DateTo *time.Time)
- [ ] Create `internal/domain/transaction/validator.go` with:
  - `ValidateCreateRequest(req CreateRequest) error` — validates all create fields
  - `ValidateUpdateRequest(req UpdateRequest) error` — validates only non-nil fields
  - `validateType(s string) error` — must be one of 8 allowed types
  - `validateQuantity(q decimal.Decimal) error` — must be non-zero
  - `validatePrice(p decimal.Decimal) error` — must be > 0
  - `validateCurrency(s string) error` — ISO 4217, 3-letter uppercase regex
  - `validateDate(s string) error` — must parse as YYYY-MM-DD
  - `validateSymbol(s string) error` — trimmed, non-empty
  - `validateCashSymbolMatch(symbol, currency string) error` — `$CASH-{currency}` must match transaction currency
  - `validateExternalFields(externalSystem, externalReference *string) error` — max 100 chars
- [ ] Define dependency interfaces in `internal/domain/transaction/service.go`:
  - `AccountChecker` interface: `AccountExists(ctx context.Context, id int64) bool`
  - (SymbolChecker/SymbolCreator provided by f003 domain — no interfaces needed in transaction domain; f003 types used directly)
- [ ] Define service errors: `ErrNotFound`, `ErrAccountNotFound`, `ErrSymbolNotFound`, `ErrInvalidSymbol`, `ErrInvalidPrice`, `ErrInvalidCurrency`, `ErrInvalidType`, `ErrInvalidQuantity`, `ErrInvalidDate`, `ErrImmutableField`
- [ ] Write comprehensive validation unit tests in `validator_test.go` (table-driven, covering all constraints and edge cases)

**Verification:** `go test ./internal/domain/transaction/... -run Validator` passes; all validation rules tested.

### Task 3: Repository [PRIORITY: HIGH]

**Corresponds to:** All scenarios (data access)

**Description:** Implement the transaction repository, delegating to sqlc-generated queries. Handles decimal ↔ string conversion and time.Time ↔ SQLite text conversion. Routes list requests to the appropriate specialized query based on which filters are set.

- [ ] Create `internal/data/transaction_repo.go` with:
  - `TransactionRepository` struct with `q *queries.Queries` and `db queries.DBTX`
  - `toDomain(queries.Transaction) (*transaction.Transaction, error)` — converts sqlc model to domain model, parses timestamps, converts decimal strings to `decimal.Decimal`
  - `Create(ctx, *transaction.Transaction) error` — inserts (decimal → string) and sets ID
  - `GetByID(ctx, int64) (*transaction.Transaction, error)`
  - `List(ctx, transaction.ListFilters, int, int) ([]transaction.Transaction, error)` — selects appropriate specialized query based on which filters are non-nil, applies pagination
  - `Update(ctx, *transaction.Transaction) error`
  - `Delete(ctx, int64) error` — returns ErrNotFound if 0 rows affected
- [ ] Handle sqlc string timestamps with existing `parseTime()` pattern
- [ ] Handle decimal ↔ string conversion for quantity, price, net_cash (decimal.String() → decimal.NewFromString())
- [ ] Handle nullable fields (net_cash, external_system, external_reference) in toDomain conversion
- [ ] Write repository unit tests in `transaction_repo_test.go` (use in-memory SQLite or mock sqlc)

**Verification:** `go test ./internal/data/... -run Transaction` passes; all CRUD operations work; decimal conversion is lossless.

### Task 4: Service layer [PRIORITY: HIGH]

**Corresponds to:** All CRUD scenarios (business logic)

**Description:** Implement the transaction service with full CRUD, validation, filtering, and pagination. Depends on f003 symbol-map types for symbol checking and auto-creation.

- [ ] Create `internal/domain/transaction/service.go` with:
  - `Repository` interface (matching repo methods)
  - `Service` struct with Repository, AccountChecker, and f003 symbol-map service (for symbol existence checks and $CASH auto-creation)
  - `Create(ctx, CreateRequest) (*Transaction, error)` — validates, checks account/symbol, auto-creates $CASH symbol via f003, persists
  - `Get(ctx, int64) (*Transaction, error)`
  - `List(ctx, ListFilters, int, int) ([]Transaction, error)` — applies pagination defaults (limit=0 → 50, negative offset → 0)
  - `Update(ctx, int64, UpdateRequest) (*Transaction, error)` — validates changed fields, rejects account_id change, refreshes updated_at only if changed
  - `Delete(ctx, int64) error`
- [ ] Pagination logic: `limit=0` or `limit<0` defaults to 50; `offset<0` defaults to 0
- [ ] `$CASH-{currency}` auto-creation: if symbol matches `$CASH-{currency}` pattern and doesn't exist, create via f003 symbol-map service
- [ ] Date handling: parse `CreateRequest.Date` (string, YYYY-MM-DD) to `time.Time` (midnight UTC); serialize back to YYYY-MM-DD in JSON responses
- [ ] Update no-op detection: if no fields provided, return current state without touching updated_at
- [ ] Create mock implementation in `internal/domain/transaction/mock_repository.go`:
  - `mockRepository` — in-memory store with filtering/pagination
  - `mockAccountChecker` — set of account IDs
  - (Symbol checker/creator mocked at the f003 service level, not in transaction domain)
- [ ] Write comprehensive service unit tests in `service_test.go` (table-driven, covering all 56 spec scenarios)

**Verification:** `go test ./internal/domain/transaction/...` passes; all CRUD operations with validation, filtering, and pagination tested.

### Task 5: HTTP handler [PRIORITY: HIGH]

**Corresponds to:** All scenarios (API surface)

**Description:** Create JSON API handlers for transaction CRUD with filtering and pagination. Handles decimal JSON serialization (govalues/decimal implements json.Marshaler/Unmarshaler natively).

- [ ] Create `internal/api/handlers/transaction.go` with:
  - `TransactionHandler` struct with service dependency
  - `RegisterRoutes` mounting:
    - `GET /api/transactions` (list with filters)
    - `POST /api/transactions` (create)
    - `GET /api/transactions/{id}` (get by ID)
    - `PATCH /api/transactions/{id}` (update)
    - `DELETE /api/transactions/{id}` (delete)
  - `HandleList` — parses query params (account_id, symbol, type, date_from, date_to, limit, offset), delegates to service
  - `HandleCreate` — decodes JSON body (decimal fields parsed via json.Unmarshaler), delegates to service, returns 201
  - `HandleGet` — parses ID from URL, delegates to service
  - `HandleUpdate` — parses ID, decodes JSON body, delegates to service
  - `HandleDelete` — parses ID, delegates to service, returns 204
  - `handleServiceError` — maps all 10 error codes to HTTP status codes
- [ ] Query param parsing for list endpoint (follow existing `parsePagination` pattern, extend with filter params; date_from/date_to as YYYY-MM-DD strings parsed to time.Time)
- [ ] Write handler unit tests in `transaction_test.go` (mock service, verify HTTP status codes, error formats, pagination)

**Verification:** `go test ./internal/api/handlers/... -run Transaction` passes; all endpoints return correct status codes and error formats.

### Task 6: Router wiring [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (integration)

**Description:** Wire the transaction handler into the application router. Requires f003 symbol-map service for symbol checking and auto-creation.

- [ ] Update `internal/api/router.go`:
  - Create `TransactionRepository`, `TransactionService`, `TransactionHandler`
  - Wire `AccountChecker` (reuse existing `PortfolioChecker` pattern — create `AccountCheckerImpl` in data layer)
  - Wire f003 `SymbolMappingService` for symbol existence checks and $CASH auto-creation
  - Register routes
- [ ] Create `internal/data/account_checker.go` with `AccountCheckerImpl` (follows `PortfolioCheckerImpl` pattern)
- [ ] Verify full application builds: `go build -o portfoliolab cmd/server/main.go`

**Verification:** Application builds and starts; transaction API endpoints respond.

### Task 7: Integration tests [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (end-to-end verification)

**Description:** Create integration tests using in-memory SQLite and the real router.

- [ ] Create `tests/integration/transaction_test.go` with:
  - `TestTransaction_CreateAndGet` — create a buy transaction, get by ID, verify all fields
  - `TestTransaction_CreateSell` — create with negative quantity
  - `TestTransaction_CreateCashDeposit` — create with `$CASH-USD` symbol
  - `TestTransaction_ListEmpty` — list with no transactions returns empty array
  - `TestTransaction_CreateListDelete` — full lifecycle
  - `TestTransaction_Update` — update fields, verify updated_at refreshes
  - `TestTransaction_UpdateNoChanges` — no-op, updated_at unchanged
  - `TestTransaction_FilterByAccount` — verify account filter
  - `TestTransaction_FilterBySymbol` — verify symbol filter
  - `TestTransaction_FilterByDateRange` — verify date range filter
  - `TestTransaction_Pagination` — verify limit/offset
  - `TestTransaction_CascadeDeleteAccount` — delete account, verify transactions removed
  - `TestTransaction_CascadeDeletePortfolio` — delete portfolio, verify accounts + transactions removed
  - `TestTransaction_ValidationErrors` — verify error codes for invalid inputs
- [ ] Update `setupTestDB` in `tests/integration/portfolio_test.go` to include transactions table schema

**Verification:** `go test ./tests/integration/...` passes; all end-to-end flows work.

### Task 8: Cascade delete verification [PRIORITY: LOW]

**Corresponds to:** Delete account/portfolio cascades to transactions scenarios

**Description:** Verify that the ON DELETE CASCADE foreign key constraint works correctly. The database handles this automatically, but we verify at the application level.

- [ ] Verify `AccountRepository.Delete` already triggers cascade (FK constraint handles it)
- [ ] Verify `PortfolioRepository.Delete` cascades through accounts to transactions (FK chain)
- [ ] Add integration test coverage (already in Task 7)
- [ ] Document in NOTES.md that cascade is handled by SQLite FK constraint, not application logic

**Verification:** Cascade delete integration tests pass; no orphaned transactions after account/portfolio deletion.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| **Monetary types** | `govalues/decimal.Decimal` in Go, `TEXT` in SQLite | Per AGENTS.md convention: exact decimal arithmetic for all monetary values; repo layer handles decimal ↔ string conversion; avoids float64 precision; govalues is faster, no heap allocations, correctly rounded, panic-free |
| **Date type** | `time.Time` in Go (date-only, midnight UTC), `TEXT` in SQLite (`YYYY-MM-DD`) | Spec says "date only (no time component)"; `time.Time` provides type safety and consistent formatting; serialized as YYYY-MM-DD in JSON; stored as SQLite date text |
| **quantity sign convention** | Positive = long direction, negative = short direction; position = SUM(quantity) | Matches user's convention (SELL -10 = selling 10 shares); position calculation is trivial |
| **$CASH symbol auto-creation** | Service calls f003 SymbolMappingService directly | f004 is blocked on f003; uses real symbol-map service for existence checks and auto-creation; no stubs or mocks |
| **Pagination: limit=0** | Defaults to 50 | Consistent with account service behavior; simplifies pagination logic (no special case for "no limit") |
| **List filtering SQL** | Multiple specialized queries per filter combination (17 queries) | Each query is simple and indexable; SQLite query planner optimizes each path; avoids dynamic SQL or NULL comparison overhead; tradeoff: more sqlc queries but each is trivial |
| **Update no-op detection** | Check if any fields provided; skip DB write and updated_at refresh if none | Follows account service pattern; avoids unnecessary writes |
| **Symbol validation** | Trim whitespace, reject empty; check existence via f003 service | Follows account name trimming pattern; hard dependency on f003 for real implementation |
| **External fields** | Optional, max 100 chars | Spec constraint; stored as nullable TEXT in SQLite |
| **Sorting** | date DESC, symbol ASC, type ASC, id ASC | Per spec constraint; implemented in SQL ORDER BY |
| **Error codes** | 10 codes: TRANSACTION_NOT_FOUND, ACCOUNT_NOT_FOUND, SYMBOL_NOT_FOUND, INVALID_SYMBOL, INVALID_PRICE, INVALID_CURRENCY, INVALID_TYPE, INVALID_QUANTITY, INVALID_DATE, IMMUTABLE_FIELD | Per spec constraint; each maps to a specific HTTP status code |
| **Dependency on f003** | Hard block — f004 implementation starts after f003 is complete | Symbol validation and $CASH auto-creation require f003 types; no interfaces or stubs; clean dependency chain |

## Risks

- **f003_symbol-map must be complete first**: f004 cannot start implementation until f003 is done (symbol existence checks, $CASH auto-creation). Mitigation: f003 is a prerequisite; plan tasks assume f003 types exist.
- **17 specialized list queries**: More queries to maintain, but each is trivial and indexable. Mitigation: repository dispatch logic is a simple switch on which filters are set; query names follow a consistent pattern.
- **govalues/decimal JSON serialization**: decimal implements json.Marshaler/Unmarshaler natively, marshaling as a number (not string), which matches spec JSON examples. No custom marshaler needed.
- **sqlc not installed**: Queries may need to be hand-written, following the existing pattern in `internal/data/queries/`. The `.sql` files are prepared for sqlc generation when available.
- **ON DELETE CASCADE relies on SQLite FK support**: FK constraints must be enabled (`PRAGMA foreign_keys=ON`) — already done in `data.Open()`. If FK support is somehow disabled, cascade won't work. Integration tests verify this.
