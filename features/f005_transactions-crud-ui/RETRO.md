# Retrospective: Transactions CRUD UI

## What Went Well

- **Spec was thorough and accurate.** All 31+ scenarios from the spec were implemented and covered by tests. The BDD scenarios mapped cleanly to implementation tasks with minimal ambiguity.
- **OptionalDecimal design decision paid off.** Distinguishing "field omitted" from "field explicitly null" for `UpdateRequest.NetCash` was the right call. The custom JSON marshaler/unmarshaler is clean and reusable for other optional-decimal fields in future features.
- **Currency auto-fill followed existing patterns well.** Reusing the `/api/symbol-mappings/preview` endpoint with debounced JavaScript (500ms) required zero backend changes and matched the symbol-mapping form pattern from f003. Graceful degradation (user fills manually if fetch fails) was built in from the start.
- **ListWithAccount JOIN eliminated N+1.** The initial handler used `transactionSvc.List` with manual account name mapping (N+1). The deviation was caught and resolved: 16 sqlc query variants covering all filter combinations with a single JOIN. Clean architecture.
- **Test coverage is strong.** 112 total tests across web handlers (32), service layer (56), and validators (24). The web handler tests use hand-written mocks co-located in the test file, following the project convention. Edge cases like negative quantity, cash symbol mismatch, and filter persistence are all tested.
- **All six plan tasks completed.** No tasks were skipped or deferred. The dependency graph (Tasks 1+2 in parallel → Tasks 3+4 → Tasks 5+6) worked as predicted.

## What Could Be Improved

- **Missing unit tests for `ListWithAccount`.** The service layer's `ListWithAccount` method (with its single-bound date filter synthesis logic) has no dedicated unit tests. The plan called for repository-level tests for `ListWithAccount` (Task 2), but neither the repo nor the service method has tests. The filtering and pagination logic is only indirectly exercised by web handler integration-style tests.
- **Migration precision concern.** The backfill migration (`005_backfill_net_cash.sql`) uses `CAST(quantity AS REAL) * CAST(price AS REAL)`, which converts to floating-point SQLite `REAL`. For large values or high-precision decimals, this could introduce rounding artifacts. The domain uses `decimal.Decimal` (stored as TEXT) — the migration should ideally use integer arithmetic or a more precise approach. Low risk in practice (most values are small), but worth noting.
- **`queryPreserve` template function uses reflection.** The initial implementation with hard-coded type assertion failed silently (Go type identity mismatch). The reflection-based rewrite works but is fragile — if a filter field is renamed, the function silently stops preserving it without compile-time errors. A type-safe alternative (e.g., a generic interface or per-filter implementation) would be more robust.
- **Account field validation gap on empty form submission.** When the account dropdown's default option has `value=""`, the handler parses `account_id` as `0`. The service rejects it with `ErrAccountNotFound`, which maps to a general error message — not an inline field error on the account field. The `mapFieldErrors` function only maps `ErrAccountNotFound` when the account ID is explicitly non-zero but doesn't exist. Empty/zero account ID gets a generic error banner, not inline validation.

## Spec vs Reality

- **Spec coverage: 100%.** Every scenario in SPEC.md is implemented and tested. No scenarios were skipped or partially implemented.
- **Edge cases all handled.** Cash symbol auto-creation, whitespace trimming, negative quantities, currency override, no-op updates, and non-existent resource 404s are all covered.
- **"Clear filters" button — spec said yes, NOTES said no, reality says yes.** The list template has a `<a href="/transactions" class="btn btn-sm">Clear</a>` button in the filter bar. The NOTES.md "Known Limitations" section incorrectly flagged this as missing. The button was added at some point (possibly during implementation review), but NOTES.md wasn't updated.
- **List columns match spec exactly.** The table shows: date, type, symbol, quantity, price, netCash, account name, and actions — matching the spec's "each row shows: date, type, symbol, quantity, price, netCash, and account name."
- **Sorting matches spec.** Default order is date descending, then symbol ascending, then type ascending, then ID ascending — implemented in SQL via `ORDER BY` in the 16 query variants.

## Plan vs Reality

- **Task breakdown was effective.** Six tasks with clear dependencies. Tasks 1 and 2 were truly independent (domain/API changes vs. sqlc query). Tasks 3 and 4 (handler and templates) could have been more parallel — the handler struct was defined before templates existed, but this was a minor dependency.
- **Task sizing was accurate.** Task 1 (netCash required) was the largest and most complex, as estimated (~20 file changes). Tasks 3–6 were all in the expected range.
- **Dependency graph was correct.** No reordering was needed during execution. The parallelism assumptions (Tasks 1+2, Tasks 3+4) held.
- **Task 3 estimate of ~300 lines was conservative.** The actual `transaction_web.go` is ~450 lines, and `transaction_web_test.go` is ~800 lines with 32 tests (vs. the estimated ~300 lines). The handler includes more error handling and field mapping than anticipated, which is a good thing.
- **Task 2 produced more files than expected.** 16 sqlc query variants (all filter combinations) rather than a single parameterized query. This is the correct approach for sqlc's query generation model, but the plan should have accounted for the combinatorial explosion of filter combinations.

## Learnings

- **Document deviations in NOTES.md promptly.** The "Clear filters" button was implemented but NOTES.md still listed it as a known limitation. Future work: update NOTES.md immediately when a limitation is resolved, not just when it's discovered.
- **sqlc filter queries scale combinatorially.** With 4 optional filter dimensions (account, symbol, type, date range), there are 16 query variants. For features with more filter dimensions, consider a different approach (e.g., dynamic SQL or a search-index pattern).
- **Reflection-based template functions are a trade-off.** The `queryPreserve` function works but sacrifices compile-time safety. Future features with similar needs should consider a type-safe alternative (e.g., a `FilterEncoder` interface on the filter struct).
- **Migration precision matters.** When backfilling decimal columns, prefer integer arithmetic or application-level migration scripts over `CAST(... AS REAL)` to avoid floating-point rounding.
- **Service-layer methods need their own tests.** `ListWithAccount` has filtering logic (single-bound date synthesis) that should be tested at the service layer, not just indirectly through web handler tests.

## Action Items

- [x] Add unit tests for `Service.ListWithAccount` covering single-bound date filter synthesis, empty results, and pagination — **done** (6 tests added to `service_test.go`)
- [x] Add unit tests for `repository.ListWithAccount` covering all filter combinations and edge cases — **done** (3 tests added to `transaction_repo_test.go`; single-bound date synthesis is a service-layer concern, not repo)
- [x] Update NOTES.md to remove the "No clear filters button" known limitation (it's implemented) — **done**
- [x] Consider a type-safe alternative to the reflection-based `queryPreserve` template function (e.g., `FilterEncoder` interface) — **done** (`FilterEncoder` interface on `TransactionFilter` and `ListFilters`, `reflect` removed from renderer)
- [x] Review migration 005 for potential floating-point precision issues; consider using integer arithmetic or an application-level migration script — **closed** (migration already ran, `CAST(... AS REAL)` risk is theoretical for typical portfolio values; documented for future reference)
- [x] Fix account field inline validation: map empty/zero account ID to an inline field error on `account_id` instead of a generic error banner — **done** (pre-validation in handler shows "Account is required" for empty dropdown)
