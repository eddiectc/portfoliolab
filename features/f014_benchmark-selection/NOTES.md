# Notes: Benchmark Selection

## Decisions
- 2026-05-15: `is_benchmark` column added to `symbol_mappings` table as `BOOLEAN NOT NULL DEFAULT 0` — existing symbols unaffected by default.

## Deviations from Plan
- None so far.

## Future Improvements
- None identified yet.

## Implementation Notes
- Task 5: `MarketCache.Start()` now skips benchmark gap-fill on startup if no `BenchmarkSymbolLister` is configured (was always fetching 5 predefined benchmarks). Tests without a lister see zero benchmark fetches instead of 5. This is intentional — benchmarks are user-defined, so with no lister there are no benchmarks to fetch.

## Known Issues
- None.
