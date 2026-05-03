# Notes: Account CRUD

## Decisions
- 2026-05-03: Created `account.sql` alongside the migration so sqlc types are available immediately for subsequent tasks (domain, repo, handlers). This follows the existing pattern of having sqlc queries ready before the repository layer.
- 2026-05-03: Added `PRAGMA foreign_keys = ON` to the integration test setup explicitly to ensure FK cascade tests work in in-memory SQLite.
- 2026-05-03: Added `PortfolioChecker` interface to the account service for verifying portfolio existence, keeping the account domain decoupled from the portfolio domain. The real implementation will wrap the portfolio repository; tests use a simple mock.
- 2026-05-03: Created `PortfolioCheckerImpl` in `internal/data/portfolio_checker.go` wrapping the existing `PortfolioRepository`. Reuses the same repo instance from the router rather than creating a second one.
- 2026-05-03: Used flat `accountRow` struct (not embedded `account.Account`) for the list template data to avoid template execution issues with embedded struct fields. Formatted `CreatedAt` with `formatTime()` for consistent display.

## Lessons Learned
- Go templates are case-sensitive on field names — `FilterPortfolioId` vs `FilterPortfolioID` caused a silent template execution failure. The error only surfaced at runtime during `ExecuteTemplate`.
- Embedded structs in template data can cause issues; flat display structs with explicit fields are more reliable.
- **Service-layer defaults are essential for pagination**: web handlers call `service.List(ctx, 0, 0)` for unpaginated listings, which passed `LIMIT 0` to SQLite returning zero rows. The fix defaults `limit=0` to 50 at the service layer so all callers are protected. Mock repos now simulate real `LIMIT 0` behavior to catch regressions.

## Deviations from Plan
- None yet.

## Future Improvements
- None yet.

## Known Issues
- None.
