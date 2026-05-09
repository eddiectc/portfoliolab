# Notes: Historical Market Data Caching

## Decisions
- 2026-05-10: `GetLatestQuotesBatch` renamed to `GetLatestQuote` (single-symbol). sqlc doesn't support dynamic `IN` clauses with variable-length slices for SQLite. The batch version will be implemented in the repo layer by looping `GetLatestQuote` per symbol.

## Deviations from Plan
- Task 1: `GetLatestQuotesBatch` → `GetLatestQuote` (single symbol). Batch logic moved to repo layer (Task 2).

## Future Improvements

## Known Issues
