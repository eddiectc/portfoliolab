---
project: arch-portfolio-lab
created: 2026-05-02
---

# Arch Portfolio Lab

A self-hosted investment portfolio management platform for personal investors to track stocks and ETFs across multiple accounts and currencies.

## Documentation

- [Project Overview](docs/PROJECT.md) — goals, non-goals, constraints, doc index
- [Coding Conventions](docs/CONVENTIONS.md) — style, naming, testing, domain, DB, API, web, security
- [FX Conventions](docs/FX_CONVENTIONS.md) — foreign exchange rate conventions
- [API Reference](API.md) — REST API endpoints, request/response schemas
- [Features](features/) — feature specs, plans, notes, retrospectives

## Problem Statement

Personal investors lack a simple, self-hosted tool to aggregate and analyze their investment portfolios across multiple brokers and currencies. Existing solutions are either cloud-based (privacy concerns), overly complex, or focused on institutional use cases. Arch Portfolio Lab fills this gap with a lightweight, privacy-first platform that provides deep P&L analytics without relying on third-party services.

## Feature List

### Must Have (v1)

- [x] **Web Interface** — Server-rendered HTML (Go templates), responsive, with tables, charts, and heatmaps
- [x] **Portfolio Management** — Create, read, update, delete portfolios (logical groupings of accounts)
- [x] **Account Management** — CRUD for brokerage accounts (e.g., "IBKR Main", "Trading 212 ISA")
- [x] **Transaction CRUD** — Manual entry of buys, sells, dividends, fees, and corporate actions
- [x] **Multi-Currency Support** — Accounts and transactions in different currencies; exchange rate tracking and conversion
- [ ] **CSV Import** — Generic CSV import with column mapping for transaction data
- [x] **Broker Import — IBKR Flex Report** — Parse IBKR XML flex reports and extract positions/transactions
- [x] **Broker Import — Trading 212** — Parse Trading 212 CSV export and extract positions/transactions
- [x] **Position Aggregation** — Real-time calculation of outstanding (open) positions across all accounts
- [x] **Closed Positions** — History of fully closed positions with realized P&L
- [x] **Cash Balance Tracking** — Per-account cash balance derived from transactions
- [x] **Portfolio Performance** — Equity curve (portfolio value vs net deposits), time-weighted return (TWR), annualized TWR, money-weighted return (MWR/IRR), simple return (profit/net deposit), annualized simple return, annualized volatility, drawdown analysis (max/current/duration), yearly performance, period selector, multi-currency FX conversion (using yfinance for prices)
- [x] **Historical P&L** — Time-series of portfolio value, daily returns, cumulative returns
- [x] **Detailed P&L Analysis** — Drawdown analysis, benchmark comparison (S&P 500, NASDAQ, custom), sector/currency breakdown, win rate, avg hold period
- [x] **Portfolio Comparison** — Side-by-side comparison of any two portfolios (model vs model, model vs real, real vs real) with performance (TWR, CAGR, money-weighted return), risk (Sharpe, Sortino, volatility), drawdown, return distribution, holdings overlap, and cross-portfolio correlation/beta/alpha

### Nice to Have (v2)

- [ ] **Mobile App** — Leverage the API layer for a native mobile frontend
- [ ] **Additional Broker Imports** — Schwab, Fidelity, eToro, etc.
- [ ] **Tax Lot Tracking** — FIFO, LIFO, specific lot identification for tax reporting
- [ ] **Watchlist** — Track symbols not yet in the portfolio
- [ ] **Notifications** — Price alerts, portfolio milestones
- [ ] **Data Export** — Export portfolio data and reports

### Out of Scope

- **Trading execution** — This is a tracking tool, not a trading platform
- **Multi-user/multi-tenant** — Single-user personal tool
- **Authentication** — Self-hosted; rely on reverse proxy (e.g., Caddy with basic auth) for access control

## High-Level Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        Web Browser                            │
│         (Server-rendered HTML pages + JS charts/tables)       │
└──────────────────────────────┬───────────────────────────────┘
                               │ HTTP/REST
┌──────────────────────────────▼───────────────────────────────┐
│                  Go Backend (Single Binary)                   │
│                                                               │
│  ┌───────────┐  ┌───────────┐  ┌──────────────┐ ┌─────────┐ │
│  │  HTTP     │  │  HTML     │  │  Import      │ │ Market  │ │
│  │  API      │  │  Renderer │  │  Parsers     │ │ Data    │ │
│  │  Routes   │  │  (Pages)  │  │  (CSV, XML)  │ │ Fetcher │ │
│  └─────┬─────┘  └─────┬─────┘  └──────┬───────┘ │(yfinance)│ │
│        │              │                │         └────┬────┘ │
│  ┌─────▼──────────────▼────────────────▼──────────────▼────┐ │
│  │                   Domain Layer                           │ │
│  │  Portfolio │ Account │ Transaction │ Position │ Analytics│ │
│  │  P&L Calc  │ Cash    │ Import      │ Currency │ Bench   │ │
│  └─────┬───────────────────────────────────────────────────┘ │
│        │                                                     │
│  ┌─────▼───────────────────────────────────────────────────┐ │
│  │                   Data Layer                             │ │
│  │  SQLite (single file, WAL mode)                          │ │
│  └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### Key Design Decisions

1. **Single Go binary** — Simple deployment, no build pipeline complexity
2. **API-first** — All web pages consume the same API; enables future mobile app
3. **Server-rendered pages** — No SPA complexity; pages are fast and simple
4. **No auth in the app** — Rely on infrastructure (reverse proxy) for security
5. **SQLite** — Single-user tool; no DB server process, single file, WAL mode for concurrent reads, pure Go driver (`modernc.org/sqlite`)
6. **go-yfinance for market data** — Fetches stock/ETF prices and benchmark data via `github.com/wnjoon/go-yfinance` (pure Go, no Python dependency)
7. **Benchmarks** — S&P 500 (^GSPC), NASDAQ Composite (^IXIC), and user-defined custom benchmarks

## Tech Stack

| Component | Choice | Rationale |
|---|---|---|
| **Language** | Go | Single binary, fast, great for data processing, self-hosting friendly |
| **Web Framework** | `chi` or std `net/http` | Minimal routing; chi adds middleware support |
| **HTML Templating** | `html/template` (std) | Built-in, secure against XSS, no build step |
| **Database** | SQLite (`modernc.org/sqlite`) | Single file, no server, WAL mode, pure Go (no CGO) |
| **Queries** | `sqlc` | Type-safe SQL; generates Go types from queries |
| **Migrations** | `goose` (SQL migrations) | Versioned .sql migration files, SQLite driver |
| **Charts** | Apache ECharts | Excellent financial charting (candlestick, heatmap, waterfall), interactive |
| **Tables** | Plain HTML + DataTables.js | Sortable, searchable, paginated tables with minimal JS |
| **Heatmaps** | ECharts heatmap series | For sector/currency/allocation visualization |
| **CSV Parsing** | `encoding/csv` (std) | Built-in, reliable |
| **XML Parsing** | `encoding/xml` (std) | For IBKR flex reports |
| **Market Data** | `github.com/wnjoon/go-yfinance` | Pure Go yfinance client, no Python dependency |
| **Config** | YAML config file | Simple, no env var sprawl |
| **Logging** | `slog` (std) | Structured logging, built into Go 1.21+ |
| **Unit Testing** | `testing` (std) + hand-written mocks | Minimal mock structs in `*_test.go`; Arrange-Act-Assert |
| **Integration Testing** | In-memory SQLite + httptest | Real SQL, real schema, isolated per test |
| **Build** | Makefile | Simple build targets |
| **Deployment** | Docker (optional) + binary | Run directly or containerized |

## Required Tools

| Tool | Install |
|---|---|
| **Go** (1.21+) | https://go.dev/doc/install |
| **sqlc** | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` |
| **goose** | `go install github.com/pressly/goose/v3/cmd/goose@latest` |
| **goimports** | `go install golang.org/x/tools/cmd/goimports@latest` |

## Testing Strategy

### Unit Tests (co-located with source: `*_test.go`)
- **Scope**: Pure domain logic — P&L calculations, position aggregation, currency conversion, import parsing, benchmark math
- **Approach**: Hand-written mocks — minimal mock structs co-located in `*_test.go` files, simulating real repository behavior
- **Pattern**: Arrange-Act-Assert; table-driven tests (`[]struct{name, input, want}`)
- **Target**: 80%+ coverage on `internal/domain/`
- **No DB, no network** — unit tests run fast and deterministically

### Integration Tests (`tests/integration/`)
- **Scope**: Repository layer + domain services together; API handlers with `httptest`
- **Approach**: In-memory SQLite (`file::memory:?cache=shared`) with real schema via goose migrations
- **Isolation**: Each test gets a fresh in-memory DB; run migrations before each test
- **No mocking** — test the real SQL and real data flow

### API E2E Tests (`tests/e2e/api/`)
- **Scope**: Full HTTP request → response cycle through real routes
- **Approach**: Start the real router with in-memory DB; hit endpoints via `httptest.NewServer`
- **Assertions**: HTTP status codes, JSON response shapes, content-type headers

### Web UI Tests
- **Scope**: Minimal — template rendering only (no browser E2E)
- **Rationale**: Server-rendered pages are thin wrappers around API data; if the API is correct and templates compile, the UI is correct
- **Optional**: Add a few `html/template` rendering tests to verify no panics on edge-case data

### Other Tests
- **Import parsing**: Fixture-driven with real (anonymized) broker export files
- **Market data**: Mock go-yfinance responses; test price parsing and benchmark calculations
- **Tools**: `testing` (std), hand-written mocks (no testify/mockery)

## Definition of Done

- [ ] All must-have features implemented
- [ ] Unit tests passing at 80%+ coverage on domain layer
- [ ] Integration tests passing for all API endpoints
- [ ] Import parsers tested against real broker file samples
- [ ] P&L calculations verified against known-correct spreadsheets
- [ ] Configuration documented (config.example.yaml)
- [ ] README.md complete with setup instructions
- [ ] Database migrations idempotent and documented (via goose)
- [ ] Market data fetching works (go-yfinance, prices cached)
- [ ] Single binary builds cleanly with `go build`
- [ ] No critical bugs in position calculation or P&L analysis

## Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| P&L calculation bugs | High — wrong numbers erode trust | Extensive unit tests + manual verification against spreadsheet |
| IBKR flex report schema changes | Medium — import breaks | Version detection in XML parser; graceful degradation |
| Multi-currency edge cases | Medium — rounding errors | Use integer arithmetic (minor units) internally; document rounding rules |
| go-yfinance API changes | Medium — upstream Yahoo Finance changes | Version-pin the library; graceful fallback if fetch fails |
| SQLite concurrency | Low — single user | WAL mode; single writer pattern (sequential imports) |
| Performance with large histories | Low — personal use | Index optimization; paginate queries; defer heavy analytics |
