# Retrospective: Transaction CRUD

## What Went Well

- **Task breakdown was effective.** 8 tasks, each independently testable with clear verification criteria. Dependencies were ordered correctly (migration → domain → repo → service → handler → router → integration → cascade), and no task blocked on another unexpectedly.

- **Comprehensive test coverage.** 129 top-level tests across 5 files (20 validator, 56 service, 17 repo, 22 handler, 14 integration), plus 85+ table-driven sub-tests in the validator and 8 validation sub-tests in integration. Every spec scenario has at least one corresponding test.

- **Existing patterns followed consistently.** Router wiring reused `parsePagination`, `parseID`, `writeJSON`, `writeJSONError`. Adapter pattern (`AccountCheckerImpl`, `SymbolCheckerImpl`, `SymbolCreatorImpl`) mirrored `PortfolioCheckerImpl`. Mock structure in `service_test.go` followed the `account/service_test.go` pattern (hand-written mocks with internal state, not expectation-based).

- **Integration tests caught real issues.** The `UpdateNoChanges` test revealed SQLite `datetime('now')` precision differs from Go `time.Now()`, requiring comparison against the DB-stored timestamp rather than the create-time value. This would have been missed without real-SQL integration tests.

- **Clean separation of concerns.** Domain (validation + business logic), data (persistence + adapters), API (HTTP + routing) layers are well-isolated. No cross-layer dependencies.

## What Could Be Improved

- **`ErrImmutableField` is dead code.** The error is defined in `service.go:71-72` and mapped in the handler (`transaction.go:163-164`), but `UpdateRequest` doesn't include `account_id`, so the scenario "Reject update of account_id (immutable)" can never trigger through the API. The spec scenario is implicitly covered by DTO design (field not exposed), but this leaves unreachable code. Either add `account_id` to `UpdateRequest` with explicit rejection, or remove the error code.

- **Mock duplication.** `mock_repository.go` (service tests) and `transaction_test.go` (handler tests) both implement in-memory repository mocks with similar filtering logic. A shared mock or the service-layer mock reused in handler tests would reduce duplication.

- **Decimal API learning curve.** Three separate fixes were needed during implementation: `IsPositive()` → `IsPos()`, `New()` → `MustNew()`, and scale misunderstanding (`MustNew(15000, 2)` = 150.00, not 150). Documenting the `govalues/decimal` API in `CONVENTIONS.md` would help with future features.

- **Adapter types not planned.** The plan for Task 6 (router wiring) didn't anticipate needing `AccountCheckerImpl`, `SymbolCheckerImpl`, and `SymbolCreatorImpl` adapter types to bridge transaction domain interfaces with existing data layer types. This was a minor deviation but should be considered in future plans when domain interfaces cross feature boundaries.

## Spec vs Reality

- **56 spec scenarios, all covered.** Every scenario is implemented and tested. No scenarios were dropped or deferred.

- **One spec correction during implementation.** The spec originally said "limit=0 means no limit" but was updated to "limit=0 defaults to 50" to match the existing account service convention. This was the right call — consistency matters more than the original wording.

- **Immutable account_id handled by DTO design.** The spec scenario "Reject update of account_id (immutable)" is satisfied by not including `account_id` in `UpdateRequest`. This is a valid approach (defense in depth at the type level) but differs from the spec's implication of explicit rejection logic with `IMMUTABLE_FIELD` error code.

- **Single-bound date filters.** The spec described "date from only" and "date until only" scenarios. The implementation handles these at the service layer by synthesizing the missing bound (far-future / far-past), which is a clean approach not explicitly mentioned in the spec but consistent with the intent.

## Plan vs Reality

- **Task sizing was appropriate.** Each task was completed in one focused session. No task needed to be split or merged.

- **Dependencies were accurate.** The migration → domain → repo → service → handler → router → integration ordering worked without rework.

- **One documented deviation.** Migration numbered `004` instead of `003` (sequential after symbol_mappings). Noted in NOTES.md.

- **17 specialized SQL queries.** The plan called for this approach and it was executed as designed. Trade-off: verbose but explicit, type-safe via sqlc, no dynamic SQL. Works well for the 5 filter dimensions (account, symbol, type, date_from, date_to).

## Learnings

- **Document third-party API quirks early.** The `govalues/decimal` API has non-obvious methods (`IsPos` not `IsPositive`, `MustNew` not `New`). Add a decimal usage reference to `CONVENTIONS.md` or `AGENTS.md` for future features.

- **Plan adapter types when crossing feature boundaries.** When a feature depends on interfaces from another feature (e.g., transaction depends on f002's account repo and f003's symbol service), plan for adapter/wrapper types in the router wiring task.

- **CRUD features must include both API and web UI.** This feature only implemented the REST API layer. The established pattern from f001/f002/f003 is that every resource gets server-rendered web pages (`*_web.go` + templates + nav link) alongside the API. The plan missed this entirely — no web handler, no templates, no nav link activation. Added a "Feature Scoping" convention to AGENTS.md to prevent this in future features.

- **Integration tests are worth the effort.** The SQLite timestamp precision issue and cascade delete verification would have been missed with unit tests alone. Keep this pattern for future features.

- **DTO-level immutability is sufficient.** Not exposing `account_id` in `UpdateRequest` is a simpler and more robust approach than explicit rejection logic. Prefer this pattern for future immutable fields.

## Action Items

- [x] Update `features/README.md` to mark f004 as "done"
- [x] Address `ErrImmutableField` dead code: removed error constant from `service.go` and handler mapping from `transaction.go`
- [x] Add `govalues/decimal` API reference to `AGENTS.md` (method names: `MustNew`, `IsPos`, `String`, `Equal`, `Parse`, `MustParse`)
- [x] Add "Feature Scoping" convention to AGENTS.md: user-facing features include web UI by default
