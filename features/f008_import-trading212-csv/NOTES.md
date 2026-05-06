# Notes: Import Trading 212 CSV

## Decisions
- 2026-05-06: Task 0 extracted shared types/interfaces into `internal/domain/brokerimport/`. The `ImportService` handler interface was kept in the handlers package (not moved to brokerimport) since it's a handler-layer concern that references `brokerimport.PreviewResponse`/`ImportResult` as return types. The domain-level interfaces (SymbolResolver, DuplicateChecker, etc.) were moved to brokerimport as planned.

## Deviations from Plan
- None. Task 0 followed the plan exactly.

## Future Improvements
- None noted.

## Known Issues
- None.
