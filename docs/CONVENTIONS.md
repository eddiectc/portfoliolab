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
- **Mock all external deps**: Define interfaces for repos and fetchers; generate mocks with `mockery`
- **Mocks must simulate real behavior**: If the real implementation would return empty/error for edge-case inputs (e.g. `limit=0` → `LIMIT 0` → zero rows), the mock must do the same. Permissive mocks hide bugs in the service/handler layer.
- **Table-driven tests**: Use `[]struct{name, input, want}` for comprehensive coverage
- **Arrange-Act-Assert**: Clear separation; no setup in the act phase
- **No DB, no network**: Unit tests run fast and deterministically
- **Use `-short` flag**: Skip integration-only tests with `if testing.Short() { t.Skip() }`
- **Integration tests**: In-memory SQLite (`file::memory:?cache=shared`) with goose migrations
- **API integration**: `httptest.NewRecorder` + real router + in-memory DB

## Domain Logic
- **Position calculations** are the heart of the app — be extra careful with P&L math
- Use integer arithmetic (minor units) for money to avoid floating-point issues
- Document all rounding rules and edge cases in code comments

## Database (SQLite)
- Use `sqlc` for type-safe queries — write SQL, generate Go
- Driver: `modernc.org/sqlite` (pure Go, no CGO)
- Migrations via `goose`; SQLite-compatible SQL only
- Enable WAL mode: `PRAGMA journal_mode=WAL`
- Use `BIGINT` for monetary amounts (stored in minor units)
- For integration tests, use `file::memory:?cache=shared` for in-memory SQLite
- Timestamps: `modernc.org/sqlite` stores datetime as TEXT and doesn't scan into `time.Time`. sqlc generates `string` for timestamp columns; the repo layer converts with `parseTime()` (handles RFC3339 and SQLite format) and formats with `.Format(time.RFC3339)` on write.

## API Design
- Consistent error responses: `{"error": "message", "code": "ERROR_CODE"}`
- Use standard HTTP status codes
- Paginate list endpoints with `?limit=&offset=` or cursor-based
- Return ETags for cacheable resources

## Web UI
- Keep templates simple — no complex logic in templates
- Use partials for reusable components (nav, footer, table rows)
- Charts are rendered client-side with ECharts; pass data as JSON in `<script>` tags
- Static assets are served from `internal/web/static/`
- Go template rendering: parse layout + each page file separately to avoid template name conflicts

## Market Data (go-yfinance)
- Use `github.com/wnjoon/go-yfinance` for fetching prices (pure Go, no Python)
- Cache fetched prices in SQLite to avoid repeated API calls
- Handle fetch failures gracefully — log warning, serve stale data
- Never block the main HTTP server on market data fetches — use background goroutines

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
