# AGENTS.md — AI Agent Entry Point

This project keeps **one set of conventions for humans and agents**. There is no separate agent rulebook: an AI agent works here exactly like a human contributor — read the shared docs below and follow the same rules.

## Start here

| Doc | What it tells you |
|---|---|
| [README.md](README.md) | What the project is, Quickstart, feature list |
| [docs/PROJECT.md](docs/PROJECT.md) | Problem statement, design principles, constraints, roadmap |
| [docs/CONVENTIONS.md](docs/CONVENTIONS.md) | **The rules** — code style, testing, domain logic, database, API, web, security |
| [features/README.md](features/README.md) | Feature index + the spec-driven workflow (how work is planned and executed) |
| [API.md](API.md) | REST API contract |
| [DoD.md](DoD.md) | Definition of Done checklist (per feature) |

## Working on features

Development is spec-driven. Read [features/README.md](features/README.md) before starting any feature work. In short: write spec → review → plan → implement → retrospective, with human review gates before implementation.

- One task at a time; each task is independently testable
- Write tests alongside implementation, not after
- Follow existing patterns before introducing new approaches
- Flag spec drift before deviating from the plan; document it in the feature's NOTES.md
- Keep changes minimal and focused — one concern per commit

## Environment guardrails

Working-environment rules for this checkout (not project-wide conventions):

- **Do NOT start the server** (`go run cmd/server/main.go`) — the developer manages the server lifecycle.
- **Do NOT read or modify the production database** (`data/portfoliolab.db`) — never run `sqlite3` against it. Use in-memory SQLite in tests; fix data issues through the application's API/UI.
