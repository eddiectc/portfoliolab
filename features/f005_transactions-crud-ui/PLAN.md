# Plan: Transactions CRUD UI

## Technical Decisions

### Decision 1: Currency auto-fill — Client-side JS fetch
Use the existing `/api/symbol-mappings/preview?symbol=X` endpoint via debounced JavaScript (same pattern as `symbol_mapping/form.html`). The form works without it (user fills currency manually), and it's zero backend changes.

### Decision 2: NetCash null detection — Custom `OptionalDecimal` type
Create a small `OptionalDecimal` wrapper for `UpdateRequest.NetCash` that distinguishes "omitted" (nil) from "present but null" (reject). This is a general-purpose pattern reusable for other optional decimal fields.

### Decision 3: NetCash API change — Single atomic task
All changes (domain model, validator, service, repo, handlers, tests) are in one task. They must compile together and tests must pass together.

### Decision 4: Account name resolution — SQL JOIN in repo
Add a new sqlc query (`ListTransactionsWithAccount`) that joins the accounts table. Single query, no N+1, cleaner than fetching accounts separately in the handler.

---

## Task Breakdown

### Task 1: Make netCash required (domain + API + migration + existing tests)

**Goal:** Change `netCash` from optional to required at the domain, API, and test levels. Migrate existing null rows.

**Changes:**

| File | Change |
|------|--------|
| `internal/domain/transaction/transaction.go` | `Transaction.NetCash`: `*decimal.Decimal` → `decimal.Decimal` (non-pointer, remove `omitempty` tag). `CreateRequest.NetCash`: same. `UpdateRequest.NetCash`: new `OptionalDecimal` wrapper type (defined in this file). |
| `internal/domain/transaction/validator.go` | Add `ErrInvalidNetCash`. `ValidateCreateRequest`: reject if `NetCash` is zero. `ValidateUpdateRequest`: reject if `NetCash` is present-but-null or present-but-zero. |
| `internal/domain/transaction/service.go` | `Create`: pass `req.NetCash` directly (non-pointer). `Update`: handle `OptionalDecimal` — only update if present and valid. |
| `migrations/` | Add goose migration: `UPDATE transactions SET net_cash = quantity * price WHERE net_cash IS NULL` |
| `internal/data/repository/transaction_repository.go` | `Create`: always write `tx.NetCash.String()` (no nil check). `Update`: handle non-pointer. Remove `sql.NullString` / nil handling for net_cash. |
| `internal/api/handlers/transaction.go` | Add error mapping for missing/invalid netCash in `handleServiceError`. |
| `internal/domain/transaction/validator_test.go` | Add test cases for missing/zero netCash on create. Add test cases for null/zero netCash on update. Remove any "netCash optional" test cases. |
| `internal/domain/transaction/service_test.go` | Update all test data to include non-nil NetCash. Add test for create with zero netCash. Add test for update with null netCash. Update `TestService_Update_NetCash` to use non-pointer. |
| `internal/api/handlers/transaction_test.go` | Update `txBody` helper (already includes netCash). Add test: create without netCash → 400 error. Add test: update with netCash null → 400 error. |
| `tests/integration/transaction_test.go` | All existing tests already include netCash (confirmed by grep). No changes needed unless new test cases are added. |

**Verification:** `go test ./...` passes (all existing + new tests).

**Estimated effort:** ~20 file changes across domain, repo, handler, and test files.

---

### Task 2: Add sqlc query for transaction list with account name

**Goal:** Add a new sqlc query that returns transactions joined with account names for the list page.

**Changes:**

| File | Change |
|------|--------|
| `internal/data/queries/transactions.sql` | Add `ListTransactionsWithAccount` query: SELECT from transactions JOIN accounts on account_id, with same filtering as `ListTransactions` (account_id, symbol, type, date range) + ORDER BY + LIMIT/OFFSET. Returns transaction fields + account_name. |
| `sqlc generate` | Regenerate types and Go code. |
| `internal/data/repository/transaction_repository.go` | Add `ListWithAccount(ctx, filter, limit, offset) ([]TransactionWithAccount, error)` method using the new sqlc query. Map sqlc result to a `TransactionWithAccount` struct (Transaction + AccountName). |
| `internal/data/repository/transaction_repository_test.go` | Add tests for `ListWithAccount` — empty, single, multiple, with filters, with pagination. |

**Verification:** `go test ./internal/data/...` passes.

**Estimated effort:** 3-4 file changes.

---

### Task 3: Create transaction web handler (`transaction_web.go`)

**Goal:** Server-rendered CRUD pages for transactions with filtering, pagination, and currency auto-fill.

**New file:** `internal/api/handlers/transaction_web.go`

**Handlers:**

| Route | Handler | Description |
|-------|---------|-------------|
| `GET /transactions` | `HandleListPage` | List page with filters and pagination. Uses `ListWithAccount` from Task 2. |
| `GET /transactions/new` | `HandleNewPage` | Empty form with account dropdown and symbol list. |
| `POST /transactions` | `HandleCreatePage` | Validate form → call service.Create → redirect to list with flash. |
| `GET /transactions/{id}` | `HandleDetailPage` | Detail page showing all fields + account name. |
| `GET /transactions/{id}/edit` | `HandleEditPage` | Pre-populated form with account dropdown and symbol list. |
| `POST /transactions/{id}/edit` | `HandleEditPost` | Validate form → call service.Update → redirect to detail with flash. |
| `POST /transactions/{id}/delete` | `HandleDeletePost` | Call service.Delete → redirect to list with flash. |

**Form data struct (passed to templates):**

```go
type TransactionForm struct {
    Transaction   *transaction.Transaction // nil for create
    Accounts      []account.Account
    Symbols       []symbolmapping.SymbolMapping
    Types         []string                  // allowed types
    Error         string                    // general error (e.g., server error)
    FieldErrors   map[string]string         // inline per-field validation errors
    // Form fields
    AccountID     string
    Date          string
    Type          string
    Symbol        string
    Quantity      string
    Price         string
    Currency      string
    NetCash       string
    ExternalSystem string
    ExternalRef   string
}
```

**Filter struct (from query params):**

```go
type TransactionFilter struct {
    AccountID string
    Symbol    string
    Type      string
    DateFrom  string
    DateTo    string
    Limit     int
    Offset    int
}
```

**Key behaviors:**
- Form validation uses the existing `ValidateCreateRequest` / `ValidateUpdateRequest`
- Error display: inline validation errors next to each field, all previously entered values preserved
- Success: confirmation message displayed after create/update/delete
- Delete: POST with explicit user confirmation (JS `confirm()` dialog, consistent with symbol_mapping pattern)
- Currency auto-fill: handled in template via client-side JS (Task 4)
- Cash symbol currency matching: `$CASH-{currency}` symbol's currency portion must match the transaction's currency field
- External fields: `external_system` and `external_reference` validated to max 100 characters

**Verification:** Code compiles, `go vet` passes.

**Estimated effort:** 1 new file, ~300 lines.

---

### Task 4: Create transaction templates + nav link

**Goal:** HTML templates for list, form (create/edit), and detail pages, plus nav link.

**New files:**

| File | Description |
|------|-------------|
| `templates/transaction/list.html` | Table with date, type, symbol, quantity, price, netCash, account_name. Filter form (account dropdown, symbol text, type dropdown, date from/to). "Clear filters" button. Pagination controls (prev/next + current page indicator). "Add Transaction" button. Empty state. |
| `templates/transaction/form.html` | Single-page form with all fields. Account dropdown (select), date input (type="date"), type dropdown (select), symbol text input, quantity number input, price number input, currency text input, netCash number input (required), external_system text (maxlength 100), external_reference text (maxlength 100). Inline error display per field. Currency auto-fill JS (debounced fetch to `/api/symbol-mappings/preview`). |
| `templates/transaction/detail.html` | Detail grid showing all fields. Edit and Delete buttons. Link back to list. |
| `templates/partials/nav.html` | Add "Transactions" link (replace existing disabled placeholder). |

**Filter bar (list.html):**
- Account: `<select>` with all accounts, empty option for "All"
- Symbol: text input
- Type: `<select>` with all 8 types, empty option for "All"
- Date from / Date to: `<input type="date">`
- Submit button: "Filter"
- Filters persist in URL query params

**Pagination (list.html):**
- "Previous" / "Next" links (disabled when at boundary)
- Current page indicator
- Consistent with existing pagination pattern

**Currency auto-fill JS (form.html):**
```js
// Debounced fetch on symbol input
// GET /api/symbol-mappings/preview?symbol=VALUE
// On success: populate currency field with response.currency (only if field is empty)
// On failure: do nothing (user fills manually)
// User override takes precedence — only auto-fill when currency field is empty
```

**Clear filters (list.html):**
- "Clear filters" button resets all filter inputs and navigates to `/transactions` (no query params)
- Equivalent to viewing all transactions unfiltered

**Verification:** Templates render without errors; nav link is active.

**Estimated effort:** 4 files, ~200 lines total.

---

### Task 5: Wire routes

**Goal:** Register transaction web routes in router.

**Changes:**

| File | Change |
|------|--------|
| `internal/api/router.go` | Add transaction web routes (`/transactions/*`). Register `TransactionWebHandler` alongside existing web handlers. Pass required dependencies (transaction service, account service, symbol mapping service) to constructor. |

**Verification:** Routes are accessible; navigation works.

**Estimated effort:** 1 file change.

---

### Task 6: Web handler unit tests

**Goal:** Unit tests for `transaction_web.go` covering all handlers.

**New file:** `internal/api/handlers/transaction_web_test.go`

**Test strategy:**
- Hand-written mocks for transaction service, account service, symbol mapping service (co-located in test file, following `account/service_test.go` pattern)
- Table-driven tests with `httptest.NewRecorder` + real handler
- No DB, no network

**Test cases:**

| Handler | Test Cases |
|---------|------------|
| `HandleListPage` | Empty list, list with data, with filters, with pagination |
| `HandleNewPage` | Renders form with accounts and symbols |
| `HandleCreatePage` | Valid create (redirect + flash), missing fields (error), invalid type (error), missing netCash (error), zero quantity (error), lowercase currency (error), cash symbol currency mismatch (error), external field too long (error) |
| `HandleDetailPage` | Existing transaction (renders), non-existent (404) |
| `HandleEditPage` | Existing transaction (pre-populated form), non-existent (404) |
| `HandleEditPost` | Valid update (redirect), invalid data (error), missing netCash (error), user currency override accepted (success) |
| `HandleDeletePost` | Existing transaction (redirect), non-existent (redirect with no error) |

**Verification:** `go test ./internal/api/...` passes.

**Estimated effort:** 1 new file, ~300 lines.

---

## Execution Order

```
Task 1 (netCash required) ──┐
                              ├──> Task 3 (web handler) ──> Task 6 (web tests)
Task 2 (sqlc query) ─────────┘                              │
Task 4 (templates) ────────────────────────────────────────>│
Task 5 (routes) ───────────────────────────────────────────>┘
```

- **Task 1 and Task 2** can be done in parallel (independent)
- **Task 3** depends on Tasks 1 and 2
- **Task 4** can be done in parallel with Task 3 (templates are independent of handler logic)
- **Task 5** depends on Task 3 (needs handler to exist)
- **Task 6** depends on Task 3 (tests the handler)

---

## Checklist

- [ ] **Task 1:** Make netCash required (domain + API + migration + existing tests)
- [ ] **Task 2:** Add sqlc query for transaction list with account name
- [ ] **Task 3:** Create transaction web handler
- [ ] **Task 4:** Create transaction templates + nav link
- [ ] **Task 5:** Wire routes
- [ ] **Task 6:** Web handler unit tests
