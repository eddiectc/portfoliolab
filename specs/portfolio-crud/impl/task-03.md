---
task: 03
feature: portfolio-crud
depends-on: [02]
status: pending
---
# Task 03: Portfolio CRUD API (REST + Domain + Repository)

## Goal
End-to-end REST API for portfolio CRUD: create, list, get, update, and delete portfolios. Includes domain model, repository with sqlc, service layer, and HTTP handlers.

## Files to Create/Modify
| File | Action | Description |
|------|--------|-------------|
| `internal/domain/portfolio/portfolio.go` | create | Portfolio domain model |
| `internal/domain/portfolio/service.go` | create | Portfolio service (business logic) |
| `internal/domain/portfolio/service_test.go` | create | Unit tests for service |
| `internal/data/portfolio_repo.go` | create | PortfolioRepository interface |
| `internal/data/portfolio_repo_impl.go` | create | Repository implementation (hand-written SQL) |
| `internal/data/queries/portfolio.sql` | create | sqlc queries for portfolios |
| `internal/data/queries/sqlc.yaml` | create | sqlc config |
| `internal/data/queries/portfolio_db.go` | generate | sqlc-generated code |
| `internal/api/handlers/portfolio.go` | create | HTTP handlers (Create, List, Get, Update, Delete) |
| `internal/api/handlers/portfolio_test.go` | create | Handler unit tests with mocked repo |
| `internal/api/router.go` | modify | Add portfolio routes |
| `tests/integration/portfolio_test.go` | create | Integration tests with in-memory DB |

## Implementation Steps
1. Define `Portfolio` domain model in `domain/portfolio/portfolio.go`:
   - Fields: ID (int64), Name (string), Currency (string), CreatedAt, UpdatedAt (time.Time)
2. Define `PortfolioRepository` interface with: Create, GetByID, GetAll, Update, Delete, GetByName
3. Write SQL queries in `queries/portfolio.sql` and run `sqlc generate`
4. Implement repository using sqlc-generated code
5. Implement `PortfolioService` with validation:
   - Name must be non-empty, max 100 chars
   - Name must be unique (checked at service layer)
   - Currency must be a valid ISO 4217 code (validate against a small allowlist or regex `^[A-Z]{3}$`)
6. Implement HTTP handlers:
   - `POST /api/portfolios` → Create (201)
   - `GET /api/portfolios` → List (200, paginated with `?limit=&offset=`)
   - `GET /api/portfolios/{id}` → Get (200)
   - `PATCH /api/portfolios/{id}` → Update (200)
   - `DELETE /api/portfolios/{id}` → Delete (204)
7. Add routes to chi router with proper grouping
8. Write unit tests for service (validation logic) and handlers (with mocked repo)
9. Write integration tests: full CRUD cycle against in-memory SQLite

## Acceptance Criteria
- [ ] `POST /api/portfolios` with valid JSON creates a portfolio and returns 201
- [ ] `GET /api/portfolios` returns all portfolios as JSON array
- [ ] `GET /api/portfolios/{id}` returns single portfolio or 404
- [ ] `PATCH /api/portfolios/{id}` updates name/currency and returns 200
- [ ] `DELETE /api/portfolios/{id}` deletes and returns 204
- [ ] Duplicate name returns 409 Conflict
- [ ] Invalid currency code returns 400 Bad Request
- [ ] Non-existent ID returns 404 Not Found
- [ ] List endpoint supports `?limit=10&offset=0` pagination
- [ ] All error responses use consistent format: `{"error": "message", "code": "ERROR_CODE"}`
- [ ] Unit tests pass for service validation logic
- [ ] Integration tests pass for full CRUD cycle

## Testing Requirements
- Unit tests: Service validation (empty name, long name, invalid currency, duplicate name)
- Unit tests: Handlers with mocked repository (all CRUD operations, error cases)
- Integration tests: Full CRUD cycle against in-memory SQLite
- Manual verification: curl all endpoints against running server

## Notes
- Use integer arithmetic for any monetary values (not applicable yet for portfolios, but set the pattern)
- Error codes: `PORTFOLIO_NOT_FOUND`, `PORTFOLIO_NAME_EXISTS`, `INVALID_CURRENCY`, `INVALID_NAME`
- Follow AGENTS.md naming: handlers prefixed `Handle`, service `PortfolioService`, repo `PortfolioRepository`
- sqlc config should target `sqlite` database and `modernc.org/sqlite` driver
