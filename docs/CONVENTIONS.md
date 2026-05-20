# Coding Conventions

## Code Style
- Follow `gofmt` / `goimports` formatting
- Run `errcheck` — never ignore errors silently
- Prefer composition over embedding in domain models
- Use interfaces sparingly — define them where needed (e.g., repositories, fetchers)
- Struct tags: `json` for API, `db` for sqlc, `yaml` for config

## File Organization
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

## Naming
- Packages: lowercase, short, no underscores (e.g., `portfolio`, `analytics`)
- Handlers: `HandleCreatePortfolio`, `HandleListTransactions`
- Services: `PortfolioService`, `PositionAggregator`
- Repositories: `PortfolioRepository`, `TransactionRepository`
- Domain models: singular nouns (`Portfolio`, `Transaction`, `Position`)
- DTOs: suffixed with `Request`/`Response` (e.g., `CreateTransactionRequest`)
- Interfaces: suffixed with `er` where natural (`Fetcher`, `Repository`) or descriptive (`PositionAggregator`)

## Testing
- **Co-locate tests**: `calculator_test.go` next to `calculator.go`
- **Hand-written mocks only**: Define minimal mock structs co-located in `*_test.go` files (no mockery, no testify). Follow the `account/service_test.go` pattern: mocks maintain internal state (maps, slices) and simulate real repository behavior.
- **Mocks must simulate real behavior**: If the real implementation would return empty/error for edge-case inputs (e.g. `limit=0` → `LIMIT 0` → zero rows), the mock must do the same. Permissive mocks hide bugs in the service/handler layer.
- **Table-driven tests**: Use `[]struct{name, input, want}` for comprehensive coverage
- **Arrange-Act-Assert**: Clear separation; no setup in the act phase
- **No DB, no network**: Unit tests run fast and deterministically
- **Use `-short` flag**: Skip integration-only tests with `if testing.Short() { t.Skip() }`
- **Integration tests**: In-memory SQLite (`file::memory:?cache=shared`) with goose migrations
- **Shared test helper**: `tests/integration/db.go` — opens in-memory DB, runs migrations, returns cleanup func
- **API integration**: `httptest.NewRecorder` + real router + in-memory DB

## Domain Logic
- **Position calculations** are the heart of the app — be extra careful with P&L math
- Use `github.com/govalues/decimal` for all monetary values (prices, costs, P&L) — never `float64`
- Decimal is stored as `TEXT` in SQLite; repo layer handles `decimal.Decimal` ↔ string conversion
- Document all rounding rules and edge cases in code comments
- **Trade suggestions need price resolution for unheld symbols** — when computing rebalancing or trade suggestions, symbols in the target allocation may not yet be held. The interface (e.g., `PositionSource`) must expose a price lookup method (e.g., `GetMarketPrice`) independent of position enrichment.

## Database (SQLite)
- Use `sqlc` for type-safe queries — write SQL, generate Go
- Driver: `modernc.org/sqlite` (pure Go, no CGO)
- Migrations via `goose`; SQLite-compatible SQL only
- Enable WAL mode: `PRAGMA journal_mode=WAL`
- Use `TEXT` for monetary amounts (stores `decimal.Decimal` as string); repo layer converts to/from `decimal.Decimal`
- For integration tests, use `file::memory:?cache=shared` for in-memory SQLite
- Timestamps: `modernc.org/sqlite` stores datetime as TEXT and doesn't scan into `time.Time`. sqlc generates `string` for timestamp columns; the repo layer converts with `parseTime()` (handles RFC3339 and SQLite format) and formats with `.Format(time.RFC3339)` on write.

## API Design
- Consistent error responses: `{"error": "message", "code": "ERROR_CODE"}`
- Use standard HTTP status codes
- Paginate list endpoints with `?limit=&offset=` or cursor-based
- Return ETags for cacheable resources
- **Explicit errors, no silent fallbacks** — if a required prerequisite can't be resolved (e.g., no base currency, missing portfolio), return an explicit error response. Never silently return zeros, empty data, or degraded results that mask the underlying failure. The caller must know *why* something failed.

## API-First Architecture

The API is the **single source of truth** for all business logic and data computation. The web UI is a thin presentation layer on top.

- **Web handlers delegate to the API** — web handlers should not duplicate computation logic. They call the same service methods the API uses, then add only presentation concerns (chart serialization, template data shaping, URL building).
- **API responses are mobile-ready** — API endpoints return complete, self-contained data. If a mobile client needs the same data as the web page, it should be available from the API without extra endpoints.
- **No computation in web handlers** — data transformation, aggregation, and analytics live in the domain/service layer. Web handlers only:
  - Parse request params → build filter structs
  - Call service/API methods
  - Serialize chart data (JSON for ECharts)
  - Build template data (URLs, labels, page metadata)
  - Render templates
- **Shared computation** — if both API and web need the same derived data (e.g., monthly returns), compute it once in the service layer and include it in the result struct. Don't compute separately in each handler.

## Web UI
- Keep templates simple — no complex logic in templates
- Use partials for reusable components (nav, footer, table rows)
- Charts are rendered client-side with ECharts; pass data as JSON in `<script>` tags
- Static assets are served from `internal/web/static/`
- Go template rendering: parse layout + each page file separately to avoid template name conflicts
- **URLs in templates**: Pre-build full URLs in the handler (e.g., `RefreshURL`, `PeriodURLs` map) instead of using template expressions inside `href`/`action` attributes. Go's `html/template` treats expressions in URLs as "ambiguous context" — it can't distinguish `&` as an HTML entity vs. a query parameter separator.
- **Map access in templates**: Use `{{index .Map "key"}}` for map access with string keys containing digits or special characters. Dot notation like `{{.Map."1W"}}` causes a parse error (`bad character U+0022 '"'`).

## Market Data (go-yfinance)
- Use `github.com/wnjoon/go-yfinance` for fetching prices (pure Go, no Python)
- Cache fetched prices in SQLite to avoid repeated API calls
- Handle fetch failures gracefully — log warning, serve stale data
- Never block the main HTTP server on market data fetches — use background goroutines

## Import Parsers
- Parse broker files into an intermediate format first, then validate before persisting
- Log parsing errors with line numbers for debugging
- Never silently skip malformed rows — report and let the user review

## Security
- No authentication in the app — assume reverse proxy handles access control
- Never log sensitive data (account balances, personal info)
- Validate all user input — especially CSV/XML imports
- Use parameterized queries exclusively (no string concatenation for SQL)
- SQLite file permissions: ensure the DB file is not world-readable

## Build
- `go build -o portfoliolab cmd/server/main.go`
- Use `GOCACHE=/tmp/go-cache GOPATH=/tmp/go-path` due to read-only `~/.cache/go-build/`
- Use `-a` flag to force rebuild when cache is stale

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
