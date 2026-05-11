# Retrospective: Historical Market Data Caching

## What Went Well

- **Spec was comprehensive and well-scoped** — 16 scenarios covering background fetches, manual refresh, staleness warnings, edge cases, and failure modes. All 16 scenarios were implemented faithfully. Non-goals were explicit and respected (no dedicated management page, no per-symbol refresh, no intraday data).

- **MarketCache service is clean and well-architected** — Channel-based single worker for on-demand fetches, periodic 2-minute ticker for quotes + gap-fill, dual-set concurrent protection (`queued` + `inProgress`), graceful shutdown. ~400 lines of well-structured code with 22 unit tests covering happy paths, failures, FX, lifecycle, and edge cases.

- **MarketDataService abstraction (Task 5.5) was a strong architectural decision** — Consolidated two dependencies (`marketFetcher` + `marketDataRepo`) into one (`marketService`) for consumers. Simplified the dependency graph and made mocking easier. 25 tests covering stock and FX operations.

- **FX normalization (Task 5.6) improved the codebase** — Removed `FxConverter`/`FxRateProvider` abstraction and merged FX rate access into `MarketDataService`. FX rates now follow the same cache-read-only pattern as stock data. Eliminated the inline fallback chain (historical DB → live fetch → spot rate) and simplified the dependency graph significantly.

- **GBp → GBP conversion at the fetcher layer** — Single source of truth for currency quirks. Position service no longer needs to handle currency edge cases.

- **Test quality is solid** — 1,292 tests across the project, 994 passing (all pass with `-short`). Hand-written mocks simulate real behavior. Table-driven tests in both `marketcache` and `marketservice` packages.

- **All tasks completed, no TODOs remaining** — `go vet` clean, no unresolved issues.

## What Could Be Improved

- **Task 4.1 (post-completion bug fixes) was large and critical** — 11 sub-tasks including two critical bugs (double-negated sell quantities, FX SQL filter excluding `data_type='fx'`). These should have been caught during initial implementation. The equity curve at 1,201 lines is complex enough that critical path bugs can hide.

- **Missing integration-level validation** — The double-negation bug (sells adding to positions instead of subtracting) and the FX forward-fill SQL filter bug (inflating GBP portfolios by ~30%) both went undetected by unit tests. An integration test with real transactions (buys + sells + FX) and verifying the equity curve end-to-end would have caught these.

- **Weekend staleness logic required a post-hoc fix** — The initial staleness check used calendar days, triggering false warnings on weekends. The fix (`tradingDayBeforeOrOn` / `nextTradingDay` helpers) was correct but should have been in the initial design.

- **Equity curve (1,201 lines) is large** — While the refactor extracted `WalkPortfolioState` as a single source of truth, the file remains dense. Further decomposition (e.g., separating FX conversion, interpolation, and TWR calculation into their own files) could improve readability.

- **Positions page staleness is less precise** — The positions template uses `CacheStatus.FailedSymbols` as a staleness proxy since `EnrichWithMarketData` doesn't produce per-symbol staleness warnings. This is acceptable for the aggregate indicator but worth noting.

- **Recalculate hooks schedule fetches for ALL open position symbols** — Even if cache already exists. The concurrent protection deduplicates, but this means extra channel messages. Documented as a future improvement in NOTES.md.

## Spec vs Reality

| Aspect | Assessment |
|---|---|
| **Scenario coverage** | All 16 scenarios implemented. No scenarios were dropped or significantly altered. |
| **Edge cases** | All spec edge cases handled (delisted symbols, long history, concurrent refresh, empty portfolio, FX with no data, rate limiting). Two additional edge cases discovered during implementation (GBp currency, weekend staleness) were fixed. |
| **Non-goals respected** | Yes — no dedicated management page, no per-symbol refresh, no intraday data, no alternative providers, no price adjustment. |
| **Constraints** | Append-only cache ✓, daily only ✓, single provider ✓, persisted in existing price cache ✓, non-blocking ✓ |
| **Spec missed** | The spec didn't anticipate the GBp currency quirk or the weekend staleness false-positives. These are provider-specific and calendar-specific edge cases that are hard to foresee without real-world data. |

## Plan vs Reality

| Aspect | Assessment |
|---|---|
| **Task breakdown** | 10 tasks (1–9 + 4.1). Task 4.1 was unplanned and substantial (11 sub-tasks). Tasks 5.5 and 5.6 were added mid-implementation as architectural improvements. |
| **Task sizing** | Task 3 (MarketCache) was appropriately sized. Task 4 (equity curve changes) was underestimated — the complexity of position tracking, FX conversion, and interpolation made it a large refactoring. Task 4.1 revealed that the initial Task 4 implementation had critical bugs. |
| **Dependencies** | Mostly accurate. The FX normalization (Task 5.6) created more downstream refactoring than anticipated (5 test files updated, `FxConverter` deleted, interfaces extended). |
| **Technical decisions** | All 4 documented decisions (A–D) were sound and validated by implementation. Channel-based worker, separate `marketcache` package, aggregate status UI, and transaction service hooks all worked as intended. |
| **Verification criteria** | All tasks met their verification criteria. `go build ./...` and `go test ./...` pass cleanly. |

## Learnings

- **Complex position logic needs integration-level validation** — Unit tests with mocks can verify individual functions, but the interaction between transaction quantities (signed), position tracking, FX conversion, and equity curve interpolation requires end-to-end tests. Consider adding a golden-file integration test: given a set of transactions (buys, sells, multiple currencies), verify the equity curve values match expected output.

- **FX data requires cross-layer attention** — The FX SQL filter bug (`data_type = 'stock'` excluding FX) showed that changes in one layer (cache storing FX with `data_type='fx'`) can silently break consumers (equity curve reading historical data). When adding a new data type, audit all read paths.

- **Signed quantity conventions are fragile** — The double-negation bug (sells stored as negative, then `Neg()`d to positive) was a convention mismatch between the calculator and the equity curve. Extracting `WalkPortfolioState` as a shared component was the right fix. Document signed conventions explicitly at the type level.

- **Dual-set concurrent protection (`queued` + `inProgress`) is a useful pattern** — Prevents both in-progress duplicates and channel-in-flight duplicates. Worth documenting as a reusable pattern for similar scenarios.

- **Post-completion fixes should be planned as a task** — Task 4.1 was added after production use revealed bugs. For future features, consider a "validation" or "hardening" task that runs after initial implementation to stress-test edge cases before declaring the feature done.

- **The `MarketDataService` abstraction pattern is worth reusing** — Concrete struct in one package, interface defined by consumers. Avoids circular dependencies while keeping the abstraction flexible.

## Action Items

- [x] Add an end-to-end equity curve integration test with buys, sells, and FX conversions to catch cross-layer bugs earlier
  → `tests/integration/equity_curve_test.go` (3 tests: buys/sells/deposits, FX forward-fill, full sell)
- [x] Document the signed quantity convention in `transaction.Transaction` struct comments
  → Added type-level doc + field-level doc on `Quantity` in `internal/domain/transaction/transaction.go`
- [x] Decompose `equity_curve.go` (1,201 lines) into smaller files
  → Split into 4 files: `equity_curve.go` (652, orchestrator), `fx_conversion.go` (224), `interpolation.go` (169), `twr_breakpoints.go` (189)
- [x] Add a "validation/hardening" task template to the implementation plan for future features
  → Added "Task N: Validation / Hardening" section to the PLAN.md template in `.pi/skills/agile-workflow/SKILL.md`
- [x] Audit all read paths when adding new `data_type` values to the market_data table (create a checklist)
  → Added "Cross-Layer Data Audit" section to `AGENTS.md` with 4-step checklist
- [x] Optimize recalculate hooks to check cache existence before scheduling fetches (trade-off: extra DB query vs. channel messages)
  → `position/service.go` `scheduleCacheFetches` now checks `GetLatestPriceDatePerSymbol` and skips symbols with today's cache
