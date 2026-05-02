# Project Progress

## Features

## Technical Debt
- [2026-05-02] ~~sqlc not installed — writing repository with hand-written SQL queries instead of sqlc-generated code. sqlc generate should be run when sqlc is available to get type-safe query generation. The queries/portfolio.sql file is ready for sqlc when installed.~~ **RESOLVED** — sqlc v1.31.1 installed, `sqlc.yaml` updated for v2 config format, queries converted from PostgreSQL `$N` to SQLite `?` placeholders, and code generated in `internal/data/queries/`.
