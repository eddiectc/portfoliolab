# Notes: symbol-map

## Decisions
- 2026-05-04: Used `sqlc` (already installed) to generate query types and functions for symbol mappings. Followed existing portfolio/account repo patterns exactly.
- 2026-05-04: Created minimal domain model types in `internal/domain/symbolmapping/symbol_mapping.go` to unblock the repository. Full service logic (validation, business rules) deferred to Task 2.
- 2026-05-04: `HasReferencingTransactions` is a stub returning `false` — will be implemented in f004.
- 2026-05-04: Mock repository placed in `service_test.go` (co-located with tests) following the portfolio/account pattern, rather than a separate `mock_repository.go` file.
- 2026-05-04: REST API handler uses `handleServiceError` to map domain errors to specific HTTP status codes: `ErrInvalidSymbol` → 400, `ErrInternalSymbolExists` → 409, `ErrBrokerSymbolExists` → 409, `ErrInUse` → 409, `ErrNotFound` → 404.
- 2026-05-04: Web handler follows `portfolio_web.go`/`account_web.go` patterns exactly: `SymbolMappingWebHandler` with `RegisterRoutes`, flash messages, and `userFriendlyError` for form validation feedback.
- 2026-05-04: Form template uses `r.Form` (not `r.FormValue`) for broker symbols since they come as repeated field names ("broker_name", "broker_symbol"); `parseBrokerSymbols()` helper extracts pairs and skips empty entries.
- 2026-05-04: Web handler tests use hand-written `testSMWebRepo` mock co-located in `symbol_mapping_web_test.go`, following the `portfolio_web_test.go` pattern.
- 2026-05-04: Router wiring follows existing pattern: repo/service/handler created inline in `router.go` and registered via `RegisterRoutes`. Both API (`SymbolMappingHandler`) and web (`SymbolMappingWebHandler`) handlers share the same service instance.
- 2026-05-04: Nav link labeled "Symbol Maps" and placed between Portfolios and Transactions, matching the logical workflow order.
- 2026-05-04: `Service` constructor uses functional options pattern (`WithQuoteFetcher`) to keep the quote fetcher optional. Existing callers (tests, router) work without changes when no fetcher is configured.
- 2026-05-04: Preview endpoint returns HTTP 503 (Service Unavailable) when no fetcher is configured or fetch fails, matching the "soft dependency" design — the feature works without it.
- 2026-05-04: Form template JS uses a 2-second debounce on the market data symbol input. Preview area has three states: loading, success, and error (warning icon).
- 2026-05-04: `govalues/decimal` was added to go.mod but not used in Task 6 (go-yfinance returns `float64` for prices). Deferred to f004 where actual monetary calculations need decimal precision.

## Deviations from Plan
- Plan called for `mock_repository.go` as a separate file, but the portfolio/account pattern puts the hand-written mock inside `service_test.go`. Followed the actual codebase pattern instead.

## Future Improvements
- Search/filter on symbol mapping list page (deferred in spec; existing list pages don't have it either)

## Known Issues
- `HasReferencingTransactions` is a stub — safe because no transactions table exists yet.
- `Quote.LatestPrice` is `float64` (go-yfinance returns float64). Convert to `decimal.Decimal` in f004 when actual monetary calculations are needed.
