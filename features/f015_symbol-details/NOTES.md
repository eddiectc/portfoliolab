# Notes: Symbol Details

## Decisions
- 2026-05-16 (Task 6): Plan says "`symbols.Service` (the symbol CRUD service)" but `symbols.Service` is the details service — the CRUD service is `symbolmapping.Service`. Followed the intent (CRUD service). Defined a new `SymbolDetailsFetcher` interface in `symbolmapping` (with `FetchAndStore`) rather than reusing `market.SymbolDetailsFetcher` (with `FetchSymbolDetails`), since the CRUD service needs the full fetch-and-store orchestration, not just the raw fetch.
- 2026-05-16 (Task 6): Background goroutine in `Create()` uses `context.Background()` (not the incoming `ctx`) so the details fetch survives if the HTTP caller disconnects before the goroutine runs.
- 2026-05-16: Domain model (`internal/domain/symbols/symbol_details.go`) created ahead of Task 2 (fetcher) because the repository needed a target type. The structs mirror the RESEARCH.md response structure and will be populated by the fetcher in Task 2.
- 2026-05-16: Yahoo endpoint URLs are package-level vars (not consts) in `symbol_details.go` so they can be overridden in tests with a mock server.
- 2026-05-16: JSON columns in SQLite stored as TEXT; repo handles marshal/unmarshal. Empty/nil values stored as NULL (sql.NullString).
- 2026-05-16: `ListStaleSymbolDetails` uses a LEFT JOIN with `symbol_mappings` — symbols with a mapping but no details row are included (fetched_at IS NULL), and symbols with stale details (fetched_at < threshold) are included. Symbols without any mapping are excluded (they start from symbol_mappings).
- 2026-05-16 (Task 4): `SymbolGetResponse` is a separate struct from `SymbolMapping` to include the optional `SymbolDetails` field without changing the domain model. `HandleList` still returns `[]SymbolMapping` (lean, no details).
- 2026-05-16 (Task 4): Route rename from `symbol-mappings` to `symbols` required updating integration tests and templates that hardcoded the old API path.
- 2026-05-16 (Task 5): Web handler pre-formats all percentage/number values (holdings %, sector %, net assets with B/M suffix) into the display struct, since Go templates lack `printf`/`mul` functions. This keeps templates simple and follows the API-first pattern of computing in the handler layer.
- 2026-05-16 (Task 5): `SymbolDetailsWebHandler` accepts `nil` for optional dependencies (detailsSvc, fetcher) and gracefully skips enrichment when they're absent. The symbolMappingSvc is required and returns 500 if nil.

## Deviations from Plan
- Task 2: Initially created `YahooSymbolDetailsFetcher` as a separate type, then refactored to consolidate under `YahooFinanceFetcher` per user feedback. `YahooFinanceFetcher` now implements both `MarketDataFetcher` and `SymbolDetailsFetcher`. The direct HTTP logic (crumb/cookie auth, quoteSummary parsing) lives in `symbol_details.go` as methods on `YahooFinanceFetcher`, keeping the go-yfinance code in `quote.go` separate. Single fetcher type, single wiring point in router.
- Task 3: Moved `SymbolDetails`, `TopHolding`, `SectorWeighting`, `AggregatePositions`, `FundProfile`, `EquityValuation`, and `StaleSymbol` types from `internal/domain/symbols/symbol_details.go` to `internal/market/symbol_details.go` to break an import cycle, then moved them again to `internal/types/symbol/symbol_details.go` (a shared types package) so the types live at the domain level rather than in the infrastructure package. Dependency graph: `market` → `types/symbol`, `symbols` → `types/symbol`, `data` → `types/symbol`. No cycles.
- Task 7: Plan specified two interfaces (`SymbolDetailsRefreshRepository` + `SymbolDetailsRefreshFetcher`) but consolidated into a single `SymbolDetailsRefreshSource` interface with `GetStaleSymbols` and `RefreshSymbol`. The split didn't work architecturally — the "fetcher" would need both the HTTP client (Yahoo) and the repo (DB upsert), which is the full fetch-and-store orchestration already provided by the symbols service. The single interface is implemented directly by `symbols.Service`, following the same pattern as `BenchmarkSymbolLister` (implemented by `symbolmapping.Repository`).

## Future Improvements
- None yet.

## Known Issues
- None.

## Validation (Task 9, 2026-05-17)
- `go test -count=1 ./...` — all 18 packages pass (0 failures)
- `go vet ./...` — clean
- `go build ./...` — clean
- Cross-layer consistency verified: DB schema (TEXT columns) ↔ repo (JSON marshal/unmarshal) ↔ domain types (`internal/types/symbol/`) ↔ API response (`SymbolDetailsResponse`) ↔ web template (`view.html`) all aligned
- Spec scenario coverage confirmed via tests: fetch on creation (service + market tests), API enrich (handler tests with details/no-details), web UI (6 web handler tests covering with-details/no-details/not-found/stale/invalid-ID/no-fetcher), background refresh (8 marketcache tests covering stale/fresh/failure/rate-limiting/integration)
- One pre-existing TODO in `symbol_mapping_repo.go:240` (f004 stub, unrelated to this feature)
