# Notes: Model Portfolio

## Decisions
- 2025-05-20: Table schema follows the plan — single table with `entries` as TEXT (JSON). No separate entries table since entries are always CRUD'd as a complete set.
- 2025-05-20: sqlc queries include a `CountModelPortfolios` helper beyond the plan's CRUD set — useful for pagination in the service layer.

## Deviations from Plan
- None so far.

## Future Improvements
- None noted.

## Known Issues
- None.
