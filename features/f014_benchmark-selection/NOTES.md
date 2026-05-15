# Notes: Benchmark Selection

## Decisions
- 2026-05-15: `is_benchmark` column added to `symbol_mappings` table as `BOOLEAN NOT NULL DEFAULT 0` — existing symbols unaffected by default.

## Deviations from Plan
- None so far.

## Future Improvements
- None identified yet.

## Implementation Notes
- Task 5: `MarketCache.Start()` now skips benchmark gap-fill on startup if no `BenchmarkSymbolLister` is configured (was always fetching 5 predefined benchmarks). Tests without a lister see zero benchmark fetches instead of 5. This is intentional — benchmarks are user-defined, so with no lister there are no benchmarks to fetch.
- Task 7: `comparison` package still used by API handler (`performance.go`) for `ComputeMWRForPeriod` and `ComputeMonthlyReturns` — just no longer used by the web handler for benchmark names. No cleanup needed.
- Task 7: Typed nil gotcha — assigning a typed `nil *mockBenchmarkLister` to an interface field produces a non-nil interface containing a nil pointer. `h.benchmarkLister == nil` returns false. Workaround: use empty struct literal or explicit type assertion.
- Task 7: Integration tests needed updating — `setupPerf()` now creates `^GSPC` as a benchmark symbol mapping (`is_benchmark: true`) since the handler validates benchmarks against the user-defined list.

## Known Issues
- None.
