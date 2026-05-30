# AGENTS.md — Agentic Coding Instructions

## Project Context

Arch Portfolio Lab is a self-hosted investment portfolio management platform built in Go. It tracks stocks and ETFs across multiple accounts and currencies, with deep P&L analytics including drawdown analysis and benchmark comparison (S&P 500, NASDAQ, custom). Market data is fetched via `go-yfinance` (pure Go). The web UI is server-rendered (Go templates), and all functionality is exposed via a REST API for future mobile app support. Database is SQLite (single file, WAL mode).

## Conventions Reference

Coding conventions (style, naming, testing, domain logic, database, API, web, security) are documented in **docs/CONVENTIONS.md**. Read that file for the full set of rules. AGENTS.md covers agent-specific workflow and practices only.

## Common Commands

```bash
# Run the server
go run cmd/server/main.go

# Run with config file
go run cmd/server/main.go --config config/config.yaml

# Build binary
go build -o portfoliolab cmd/server/main.go

# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run only unit tests (fast, no DB)
go test -short ./...

# Generate sqlc types
sqlc generate

# Run database migrations (goose)
goose sqlite3 data/portfoliolab.db up

# Rollback last migration
goose sqlite3 data/portfoliolab.db down

# Format code
goimports -w .

# Lint (if golangci-lint is installed)
golangci-lint run
```

## Agile Workflow

Features live in `features/<id>_<name>/` with `SPEC.md`, `PLAN.md`, `NOTES.md`.
Project docs: `API.md`, `docs/PROJECT.md`, `docs/CONVENTIONS.md`.
Feature index: `features/README.md`.

### Workflow Phases
1. **Setup project** (`/setup-project`) — analyze codebase, create docs/features structure
2. **Write spec** (`/write-spec <name>`) — BDD feature spec, user reviews before proceeding
3. **Review spec** (`/review-spec <name>`) — check quality, completeness, edge cases
4. **Plan implementation** (`/plan-impl <name>`) — break into small, testable tasks, user reviews
5. **Implement** — execute tasks one at a time, update checkboxes in PLAN.md
6. **Retrospective** (`/retro <name>`) — review spec vs reality, document learnings

### Principles
- **One task at a time** — each task is independently testable
- **Follow existing patterns** before introducing new approaches
- **Write tests alongside implementation** — not after
- **Flag spec drift** before deviating from the plan; document in NOTES.md
- **Ask for clarification** on ambiguous requirements
- **Keep changes minimal and focused** — one concern per PR/commit

### Feature Scoping
- **User-facing features include web UI by default** — any feature that involves user interaction gets server-rendered web pages (`*_web.go` + templates + nav link) alongside the API. The plan must include tasks for both layers. If a feature is API-only or backend-only, explicitly state why in the spec's Non-Goals.
- **Spec user stories are medium-agnostic** — write "I want to create X" not "I want to POST to /api/X". The plan decomposes into API + web tasks.

## Best Practices

### Unit Testing
- **Hand-written mocks only**: Define minimal mock structs co-located in `*_test.go` files (not separate mock files, no mockery/testify). Follow the `account/service_test.go` pattern: the mock maintains internal state (maps, slices) and simulates real repository behavior — e.g., `Create` then `GetByID` returns the created item. This catches service-layer bugs that permissive expectation-based mocks would hide.
- **Mocks must simulate real behavior**: If the real implementation returns empty/error for edge-case inputs (e.g. `limit=0` → `LIMIT 0` → zero rows), the mock must do the same.

### Integration Testing
- **Fresh DB per test**: In-memory SQLite (`file::memory:?cache=shared`) with goose migrations
- **Real SQL, real schema**: No mocking at the repo layer in integration tests
- **Shared test helper**: `tests/integration/db.go` — opens in-memory DB, runs migrations, returns cleanup func
- **API integration**: `httptest.NewRecorder` + real router + in-memory DB

### Domain Logic
- **Position calculations** are the heart of the app — be extra careful with P&L math
- The `calculator.go` in the transaction domain is critical — test exhaustively

#### govalues/decimal API Reference
| Operation | Function | Notes |
|---|---|---|
| Create from integer + scale | `decimal.MustNew(value, scale)` | value is integer shifted by 10^scale; e.g. `MustNew(15000, 2)` = 150.00 |
| Create from string | `decimal.MustParse(s)` | panics on error; use `decimal.Parse(s)` for error-returning version |
| Check positive | `d.IsPos()` | not `IsPositive()` |
| Compare | `a.Equal(b)` | not string equality; `String()` preserves scale ("150.00" ≠ "150") |
| Serialize | `d.String()` | preserves scale (e.g. "150.00") |
| Deserialize | `decimal.MustParse(s)` | parses back to Decimal |
| JSON | native | implements `json.Marshaler`/`json.Unmarshaler` automatically |

### Import Parsers
- Parse broker files into an intermediate format first, then validate before persisting
- Log parsing errors with line numbers for debugging
- Never silently skip malformed rows — report and let the user review

### Cross-Layer Data Audit

When storing data with a new filterable field (e.g., a new `data_type`, `source`, or status value), verify all read paths are updated before declaring the task done:

1. **SQL queries** — every `SELECT` that filters on the field must include the new value (e.g., `data_type IN ('stock', 'fx')` not `data_type = 'stock'`)
2. **Repository methods** — confirm the repo returns the new data type to callers
3. **Service layer** — verify consumers handle the new data type (no silent drops)
4. **Tests** — add a test case with the new value exercising the full path (repo → service → output)

### Cross-Layer Field Mapping Audit

When adding new fields to shared types (e.g., `extractor.FundProfile`, `symbol.FundProfile`), every mapping layer must be audited before declaring the task done. A field added to the type definition is not enough — it must flow through all layers:

1. **Type definition** — field added to both `extractor` and `symbol` package types
2. **Parser** — field populated from source data
3. **Service mapping** (`extractResultToSymbolDetails`) — field mapped from `ExtractResult` to `SymbolDetails`
4. **Repository serialization** — field serialized in `toSQLNullJSON` and deserialized in `toSymbolDetail()`
5. **Web display** — field included in `toDisplayDetails()` and rendered in template
6. **Tests** — repo round-trip test covers the new field; integration test exercises the full path

**Rule of thumb**: After adding a field to a shared type, grep for every other field in the same struct and verify the new field appears in the same locations (mapping functions, serialization, display). If it doesn't, it will be silently dropped.
