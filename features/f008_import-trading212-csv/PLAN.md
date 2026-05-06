# Implementation Plan: Import Trading 212 CSV

## Overview

Build a Trading 212 CSV import feature that lets users upload broker CSV exports, preview parsed transactions (with inline symbol creation/mapping), and confirm the import. The import is transactional (all-or-nothing). Symbol creations and mappings during preview are committed immediately and are not rolled back.

The feature introduces a new `internal/domain/trading212import/` package for the CSV parser and import service, new API endpoints for upload/preview/confirm, and a web UI import page. The workflow mirrors the IBKR Flex XML import (f007): upload → preview → map → confirm.

## Task Dependencies

```
Task 0 (Extract Shared Types + Interfaces)
       ↓
Task 1 (CSV Parser)
       ↓
Task 2 (Import Domain Service) ← depends on Task 0, Task 1
       ↓
Task 3 (API Handlers) ← depends on Task 0, Task 2
       ↓
Task 4 (Web UI) ← depends on Task 3
       ↓
Task 5 (Router Wiring + Navigation)
```

Tasks 0–5 are sequential. Task 0 is a small refactoring of existing IBKR code. Tasks 1–5 follow the same pattern as IBKR. No separate "Duplicate Detection DB" task is needed — the `ExternalReferenceExists` query and repository method were added by f007 and are reused directly.

## Tasks

### Task 0: Extract Shared Import Types + Interfaces [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundation for the feature)

**Description:** Extract the broker-agnostic DTO types and handler interfaces from the IBKR-specific packages into neutral locations. This enables both IBKR and Trading 212 import handlers to share the same contracts without cross-package coupling.

- [x] Create `internal/domain/brokerimport/types.go`:
  - [x] Move `PreviewResponse`, `PreviewTransaction`, `SkippedTransaction`, `ErroredTransaction`, `ImportResult` from `internal/domain/ibkrimport/models.go` to `brokerimport/types.go`
  - [x] Keep the same field definitions, json tags, and comments
- [x] Create `internal/domain/brokerimport/interfaces.go`:
  - [x] Move `ImportService` interface from `internal/api/handlers/ibkr_import.go` to `brokerimport/interfaces.go`
  - [x] Update method signatures to use `brokerimport.PreviewResponse` and `brokerimport.ImportResult`
  - [x] Also move the domain-level interfaces (`SymbolResolver`, `DuplicateChecker`, `TransactionCreator`, `AccountChecker`, `SymbolCreator`, `BrokerSymbolAdder`) — these are already broker-agnostic and used by both import services
- [x] Create `internal/api/handlers/import_service.go`:
  - [x] Move `SymbolService` interface from `internal/api/handlers/ibkr_import.go` to `import_service.go`
  - [x] This is a handler-layer concern that both IBKR and Trading 212 handlers need
- [x] Update `internal/domain/ibkrimport/models.go`:
  - [x] Remove type definitions (now in `brokerimport`)
  - [x] Add type aliases: `PreviewResponse = brokerimport.PreviewResponse`, etc. (for backward compatibility within the package)
- [x] Update `internal/domain/ibkrimport/service.go`:
  - [x] Remove interface definitions (now in `brokerimport`)
  - [x] Import from `brokerimport` for types and interfaces
  - [x] Update `Preview` and `ConfirmImport` return types to use `brokerimport` types
- [x] Update `internal/api/handlers/ibkr_import.go`:
  - [x] Remove `ImportService` and `SymbolService` interface definitions
  - [x] Import `ImportService` from `brokerimport`
  - [x] Import `SymbolService` from handlers package (`import_service.go`)
- [x] Update `internal/api/handlers/ibkr_import_web.go`:
  - [x] Update imports to use `brokerimport` types and shared interfaces
- [x] Run `go build ./...` and `go test ./...` — ensure no regressions

**Verification:** `go build ./...` succeeds. All existing tests pass (`go test ./...`). No behavioral changes to IBKR import.

---

### Task 1: Trading 212 CSV Parser [PRIORITY: HIGH]

**Corresponds to:** Scenario "Upload and parse a Trading 212 CSV file", "Buy transactions are classified correctly", "Sell transactions are classified correctly", "Deposit and withdrawal transactions are classified correctly", "Interest on cash transactions are classified correctly", "Share price in pence (GBX) is converted to GBP", "Upload invalid or empty CSV file", "Upload CSV with no matching transactions"

**Description:** Create a pure Go CSV parser that reads Trading 212 CSV exports and extracts structured records. Uses `encoding/csv` (stdlib). Returns a `ParsedReport` with all rows and a `ParsedRow` struct per record. The parser handles GBX→GBP price conversion at the parsing layer.

- [x] Define `internal/domain/trading212import/parser.go` with:
  - [x] `ParsedRow` struct with fields: Action, Date (parsed from Time), Ticker, Name, ID, Quantity, Price (already converted from GBX if needed), Currency (already converted to GBP if needed), Total, TotalCurrency
  - [x] `ParsedReport` struct containing `[]ParsedRow` slice
  - [x] Action constants: `LimitBuy`, `MarketBuy`, `LimitSell`, `MarketSell`, `Deposit`, `Withdrawal`, `InterestOnCash`, `Unknown`
- [x] Implement `ParseCSV(data []byte) (*ParsedReport, error)`:
  - [x] Use `encoding/csv.Reader` with standard settings
  - [x] Read header row and validate expected columns (Action, Time, Ticker, ID, No. of shares, Price / share, Currency (Price / share), Total, Currency (Total))
  - [x] For each data row: extract fields, parse date from "Time" column (format: `2006-01-02 15:04:05`), parse numeric fields
  - [x] GBX conversion: if `Currency (Price / share)` is "GBX", divide price by 100 and set currency to "GBP"
  - [x] Classify action into known types; unrecognized actions → `Unknown`
  - [x] Handle empty/missing fields gracefully (e.g., Ticker empty for deposits/withdrawals/interest)
  - [x] Return descriptive errors for malformed CSV, empty file, missing columns
- [x] Copy `trading212_sample.csv` to `internal/domain/trading212import/testdata/trading212_sample.csv` as test fixture
- [x] Write unit tests using the sample CSV:
  - [x] Parse all 15 rows — correct count
  - [x] Verify Deposit row: action=Deposit, no ticker, total=5000.00, currency=GBP
  - [x] Verify Limit buy row (AAPL): ticker=AAPL, quantity=10, price=150.00 (converted from 15000 GBX), currency=GBP
  - [x] Verify Market buy row (MSFT): ticker=MSFT, quantity=5, price=380.00 (converted from 38000 GBX), currency=GBP
  - [x] Verify Interest on cash row: action=InterestOnCash, no ticker, total=0.68, currency=GBP
  - [x] Verify Limit sell row (AAPL): ticker=AAPL, quantity=5, price=155.00 (converted from 15500 GBX), currency=GBP
  - [x] Verify Market sell row (MSFT): ticker=MSFT, quantity=2, price=390.00 (converted from 39000 GBX), currency=GBP
  - [x] Verify Withdrawal row: action=Withdrawal, no ticker, total=2000.00, currency=GBP
  - [x] Verify ETF rows (QGRP, DBMG): correct ticker, quantity, price conversion
  - [x] Reject empty CSV data
  - [x] Reject CSV with no header row
  - [x] Reject CSV with only header and no data rows
  - [x] Handle CSV with unexpected Action values (classified as Unknown)

**Verification:** `go test ./internal/domain/trading212import/... -run Parser` passes. Parser correctly extracts all 15 records from the sample CSV with GBX→GBP conversion.

---

### Task 2: Import Domain Service [PRIORITY: HIGH]

**Corresponds to:** Scenario "Upload and parse a Trading 212 CSV file", "Preview shows importable and skipped transactions", "Confirm import creates transactions (all-or-nothing)", "Duplicate detection prevents re-import", "Buy/Sell/Deposit/Withdrawal/Interest transactions are classified correctly", "Share price in pence (GBX) is converted to GBP", "Unsupported action types are skipped", "No accounts exist", "Create new internal symbol during import preview", "Add broker symbol mapping during import preview"

**Description:** Create the import domain service that orchestrates preview generation (symbol resolution, duplicate detection, classification, net cash calculation) and transactional import. Reuses existing interfaces from f007 (SymbolResolver, DuplicateChecker, TransactionCreator, AccountChecker, SymbolCreator, BrokerSymbolAdder).

- [x] Define `internal/domain/trading212import/models.go` with:
  - [x] Type aliases importing from `brokerimport`: `PreviewResponse`, `PreviewTransaction`, `SkippedTransaction`, `ErroredTransaction`, `ImportResult` (shared types from Task 0)
- [x] Define service interfaces (reusing from `brokerimport/interfaces.go` via Task 0):
  - [x] `SymbolResolver`, `DuplicateChecker`, `TransactionCreator`, `AccountChecker`, `SymbolCreator`, `BrokerSymbolAdder` — all defined in `brokerimport`
- [x] Define service errors: `ErrAccountNotFound`, `ErrInvalidCSV`, `ErrNoImportableTransactions`
- [x] Implement `Service` struct with dependencies injected via constructor `NewService(...)`
- [x] Implement `Preview(ctx, csvData []byte, accountID int64) (*PreviewResponse, error)`:
  - [x] Check account exists
  - [x] Parse CSV using parser from Task 1
  - [x] For each row:
    - [x] Classify by action type (buy/sell/deposit/withdrawal/interest/unknown)
    - [x] Unknown actions → skipped with reason "unsupported action type"
    - [x] For trades (buy/sell): resolve symbol via broker symbol map (broker name = "Trading212"), skip if unmapped with brokerSymbol populated
    - [x] For cash transactions (deposit/withdrawal/interest): use `$CASH-{currency}` symbol
    - [x] Check duplicate via `ExternalReferenceExists(ctx, "Trading212", row.ID)`
    - [x] Calculate net cash: buys → negative total, sells → positive total, deposits/interest → positive total, withdrawals → negative total
    - [x] Build preview entry with all display fields
  - [x] Return categorized results (importable, skipped, errored)
  - [x] Ensure empty slices (not nil) for JSON serialization consistency
- [x] Implement `ConfirmImport(ctx, csvData []byte, accountID int64) (*ImportResult, error)`:
  - [x] Check account exists
  - [x] Parse CSV again (stateless — no server-side session)
  - [x] Re-resolve symbols and re-check duplicates
  - [x] Build `*transaction.Transaction` for each importable row:
    - [x] Set `ExternalSystem = "Trading212"` and `ExternalReference = row.ID`
    - [x] Set correct date, type, symbol, quantity, price, currency, netCash
    - [x] For trades: quantity positive, price from parser (already GBX→GBP converted)
    - [x] For cash: quantity = total amount, price = 1
  - [x] Create all transactions within a single SQLite transaction via `BatchCreate`
  - [x] If any creation fails, rollback — none committed
  - [x] Return summary of created/skipped counts
- [x] Implement `CreateSymbol(ctx, internalSymbol, marketDataSymbol string) error` — delegates to symbol creator
- [x] Implement `AddBrokerSymbolMapping(ctx, brokerName, brokerSymbol, internalSymbol string) error` — delegates to broker symbol adder
- [x] Write comprehensive unit tests (following IBKR service_test.go pattern with hand-written mocks):
  - [x] Mocks: `mockSymbolResolver`, `mockDuplicateChecker`, `mockTransactionCreator`, `mockAccountChecker`, `mockSymbolCreator`, `mockBrokerSymbolAdder`
  - [x] Preview with sample CSV — correct counts for importable/skipped/errored
  - [x] Preview with all duplicates — all skipped with "duplicate" reason
  - [x] Preview with unmapped symbols — skipped with "unmapped symbol" reason and BrokerSymbol populated
  - [x] Preview with unsupported action types — skipped with "unsupported action type" reason
  - [x] Preview net cash calculation: buy → negative, sell → positive, deposit → positive, withdrawal → negative, interest → positive
  - [x] Preview GBX price conversion already applied (parser handles it)
  - [x] Confirm import creates transactions atomically
  - [x] Confirm import rolls back on BatchCreate failure
  - [x] Confirm import detects new duplicates (added between preview and confirm)
  - [x] Reject preview for invalid CSV
  - [x] Reject preview for non-existent account
  - [x] Reject confirm for non-existent account
  - [x] CreateSymbol delegates correctly
  - [x] AddBrokerSymbolMapping delegates correctly
  - [x] Empty CSV (header only) — zero importable, zero skipped

**Verification:** `go test ./internal/domain/trading212import/... -run Service` passes. All scenarios from the spec are covered by unit tests.

---

### Task 3: API Handlers [PRIORITY: HIGH]

**Corresponds to:** Scenario "Import via API", "Upload and parse a Trading 212 CSV file", "Upload invalid or empty CSV file"

**Description:** Create API handlers for the Trading 212 import workflow: upload CSV → get preview → confirm import. Also handles inline symbol creation and broker symbol mapping. Routes are under `/api/transactions/import/trading212/`.

- [ ] Create `internal/api/handlers/trading212_import.go`:
  - [ ] `Trading212ImportHandler` struct with dependencies (import service, symbol service, max file size)
  - [ ] Import `ImportService` from `brokerimport` and `SymbolService` from `import_service.go` (shared interfaces from Task 0)
  - [ ] `HandlePreview(w, r)` — POST `/api/transactions/import/trading212/preview`:
    - [ ] Parse multipart form with CSV file and account_id
    - [ ] Call import service Preview
    - [ ] Return JSON preview response
    - [ ] Handle errors: invalid CSV → 400, account not found → 404
  - [ ] `HandleConfirm(w, r)` — POST `/api/transactions/import/trading212/confirm`:
    - [ ] Parse multipart form with CSV file and account_id
    - [ ] Call import service ConfirmImport
    - [ ] Return JSON import result summary
    - [ ] Handle errors: account not found → 404, creation failure → 500
  - [ ] `HandleCreateSymbol(w, r)` — POST `/api/transactions/import/trading212/symbols`:
    - [ ] Accept JSON body with internal_symbol and market_data_symbol
    - [ ] Call symbol service Create
    - [ ] Return created symbol mapping
  - [ ] `HandleAddBrokerSymbol(w, r)` — POST `/api/transactions/import/trading212/broker-symbols`:
    - [ ] Accept JSON body with broker_name, broker_symbol, internal_symbol
    - [ ] Add broker symbol via import service
    - [ ] Return success
  - [ ] `RegisterRoutes(r)` — mount routes on chi.Mux
  - [ ] Helper: `readCSVFile(r *http.Request) ([]byte, error)` — reads CSV file from multipart form
- [ ] Write unit tests for handlers:
  - [ ] Preview with valid CSV → 200 with preview data
  - [ ] Preview with invalid CSV → 400
  - [ ] Preview with missing account → 404
  - [ ] Confirm with valid CSV → 200 with result
  - [ ] Confirm with all duplicates → 200 with zero created
  - [ ] Create symbol → 201
  - [ ] Add broker symbol → 200

**Verification:** `go test ./internal/api/handlers/... -run Trading212` passes.

---

### Task 4: Web UI [PRIORITY: HIGH]

**Corresponds to:** Scenario "Upload and parse a Trading 212 CSV file", "Create new internal symbol during import preview", "Add broker symbol mapping during import preview", "Preview shows importable and skipped transactions", "No accounts exist"

**Description:** Create server-rendered web pages for the Trading 212 import workflow with inline symbol management. Structure mirrors the IBKR import pages.

- [ ] Create `internal/api/handlers/trading212_import_web.go`:
  - [ ] `Trading212ImportWebHandler` struct with dependencies (import service, account service, symbol mapping service, renderer, max file size)
  - [ ] `HandleImportPage(w, r)` — GET `/transactions/import/trading212`:
    - [ ] Fetch accounts list
    - [ ] Render upload page with file upload form, account dropdown, supported types info panel
  - [ ] `HandleImportPost(w, r)` — POST `/transactions/import/trading212`:
    - [ ] Parse multipart form (CSV file + account_id)
    - [ ] Also accept base64-encoded CSV data for re-submission after resolving symbols
    - [ ] Call import service Preview
    - [ ] Render preview page showing importable/skipped/errored transactions with filter tabs
    - [ ] Include inline forms for creating symbols and adding broker symbol mappings (via AJAX to API endpoints)
    - [ ] Include confirm button (POST with base64-encoded CSV data)
    - [ ] Fetch existing internal symbols for client-side existence check
  - [ ] `HandleConfirmPost(w, r)` — POST `/transactions/import/trading212/confirm`:
    - [ ] Decode base64-encoded CSV data and account_id
    - [ ] Call import service ConfirmImport
    - [ ] Redirect to /transactions with flash message summarizing result
    - [ ] On failure: re-render preview page with error
  - [ ] `RegisterRoutes(r)` — mount web routes (POST confirm before POST catch-all, GET last)
  - [ ] Helper: `renderImportPage` — re-renders upload page with error message
  - [ ] Helper: `getExistingSymbols` — fetches unique internal symbols for client-side checks
- [ ] Create `templates/transaction/t212_import.html` — upload page:
  - [ ] Page header: "Import Trading 212 CSV" with "Back to Transactions" link
  - [ ] Error display area
  - [ ] Info panel showing supported transaction types (Limit buy, Market buy, Limit sell, Market sell, Deposit, Withdrawal, Interest on cash)
  - [ ] Note about GBX→GBP price conversion
  - [ ] Form: account dropdown, CSV file input (accept=".csv"), submit button, cancel link
- [ ] Create `templates/transaction/t212_import_preview.html` — preview page:
  - [ ] Page header: "Import Preview" with "Upload New File" and "Cancel" links
  - [ ] Summary counts (importable, skipped, errored)
  - [ ] Filter tabs (all, importable, skipped, errored)
  - [ ] Hidden re-submission form (for after resolving symbols)
  - [ ] Confirm form wrapping the table:
    - [ ] Hidden fields for base64 CSV data and account_id
    - [ ] Unified transactions table with columns: Date, Type, Symbol, Qty, Price, Net Cash, Currency, Description, Ref, Reason, Action
    - [ ] Importable rows: full details, no action button
    - [ ] Skipped rows: show reason, "Resolve Symbol" button for unmapped symbols
    - [ ] Errored rows: show error message
    - [ ] Confirm button (disabled if zero importable) with count
  - [ ] Resolve symbol modal (same structure as IBKR):
    - [ ] Broker symbol display
    - [ ] Market data symbol input with debounced preview fetch
    - [ ] Internal symbol input
    - [ ] Symbol preview panels (existing, new, corrected, error)
    - [ ] "Create & Map" button calling Trading 212 API endpoints
  - [ ] Embedded existing symbols JSON for client-side existence check
  - [ ] JavaScript for filter tabs, resolve modal, preview fetching (same pattern as IBKR, targeting Trading 212 API endpoints)
- [ ] Write tests for web handlers:
  - [ ] GET /transactions/import/trading212 → 200, renders upload page
  - [ ] GET /transactions/import/trading212 with no accounts → 200, shows empty account dropdown
  - [ ] POST /transactions/import/trading212 with valid CSV → 200, renders preview
  - [ ] POST /transactions/import/trading212 with invalid CSV → 200, shows error on upload page
  - [ ] POST /transactions/import/trading212 with missing account → 200, shows error on upload page
  - [ ] POST /transactions/import/trading212/confirm → redirects with flash
  - [ ] POST /transactions/import/trading212/confirm with missing CSV data → 400
  - [ ] POST /transactions/import/trading212/confirm with missing account → 400
  - [ ] RegisterRoutes — all routes wired

**Verification:** `go test ./internal/api/handlers/... -run Trading212.*Web` passes. Pages render correctly with `go run cmd/server/main.go`.

---

### Task 5: Router Wiring + Navigation [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (enables access to the feature)

**Description:** Wire the Trading 212 import handlers into the application router and add "Trading 212 CSV" to the Import dropdown on the Transactions page.

- [ ] Update `internal/api/router.go`:
  - [ ] Create `trading212import.Service` instance with dependencies (reuse existing symbolResolver, transactionRepo, accountChecker, symbolCreator, brokerSymbolAdder from IBKR wiring)
  - [ ] Create `Trading212ImportHandler` and register API routes
  - [ ] Create `Trading212ImportWebHandler` and register web routes (inside the renderer block)
- [ ] Update `templates/transaction/list.html`:
  - [ ] Add "Trading 212 CSV" link to the Import dropdown (below "IBKR Flex XML")
- [ ] Update `features/README.md`:
  - [ ] Change f008 status from "spec" to "planning"
- [ ] Verify the full flow end-to-end:
  - [ ] Navigate to /transactions, click Import → Trading 212 CSV
  - [ ] Upload sample CSV, see preview with correct counts
  - [ ] Verify GBX→GBP price conversion in preview
  - [ ] Verify duplicate detection works
  - [ ] Confirm import, see result flash message
  - [ ] Verify transactions appear in /transactions list with correct data

**Verification:** Server starts without errors. Full import flow works end-to-end with sample CSV. `go test ./...` passes.

---

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| CSV parsing library | `encoding/csv` (stdlib) | No external dependency needed; Trading 212 CSV is standard RFC 4180 format with quoted fields |
| Package location | `internal/domain/trading212import/` | Follows `internal/domain/<feature>/` convention; Trading 212-specific parser is isolated from generic import workflow |
| External system identifier | `"Trading212"` | Distinct from `"IBKR"`; human-readable; allows same transaction IDs across brokers without collision |
| Broker name for symbol resolution | `"Trading212"` | Same as external system ID; human-readable broker name used in broker symbol map lookups |
| GBX→GBP conversion | Done in parser layer | Parser outputs price in GBP with currency="GBP"; service layer receives clean data. Follows Rust reference implementation |
| Net cash calculation | Done in service layer | Parser provides raw Total; service computes sign based on action type. Keeps parser focused on extraction |
| Preview state management | Stateless — re-parse on confirm | Follows IBKR pattern; avoids server-side session state; CSV re-parsing is fast |
| Confirm endpoint input | Multipart form (CSV file + account_id) for API; base64-encoded CSV for web | API accepts file upload; web uses base64 for re-submission after resolving symbols (same as IBKR) |
| Preview/Skipped/Errored models | Shared in `internal/domain/brokerimport/types.go` | Single source of truth; both IBKR and Trading 212 use the same DTO types; extracted from IBKR in Task 0 |
| Handler interfaces | `ImportService` in `brokerimport/interfaces.go`, `SymbolService` in `handlers/import_service.go` | Neutral package names; both broker handlers import from the same location; extracted from IBKR in Task 0 |
| Domain interfaces | `SymbolResolver`, `DuplicateChecker`, etc. in `brokerimport/interfaces.go` | Broker-agnostic interfaces shared by all import services; extracted from IBKR in Task 0 |
| Templates | Separate files (`t212_import.html`, `t212_import_preview.html`) | Separate from IBKR templates; same structure but broker-specific text, supported types, and API endpoints |
| File upload size limit | 50 MB (same as IBKR) | Sufficient for large CSV exports; consistent with existing pattern |
| Duplicate detection | `external_system = "Trading212"` + `external_reference = ID column` | Reuses existing `HasExternalReference` query from f007; unique per-broker |
| Date parsing | `2006-01-02 15:04:05` format | Trading 212 CSV uses this format in the "Time" column; date portion extracted for transaction date |
| Cash transaction symbol | `$CASH-{currency}` for deposits/withdrawals/interest | Follows existing convention; auto-created by transaction service if missing |

## Risks

- **Large CSV files** — Very large exports (10,000+ rows) could slow down preview/confirm. Mitigation: Go's `encoding/csv` is fast; 50 MB limit prevents runaway files. CSV parsing is typically < 100ms for files up to 10,000 rows.
- **Symbol resolution changes between preview and confirm** — User might create a symbol after preview but before confirm. Mitigation: Confirm re-resolves symbols; if a symbol was unmapped at preview but resolved at confirm, it's now importable (correct behavior).
- **Race condition on duplicate detection** — Two concurrent imports could both pass the duplicate check. Mitigation: Same as IBKR — rely on SQLite's single-connection serialization; unique index on `(external_system, external_reference)` can be added in a future migration if needed.
- **CSV encoding issues** — Trading 212 may produce files with BOM or non-UTF-8 encoding. Mitigation: Strip UTF-8 BOM if present; log warning for non-UTF-8 bytes.
- **GBX precision loss** — Dividing integer pence by 100 could lose precision. Mitigation: Use `decimal.Decimal` throughout; parser converts GBX string to decimal then divides by `decimal.New(100, 0)`.
