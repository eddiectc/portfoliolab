# Implementation Plan: auto-create-symbol

## Overview

Enhance the transaction form (create and edit) with an inline symbol preview and creation workflow. When the user types a symbol, a preview panel shows market data details. For symbols not yet in the symbol map, a "Create" button lets the user create the mapping without leaving the form. If the market data fetch fails, manual entry fields appear. Cash symbols (`$CASH-{currency}`) are handled silently with no preview.

The form loads all existing symbols at page time (already done via `symbolSvc.List()`). Client-side existence checks use this embedded list — no server-side existence endpoint needed.

## Task Dependencies

```
Task 1 (auto-correction detection in preview endpoint)
    ↓
Task 2 (enhanced form template)
    ↓
Task 3 (form JavaScript)
    ↓
Task 4 (cash symbol client-side validation)
    ↓
Task 5 (CSS updates)
    ↓
Task 6 (tests)
    ↓
Task 7 (update feature index)
```

Task 1 is backend. Tasks 2–5 are frontend (template + JS + CSS). Task 6 is testing. Each task is independently testable.

## Tasks

### Task 1: Auto-corrected symbol detection in preview endpoint [PRIORITY: HIGH]
**Corresponds to:** Scenario "Preview shows auto-corrected symbol"
**Description:** Modify `HandlePreview` in `symbol_mapping.go` to detect when go-yfinance returns data for a different symbol than requested (e.g. `AAP` → `AAPL`) and include the corrected symbol in the response.

- [x] Define `PreviewResponse` struct in `internal/api/handlers/` with fields: all fields from `market.Quote` plus `CorrectedSymbol string` (empty string when no correction)
- [x] Update `HandlePreview` to compare `quote.Symbol` vs. the requested `symbol` query param; if they differ (case-insensitive), populate `CorrectedSymbol`
- [x] Return `PreviewResponse` instead of bare `*market.Quote`
- [x] Write unit tests in `symbol_mapping_test.go` for auto-correction detection (matched vs. corrected)

**Verification:** `go test ./...` passes; `GET /api/symbol-mappings/preview?symbol=AAP` returns quote data with `corrected_symbol: "AAPL"` when go-yfinance auto-corrects; empty string when symbols match.

---

### Task 2: Enhanced transaction form template [PRIORITY: HIGH]
**Corresponds to:** Scenario "Preview new symbol while typing", "Create new symbol from preview and submit transaction", "Preview fails — create symbol manually", "Preview does not show for cash symbols"
**Description:** Update `templates/transaction/form.html` to include a preview panel below the symbol field with multiple states and a hidden JSON list of existing symbols for client-side existence checking.

- [x] Add `<script id="existing-symbols" type="application/json">` tag embedding internal symbols as a JSON array (extract from `.Symbols` via template range)
- [x] Add preview panel div below the symbol field with sub-panels:
  - **Loading**: "Fetching market data..."
  - **Existing symbol**: name, exchange, currency, latest price (green indicator)
  - **New symbol**: same details + "This symbol does not yet exist" + "Create symbol" button
  - **Auto-corrected**: "You typed X, market data returned Y" + accept button
  - **Error / manual entry**: warning + input fields for name, exchange, currency, market data symbol + "Create" button
  - **Cash symbol**: panel hidden
- [x] All sub-panels hidden by default, shown/hidden by JavaScript

**Verification:** Form renders with preview panel (hidden by default); `#existing-symbols` script tag present with valid JSON; panel has all required sub-elements.

---

### Task 3: Form JavaScript [PRIORITY: HIGH]
**Corresponds to:** All scenarios involving preview, creation, manual entry, cash symbols, and auto-correction
**Description:** Rewrite the inline JavaScript in the transaction form to handle the full preview and symbol creation workflow. Uses the embedded existing-symbols list for client-side existence checks.

- [x] On page load: parse `#existing-symbols` JSON into a `Set` of known internal symbols
- [x] Debounced fetch (500ms) on symbol input: call `/api/symbol-mappings/preview?symbol=X`
- [x] On success response:
  - Check if symbol is in the existing-symbols set
  - If **exists**: show existing symbol preview (green)
  - If **new** and `corrected_symbol` set: show auto-corrected preview with accept button
  - If **new** and no correction: show new symbol preview with "Create symbol" button
- [x] "Create symbol" button: POST to `/api/symbol-mappings` with `internal_symbol` = typed symbol (or corrected symbol if accepted), `market_data_symbol` = `corrected_symbol` or typed symbol
  - On success: update symbol field, show success indicator, clear preview
  - On 409 (already exists): add to existing-symbols set, refresh preview as "existing"
- [x] On error response (non-2xx): show manual entry fields (name, exchange, currency, market data symbol)
- [x] Manual entry "Create" button: POST to `/api/symbol-mappings` with user-provided values
- [x] Clear preview when symbol field is cleared or changes to a different symbol
- [x] Preserve existing currency auto-fill behavior

**Verification:** All preview states work; symbol creation succeeds; manual entry works; debouncing prevents excessive calls; 409 handled gracefully.

---

### Task 4: Cash symbol client-side validation [PRIORITY: MEDIUM]
**Corresponds to:** Scenario "Cash symbol auto-created silently", "Cash symbol with mismatched currency rejected", "Malformed cash symbol rejected"
**Description:** Add client-side validation for cash symbols in the form JavaScript.

- [x] Detect `$CASH-{currency}` pattern with regex
- [x] Validate format: must be `$CASH-` followed by exactly 3 uppercase letters
- [x] On valid cash symbol: auto-fill currency field if empty, hide preview panel
- [x] On malformed cash symbol (`$CASH-`, `$CASH-USDX`): show inline field error, hide preview
- [x] On currency mismatch (cash symbol currency ≠ transaction currency field): show warning (server will reject on submit, but give early feedback)

**Verification:** `$CASH-USD` accepted silently with no preview; `$CASH-` shows error; `$CASH-USD-EXTRA` shows error; currency mismatch shows warning.

---

### Task 5: CSS updates [PRIORITY: LOW]
**Corresponds to:** All UI scenarios
**Description:** Add CSS classes for the new preview panel elements.

- [x] Add `.symbol-preview` container styles (border, padding, background, margin)
- [x] Add `.symbol-preview-existing` (green accent for existing symbols)
- [x] Add `.symbol-preview-new` (blue accent for new symbols)
- [x] Add `.symbol-preview-corrected` (amber accent for auto-corrected)
- [x] Add `.symbol-preview-error` (red accent for failed preview)
- [x] Add `.manual-entry-fields` styles for the inline manual entry form
- [x] Add `.create-symbol-btn` style for the create button
- [x] Add `.symbol-preview-loading` style

**Verification:** Preview panel renders cleanly with appropriate colors and spacing.

---

### Task 6: Tests [PRIORITY: MEDIUM]
**Corresponds to:** All scenarios
**Description:** Add tests for the enhanced preview endpoint and updated form rendering.

- [x] Unit test in `symbol_mapping_test.go`: preview returns `corrected_symbol` when quote symbol differs from requested symbol
- [x] Unit test in `symbol_mapping_test.go`: preview returns empty `corrected_symbol` when symbols match
- [x] Web handler test in `transaction_web_test.go`: form renders with `#existing-symbols` script tag and preview panel elements
- [x] Web handler test in `transaction_web_test.go`: form renders with embedded symbol list populated from `.Symbols`

**Verification:** `go test ./...` passes; all new tests pass.

---

### Task 7: Update feature index [PRIORITY: LOW]
**Corresponds to:** N/A (bookkeeping)
**Description:** Mark feature as in-progress in the feature index.

- [x] Update `features/README.md` to add f006 row with "in-progress" status

**Verification:** Feature index is up to date.

---

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Symbol existence check | Client-side against embedded list | Form already loads all symbols; no extra API call; simplifies backend |
| Preview response | `market.Quote` + `CorrectedSymbol` field | Minimal change to existing endpoint; backward-compatible |
| Symbol creation from form | Client-side `POST /api/symbol-mappings` | Matches spec's "immediate creation"; reuses existing API; gives user immediate feedback |
| Manual entry UI | Inline expand within preview area | Follows existing symbol mapping form pattern; less visual clutter |
| Cash symbol handling | Client-side detection + existing server validation | Server validator already handles all cases; client-side is UX-only |
| Debounce interval | 500ms (matching existing form JS) | Consistent with existing code |
| Auto-correction | Show corrected symbol, require explicit accept | Matches spec; prevents silent symbol changes |

## Risks

- **Yahoo Finance auto-correction behavior is unpredictable** — go-yfinance may or may not return a corrected symbol for partial inputs. Mitigation: the preview shows whatever go-yfinance returns and lets the user accept or reject.
- **Concurrent symbol creation** — two tabs could try to create the same symbol. Mitigation: the API returns 409 (already exists), form JS treats it as success and refreshes the preview.
- **Form state complexity** — tracking whether symbol was created inline, pending creation, etc. Mitigation: keep state minimal; the symbol field value is the source of truth.
