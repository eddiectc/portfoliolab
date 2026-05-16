# Notes: Symbol Details

## Decisions
- 2026-05-16: Domain model (`internal/domain/symbols/symbol_details.go`) created ahead of Task 2 (fetcher) because the repository needed a target type. The structs mirror the RESEARCH.md response structure and will be populated by the fetcher in Task 2.
- 2026-05-16: JSON columns in SQLite stored as TEXT; repo handles marshal/unmarshal. Empty/nil values stored as NULL (sql.NullString).
- 2026-05-16: `ListStaleSymbolDetails` uses an INNER JOIN with `symbol_mappings` — symbols without a mapping are excluded from stale list (they can't be refreshed without a market_data_symbol).

## Deviations from Plan
- None yet.

## Future Improvements
- None yet.

## Known Issues
- None.
