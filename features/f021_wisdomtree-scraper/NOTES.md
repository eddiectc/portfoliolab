# Notes: WisdomTree Scraper

## Decisions
- 2026-05-26: Migration 021 adds `data_source_url` to `symbol_mappings` and `extractor_as_of_date` to `symbol_details`. Both nullable (NULL = default Yahoo behavior).
- 2026-05-26: `Extractor` and `URLMatcher` are separate interfaces. Registry uses a `registryEntry` struct pairing both, since not all extractors may need URL matching and Go doesn't allow casting between unrelated interface types.
- 2026-05-26: Dispatcher validates URL with `url.Parse` before lookup — rejects malformed URLs early with explicit error.
- 2026-05-26: WisdomTree extractor is a stub returning `ErrNotImplemented` until Task 3 adds the actual HTTP client and parsers. This allows the framework (Task 2) to be registered and tested independently.
- 2026-05-26: `GetNavHistoryBySymbol` query filters on `data_type = 'nav'` and `date != ''` (excludes current entries). Reuses existing `market_data` table with existing UNIQUE(symbol, source, date) constraint — NAV uses source='wisdomtree'.
- 2026-05-26: `ListStaleSymbolDetails` now returns `data_source_url` alongside internal_symbol and market_data_symbol, enabling the service layer to route fetches to the correct provider.

## Deviations from Plan
- Task 1: `InsertSymbolDetails` SQL query was missing `extractor_as_of_date` in INSERT columns and ON CONFLICT UPDATE. Fixed during implementation review (add column to write path alongside the read path that was already updated).
- Task 2: `extractorReg` is created in `router.go` but the `Dispatcher` is not wired yet — deferred to Task 4 where the service layer consumes it. Registry is registered with `_ = extractorReg` to suppress unused variable until then.

## Future Improvements
- None yet.

## Known Issues
- None.
