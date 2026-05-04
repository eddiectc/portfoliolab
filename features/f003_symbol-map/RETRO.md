# Retrospective: symbol-map (f003)

## What Went Well

- **Clean layering**: Domain, data, and API layers follow existing `portfolio`/`account` patterns exactly. No new patterns introduced, making the code familiar to anyone who's read the existing codebase.

- **Hand-written mocks proved superior**: The decision to use hand-written mocks co-located in `*_test.go` (following the `account` pattern) was validated during this feature. The mocks simulate real behavior (e.g., `Create` then `GetByID` returns the created item; `GetAll` with `limit=0` returns empty), catching service-layer bugs that expectation-based mocks would hide. This learning led to removing `mockery` and `testify/mock` from the entire project.

- **Soft dependency pattern**: The functional options pattern (`WithQuoteFetcher`) for the optional market data fetcher is clean and nil-safe. The preview endpoint returns 503 when unavailable without blocking core functionality. This is a reusable pattern for future optional features.

- **Comprehensive error mapping**: `handleServiceError` maps each domain error to a specific HTTP status code and error code string. This gives API consumers precise feedback (409 for conflicts, 400 for validation, 404 for not found, 503 for preview unavailable).

- **No scope creep**: Implementation stayed within feature boundaries. No transaction CRUD, no broker import, no automatic symbol detection — all explicitly deferred to future features.

- **sqlc worked well**: Plan assumed sqlc might not be installed. It was already available, and the generated code integrated cleanly with the existing repo pattern.

## What Could Be Improved

- **Search/filter on list page**: The spec scenario "View symbol mappings list" says "I can filter or search by internal symbol, broker, or market data provider symbol." This was not implemented. The existing `portfolio` list page also lacks search, so this followed the existing pattern — but the spec explicitly called for it. **Lesson**: When the spec says something the existing codebase doesn't do, flag it as a deviation rather than silently following the existing pattern.

- **No tests for `market/quote.go`**: The `YahooFinanceFetcher` has no unit tests. It's a thin wrapper around `go-yfinance`, and the service layer tests cover the `QuoteFetcher` interface via `mockQuoteFetcher`. Still, a basic test (even just verifying the struct compiles and the interface is satisfied) would be a safety net.

- **`Quote.LatestPrice` is `float64`**: The plan specified `decimal.Decimal`, but `go-yfinance` returns `float64` and this is display-only. Deferred to f004. Acceptable, but should be tracked.

- **No repository tests in `internal/data/`**: The `SymbolMappingRepository` has no dedicated tests. The service tests exercise the repo interface thoroughly, and the existing `portfolio_repo_test.go` pattern suggests repo tests exist for other domains. This is a minor gap but worth noting for consistency.

## Spec vs Reality

| Spec Item | Status | Notes |
|---|---|---|
| 12 scenarios | ✅ 11/12 fully implemented | Search/filter on list page not implemented |
| Edge cases | ✅ All handled | Empty symbols, duplicates, preview failure, etc. |
| Constraints | ✅ All met | Single market data provider, user-defined mappings only |
| Non-goals | ✅ All respected | No auto-detection, no import, no transaction CRUD |
| Soft dependency | ✅ Implemented | Preview gracefully degrades with 503 |

**Gap**: Search/filter on the list page (spec scenario "View symbol mappings list"). The existing codebase pattern doesn't have search on list pages, so this was silently skipped. Should have been flagged in NOTES.md.

## Plan vs Reality

| Aspect | Assessment | Notes |
|---|---|---|
| Task breakdown | ✅ Effective | 6 tasks, each independently testable |
| Task sizing | ✅ Appropriate | No task was too large; all completed in single sessions |
| Dependencies | ✅ Accurate | Sequential tasks 1→6, no surprises |
| Deviations | ✅ Documented | Mock strategy deviation documented in NOTES.md |
| Risks | ✅ Mitigated | go-yfinance stability addressed via graceful degradation |

**One deviation**: Plan called for `mock_repository.go` as a separate file. Followed the actual codebase pattern (co-located in `service_test.go`) instead. This was the right call and led to the broader decision to remove mockery from the project.

## Learnings

1. **Hand-written mocks > generated mocks for this codebase**: They simulate real behavior, are more readable, and don't require external tooling. This became a project-wide convention (mockery removed).

2. **Functional options pattern works well for optional dependencies**: Clean, nil-safe, and doesn't bloat the constructor signature. Reusable for future features with optional components.

3. **Spec should call out "follow existing pattern even if spec says X"**: When the spec describes behavior the existing codebase doesn't have (e.g., search/filter), the plan should explicitly decide whether to implement it or defer it, rather than leaving it ambiguous.

4. **Stub methods need explicit tracking**: `HasReferencingTransactions` returning `false` is documented in NOTES.md and scoped to f004. This pattern works — just ensure the TODO comment in code references the feature ID.

5. **sqlc integration is straightforward**: Once the SQL queries are written, `sqlc generate` produces clean Go code that fits the existing repo pattern. No special handling needed.

## Action Items

- [x] Search/filter on list page: explicitly marked as deferred in SPEC.md
- [x] Add basic test for `market/quote.go`: created `quote_test.go` with interface satisfaction and struct tests
- [x] Track `Quote.LatestPrice` `float64` → `decimal.Decimal`: documented in NOTES.md for f004
- [x] Repository tests: already exist in `symbol_mapping_repo_test.go` (14 tests)
