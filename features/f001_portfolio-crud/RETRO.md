# Retrospective: Portfolio CRUD

## What Went Well

- **Clean layered architecture from day one** — Domain model, service, repository, and HTTP handlers are cleanly separated. The `Repository` interface and `Service` struct made it easy to swap in sqlc-generated queries later without touching the service or handler layers.
- **Four-task dependency chain was effective** — Task 1 (skeleton) → Task 2 (DB) → Task 3 (API) → Task 4 (web UI) worked without backtracking. Each task was independently testable and verifiable.
- **Three-layer test coverage** — Service unit tests (mock repo), handler unit tests (test repo), and integration tests (real SQLite) catch bugs at different levels. The `TestHandleNewPage_RendersCompleteForm` regression test specifically caught the template field-panic bug.
- **Template design was robust** — The `portfolioFormPageData` struct with explicit fields prevented silent template panics. The `newPortfolioFormPageData` factory ensured all form pages get consistent defaults (currencies, cancel href).
- **Error handling is consistent** — Service errors map cleanly to HTTP status codes via `handleServiceError`. Error codes (`PORTFOLIO_NOT_FOUND`, `PORTFOLIO_NAME_EXISTS`, etc.) are consistent across API and web handlers.
- **sqlc migration was smooth** — Hand-written repos were written with sqlc in mind. When sqlc became available, the repo layer was replaced with a thin wrapper over generated queries. No service or handler changes needed.

## What Could Be Improved

- **(Resolved)** Default currency on create — Fixed by adding `defaultCurrency = "USD"` constant and empty check in `service.Create`.
- **(Resolved)** Mock consolidation — Migrated handler tests to generated `MockRepository` via mockery. Service tests retain hand-written mock for realistic behavior simulation.
- **(Resolved)** Handler mock fidelity — Generated mock uses explicit expectations per call, eliminating permissive behavior.
- **(Resolved)** Handler-level pagination test — Added `TestHandleList_DefaultPagination` in `portfolio_test.go`.
- **(Resolved)** Flash cookie clearing — `getFlash` now accepts `http.ResponseWriter` and calls `clearFlash(w)` after reading.

## Spec vs Reality

- **27 of 28 scenarios implemented** — The "Create a portfolio with default currency" scenario is not implemented. The service requires currency and rejects empty strings.
- **All 9 edge cases handled** — Empty name, long name, duplicate name, invalid currency, non-existent ID, no-changes update, pagination defaults, empty list, and whitespace trimming are all covered.
- **No scenarios missed beyond the default currency gap** — Every other "Given/When/Then" block maps to at least one test.
- **Constraints followed** — Name 1-100 chars, currency `^[A-Z]{3}$`, error format and error codes all match the spec.
- **Non-goals respected** — No accounts, transactions, P&L, sharing, archival, or bulk operations were added.

## Plan vs Reality

- **Task breakdown was effective** — 4 tasks in strict dependency order, all completed. No reordering or backtracking needed.
- **Tasks were appropriately sized** — Each task was completable in one focused session. Task 3 (API) was the largest but manageable because it followed a clear pattern.
- **Dependencies were accurate** — No task blocked on an unlisted dependency.
- **Technical decisions held up** — Chi router, modernc.org/sqlite, hand-written SQL (later sqlc), and html/template all worked well. The chi route ordering note (specific before catch-all) was validated by the web handler implementation.
- **Post-implementation sqlc migration** — Documented in NOTES.md. This was a valid improvement that didn't require plan changes because the repo interface was stable.

## Learnings

- **Service-layer validation can block DB defaults** — The migration has `DEFAULT 'USD'` but the service validates currency before reaching the DB. If a field should be optional with a default, the service should apply the default before validation, not rely on the DB.
- **Template data structs need explicit fields** — The `portfolioFormPageData` struct with all fields explicit prevented "can't evaluate field" panics. Embedded structs (e.g., `web.PageData`) can hide missing fields. The factory function `newPortfolioFormPageData` ensures consistency.
- **Mock fidelity matters** — The service-level `mockRepo.GetAll` simulates `LIMIT 0` → empty result, catching the pagination default bug. The handler-level `testRepo.GetAll` doesn't, which is a fidelity gap.
- **Flash cookies need explicit clearing** — The `getFlash` function reads the cookie but doesn't clear it. Rapid navigation can show stale flashes. The `clearFlash` function exists but is unused.
- **Write tests for regression bugs** — The `TestHandleNewPage_RendersCompleteForm` test was added after discovering the template field-panic bug. This pattern (reproduce → fix → test) should be standard practice.
- **Follow existing patterns** — The portfolio feature established patterns (domain model → service → repo → handlers → templates) that f002_account-crud followed exactly. This made the second feature faster and more predictable.

## Action Items

- [x] **Fix "Create a portfolio with default currency"** — Default currency to "USD" in `Service.Create` when empty, and add test. Done: added `defaultCurrency = "USD"` constant and empty check in `service.Create`, added `TestService_Create_DefaultCurrency`.
- [x] **Consolidate mock repos** — Use `mockery` to generate a single shared `Repository` mock for portfolio tests. Done: generated `mock_repository.go` via `.mockery.yml`, migrated `portfolio_test.go` and `portfolio_web_test.go` to use generated mock. Kept hand-written service mock in `service_test.go` for realistic behavior simulation.
- [x] **Fix `getFlash` to clear the cookie on read** — Done: updated `getFlash` signature to accept `http.ResponseWriter`, calls `clearFlash(w)` after reading. Updated all callers (`HandleListPage`, `HandleDetailPage`).
- [x] **Add handler-level test for default pagination** — Done: `TestHandleList_DefaultPagination` in `portfolio_test.go` creates 10 portfolios, requests with no params, verifies all 10 returned.
- [x] **Make handler-level mock simulate SQL LIMIT 0** — Done: migrated handler tests to generated `MockRepository` (mockery) which uses explicit expectations per call, eliminating the permissive mock behavior.
