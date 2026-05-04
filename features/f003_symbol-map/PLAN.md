# Implementation Plan: symbol-map

## Overview

Build the symbol mapping infrastructure: database schema, domain layer, repository, REST API, and web CRUD UI. Includes optional market data preview via Yahoo Finance (soft dependency — feature works without it).

## Data Model

### Relationships

```
symbol_mappings (1) ──── (N) broker_symbol_mappings
     │
     └── internal_symbol (UNIQUE) ──┬── market_data_symbol (Yahoo Finance ticker)
                                    └── broker symbols (via child table)
```

One internal symbol maps to exactly one market data provider symbol.
Multiple broker symbols (from different brokers) can map to the same internal symbol.
A broker symbol from a specific broker maps to exactly one internal symbol.

### Database Schema

```sql
-- symbol_mappings: core mapping from internal symbol to market data provider
CREATE TABLE symbol_mappings (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    internal_symbol     TEXT    NOT NULL UNIQUE,
    market_data_symbol  TEXT    NOT NULL,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- broker_symbol_mappings: broker-specific symbols mapped to an internal symbol
CREATE TABLE broker_symbol_mappings (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol_mapping_id   INTEGER NOT NULL,
    broker_name         TEXT    NOT NULL,
    broker_symbol       TEXT    NOT NULL,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (symbol_mapping_id) REFERENCES symbol_mappings(id) ON DELETE CASCADE,
    UNIQUE(broker_name, broker_symbol)
);

CREATE INDEX idx_broker_symbol_mappings_mapping_id
    ON broker_symbol_mappings(symbol_mapping_id);
```

**Key constraints:**
- `internal_symbol` UNIQUE — no duplicate internal symbols
- `UNIQUE(broker_name, broker_symbol)` — same broker + symbol pair maps to only one internal symbol
- `ON DELETE CASCADE` — deleting a symbol mapping removes its broker symbols
- Index on `symbol_mapping_id` for efficient lookup of broker symbols by mapping

### Go Domain Model

```go
// symbolmapping/symbol_mapping.go

type BrokerSymbol struct {
    ID         int64  `json:"id"`
    BrokerName string `json:"broker_name"`
    BrokerSymbol string `json:"broker_symbol"`
}

type SymbolMapping struct {
    ID               int64        `json:"id"`
    InternalSymbol   string       `json:"internal_symbol"`
    MarketDataSymbol string       `json:"market_data_symbol"`
    BrokerSymbols    []BrokerSymbol `json:"broker_symbols"`
    CreatedAt        time.Time    `json:"created_at"`
    UpdatedAt        time.Time    `json:"updated_at"`
}

type CreateRequest struct {
    InternalSymbol   string       `json:"internal_symbol"`
    MarketDataSymbol string       `json:"market_data_symbol"`
    BrokerSymbols    []BrokerSymbolRequest `json:"broker_symbols,omitempty"`
}

type BrokerSymbolRequest struct {
    BrokerName   string `json:"broker_name"`
    BrokerSymbol string `json:"broker_symbol"`
}

type UpdateRequest struct {
    InternalSymbol   *string `json:"internal_symbol,omitempty"`
    MarketDataSymbol *string `json:"market_data_symbol,omitempty"`
}
```

### Market Data Quote (for preview)

```go
// market/quote.go

import "github.com/govalues/decimal"

type Quote struct {
    Symbol      string          `json:"symbol"`
    Name        string          `json:"name"`
    Exchange    string          `json:"exchange"`
    Currency    string          `json:"currency"`
    LatestPrice decimal.Decimal `json:"latest_price"`
}

type QuoteFetcher interface {
    FetchQuote(ctx context.Context, symbol string) (*Quote, error)
}
```

## Task Dependencies

```
Task 1 (Migration + Repository)
    ↓
Task 2 (Domain Model + Service)
    ↓
Task 3 (REST API Handlers)
    ↓
Task 4 (Web UI: Templates + Handlers)
    ↓
Task 5 (Router Wiring + Nav)
    ↓
Task 6 (Market Data Preview) — soft dep, non-blocking
```

Tasks 1–5 are sequential. Task 6 can be done after Task 4 and is non-blocking (preview gracefully degrades if market data is unavailable).

## Tasks

### Task 1: Database migration and repository [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundation for everything)

**Description:** Create the database schema for symbol mappings and broker symbol associations. Implement the repository with all CRUD operations.

- [ ] Create migration `003_create_symbol_mappings.sql` with two tables:
  - `symbol_mappings` (id, internal_symbol UNIQUE, market_data_symbol, created_at, updated_at)
  - `broker_symbol_mappings` (id, symbol_mapping_id FK, broker_name, broker_symbol, created_at; UNIQUE(broker_name, broker_symbol))
- [ ] Add sqlc query files (`internal/data/queries/symbol_mapping.sql`) for all needed queries
- [ ] Run `sqlc generate` (or hand-write queries if sqlc unavailable, following existing pattern)
- [ ] Create `SymbolMappingRepository` in `internal/data/symbol_mapping_repo.go` implementing:
  - `Create`, `GetByID`, `GetByInternalSymbol`, `GetAll` (with pagination)
  - `Update`, `Delete`
  - `AddBrokerSymbol`, `GetBrokerSymbolByBroker`, `HasReferencingTransactions` (stub — returns false until transaction feature exists)
- [ ] Handle sqlc string timestamps with existing `parseTime()` pattern
- [ ] Write repository unit tests (mock sqlc queries or use in-memory SQLite)

**Verification:** `go test ./internal/data/...` passes; migration runs cleanly with `goose up`.

### Task 2: Domain model and service [PRIORITY: HIGH]

**Corresponds to:** Create, Update, Delete, Add broker symbol, Duplicate prevention, Broker symbol conflict scenarios

**Description:** Define the domain model, DTOs, service interface, and business logic for symbol mappings.

- [ ] Create `internal/domain/symbolmapping/symbol_mapping.go` with:
  - `SymbolMapping` struct (ID, InternalSymbol, MarketDataSymbol, BrokerSymbols []BrokerSymbol, CreatedAt, UpdatedAt)
  - `BrokerSymbol` struct (ID, BrokerName, BrokerSymbol)
  - `CreateRequest`, `UpdateRequest` DTOs
- [ ] Create `internal/domain/symbolmapping/service.go` with:
  - `Repository` interface (matching repo methods)
  - Service errors: `ErrNotFound`, `ErrInternalSymbolExists`, `ErrBrokerSymbolExists`, `ErrInvalidSymbol`, `ErrInUse`
  - `Service` struct with `Create`, `Get`, `List`, `Update`, `Delete`, `AddBrokerSymbol` methods
  - Validation: non-empty/trimmed symbols, max length (~20 chars for ticker symbols)
  - Uniqueness checks: internal symbol, broker symbol (broker_name + broker_symbol pair)
  - Delete guard: check `HasReferencingTransactions`
- [ ] Create hand-written mock repository in `internal/domain/symbolmapping/mock_repository.go` (following portfolio pattern)
- [ ] Write comprehensive service unit tests in `service_test.go` (table-driven, covering all scenarios)

**Verification:** `go test ./internal/domain/symbolmapping/...` passes; all 12 spec scenarios covered by service tests.

### Task 3: REST API handlers [PRIORITY: HIGH]

**Corresponds to:** All CRUD scenarios (API consumers)

**Description:** Create JSON API handlers for symbol mapping CRUD operations.

- [ ] Create `internal/api/handlers/symbol_mapping.go` with:
  - `SymbolMappingHandler` struct
  - `RegisterRoutes` mounting: GET/POST `/api/symbol-mappings`, GET/PATCH/DELETE `/api/symbol-mappings/{id}`, POST `/api/symbol-mappings/{id}/broker-symbols`
  - `HandleList` (GET, with pagination)
  - `HandleCreate` (POST, accepts CreateRequest)
  - `HandleGet` (GET by ID)
  - `HandleUpdate` (PATCH, accepts UpdateRequest)
  - `HandleDelete` (DELETE by ID)
  - `HandleAddBrokerSymbol` (POST to mapping's broker symbols)
  - `handleServiceError` mapping domain errors to HTTP responses
- [ ] Write handler unit tests in `symbol_mapping_test.go` (mock service, verify HTTP status codes and response bodies)

**Verification:** `go test ./internal/api/handlers/... -run Symbol` passes; API returns correct status codes and error formats.

### Task 4: Web UI — templates and handlers [PRIORITY: HIGH]

**Corresponds to:** Create via CRUD page, View list, Update, Delete scenarios

**Description:** Create server-rendered web pages for managing symbol mappings.

- [ ] Create template `templates/symbol_mapping/list.html`:
  - Table showing: internal symbol, market data symbol, broker symbols, actions (edit/delete)
  - "New Symbol Mapping" button
  - Search/filter input (client-side or server-side — follow portfolio pattern)
  - Empty state message
- [ ] Create template `templates/symbol_mapping/form.html`:
  - Fields: internal symbol, market data provider symbol
  - Optional: add broker symbols (broker name + broker symbol pairs, repeatable)
  - Error display area
  - Submit/Cancel buttons
  - Market data preview area (placeholder for Task 6)
- [ ] Create `internal/api/handlers/symbol_mapping_web.go` with:
  - `SymbolMappingWebHandler` struct
  - `RegisterRoutes` mounting: GET `/symbol-mappings`, GET/POST `/symbol-mappings/new`, GET/POST `/symbol-mappings/{id}/edit`, POST `/symbol-mappings/{id}/delete`
  - `HandleListPage` — list all mappings
  - `HandleNewPage` — render empty form
  - `HandleCreatePage` — process form submission, redirect on success
  - `HandleEditPage` — render form pre-populated
  - `HandleUpdatePage` — process form submission
  - `HandleDeletePage` — delete with confirmation
  - Flash messages for success/error feedback
  - `userFriendlyError` for service errors
- [ ] Write web handler tests in `symbol_mapping_web_test.go` (follow portfolio_web_test.go pattern)

**Verification:** Web pages render correctly; form submissions create/update/delete mappings; flash messages appear.

### Task 5: Router wiring and navigation [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (integration)

**Description:** Wire the symbol mapping handlers into the application router and add navigation link.

- [ ] Update `internal/api/router.go`:
  - Create `SymbolMappingRepository`, `SymbolMappingService`, `SymbolMappingHandler`, `SymbolMappingWebHandler`
  - Register routes
- [ ] Update `templates/partials/nav.html`:
  - Add "Symbol Maps" link (or whatever label you prefer)
- [ ] Verify full application builds and runs: `go build -o portfoliolab cmd/server/main.go`

**Verification:** Application starts; `/symbol-mappings` page loads; API endpoints respond.

### Task 6: Market data preview (soft dependency) [PRIORITY: MEDIUM]

**Corresponds to:** Market data preview during creation/update scenarios

**Description:** Add debounced market data preview when typing the market data provider symbol. Fetches quote from Yahoo Finance. Non-blocking — gracefully degrades if unavailable.

- [ ] Add `github.com/wnjoon/go-yfinance` and `github.com/govalues/decimal` to `go.mod`
- [ ] Create `internal/market/quote.go` with:
  - `Quote` struct (Symbol, Name, Exchange, Currency, LatestPrice)
  - `QuoteFetcher` interface: `FetchQuote(ctx context.Context, symbol string) (*Quote, error)`
  - `YahooFinanceFetcher` implementing `QuoteFetcher` using go-yfinance
  - Error handling: log warning, return nil (caller handles gracefully)
- [ ] Update `internal/domain/symbolmapping/service.go`:
  - Add optional `QuoteFetcher` dependency to Service (nil-safe)
  - Add `PreviewSymbol(ctx, marketDataSymbol string) (*Quote, error)` method
- [ ] Create API endpoint in `internal/api/handlers/symbol_mapping.go`:
  - `GET /api/symbol-mappings/preview?symbol=AAPL`
  - Returns JSON quote or `{"error": "could not fetch data"}` (non-blocking)
- [ ] Update `templates/symbol_mapping/form.html`:
  - Add preview area below market data symbol input
  - Add JavaScript for debounced fetch (2-second pause)
  - Display: name, exchange, currency, latest price
  - Show warning icon/text on fetch failure
- [ ] Update router to inject `YahooFinanceFetcher` into service and handler
- [ ] Write unit tests for quote fetcher (mock HTTP) and preview endpoint

**Verification:** Typing a symbol shows preview after 2s pause; invalid symbols show warning; creation proceeds regardless of preview success/failure.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Database schema | Two tables: `symbol_mappings` + `broker_symbol_mappings` | Follows existing FK pattern (like portfolios → accounts); enables multiple broker symbols per mapping; UNIQUE constraint on (broker_name, broker_symbol) prevents duplicates |
| sqlc queries | Hand-written (sqlc not installed) | Follows existing constraint noted in PROJECT.md; query files prepared for when sqlc becomes available |
| Broker symbols in CreateRequest | Included as optional slice in DTO | Matches spec: "optionally associate one or more broker symbols with the mapping" at creation time |
| Market data preview | Client-side debounced AJAX call | Follows existing pattern of client-side JS (ECharts); 2-second debounce avoids excessive API calls; non-blocking per spec |
| Delete guard for transactions | Stub returning `false` in repo | Transaction feature doesn't exist yet; stub allows delete to work now; real check added when transaction feature is built |
| Symbol validation | Non-empty, trimmed, max 20 chars | Ticker symbols are short; 20 chars covers exotic formats like `BRK.A.LON` |
| go-yfinance addition | Added to go.mod for Task 6 | Already planned per PROJECT.md; pure Go, no Python dependency |
| govalues/decimal | Added to go.mod; replaces int64 minor units convention | Exact decimal arithmetic for all monetary values; stored as TEXT in SQLite; repo layer handles decimal ↔ string conversion; avoids float64 precision issues in P&L math; faster, no heap allocations, correctly rounded, panic-free |

## Risks

- **go-yfinance API changes**: Yahoo Finance doesn't guarantee API stability. Mitigation: graceful degradation — preview failure doesn't block the feature.
- **sqlc not installed**: Queries must be hand-written, following the existing pattern in `internal/data/queries/`. This adds manual work but is already the project convention.
- **Transaction delete guard is a stub**: The `HasReferencingTransactions` check returns `false` until the transaction feature exists. This is acceptable since no transactions exist yet, but must be updated when that feature is built. Document in NOTES.md.
