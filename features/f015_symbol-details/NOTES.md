# Notes: Symbol Details

## Decisions
- 2026-05-16: Domain model (`internal/domain/symbols/symbol_details.go`) created ahead of Task 2 (fetcher) because the repository needed a target type. The structs mirror the RESEARCH.md response structure and will be populated by the fetcher in Task 2.
- 2026-05-16: Yahoo endpoint URLs are package-level vars (not consts) in `symbol_details.go` so they can be overridden in tests with a mock server.
- 2026-05-16: JSON columns in SQLite stored as TEXT; repo handles marshal/unmarshal. Empty/nil values stored as NULL (sql.NullString).
- 2026-05-16: `ListStaleSymbolDetails` uses an INNER JOIN with `symbol_mappings` — symbols without a mapping are excluded from stale list (they can't be refreshed without a market_data_symbol).
- 2026-05-16 (Task 4): `SymbolGetResponse` is a separate struct from `SymbolMapping` to include the optional `SymbolDetails` field without changing the domain model. `HandleList` still returns `[]SymbolMapping` (lean, no details).
- 2026-05-16 (Task 4): Route rename from `symbol-mappings` to `symbols` required updating integration tests and templates that hardcoded the old API path.
- 2026-05-16 (Task 5): Web handler pre-formats all percentage/number values (holdings %, sector %, net assets with B/M suffix) into the display struct, since Go templates lack `printf`/`mul` functions. This keeps templates simple and follows the API-first pattern of computing in the handler layer.
- 2026-05-16 (Task 5): `SymbolDetailsWebHandler` accepts `nil` for optional dependencies (detailsSvc, fetcher) and gracefully skips enrichment when they're absent. The symbolMappingSvc is required and returns 500 if nil.

## Deviations from Plan
- Task 2: Initially created `YahooSymbolDetailsFetcher` as a separate type, then refactored to consolidate under `YahooFinanceFetcher` per user feedback. `YahooFinanceFetcher` now implements both `MarketDataFetcher` and `SymbolDetailsFetcher`. The direct HTTP logic (crumb/cookie auth, quoteSummary parsing) lives in `symbol_details.go` as methods on `YahooFinanceFetcher`, keeping the go-yfinance code in `quote.go` separate. Single fetcher type, single wiring point in router.
- Task 3: Moved `SymbolDetails`, `TopHolding`, `SectorWeighting`, `AggregatePositions`, `FundProfile`, `EquityValuation`, and `StaleSymbol` types from `internal/domain/symbols/symbol_details.go` to `internal/market/symbol_details.go` to break an import cycle, then moved them again to `internal/types/symbol/symbol_details.go` (a shared types package) so the types live at the domain level rather than in the infrastructure package. Dependency graph: `market` → `types/symbol`, `symbols` → `types/symbol`, `data` → `types/symbol`. No cycles.

## Future Improvements
- None yet.

## Known Issues
- None.
