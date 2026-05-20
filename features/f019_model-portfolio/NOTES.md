# Notes: Model Portfolio

## Decisions
- 2025-05-20: Table schema follows the plan — single table with `entries` as TEXT (JSON). No separate entries table since entries are always CRUD'd as a complete set.
- 2025-05-20: sqlc queries include a `CountModelPortfolios` helper beyond the plan's CRUD set — useful for pagination in the service layer.

## Deviations from Plan
- Task 6: The plan listed "trigger market data fetch" as a separate sub-step, but `symbolmapping.Service.Create` already does this via a background goroutine (`SymbolDetailsFetcher.FetchAndStore`). No extra wiring needed.

## Future Improvements
- None noted.

## Known Issues
- None.
