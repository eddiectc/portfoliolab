# Retrospective: auto-create-symbol

## What Went Well

- **Clean backend/frontend separation**: The backend change was minimal (one new `PreviewResponse` struct and `toPreviewResponse` helper in `symbol_mapping.go`). The frontend did the heavy lifting in the template and inline JS. This kept the change surface small and reviewable.

- **Plan accuracy**: All 7 tasks completed as described in PLAN.md with no task additions, deletions, or reordering. The linear dependency chain was accurate — each task built on the previous one.

- **Test coverage for backend**: 6 preview-related handler tests covering happy path, auto-correction, case-insensitive matching, missing symbol, no fetcher, and fetch error. These are well-structured and follow the existing mock pattern.

- **Bug caught during implementation**: The stuck "Creating..." button on 409 responses was identified and fixed by moving button reset to `.finally()`. This was a real user-facing issue that testing alone wouldn't have caught.

- **Review process caught misleading UI**: The cosmetic name/exchange/currency fields in the manual entry form collected input never sent to the API. Removing them during review improved UX clarity.

- **CSS follows existing conventions**: Preview panel styles use the same color palette and spacing patterns as existing form/alert components.

## What Could Be Improved

- **Manual entry form deviation from spec should have been flagged earlier**: The spec says "fields to manually enter all symbol details: name, exchange, currency, and market data symbol." Only the market data symbol field is sent to the API. This was documented in NOTES.md but only after the fact. For future features, flag spec-vs-API gaps during planning rather than implementation.

- **Linear task dependency was overly conservative**: Task 5 (CSS) could have been done in parallel with Task 2 (template), since CSS classes are known upfront. The strict sequential chain added no value here.

- **No integration test for the preview endpoint**: The preview handler tests use hand-written mocks. An integration test against the real symbol mapping service with a real fetcher would catch service-layer bugs that mocks hide.

- **Inline JS not tested**: Consistent with existing project conventions, but worth noting as a known limitation. The preview workflow has multiple state transitions (loading → existing/new/corrected/error) that could regress without tests.

## Spec vs Reality

- **All 16 spec scenarios are implemented**: Every scenario from the spec has a corresponding implementation in the template, JavaScript, or backend handler.

- **Manual entry form is narrower than spec**: The spec envisions manual entry of name, exchange, currency, and market data symbol. The API only accepts `internal_symbol` and `market_data_symbol`. The implementation correctly sends only what the API accepts, but the spec should be updated to reflect this constraint (or the API should be extended in a future feature).

- **Cash symbol auto-creation relies on existing behavior**: Scenarios 7–10 (cash symbols) are handled by the existing transaction service, not by new code in this feature. The feature adds client-side validation (format checking, currency mismatch warning) but the actual auto-create was already implemented. This was correctly noted as a dependency on f004.

- **Auto-correction detection is well-implemented**: The `toPreviewResponse` helper with case-insensitive comparison covers the spec's requirement cleanly. The bonus test for case-insensitive matching (lowercase response for uppercase request) catches a real edge case.

- **Concurrent conflict handling works as specified**: The 409 response from the API is treated as success by the JS (symbol already exists), and `fetchPreviewSilent` refreshes the panel without re-showing errors.

## Plan vs Reality

- **Task breakdown was effective**: Each task was independently testable and delivered a complete slice. No task was split or merged during implementation.

- **Task sizing was appropriate**: No task was too large or too small. The largest task (Task 3: form JavaScript) was still manageable in a single focused session.

- **Dependencies were accurate**: The linear chain (Task 1 → 2 → 3 → 4 → 5 → 6 → 7) reflected actual build order. However, as noted above, Task 5 could have been parallel with Task 2.

- **Technical decisions were sound**: All 7 technical decisions in the plan were implemented as chosen. The `PreviewResponse` flat struct (not embedding `market.Quote`) kept the API boundary clean.

## Learnings

- **Cosmetic form fields that don't map to API are a UX liability**: If a field collects input that's never used, remove it. The user will be confused when their input disappears.

- **Button state management needs `.finally()`**: Any async button action that disables the button must re-enable it in `.finally()`, not just in `.then()` or `.catch()`, to handle all response codes including 409.

- **The existing-symbols JSON embedding pattern works well**: Parsing a JSON array from a `<script type="application/json">` tag gives the client a complete symbol list without an extra API call. This pattern could be reused for other inline creation workflows.

- **Case-insensitive symbol comparison is essential**: Yahoo Finance may return symbols in different cases. The `strings.EqualFold` check prevents false auto-correction detection.

- **`fetchPreviewSilent` is a useful pattern**: After symbol creation, re-fetching the preview should succeed silently — if the market data fetch fails again, the symbol was still created and the user should proceed.

## Action Items

- [x] Update SPEC.md to reflect that manual entry only sends `internal_symbol` and `market_data_symbol` to the API (name/exchange/currency are not persisted from manual entry) — **done**: updated "Preview fails — create symbol manually" scenario
- [x] Consider adding an integration test for `HandlePreview` against the real symbol mapping service with a real quote fetcher — **deferred**: the router always wires the real `YahooFinanceFetcher`, so a full-stack integration test would hit the live Yahoo Finance API. The existing unit tests (6 tests in `symbol_mapping_test.go`) cover the preview logic thoroughly with controlled mock fetchers. Adding a real-API integration test would introduce flakiness without meaningful coverage gain.
- [x] Consider parallelizing CSS and template tasks in future feature plans when CSS classes are known upfront — **process learning**: noted for future feature plans
- [x] Update `features/README.md` to mark f006 as "done" — **done**
