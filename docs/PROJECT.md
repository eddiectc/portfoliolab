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
See [README.md](../README.md) for the full architecture diagram and key design decisions.

**TL;DR:** Single Go binary → chi router → domain services → SQLite. Server-rendered HTML templates with ECharts. Market data via go-yfinance.

## Tech Stack
See [README.md](../README.md) for the full tech stack table.

## Constraints
- Single-user tool — no multi-tenant or auth in the app
- Go build cache at `~/.cache/go-build/` is on a read-only filesystem; use `GOCACHE=/tmp/go-cache GOPATH=/tmp/go-path`
- `sqlc` not installed on this machine — repositories are hand-written; sqlc query files are ready for when sqlc becomes available
- `mockery` not used — mocks are hand-written (see docs/CONVENTIONS.md)

## Documentation Index
| Document | Purpose |
|---|---|
| [README.md](../README.md) | Project overview, problem statement, feature list, architecture, tech stack, directory layout, testing strategy, risks |
| [CONVENTIONS.md](CONVENTIONS.md) | Coding conventions (style, naming, testing, domain, DB, API, web, security, build) |
| [FX_CONVENTIONS.md](FX_CONVENTIONS.md) | Foreign exchange rate conventions across multi-currency portfolios |
| [API.md](../API.md) | REST API reference (endpoints, request/response schemas) |
| [DoD.md](../DoD.md) | Definition of Done checklist (per-feature) |
| [AGENTS.md](../AGENTS.md) | Agentic coding instructions (agent-specific workflow and practices) |
| [features/](../features/) | Feature specs, plans, notes, retrospectives |
