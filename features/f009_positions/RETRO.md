# Retrospective: Positions

## What Went Well

- **Calculator decomposition (Tasks 4a-4e)** was the standout structural decision. Splitting the position calculator into lot grouping, FIFO matching, position computation, cash positions, and integration — each independently testable with table-driven tests — made a complex algorithm tractable and debuggable. 110 unit tests across 8 test files in the position domain alone.

- **Hand-written mocks** followed the project pattern consistently. Service tests use mocks that simulate real behavior (e.g., `mockPositionRepository.Recalculate` actually deletes old data and inserts new), catching service-layer bugs that permissive expectation-based mocks would miss.

- **FX API refactoring mid-stream** was a good catch. Replacing the lossy "pair string" (`"GBP/USD"`) parameter with explicit `baseCurrency, quoteCurrency` parameters eliminated parse/format round-trips and made the API self-documenting. This was noticed during Task 10 and applied across the entire FX chain.

- **Web rendering regression tests** (4 integration tests hitting actual pages and verifying 200 OK) caught real bugs: `.BaseCurrency` undefined on `Position` struct in `closed.html`, and `market_data.updated_at` migration failure. These are valuable for preventing template errors in production.

- **Batch quote fetching** optimization using `go-yfinance`'s `multi` package (shared HTTP client, auth handled once) was a natural improvement discovered during Task 11 that reduced API calls from N to 1.

- **No TODOs, FIXMEs, or HACKs** remain in the codebase. All planned work was completed and cleaned up.

- **All 13 tasks completed**, all 14 migrations (009-014) run cleanly, all tests pass (110 unit + 88 integration across the project), ~8,096 lines of code added.

## What Could Be Improved

- **Feature size.** The spec has 8 user stories and 30+ scenarios — this is the largest feature in the project by far (~8K LOC). Future features of comparable scope should consider splitting into two features (e.g., core positions + lots, then market data + FX as a follow-up). The plan handled it well, but the cognitive load was high.

- **Bubble sort in `sortPositions`.** The `sortPositions` function in `service.go` uses a hand-rolled bubble sort (O(n²)). Should use `sort.Slice` from the standard library. This is O(n²) for no reason — the function is called on every positions list fetch.

- **Summary computation duplication.** `computePositionSummary` exists in the web handler (`position_web.go`) but is unused — the actual summary is computed in the service layer via `GetOpenPositionsSummary`/`GetClosedPositionsSummary`. The dead function should be removed.

- **6 migrations for one feature** (009-014). While each migration addressed a specific concern discovered during implementation, this fragments the schema evolution. If the feature were to be rebased, consolidating to 1-2 migrations would be cleaner. Not a blocker, but worth noting for future features.

- **10,000-row fetch limit for summaries.** `GetOpenPositionsSummary` and `GetClosedPositionsSummary` fetch up to 10,000 positions to compute totals. For a single-user app this is fine, but if the portfolio grows large, aggregating in SQL would be more efficient. Acceptable for now.

- **`consumptions` parameter in `ComputePositions`** is accepted but not used (reserved for future audit trail). This is a minor interface pollution — consider removing if no concrete use case materializes.

## Spec vs Reality

- **All 8 user stories implemented:** View open/closed positions, filter by portfolio/account, drill down into lots and transactions, assign lot IDs, multi-currency P&L, manual recalc, web UI.

- **All 30+ scenarios covered.** Every scenario in the spec has corresponding implementation and tests. No scenarios were dropped.

- **Scenarios the spec missed (discovered during implementation):**
  - GBp-to-GBP conversion for UK stocks (Yahoo Finance returns some prices in pence). Added as a runtime detection in `EnrichWithMarketData`.
  - `toPosition()` crash on empty-string nullable fields (`avg_close_price`, `sell_price` stored as `""` not `NULL`). Fixed with `&& p.Field.String != ""` guards.
  - Currency column showed symbol instead of currency in `buildPosition`. Fixed with `getCurrencyFromLots()`.
  - Avg open price displayed as negative (cost basis is negative). Fixed with `.Abs()`.
  - These were implementation details, not spec gaps — the spec was correct about the behavior; the bugs were in the execution.

- **Edge cases:** All 20+ edge cases from the spec are handled. The spec's edge case list was thorough.

## Plan vs Reality

- **Task breakdown was effective.** 13 tasks, each independently testable, with clear verification criteria. The dependency graph was accurate — no task was blocked by an incomplete predecessor.

- **Tasks were well-sized.** No single task was too large. The calculator split (4a-4e) was the exemplar — each sub-task had its own test file with 8-12 table-driven cases.

- **Deviations were minimal and well-documented in NOTES.md:**
  - Task 1 required updating existing code beyond just migrations (transaction repo, integration test setup).
  - Migrations 012-014 were added after the initial plan (FX rate display, market_data updated_at, realized_pnl_pct).
  - Task 13 (position page enhancements) was added after Task 12 to handle base currency columns and summary panel.
  - All deviations were documented in NOTES.md with rationale.

- **Technical decisions held up.** Pre-computed positions, UUID-based lot IDs, synchronous recalc, unified market_data table — all proved sound during implementation.

## Learnings

- **Decompose complex algorithms into sub-tasks.** The calculator (Tasks 4a-4e) is the model for this: each sub-task is a single function with its own test file. This pattern should be reused for any future feature with non-trivial computation.

- **Challenge design decisions mid-stream.** The FX pair string refactoring showed that questioning a design choice during implementation (not just during planning) can yield meaningful improvements. The skill's "challenge over-engineering" principle applies to API design, not just architecture.

- **Web rendering regression tests are worth the effort.** 4 integration tests that hit actual web pages caught 2 real bugs that unit tests wouldn't have found. This pattern should be standard for all web-facing features.

- **Batch operations should be considered early.** The batch quote fetch optimization was added in Task 11 but should have been part of the initial market data design. Future features with N-to-1 API calls should batch from the start.

- **Decimal library quirks need a cheat sheet.** The `govalues/decimal` API has subtle differences from the expected API (`Less` not `LessThan`, `Quo` not `Div`, `Add` returns `(Decimal, error)`). Documenting these in NOTES.md during Task 4a helped subsequent tasks avoid the same mistakes.

- **Empty string vs NULL in SQLite.** When sqlc stores nullable fields, they come back as `sql.NullString{Valid: true, String: ""}` rather than `Valid: false`. This caused the `decimal.Parse("")` crash. The pattern `&& field.String != ""` is now established.

## Action Items

- [ ] Replace bubble sort in `sortPositions` with `sort.Slice` from stdlib
- [ ] Remove dead `computePositionSummary` function from `position_web.go` (replaced by service-level `GetOpenPositionsSummary`)
- [x] Remove unused `consumptions` parameter from `ComputePositions` — done. Parameter was never read inside the function; all callers passed `nil`. Removed from signature, doc comment, call site, and 10 test calls.
- [ ] Update `features/README.md` to mark f009 as "done"
- [ ] Document the `sql.NullString` empty-string pattern in CONVENTIONS.md for future reference
- [ ] Document the batch API call pattern (deduplicate, batch fetch, partial failure tolerance) in CONVENTIONS.md
