# Implementation Plan: Account CRUD

## Overview
End-to-end account CRUD: REST API (create, list, get, update, delete), server-rendered web pages, domain service with validation, SQLite repository, and full test coverage. Accounts belong to portfolios and support listing by portfolio, with globally unique names.

## Task Dependencies
```
Task 1 → Task 2 → Task 3 → Task 4
```

## Tasks

### Task 1: Database Migration [PRIORITY: HIGH]
**Corresponds to:** All scenarios (foundation — accounts table with portfolio FK)
**Description:** SQLite migration to create the `accounts` table with foreign key to `portfolios`, cascade delete on portfolio, and supporting index.

- [x] Create migration `002_create_accounts.sql`
  - `accounts` table: `id`, `name` (TEXT NOT NULL), `portfolio_id` (FK → portfolios.id, ON DELETE CASCADE), `created_at`, `updated_at`
  - `ON DELETE CASCADE` on `portfolio_id` so deleting a portfolio removes its accounts
  - `CREATE INDEX idx_accounts_portfolio_id` for efficient filtering by portfolio
- [x] Update `tests/integration/portfolio_test.go` setup to include the accounts table in the in-memory schema
- [x] Write a quick smoke test that the migration applies cleanly

**Verification:** Migration runs via goose; `accounts` table exists with correct schema; FK cascade works (delete portfolio → accounts removed)

---

### Task 2: Domain Model + Service [PRIORITY: HIGH]
**Corresponds to:** Create, Update scenarios (validation: name trimming, uniqueness, portfolio existence)
**Description:** Account domain model, DTOs, repository interface, service with all business logic and validation.

- [x] Create `internal/domain/account/account.go` — `Account` struct, `CreateRequest`, `UpdateRequest` DTOs
- [x] Create `internal/domain/account/service.go` — `Repository` interface, error variables, `Service` struct
- [x] Implement `Create`: trim name, validate 1-100 chars, check globally unique name, verify portfolio exists, set timestamps
- [x] Implement `Get`: retrieve by ID
- [x] Implement `List`: retrieve all with pagination (order by created_at DESC)
- [x] Implement `ListByPortfolio`: filter by portfolio_id with pagination
- [x] Implement `Update`: trim name if provided, validate, check uniqueness (excluding self), verify portfolio if changed, handle no-changes (skip timestamp update)
- [x] Implement `Delete`: remove account by ID
- [x] Create `internal/domain/account/service_test.go` — table-driven unit tests with hand-rolled mock repo (following portfolio pattern)
  - Create: success, empty name, whitespace-only name, name > 100 chars, duplicate name (same/diff portfolio), non-existent portfolio
  - Get: success, not found
  - List: all, paginated (limit/offset), empty
  - ListByPortfolio: filtered, non-existent portfolio returns empty
  - Update: name only, portfolio only, both, no-changes (timestamp preserved), duplicate name, empty name, non-existent portfolio, not found
  - Delete: success, not found
  - Name trimming: leading/trailing whitespace stripped before storage

**Verification:** `go test ./internal/domain/account/...` passes; all validation rules covered

---

### Task 3: Repository + API Handlers [PRIORITY: HIGH]
**Corresponds to:** All API scenarios (REST endpoints)
**Description:** SQLite repository implementation and JSON API handlers.

- [x] Create `internal/data/account_repo.go` — `AccountRepository` implementing the account `Repository` interface
  - `Create`: INSERT with returned ID
  - `GetByID`: SELECT by id
  - `GetAll`: SELECT all ORDER BY created_at DESC with optional LIMIT/OFFSET
  - `GetByPortfolio`: SELECT WHERE portfolio_id = ? with optional LIMIT/OFFSET
  - `GetByName`: SELECT WHERE name = ? (for uniqueness checks)
  - `Update`: UPDATE SET name, portfolio_id, updated_at
  - `Delete`: DELETE WHERE id = ?
  - Helper: `scanAccount` function (following `scanPortfolio` pattern)
- [x] Create `internal/api/handlers/account.go` — `AccountHandler` with REST CRUD
  - `POST /api/accounts` — create (201)
  - `GET /api/accounts` — list with `?limit=&offset=&portfolio_id=` (200, empty array when none)
  - `GET /api/accounts/{id}` — get by ID (200)
  - `PATCH /api/accounts/{id}` — update (200)
  - `DELETE /api/accounts/{id}` — delete (204)
  - `handleServiceError` mapping: `ErrInvalidName` → 400/INVALID_NAME, `ErrNameExists` → 409/ACCOUNT_NAME_EXISTS, `ErrNotFound` → 404/ACCOUNT_NOT_FOUND, `ErrPortfolioNotFound` → 404/PORTFOLIO_NOT_FOUND
- [x] Create `internal/api/handlers/account_test.go` — handler unit tests with mock repo (following portfolio_test.go pattern)
  - Create: success, invalid body, empty name, duplicate name, non-existent portfolio
  - List: success, empty, pagination, by portfolio
  - Get: success, not found, invalid ID
  - Update: success, not found, duplicate name, invalid name
  - Delete: success, not found
  - Error response format verification

**Verification:** `go test ./internal/api/handlers/...` passes; all API endpoints return correct status codes and error formats

---

### Task 4: Web UI + Router Wiring + Integration Tests [PRIORITY: HIGH]
**Corresponds to:** All user-facing scenarios (web pages, cascade delete, end-to-end)
**Description:** Server-rendered HTML pages, nav update, router wiring, cascade delete wiring, and integration tests.

- [x] Update `internal/api/handlers/portfolio_web.go` — in `HandleDetailPage`, replace "No accounts added yet" placeholder with a link to `/accounts?portfolio_id={id}` or show accounts list
- [x] Create `templates/account/list.html` — accounts table with columns: Name, Portfolio, Created, Actions (Edit/Delete). Show empty state when none.
- [x] Create `templates/account/form.html` — form with Name input and Portfolio dropdown (select from existing portfolios). Follow portfolio/form.html pattern.
- [x] Create `templates/account/detail.html` — account detail with name, portfolio link, timestamps. Follow portfolio/detail.html pattern.
- [x] Create `internal/api/handlers/account_web.go` — `AccountWebHandler` with web CRUD pages
  - `GET /accounts` — list page (with optional `?portfolio_id=` filter)
  - `GET /accounts/new` — new account form (portfolio dropdown)
  - `POST /accounts` — create from form
  - `GET /accounts/{id}` — detail page
  - `GET /accounts/{id}/edit` — edit form (pre-filled)
  - `POST /accounts/{id}/edit` — update from form
  - `POST /accounts/{id}/delete` — delete with confirmation
  - `userFriendlyError` for account-specific errors
- [x] Create `internal/api/handlers/account_web_test.go` — web handler tests (form rendering, create redirect, error re-render)
- [x] Update `internal/api/router.go` — wire account repo → service → handlers; register routes
- [x] Update `templates/partials/nav.html` — change `<a href="#" class="disabled">Accounts</a>` to `<a href="/accounts">Accounts</a>`
- [x] Update portfolio delete cascade: ensure `PRAGMA foreign_keys = ON` is set in `internal/data/db.go` (verify it already is)
- [x] Create `tests/integration/account_test.go` — end-to-end integration tests against in-memory SQLite
  - Create + Get account
  - List accounts (empty, with data, paginated, by portfolio)
  - Update account (name, portfolio, both)
  - Delete account (success, not found)
  - Duplicate name rejection
  - Cascade: delete portfolio → accounts removed
  - Cascade: delete account → (transactions stub noted for future)

**Verification:** `go test ./tests/integration/...` passes; web pages render correctly; cascade delete works; nav links to /accounts

---

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Name uniqueness | Service-layer check via `GetByName` | Follows portfolio pattern; gives controlled error responses |
| Name trimming | In service `Create`/`Update` before validation | Spec requirement; consistent with trimming constraint |
| FK cascade | SQLite `ON DELETE CASCADE` on `portfolio_id` | Simple, reliable; SQLite handles it natively with `PRAGMA foreign_keys = ON` |
| List by portfolio | Separate `ListByPortfolio` on service + `GetByPortfolio` on repo | Clean separation; matches spec scenario; efficient indexed query |
| No-changes update | Web handler detects no diff → empty `UpdateRequest`; service skips timestamp update if no fields provided | Follows portfolio pattern; preserves `updated_at` when nothing changed |
| Account detail page | Shows portfolio as a link to `/portfolios/{id}` | Consistent with portfolio detail showing related entities |
| Package name | `account` (singular) | Follows `portfolio` convention (singular domain package names) |
| Error codes | `ACCOUNT_NOT_FOUND`, `ACCOUNT_NAME_EXISTS`, `INVALID_NAME`, `PORTFOLIO_NOT_FOUND` | Matches spec exactly |
| Pagination | Reuse `parsePagination` from portfolio handler | Shared utility; consistent behavior |

## Risks

- **Foreign key cascade not working** — Mitigated: verify `PRAGMA foreign_keys = ON` is set in `db.go`; test explicitly in integration test
- **Template conflicts** — Mitigated: renderer already handles per-page parsing with layout; account templates follow same pattern as portfolio
- **Name trimming edge cases** — Mitigated: comprehensive table-driven tests for whitespace-only, leading/trailing spaces, and empty-after-trim
