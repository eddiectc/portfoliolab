# Implementation Plan: Portfolio CRUD

## Overview
End-to-end portfolio CRUD: REST API (create, list, get, update, delete), server-rendered web pages, domain service with validation, SQLite repository, and full test coverage.

## Task Dependencies
```
Task 1 → Task 2 → Task 3 → Task 4
```

## Tasks

### Task 1: Go Module + Project Skeleton [PRIORITY: HIGH]
**Corresponds to:** Infrastructure (foundation for all features)
**Description:** Initialize Go module, project directory structure, config loading, minimal HTTP server with health check.

- [x] Create `go.mod` with module `github.com/arch-portfolio-lab/portfoliolab`
- [x] Create directory structure (`cmd/`, `internal/api/`, `internal/config/`, etc.)
- [x] Implement config loading from YAML (`internal/config/config.go`)
- [x] Create chi router with `/health` endpoint
- [x] Add request logging middleware
- [x] Wire up `main.go`: load config → create router → start server
- [x] Create `Makefile` with build/run/test targets
- [x] Write unit tests for config loading

**Verification:** `go build ./...` compiles; server starts; `GET /health` returns `{"status": "ok"}`

### Task 2: Database Setup + Migration Framework [PRIORITY: HIGH]
**Corresponds to:** Infrastructure (foundation for data layer)
**Description:** SQLite database initialization with WAL mode, goose migration framework, portfolios table schema.

- [x] Add `modernc.org/sqlite` dependency
- [x] Implement `OpenDB()` with WAL mode and foreign keys
- [x] Create migration `001_create_portfolios.sql` (portfolios table with UNIQUE name, default currency)
- [x] Integrate goose migration runner into server startup
- [x] Write tests for DB connection and WAL mode

**Verification:** Server auto-runs migrations; `portfolios` table exists with correct schema; WAL mode confirmed

### Task 3: Portfolio CRUD API [PRIORITY: HIGH]
**Corresponds to:** All BDD scenarios (Create, List, Get, Update, Delete)
**Description:** REST API with domain model, service layer (validation), repository (SQL), and HTTP handlers.

- [x] Define `Portfolio` domain model + `CreateRequest`/`UpdateRequest` DTOs
- [x] Define `Repository` interface (Create, GetByID, GetAll, Update, Delete, GetByName)
- [x] Implement repository with hand-written SQL (sqlc queries ready for when sqlc is available)
- [x] Implement `PortfolioService` with validation (name: 1-100 chars, unique; currency: `^[A-Z]{3}$`)
- [x] Implement HTTP handlers (POST/GET/PATCH/DELETE `/api/portfolios`)
- [x] Add pagination support (`?limit=&offset=`)
- [x] Write unit tests for service validation (table-driven)
- [x] Write unit tests for handlers (with mocked repo)
- [x] Write integration tests (full CRUD cycle against in-memory SQLite)

**Verification:** All API endpoints return correct status codes; error responses use consistent format; 50 tests pass

### Task 4: Portfolio Web UI [PRIORITY: HIGH]
**Corresponds to:** User-facing portfolio management (list, create, edit, delete pages)
**Description:** Server-rendered HTML pages with Go templates, base layout, form handling, flash messages.

- [x] Create template renderer (`internal/web/renderer.go`) with layout/partials support
- [x] Create `templates/base.html` (HTML5 layout, nav, content block)
- [x] Create portfolio list page (`templates/portfolio/list.html`)
- [x] Create portfolio form page (`templates/portfolio/form.html`) for create/edit
- [x] Create portfolio detail page (`templates/portfolio/detail.html`)
- [x] Implement web handlers (ListPage, NewPage, CreatePage, DetailPage, EditPage, UpdatePage, DeletePage)
- [x] Add static file serving for CSS
- [x] Implement flash message system (cookie-based)
- [x] Write template rendering tests

**Verification:** `GET /portfolios` renders table; form creates portfolio; edit pre-fills values; delete works with POST method

## Technical Decisions
| Decision | Choice | Reason |
|---|---|---|
| Router | `chi` (v5) | Minimal, idiomatic, middleware support; matches README tech stack |
| DB driver | `modernc.org/sqlite` | Pure Go, no CGO; matches project constraint |
| Queries | Hand-written SQL (sqlc ready) | sqlc not installed; query files in `internal/data/queries/` ready for sqlc |
| Templating | `html/template` (std) | Built-in, XSS-safe; parse layout + page separately to avoid name conflicts |
| Error format | `{"error": "message", "code": "ERROR_CODE"}` | Consistent API errors; matches spec |
| Pagination | `?limit=&offset=` query params | Simple, standard REST pattern |

## Risks
- **Go build cache on read-only filesystem** — Mitigated: use `GOCACHE=/tmp/go-cache GOPATH=/tmp/go-path`
- **sqlc not available** — Mitigated: hand-written repos work; sqlc files ready for when tool is installed
- **Template name conflicts** — Mitigated: parse layout + each page separately (not into one shared set)
