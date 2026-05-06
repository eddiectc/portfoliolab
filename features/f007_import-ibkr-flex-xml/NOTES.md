# Notes: Import IBKR Flex XML

## Implementation Decisions

### Handler accepts interfaces, not concrete types (Task 4)

The `ImportHandler` defines its own `ImportService` and `SymbolService` interfaces rather than accepting `*ibkrimport.Service` and `*symbolmapping.Service` directly. This enables hand-written mocks in handler tests without depending on the concrete service implementations. The router passes the real services which satisfy the interfaces.

### Data-layer adapters for IBKR import (Task 4)

Created `internal/data/ibkr_resolvers.go` with two adapters:
- `SymbolResolverImpl` — bridges `SymbolMappingRepository` to `ibkrimport.SymbolResolver` (two-step lookup: broker symbol → symbol mapping → internal symbol)
- `BrokerSymbolAdderImpl` — bridges `SymbolMappingRepository` + `symbolmapping.Service` to `ibkrimport.BrokerSymbolAdder` (resolves internal symbol to mapping ID, then delegates to service)

### BatchCreate added to TransactionRepository (Task 4)

Added `BatchCreate(ctx, []*transaction.Transaction) error` to `TransactionRepository` using `sqlDB.BeginTx()` for atomic all-or-nothing inserts. The repo struct now stores both `db queries.DBTX` (for single-row queries) and `sqlDB *sql.DB` (for transaction support).

### XML attribute name preprocessing (Task 1)

### XML attribute name preprocessing (Task 1)

IBKR uses camelCase attributes (`tradePrice`, `ibOrderID`, `ibCommission`, `ibExecID`) that don't map cleanly to Go's `encoding/xml` tag conventions. Chose simple `strings.ReplaceAll` preprocessing over custom `UnmarshalXML` — fewer moving parts, easier to reason about. Required 4 replacements (not just the 2 mentioned in the plan: `ibCommission` and `ibExecID` also needed it).

### Wrapper struct slice tags (Task 1)

The inner `[]tradeXML` in the `trades` wrapper struct needed an explicit `xml:"Trade"` tag on the slice field itself — not just on the element struct's `XMLName`. Without it, Go's xml decoder silently returns zero elements. Same for `cashTransactions` and `transfers`.

### FX trades processed before instrument type check

### FX trades processed before instrument type check

The plan lists "classify instrument type" as the first step in `processTrade`. However, FX trades (`assetCategory=CASH`) must be handled **before** the `isSupportedTrade` check, since `isSupportedTrade` only accepts `STK` instruments and would reject FX trades as "unsupported". The ordering is:

1. Check if FX trade (`assetCategory=CASH`) → handle, return early
2. Check if supported instrument (`STK COMMON/ETF`) → skip if not
3. Resolve symbol → skip if unmapped
4. Check duplicate → skip if exists

### FX target currency extraction

FX symbol format is `{sourceCurrency}.{targetCurrency}` (e.g. `GBP.USD`). The target currency is extracted by splitting on `.` with `strings.SplitN(trade.Symbol, ".", 2)` and taking the second part. This is more robust than `strings.TrimSuffix` which would require knowing the exact source currency suffix.

### FX trades generate composite external references

Each FX trade splits into two transactions (withdrawal + deposit). Both share a composite external reference derived from the original transaction ID:

- `{transactionID}_fx_withdrawal` — source currency withdrawal
- `{transactionID}_fx_deposit` — target currency deposit

This ensures both halves are traceable to the original IBKR transaction and individually de-duplicated.

### FX duplicate detection is per-trade, not per-entry

When an FX trade is detected as a duplicate (by its original `transactionID`), the **entire trade** is skipped — it doesn't generate 2 skipped entries. This means the skipped count reflects the number of source records, not the number of generated preview entries. The plan's test expectation of "16 skipped" for all-duplicates was corrected to 15 (15 source records, not 16 generated entries).

### Cash transaction "Deposits/Withdrawals" classified by amount sign

Cash transactions with `type="Deposits/Withdrawals"` are classified by the sign of the `amount` field: positive → deposit, negative → withdrawal. Zero or empty amount defaults to deposit. This matches the Rust reference implementation.

### ConfirmImport builds `*transaction.Transaction` directly

The plan mentions building `transaction.CreateRequest` for each importable transaction. In practice, the service builds `*transaction.Transaction` structs directly, since the `TransactionCreator.BatchCreate` interface accepts `[]*transaction.Transaction`. The `CreateRequest` DTO is only used at the API handler layer for JSON deserialization.

### Transfer netCash uses original amount (not absolute)

For transfers, the `NetCash` field on the transaction uses the original `cashTransfer` value from the XML (positive for deposits, negative for withdrawals). The preview display shows absolute values for consistency, but the actual transaction preserves the sign. This aligns with the existing transaction model where `NetCash` is signed.

### Withdrawal cash transactions use original negative amount

For cash transactions classified as withdrawals (negative `amount`), the `Quantity` and `NetCash` fields preserve the negative sign from the XML. The preview display shows the absolute value for readability. This is consistent with how deposits work (positive amount throughout).

## Plan Deviations

| Plan Item | Actual | Reason |
|---|---|---|
| `CreateRequest` built in ConfirmImport | `*transaction.Transaction` built directly | `BatchCreate` interface accepts `[]*transaction.Transaction`; `CreateRequest` is API-layer only |
| 16 skipped for all-duplicates preview | 15 skipped | FX trade is 1 source record → 1 skip, not 2 entries |
| Instrument type check first | FX check first | `isSupportedTrade` rejects `CASH` instruments; FX must be handled before that check |
| `HandleCreateSymbolPost` form handler | AJAX to existing API endpoint | `ImportService` interface doesn't expose `CreateSymbol`; follows existing transaction form pattern (AJAX symbol creation) |
| `HandleAddBrokerSymbolPost` form handler | AJAX to existing API endpoint | Reuses existing `/api/transactions/import/ibkr/broker-symbols` endpoint |
| Separate `import_result.html` template | Redirect to `/transactions` with flash | Simpler UX — flash message summarizes result; user lands on transactions list to verify |
| XML data passed as file re-upload | Base64-encoded hidden form field | Avoids re-uploading the file; hidden input carries the data through preview → confirm flow |
| `contains` template function added | Added `strings.Contains` to renderer funcMap | Needed for conditional rendering of "Resolve Symbol" button on unmapped symbols |

## Session: Task 6 (Router Wiring + Navigation)

The API handler (`ImportHandler`) was already wired in router.go from a previous session — it was created and registered outside the renderer block. Task 6 only needed to add the web handler (`ImportWebHandler`) inside the renderer block, since it depends on the `*web.Renderer`.

Added a CSS-only dropdown (click-to-toggle via a tiny inline `onclick`) in the Transactions page header. The dropdown uses a `.dropdown.open` class toggle rather than `:hover` to work better on mobile/touch. Structured with a `.dropdown-menu` that's ready for future broker additions (e.g., Trading 212).

### Files changed
- `internal/api/router.go` — added `ImportWebHandler` registration inside renderer block
- `templates/transaction/list.html` — added Import dropdown with header-actions wrapper
- `internal/web/static/css/style.css` — added `.dropdown`, `.dropdown-toggle`, `.dropdown-menu` styles

## Test Coverage

- 74 tests total (27 parser + 32 service + 5 BatchCreate repo + 10 web handler)
- All tests pass: `go test ./...`
- Full sample XML integration test verifies all 16 transactions created correctly
- Edge cases: all-duplicates, unmapped symbols, FX trades, transfers, cash transaction types, rollback, empty report
- Web handler tests: upload page rendering, preview with valid/invalid XML, account not found, missing account, confirm success/error, route registration

## Post-Review Fixes (Implementation Review)

### Unique index on (external_system, external_reference)

Added migration `006_add_external_ref_unique_index.sql` with a partial unique index:

```sql
CREATE UNIQUE INDEX idx_transactions_external_ref
    ON transactions(external_system, external_reference)
    WHERE external_system IS NOT NULL AND external_reference IS NOT NULL;
```

The `WHERE` clause allows multiple NULL/NULL rows (manually entered transactions) while enforcing uniqueness for imported transactions. Includes a defensive `DELETE` to clean any pre-existing duplicates before creating the index. This eliminates the race condition risk where two concurrent imports could both pass the application-level duplicate check before either commits.

### Context propagation in ConfirmImport helpers

Changed `buildTradeTxns`, `buildCashTxn`, and `buildTransferTxn` to accept `ctx context.Context` as their first parameter and pass it through to `DuplicateChecker.ExternalReferenceExists` instead of using `context.Background()`. This ensures the caller's context (deadlines, cancellation) is respected during duplicate detection in the confirm phase.

### BatchCreate repository tests

Added 5 unit tests for `BatchCreate` in `transaction_repo_test.go`:
- `TestTransactionRepository_BatchCreate_Success` — 3 transactions, all inserted
- `TestTransactionRepository_BatchCreate_WithExternalFields` — external fields preserved
- `TestTransactionRepository_BatchCreate_RollbackOnDuplicate` — unique index violation mid-batch rolls back all
- `TestTransactionRepository_BatchCreate_RollbackOnFKViolation` — FK violation mid-batch rolls back all
- `TestTransactionRepository_BatchCreate_EmptyBatch` — empty batch succeeds as no-op

## Session: Import Preview UX Fixes

### Spec/Plan Gaps Identified

Three gaps discovered during manual testing of the import preview flow:

1. **Broker symbol not shown in skipped reason** (spec gap) — The reason column shows "unmapped symbol" without telling the user *which* broker symbol is unmapped. The user has no way to know what to type in the resolve modal.

2. **No market data preview in resolve modal** (plan gap) — The spec constraint states "Symbol creation, broker symbol mapping, and **market data preview are all inline on the import page**" but the plan only said "modal with AJAX to API endpoints" and the implementation produced two bare text inputs. The transaction form already has a polished preview mechanism (fetches `/api/symbol-mappings/preview`, shows name/exchange/currency/price with existing/new/corrected/error panels) that should be reused.

3. **No grouping of same unmapped symbol** (spec gap) — If 5 trades share the same unmapped broker symbol (e.g., STHY appears 5 times in the sample XML), the user sees 5 separate "Resolve Symbol" buttons and must resolve one, re-submit, repeat 4 more times.

### Implementation Decisions

#### BrokerSymbol field on SkippedTransaction

Added `BrokerSymbol string` field to `SkippedTransaction`. Only populated when `Reason` contains "unmapped symbol". The Reason string now includes the broker symbol: "unmapped symbol: {brokerSymbol}".

#### Modal field order: Market Data Symbol first, Internal Symbol second

The modal follows the same field order as the transaction form:
1. **Market Data Symbol** (first, with live preview) — user types the ticker (e.g., "AAPL"), preview shows name/exchange/currency/price inline
2. **Internal Symbol** (second, defaults to market data symbol) — user can override if they want a different internal name (e.g., market data = "BRK-B", internal = "BRK.B")

This matches the transaction form pattern where the market data symbol is the primary lookup and the internal symbol is a secondary choice.

#### Preview panels reused from transaction form

The modal reuses the same 5-panel preview pattern as `templates/transaction/form.html`:
- `preview-loading` — "Fetching market data..."
- `preview-existing` — green indicator, shows symbol exists in symbol map
- `preview-new` — blue indicator, shows market data found, "Create & Map" button
- `preview-corrected` — yellow indicator, shows auto-correction (e.g., "AAP" → "AAPL")
- `preview-error` — red indicator, fallback manual entry

The modal fetches `/api/symbol-mappings/preview?symbol=...` with 500ms debounce, same as the transaction form.

#### Existing symbols check

The modal embeds the existing symbols list (from `/api/symbol-mappings`) as a JSON script tag, parsed into a `Set` for client-side existence checks. This determines whether the preview shows as "existing" (just needs broker mapping) or "new" (needs symbol creation + mapping).

#### Create-then-map flow

When the user clicks "Create & Map" or "Map":
1. POST to `/api/symbol-mappings` (create symbol) — 201 or 409 (already exists, both OK)
2. POST to `/api/transactions/import/ibkr/broker-symbols` (add broker mapping)
3. Re-submit the confirm form to refresh the preview

This is the same two-step flow as before, but now the market data symbol from the preview (or corrected symbol) is used instead of requiring the user to type it separately.

#### Grouped unmapped symbols section

The skipped table is restructured:
- **Grouped unmapped symbols** section at the top — one row per unique broker symbol, showing count (e.g., "STHY (5 transactions)") with a single "Resolve" button
- **Other skipped** section below — duplicates and unsupported instrument types, one row each

This eliminates the repeat-resolve loop. Resolving one symbol in the group immediately resolves all its transactions.

### Files Changed

| File | Change |
|---|---|
| `internal/domain/ibkrimport/models.go` | Add `BrokerSymbol string` to `SkippedTransaction` |
| `internal/domain/ibkrimport/service.go` | Populate `BrokerSymbol` and include it in `Reason` for unmapped cases |
| `internal/domain/ibkrimport/service_test.go` | Update tests that check unmapped symbol reason |
| `templates/transaction/import_preview.html` | Restructure skipped table (grouped + other), rewrite modal with preview |
| `internal/api/handlers/ibkr_import_web.go` | Add `Symbols` to preview page data (existing symbol list for client-side checks) |
| `internal/api/handlers/ibkr_import_web_test.go` | Update test data to include `BrokerSymbol` |
| `internal/web/static/css/style.css` | Add styles for grouped section, modal preview panels |

### Implementation Plan

#### Task A: Model + Service — Add BrokerSymbol to SkippedTransaction [PRIORITY: HIGH]

**Files:** `models.go`, `service.go`, `service_test.go`

- [x] Add `BrokerSymbol string json:"broker_symbol,omitempty"` to `SkippedTransaction`
- [x] In `processTrade`: when symbol is unmapped, set `BrokerSymbol: trade.Symbol` and `Reason: "unmapped symbol: " + trade.Symbol`
- [x] In `processCashTransaction`: when symbol is unmapped, set `BrokerSymbol: ct.Symbol` and `Reason: "unmapped symbol: " + ct.Symbol`
- [x] Update `service_test.go` tests that check "unmapped symbol" reason to match new format
- [x] Verify: `go test ./internal/domain/ibkrimport/...`

#### Task B: Preview Page Data — Add Existing Symbols [PRIORITY: MEDIUM]

**Files:** `ibkr_import_web.go`, `ibkr_import.go` (for service interface)

- [x] Add method to `ImportService` interface (or reuse existing symbol service) to list existing internal symbols
- [x] Add `ExistingSymbols []string` to `previewPageData`
- [x] In `HandleImportPost`, fetch existing symbols and pass to template
- [x] Verify: `go test ./internal/api/handlers/... -run ImportWeb`

#### Task C: Skipped Table — Grouped Unmapped Symbols [PRIORITY: HIGH]

**Files:** `templates/transaction/import_preview.html`, `style.css`

- [x] Split skipped section into two subsections:
  - "Unmapped Symbols" — grouped by broker symbol, one row per unique symbol with count
  - "Other Skipped" — duplicates, unsupported types, one row each
- [x] Grouped row format: `BrokerSymbol (N transactions) | reason | Resolve button`
- [x] The Resolve button passes the broker symbol (not transaction ref) to the modal
- [x] Add CSS for `.grouped-symbol-row` styling
- [x] Verify: page renders correctly with sample XML

#### Task D: Resolve Modal — Market Data Preview [PRIORITY: HIGH]

**Files:** `templates/transaction/import_preview.html`, `style.css`

- [x] Rewrite modal HTML:
  - Show broker symbol as read-only label
  - Market Data Symbol input with debounced preview (same 5 panels as transaction form)
  - Internal Symbol input (defaults to market data symbol, editable)
  - Action button ("Create & Map" or "Map" depending on existence)
- [x] Rewrite modal JavaScript:
  - Debounced fetch to `/api/symbol-mappings/preview?symbol=...`
  - Handle existing/new/corrected/error panels
  - Auto-fill internal symbol from preview data
  - Accept correction flow
  - Create-then-map API calls
  - Re-submit preview form on success
- [x] Add CSS for modal preview panels (reuse `.symbol-preview` styles)
- [x] Embed existing symbols as JSON script tag for client-side existence check
- [ ] Verify: modal works end-to-end with live server

#### Task E: Tests [PRIORITY: MEDIUM]

**Files:** `service_test.go`, `ibkr_import_web_test.go`

- [x] Update `SkippedTransaction` test data to include `BrokerSymbol` field
- [x] Update reason string assertions to match new format
- [x] Add web handler test verifying symbols are passed to template
- [x] Verify: `go test ./...`

### Task Dependencies

```
Task A (Model + Service)
        ↓
Task B (Page Data)    Task C (Grouped Table)
        ↓                 ↓
        └─────→ Task D (Modal) ← needs symbols from B, broker symbol from C
                    ↓
              Task E (Tests)
```

Tasks B and C are independent. Task D depends on both (needs existing symbols for preview existence check, needs broker symbol from grouped table).

## Session: Integration Tests + FX Duplicate Fix

### FX trade duplicate detection bug

Discovered during integration testing: FX trades create two transactions with composite external references (`txnID_fx_withdrawal` and `txnID_fx_deposit`), but the duplicate check in both `Preview` and `ConfirmImport` was checking the raw `txnID`. This meant FX trades were never detected as duplicates on re-import.

**Fix**: Updated duplicate check to look for composite references for FX trades:
- `Preview`: checks `txnID_fx_withdrawal` OR `txnID_fx_deposit`
- `ConfirmImport` (`buildTradeTxns`): same composite reference check

### Integration test suite

Created `tests/integration/ibkr_import_test.go` with 15 tests covering:
- Full import flow (preview → confirm → verify DB)
- Duplicate detection (re-import all 15 records skipped)
- Invalid XML and empty XML handling
- Account not found and missing account_id
- FX trades create two transactions (withdrawal + deposit) with composite refs
- Cash transaction classification (dividend, interest, tax, fee, deposit)
- Transfer classification (deposit vs withdrawal)
- Unmapped symbols skipped (10 importable, 6 skipped without mappings)
- Broker symbol mapping (direct symbol match)
- Unique index prevents DB-level duplicates
- Stock/ETF trade types (buy and sell)
- Negative quantity on sells (preserved from IBKR)
- List transactions after import (external_system = "IBKR")

### Test schema update

Updated `tests/integration/portfolio_test.go` to include migration 006 unique index and bump goose version to 6.

### Date format: IBKR uses YYYYMMDD, not YYYY-MM-DD (post-implementation fix)

The sample XML in `testdata/ibkr_sample.xml` was generated with `YYYY-MM-DD` dates (e.g., `2025-04-15`), but real IBKR Flex XML uses `YYYYMMDD` without dashes (e.g., `20241204`). This caused all imported transactions to have date `0001-01-01` because `time.Parse("2006-01-02", "20241204")` fails silently.

**Fix:**
- Added `parseDate(s string) time.Time` helper in `service.go` that tries `20060102` first, then falls back to `2006-01-02` for backward compatibility
- Replaced all `time.Parse("2006-01-02", ...)` calls in `buildTradeTxns`, `buildFXTxns`, `buildCashTxn`, `buildTransferTxn` with `parseDate(...)`
- Updated `testdata/ibkr_sample.xml` to use `YYYYMMDD` format throughout
- Updated all test assertions in `parser_test.go`, `service_test.go`, and `ibkr_import_web_test.go` to match
- Added `TestParseDate` unit test covering both formats and edge cases
