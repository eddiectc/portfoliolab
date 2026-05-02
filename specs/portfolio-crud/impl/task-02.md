---
task: 02
feature: Project Skeleton + Portfolio CRUD
depends-on: [01]
status: pending
---
# Task 02: Database Setup + Migration Framework

## Goal
Set up SQLite database initialization with WAL mode, goose migration framework, and the initial schema migration for the portfolios table.

## Files to Create/Modify
| File | Action | Description |
|------|--------|-------------|
| `internal/data/db.go` | create | Open SQLite connection, enable WAL, run migrations |
| `migrations/001_create_portfolios.sql` | create | Initial migration: portfolios table |
| `cmd/migrate/main.go` | create | CLI tool to run goose migrations (optional, also callable from server) |
| `internal/config/config.go` | modify | Add database path to config |
| `cmd/server/main.go` | modify | Initialize DB on startup |
| `internal/data/db_test.go` | create | Tests for DB connection and migration |

## Implementation Steps
1. Add `modernc.org/sqlite` dependency
2. Implement `OpenDB(path string) (*sql.DB, error)` in `data/db.go`:
   - Open SQLite with WAL mode (`PRAGMA journal_mode=WAL`)
   - Set reasonable connection pool settings for single-user workload
3. Add `modernc.org/sqlite` + `goose` dependencies
4. Create migration `001_create_portfolios.sql`:
   - `portfolios` table: `id` (INTEGER PRIMARY KEY), `name` (TEXT NOT NULL UNIQUE), `currency` (TEXT NOT NULL DEFAULT 'USD'), `created_at` (TEXT NOT NULL DEFAULT (datetime('now'))), `updated_at` (TEXT NOT NULL DEFAULT (datetime('now')))
5. Integrate goose migration runner into server startup (auto-run pending migrations)
6. Update config to include database path
7. Write tests: DB opens successfully, WAL mode enabled, migration applies cleanly

## Acceptance Criteria
- [ ] Server auto-runs pending migrations on startup
- [ ] SQLite opens in WAL mode
- [ ] `portfolios` table exists after startup with correct schema
- [ ] `name` column is UNIQUE
- [ ] Default currency is 'USD'
- [ ] `created_at` and `updated_at` auto-populate

## Testing Requirements
- Unit tests: DB connection opens, WAL mode verified via PRAGMA
- Integration tests: In-memory SQLite (`file::memory:?cache=shared`), migration applies, schema verified
- Manual verification: Start server, inspect DB file with `sqlite3`

## Notes
- Use `modernc.org/sqlite` (pure Go, no CGO) — not `mattn/go-sqlite3`
- For integration tests, use `file::memory:?cache=shared` DSN
- Goose needs a pure-Go SQLite driver registration; use `modernc.org/sqlite` with goose's driver registration pattern
- Consider registering the driver with name "sqlite3" for goose compatibility
