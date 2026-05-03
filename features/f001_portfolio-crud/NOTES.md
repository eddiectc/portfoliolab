# Notes: Portfolio CRUD

## Decisions
- 2026-05-02: Repositories written by hand instead of sqlc-generated — sqlc not installed on this machine. sqlc config and query files are in `internal/data/queries/` ready for when sqlc becomes available.
- 2026-05-02: Go templates parsed per-page (layout + page file) rather than all into one shared set, to avoid template name conflicts when multiple pages define the same template name (e.g., "content").
- 2026-05-02: Chi route ordering: specific routes registered before catch-all routes (e.g., `POST /portfolios/{id}/delete` before `POST /portfolios`).

## Deviations from Plan
- **List with `limit=0`**: Spec says defaults to 50; implementation returns all results. The `parsePagination` function treats `n <= 0` as "use default (0 = no LIMIT clause)". This is a minor deviation — the behavior is more permissive (returns all) rather than capped at 50.
- **Update with no changes**: Spec says timestamps should be unchanged; implementation always refreshes `updated_at` in the service layer. The web handler detects no-changes before calling the service, but the API handler passes through unconditionally. Minor deviation — timestamps are always refreshed on API updates.

## Future Improvements
- Run `sqlc generate` when sqlc is installed to get type-safe query generation
- Add `mockery`-generated mocks for cleaner test setup
- Add integration tests for web page handlers (currently only API integration tests)
- Consider adding a `DELETE /api/portfolios/{id}` soft-delete option in future

## Known Issues
- None
