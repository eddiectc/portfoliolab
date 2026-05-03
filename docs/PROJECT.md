# Project: Arch Portfolio Lab

## Description
A self-hosted investment portfolio management platform for personal investors to track stocks and ETFs across multiple accounts and currencies. Provides deep P&L analytics (drawdown analysis, benchmark comparison) without relying on third-party services. Single-user, privacy-first.

## Goals
- Aggregate and analyze investment portfolios across multiple brokers and currencies
- Provide deep P&L analytics (drawdown, benchmark comparison, sector/currency breakdown)
- Support multiple broker import formats (IBKR Flex Report, Trading 212 CSV, generic CSV)
- Track positions, cash balances, and historical performance
- Self-hosted, single binary, no external dependencies beyond market data fetching

## Non-Goals
- Trading execution — this is a tracking tool, not a trading platform
- Multi-user/multi-tenant — single-user personal tool
- Authentication — rely on reverse proxy (e.g., Caddy with basic auth) for access control

## Architecture
```
Web Browser (HTML + JS charts)
        │ HTTP/REST
Go Backend (Single Binary)
  ┌───────────┐  ┌───────────┐  ┌──────────────┐ ┌─────────┐
  │  HTTP API │  │  HTML     │  │  Import      │ │ Market  │
  │  Routes   │  │  Renderer │  │  Parsers     │ │ Data    │
  └─────┬─────┘  └─────┬─────┘  └──────┬───────┘ │(yfinance)│
        │              │                │         └────┬────┘
  ┌─────▼──────────────▼────────────────▼──────────────▼────┐
  │                   Domain Layer                           │
  │  Portfolio │ Account │ Transaction │ Position │ Analytics│
  └─────┬───────────────────────────────────────────────────┘
        │
  ┌─────▼───────────────────────────────────────────────────┐
  │                   Data Layer                             │
  │  SQLite (single file, WAL mode)                          │
  └──────────────────────────────────────────────────────────┘
```

## Tech Stack
| Component | Choice | Reason |
|---|---|---|
| Language | Go 1.26 | Single binary, fast, great for data processing |
| Web Framework | `chi` (v5) | Minimal routing with middleware support |
| HTML Templating | `html/template` (std) | Built-in, XSS-safe, no build step |
| Database | SQLite (`modernc.org/sqlite`) | Single file, no server, WAL mode, pure Go (no CGO) |
| Queries | `sqlc` | Type-safe SQL; generates Go types from queries |
| Migrations | `goose` | SQL-only migration files, SQLite-compatible |
| Charts | Apache ECharts | Excellent financial charting (candlestick, heatmap, waterfall) |
| Tables | Plain HTML + DataTables.js | Sortable, searchable, paginated |
| CSV Parsing | `encoding/csv` (std) | Built-in, reliable |
| XML Parsing | `encoding/xml` (std) | For IBKR flex reports |
| Market Data | `github.com/wnjoon/go-yfinance` | Pure Go yfinance client, no Python dependency |
| Config | YAML (`gopkg.in/yaml.v3`) | Simple, no env var sprawl |
| Logging | `slog` (std) | Structured logging, built into Go 1.21+ |
| Unit Testing | `testify` + `mockery` (planned) | Mock interfaces for repos and external deps |
| Integration Testing | In-memory SQLite + `httptest` | Real SQL, real schema, isolated per test |
| Build | Makefile | Simple build targets |

## Constraints
- Single-user tool — no multi-tenant or auth in the app
- Go build cache at `~/.cache/go-build/` is on a read-only filesystem; use `GOCACHE=/tmp/go-cache GOPATH=/tmp/go-path`
- `sqlc` not installed on this machine — repositories are hand-written; sqlc query files are ready for when sqlc becomes available
- `mockery` not installed — mocks are hand-written for now

## Directory Structure
```
portfoliolab/
├── cmd/server/main.go                 # Application entry point
├── internal/
│   ├── api/                           # HTTP API handlers and router
│   │   ├── handlers/                  # Handler implementations
│   │   ├── middleware/                # Logging middleware
│   │   └── router.go                  # Chi router setup
│   ├── config/                        # YAML config loading
│   ├── data/                          # Data access / repositories
│   │   ├── queries/                   # sqlc query files (ready for sqlc)
│   │   ├── db.go                      # SQLite connection setup
│   │   ├── migrate.go                 # Goose migration runner
│   │   └── portfolio_repo.go          # Portfolio repository
│   ├── domain/                        # Business logic
│   │   └── portfolio/                 # Portfolio domain model + service
│   ├── market/                        # Market data (go-yfinance, planned)
│   └── web/                           # Web rendering + static assets
│       └── static/                    # CSS, JS
├── templates/                         # Go HTML templates
├── migrations/                        # SQL migration files (goose)
├── config/                            # Config files
├── tests/                             # Integration tests + fixtures
├── docs/                              # Project docs (this directory)
├── features/                          # Feature tracking (specs, plans, notes)
├── go.mod / go.sum
├── Makefile
├── README.md                          # Full project documentation
└── AGENTS.md                          # Coding conventions for agents
```
