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
