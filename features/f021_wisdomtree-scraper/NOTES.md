# Notes: WisdomTree Scraper

## Decisions
- 2026-05-26: Migration 021 adds `data_source_url` to `symbol_mappings` and `extractor_as_of_date` to `symbol_details`. Both nullable (NULL = default Yahoo behavior).
- 2026-05-26: `GetNavHistoryBySymbol` query filters on `data_type = 'nav'` and `date != ''` (excludes current entries). Reuses existing `market_data` table with existing UNIQUE(symbol, source, date) constraint — NAV uses source='wisdomtree'.
- 2026-05-26: `ListStaleSymbolDetails` now returns `data_source_url` alongside internal_symbol and market_data_symbol, enabling the service layer to route fetches to the correct provider.

## Deviations from Plan
- Task 1: `InsertSymbolDetails` SQL query was missing `extractor_as_of_date` in INSERT columns and ON CONFLICT UPDATE. Fixed during implementation review (add column to write path alongside the read path that was already updated).

## Future Improvements
- None yet.

## Known Issues
- None.
