# Notes: Account CRUD

## Decisions
- 2026-05-03: Created `account.sql` alongside the migration so sqlc types are available immediately for subsequent tasks (domain, repo, handlers). This follows the existing pattern of having sqlc queries ready before the repository layer.
- 2026-05-03: Added `PRAGMA foreign_keys = ON` to the integration test setup explicitly to ensure FK cascade tests work in in-memory SQLite.
- 2026-05-03: Added `PortfolioChecker` interface to the account service for verifying portfolio existence, keeping the account domain decoupled from the portfolio domain. The real implementation will wrap the portfolio repository; tests use a simple mock.
- 2026-05-03: Created `PortfolioCheckerImpl` in `internal/data/portfolio_checker.go` wrapping the existing `PortfolioRepository`. Reuses the same repo instance from the router rather than creating a second one.
- 2026-05-03: Used flat `accountRow` struct (not embedded `account.Account`) for the list template data to avoid template execution issues with embedded struct fields. Formatted `CreatedAt` with `formatTime()` for consistent display.
- 2026-05-03: Removed standalone accounts list page (`GET /accounts`) and portfolio dropdown from account form. All account creation flows through the portfolio detail page with `?portfolio_id=X` pre-selecting the portfolio. API routes (`/api/accounts`) remain flat and unchanged.

## Lessons Learned
- Go templates are case-sensitive on field names — `FilterPortfolioId` vs `FilterPortfolioID` caused a silent template execution failure. The error only surfaced at runtime during `ExecuteTemplate`.
- Embedded structs in template data can cause issues; flat display structs with explicit fields are more reliable.
- **Service-layer defaults are essential for pagination**: web handlers call `service.List(ctx, 0, 0)` for unpaginated listings, which passed `LIMIT 0` to SQLite returning zero rows. The fix defaults `limit=0` to 50 at the service layer so all callers are protected. Mock repos now simulate real `LIMIT 0` behavior to catch regressions.

## Deviations from Plan
- **No standalone accounts list page**: Removed `GET /accounts` web route and `templates/account/list.html`. All account management is done through the portfolio detail page. The API endpoint `GET /api/accounts` remains for programmatic access.
- **No portfolio dropdown on account form**: Portfolio is always pre-selected via `?portfolio_id=X` query parameter from the portfolio page. The form uses a hidden field instead of a `<select>`.
- **`GET /accounts/new` requires `portfolio_id`**: Without it, redirects to `/portfolios`.
- **Delete redirects to portfolio page**: `POST /accounts/{id}/delete` redirects to `/portfolios/{portfolioID}` instead of `/accounts`.
- **Detail page "Back" links to portfolio**: Changed from "Back to List" (`/accounts`) to "Back to Portfolio" (`/portfolios/{id}`).

## Future Improvements
- None yet.

## Known Issues
- None.
