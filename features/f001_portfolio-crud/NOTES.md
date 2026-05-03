# Notes: Portfolio CRUD

## Decisions
- 2026-05-02: Repositories written by hand instead of sqlc-generated — sqlc was not installed on this machine at the time.
- 2026-05-03: `sqlc` and `mockery` are now installed. Migrated `PortfolioRepository` to delegate to sqlc-generated queries in `internal/data/queries/`. The repo layer wraps sqlc calls and handles string ↔ `time.Time` conversion for timestamps (sqlite driver doesn't scan TEXT into `time.Time` directly).
- 2026-05-02: Go templates parsed per-page (layout + page file) rather than all into one shared set, to avoid template name conflicts when multiple pages define the same template name (e.g., "content").
- 2026-05-02: Chi route ordering: specific routes registered before catch-all routes (e.g., `POST /portfolios/{id}/delete` before `POST /portfolios`).

## Deviations from Plan
- None (both deviations below were fixed on 2026-05-03).

## Fixes
- **2026-05-03: List with `limit=0`** — Fixed `parsePagination` to default to 50 when limit is missing, 0, or negative. Previously returned all results.
- **2026-05-03: Update with no changes** — Fixed `Service.Update` to track whether any field changed and only refresh `updated_at` when at least one field was modified. Previously always refreshed the timestamp.

## Future Improvements
- Migrate remaining repositories (transactions, etc.) to sqlc-generated queries
- Run `mockery --all` to replace hand-written mocks with generated ones
- Add integration tests for web page handlers (currently only API integration tests)
- Consider adding a `DELETE /api/portfolios/{id}` soft-delete option in future

## Known Issues
- None
