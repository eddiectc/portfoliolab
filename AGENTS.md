# AGENTS.md — Agentic Coding Instructions

## Project Context

Arch Portfolio Lab is a self-hosted investment portfolio management platform built in Go. It tracks stocks and ETFs across multiple accounts and currencies, with deep P&L analytics including drawdown analysis and benchmark comparison (S&P 500, NASDAQ, custom). Market data is fetched via `go-yfinance` (pure Go). The web UI is server-rendered (Go templates), and all functionality is exposed via a REST API for future mobile app support. Database is SQLite (single file, WAL mode).

## Conventions

### Coding Style
- Follow standard Go formatting: `gofmt` / `goimports`
- Use `errcheck` — never ignore errors silently
- Prefer composition over embedding in domain models
- Use interfaces sparingly — define them where needed (e.g., repositories, fetchers)
- Struct tags: `json` for API, `db` for sqlc, `yaml` for config

### Naming Conventions
- Packages: lowercase, short, no underscores (e.g., `portfolio`, `analytics`)
- Handlers: `HandleCreatePortfolio`, `HandleListTransactions`
- Services: `PortfolioService`, `PositionAggregator`
- Repositories: `PortfolioRepository`, `TransactionRepository`
- Domain models: singular nouns (`Portfolio`, `Transaction`, `Position`)
- DTOs: suffixed with `Request`/`Response` (e.g., `CreateTransactionRequest`)
- Interfaces: suffixed with `er` where natural (`Fetcher`, `Repository`) or descriptive (`PositionAggregator`)

### File Organization
- `cmd/` — application entry points
- `internal/` — private application code (never imported externally)
  - `api/` — HTTP handlers and middleware
  - `domain/` — business logic and services
  - `data/` — data access / repositories
  - `market/` — go-yfinance integration and benchmark data
  - `web/` — HTML rendering and static assets
- `templates/` — Go HTML templates
- `migrations/` — SQL migration files (SQLite-compatible, managed by goose)
- `tests/` — test files and fixtures

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

# Generate mocks
mockery --all

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
Project docs: `docs/PROJECT.md`, `docs/CONVENTIONS.md`.
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

## Best Practices

### Unit Testing
- **Co-locate tests**: `domain/transaction/calculator_test.go` next to `calculator.go`
- **Mock all external deps**: Define interfaces for repos and fetchers; generate mocks with `mockery`
- **Mocks must simulate real behavior**: If the real implementation returns empty/error for edge-case inputs (e.g. `limit=0` → `LIMIT 0` → zero rows), the mock must do the same. Permissive mocks hide bugs in the service/handler layer.
- **Table-driven tests**: Use `[]struct{name, input, want}` for comprehensive coverage
- **Arrange-Act-Assert**: Clear separation; no setup in the act phase
- **No DB, no network**: Unit tests run fast and deterministically
- **Use `-short` flag**: Skip integration-only tests with `if testing.Short() { t.Skip() }`

### Integration Testing
- **Fresh DB per test**: In-memory SQLite (`file::memory:?cache=shared`) with goose migrations
- **Real SQL, real schema**: No mocking at the repo layer in integration tests
- **Shared test helper**: `tests/integration/db.go` — opens in-memory DB, runs migrations, returns cleanup func
- **API integration**: `httptest.NewRecorder` + real router + in-memory DB

### Domain Logic
- **Position calculations** are the heart of the app — be extra careful with P&L math
- Use integer arithmetic (minor units) for money to avoid floating-point issues
- Document all rounding rules and edge cases in code comments
- The `calculator.go` in the transaction domain is critical — test exhaustively

### Import Parsers
- Parse broker files into an intermediate format first, then validate before persisting
- Log parsing errors with line numbers for debugging
- Never silently skip malformed rows — report and let the user review

### Market Data (go-yfinance)
- Use `github.com/wnjoon/go-yfinance` for fetching prices (pure Go, no Python)
- Cache fetched prices in SQLite to avoid repeated API calls
- Handle fetch failures gracefully — log warning, serve stale data
- Benchmark data follows the same fetch/cache pattern
- Never block the main HTTP server on market data fetches — use background goroutines

### API Design
- Consistent error responses: `{"error": "message", "code": "ERROR_CODE"}`
- Use standard HTTP status codes
- Paginate list endpoints with `?limit=&offset=` or cursor-based
- Return ETags for cacheable resources

### Database (SQLite)
- Use `sqlc` for type-safe queries — write SQL, generate Go
- Driver: `modernc.org/sqlite` (pure Go, no CGO)
- Migrations via `goose`; SQLite-compatible SQL only (no JSONB, use TEXT + manual JSON)
- Enable WAL mode: `PRAGMA journal_mode=WAL`
- Use `BIGINT` for monetary amounts (stored in minor units)
- For integration tests, use `file::memory:?cache=shared` for in-memory SQLite
- **sqlc workflow**: Add SQL to `internal/data/queries/*.sql`, run `sqlc generate` from that dir. Repos delegate to `queries.Queries` and handle domain ↔ sqlc type mapping (timestamps are `string` in sqlc models — convert with `parseTime()` / `.Format(time.RFC3339)` in the repo layer)

### Web UI
- Keep templates simple — no complex logic in templates
- Use partials for reusable components (nav, footer, table rows)
- Charts are rendered client-side with ECharts; pass data as JSON in `<script>` tags
- Static assets are served from `internal/web/static/`

## Security Notes

- No authentication in the app — assume reverse proxy handles access control
- Never log sensitive data (account balances, personal info)
- Validate all user input — especially CSV/XML imports
- Use parameterized queries exclusively (no string concatenation for SQL)
- SQLite file permissions: ensure the DB file is not world-readable
