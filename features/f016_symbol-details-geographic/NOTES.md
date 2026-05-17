# Notes: Symbol Details — Geographic Data

## Decisions
- 2026-05-17: Tasks 2 and 3 combined into a single commit — the domain type (`GeographicAllocations` field) was a hard compilation dependency for the repo layer, so Task 3 was completed alongside Task 2 rather than waiting for its own turn.

## Deviations from Plan
- None significant. Task ordering adjusted slightly (2+3 together) due to compilation dependency.

## Future Improvements
- None yet.

## Known Issues
- Pre-existing: `TestSymbolDetails_CreateAndEnrich` in `tests/integration` is flaky (async cache timing race). Not related to this feature.
