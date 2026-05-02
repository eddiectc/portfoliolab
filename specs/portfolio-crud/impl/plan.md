---
feature: Project Skeleton + Portfolio CRUD
tech-stack: Go, SQLite (modernc.org/sqlite), sqlc, goose, chi, html/template
tasks: 4
---
# Implementation Plan: Project Skeleton + Portfolio CRUD

## Architecture Decisions
- **chi router**: Minimal, idiomatic HTTP router with middleware support; aligns with README tech stack
- **sqlc for queries**: Write SQL, generate type-safe Go code; follows AGENTS.md convention
- **goose for migrations**: SQL-only migration files, SQLite-compatible
- **Clean architecture layers**: `cmd/` → `internal/api/` → `internal/domain/` → `internal/data/`
- **Config via YAML**: Single config file with `viper`-style manual parsing (keep deps minimal; use `gopkg.in/yaml.v3`)
- **No external ORM**: sqlc generates typed queries from hand-written SQL

## Shared Components
- Go module (`go.mod`) with dependency management
- Config loading (`internal/config/config.go`)
- Database initialization (`internal/data/db.go`)
- Goose migration runner
- HTTP server bootstrap (`cmd/server/main.go`)

## Task Dependencies
```
task-01 → task-02 → task-03 → task-04
```

## Execution Order
1. **task-01**: Go module + project skeleton (no dependencies)
2. **task-02**: Database setup + migration framework (depends on task-01)
3. **task-03**: Portfolio CRUD API (depends on task-02)
4. **task-04**: Portfolio web UI pages (depends on task-03)

## Task Summary
| Task | Title | Dependencies | Estimated Complexity |
|------|-------|-------------|---------------------|
| 01 | Go module + project skeleton | none | low |
| 02 | Database setup + migration framework | 01 | low |
| 03 | Portfolio CRUD API (REST + domain + repo) | 02 | medium |
| 04 | Portfolio web UI (server-rendered pages) | 03 | medium |
