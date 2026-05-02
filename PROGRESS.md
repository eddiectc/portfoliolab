# Project Progress

## Features

### specs/portfolio-crud 🔄
| Phase | Status |
|-------|--------|
| Brainstorm | ✅ done |
| Feature Spec | ✅ done |
| BDD Groom | 🔄 in-progress |
| Impl Plan | ⏳ pending |
| Implement | ⏳ pending |
| Verify | ⏳ pending |
| Retrospective | ⏳ pending |

### portfolio-crud 🔄
| Phase | Status |
|-------|--------|
| Brainstorm | ✅ done |
| Feature Spec | ✅ done |
| BDD Groom | ✅ done |
| Impl Plan | ✅ done |
| Implement | ✅ done |
| Verify | ✅ done |
| Retrospective | 🔄 in-progress |

## Technical Debt
- [2026-05-02] sqlc not installed — writing repository with hand-written SQL queries instead of sqlc-generated code. sqlc generate should be run when sqlc is available to get type-safe query generation. The queries/portfolio.sql file is ready for sqlc when installed.
- [2026-05-02] Go build cache at /home/tc/.cache/go-build/ is on a read-only filesystem. Workaround: use `GOCACHE=/tmp/go-cache GOPATH=/tmp/go-path go build -a ./...` to force rebuild with a writable cache. The `-a` flag is needed because go still tries to read stale entries from the read-only cache.

## Lessons Learned
- Go template rendering: when multiple page templates define the same template name (e.g., "content"), parsing all files into one shared set causes conflicts. Solution: parse layout + each page file separately so each page gets its own template set.
- Chi route ordering: specific routes must be registered before catch-all routes. E.g., POST /portfolios/{id}/delete must come before POST /portfolios, otherwise chi matches the catch-all first and the specific route never fires.
- Pre-check tool availability: before writing implementation plans, verify that required tools (sqlc, mockery, goose) are available. If not, document the alternative approach in the plan to avoid drift during implementation.
