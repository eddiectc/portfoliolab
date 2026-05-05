# Notes: auto-create-symbol

## Decisions
- 2026-05-05: Task 1 implemented `PreviewResponse` as a flat struct (not embedding `market.Quote`) to keep the JSON response shape explicit and avoid exposing internal types at the API boundary.
- 2026-05-05: Added `json` template function to `web/renderer.go` to support embedding data as JSON in `<script>` tags (used by the existing-symbols list in the form template).

## Deviations from Plan
- Task 3: The manual entry form includes fields for name, exchange, and currency, but the `CreateRequest` API only accepts `internal_symbol` and `market_data_symbol`. The JS sends only those two fields. The name/exchange/currency manual fields are cosmetic (help the user understand what they're creating) but are not persisted. The spec says "the symbol mapping is created with the details I provided" — this would require extending the SymbolMapping model and CreateRequest, which is out of scope for this task.

## Future Improvements
- None noted yet.

## Task 6 Notes
- Sub-tasks 1–2 (preview unit tests) were already implemented during Task 1 (`TestHandlePreview_SymbolsMatch`, `TestHandlePreview_AutoCorrectedSymbol`). Verified they pass.
- Sub-tasks 3–4 added as new tests: `TestTxHandleNewPage_PreviewElements`, `TestTxHandleNewPage_EmbeddedSymbolList`, `TestTxHandleNewPage_EmptySymbolList`.

## Bug Fixes

### Stuck "Creating..." button (2026-05-05)
When creating a symbol via the manual entry form, if the POST returned 409 (symbol already exists), the button stayed permanently disabled with "Creating..." text. The `.then()` handler treated 409 as success and called `onSymbolCreated`, but button re-enablement was only in the `.catch()` path. Fixed by moving button reset to `.finally()` on both create-symbol buttons.

## Test Limitations

The inline JS in `templates/transaction/form.html` (symbol preview, button state management) is not unit-tested. This is consistent with existing project conventions — JS in templates and `web/static/js/` is not tested. The API endpoints (`/api/symbol-mappings`, `/api/symbol-mappings/preview`) are covered by handler and integration tests.

## Review Fixes (2026-05-05)
- Removed cosmetic name/exchange/currency fields from the manual entry form — they collected input never sent to the API, which was misleading
- Removed unused `json` template function from `web/renderer.go` — was added during implementation but the template constructs JSON manually with `{{range}}`

## Known Issues
- None.
