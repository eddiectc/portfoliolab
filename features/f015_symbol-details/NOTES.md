# Notes: Symbol Details

## Decisions
- 2026-05-16: Domain model (`internal/domain/symbols/symbol_details.go`) created ahead of Task 2 (fetcher) because the repository needed a target type. The structs mirror the RESEARCH.md response structure and will be populated by the fetcher in Task 2.
- 2026-05-16: Yahoo endpoint URLs are package-level vars (not consts) in `symbol_details.go` so they can be overridden in tests with a mock server.
- 2026-05-16: JSON columns in SQLite stored as TEXT; repo handles marshal/unmarshal. Empty/nil values stored as NULL (sql.NullString).
- 2026-05-16: `ListStaleSymbolDetails` uses an INNER JOIN with `symbol_mappings` — symbols without a mapping are excluded from stale list (they can't be refreshed without a market_data_symbol).

## Deviations from Plan
- Task 2: Initially created `YahooSymbolDetailsFetcher` as a separate type, then refactored to consolidate under `YahooFinanceFetcher` per user feedback. `YahooFinanceFetcher` now implements both `MarketDataFetcher` and `SymbolDetailsFetcher`. The direct HTTP logic (crumb/cookie auth, quoteSummary parsing) lives in `symbol_details.go` as methods on `YahooFinanceFetcher`, keeping the go-yfinance code in `quote.go` separate. Single fetcher type, single wiring point in router.
- Task 3: Moved `SymbolDetails`, `TopHolding`, `SectorWeighting`, `AggregatePositions`, `FundProfile`, `EquityValuation`, and `StaleSymbol` types from `internal/domain/symbols/symbol_details.go` to `internal/market/symbol_details.go` to break an import cycle, then moved them again to `internal/types/symbol/symbol_details.go` (a shared types package) so the types live at the domain level rather than in the infrastructure package. Dependency graph: `market` → `types/symbol`, `symbols` → `types/symbol`, `data` → `types/symbol`. No cycles.

## Future Improvements
- None yet.

## Known Issues
- None.
