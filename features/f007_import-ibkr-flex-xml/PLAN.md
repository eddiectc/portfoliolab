# Implementation Plan: Import IBKR Flex XML

## Overview

Build an IBKR Flex XML import feature that lets users upload broker statements, preview parsed transactions (with inline symbol creation/mapping), and confirm the import. The import is transactional (all-or-nothing). Symbol creations and mappings during preview are committed immediately and are not rolled back.

The feature introduces a new `internal/domain/ibkrimport/` package for the XML parser and import service, new API endpoints for upload/preview/confirm, and a web UI import page.

## Task Dependencies

```
Task 1 (XML Parser)
       ↓
Task 2 (Duplicate Detection DB)
       ↓
Task 3 (Import Domain Service) ← depends on Task 1, Task 2
       ↓
Task 4 (API Handlers) ← depends on Task 3
       ↓
Task 5 (Web UI) ← depends on Task 4
       ↓
Task 6 (Router + Nav Wiring)
```

Tasks 1 and 2 are independent and can be done in parallel. Task 3 depends on both. Tasks 4–6 are sequential.

## Tasks

### Task 1: IBKR Flex XML Parser [PRIORITY: HIGH]

**Corresponds to:** Scenario "Upload and parse an IBKR Flex XML file", "FX trades are recorded as currency conversion transactions", "CashTransactions are classified by type", "Transfers between IBKR accounts", "Upload invalid or empty XML file", "Unsupported instrument types are skipped"

**Description:** Create a pure Go XML parser that extracts Trades, CashTransactions, and Transfers from IBKR Flex Web Report XML files. Uses `encoding/xml` (stdlib). Returns structured intermediate records with all fields needed for preview and import.

- [x] Define `internal/domain/ibkrimport/parser.go` with XML structs for Trade, CashTransaction, Transfer, and FlexStatement (matching IBKR Flex XML schema — all self-closing tags with attributes)
- [x] Implement `ParseXML(data []byte) (*ParsedReport, error)` that unmarshals XML and extracts Trades, CashTransactions, Transfers
- [x] Implement `ParsedReport` struct containing slices of raw trade/cash/transfer records plus summary metadata (date range, account alias)
- [x] Handle XML parsing errors gracefully — return descriptive errors for malformed XML
- [x] Handle the `tradePrice=` / `ibOrderID=` attribute name mismatch documented in the Rust reference (preprocess XML string before parsing, or use custom UnmarshalXML)
- [x] Write unit tests using `ibkr_sample_redacted.xml` as test fixture:
  - [x] Parse all 6 Trades (2 STK COMMON, 2 STK ETF sell, 1 STK ETF buy, 1 CASH FX)
  - [x] Parse all 7 CashTransactions (dividend, 2x interest, withholding tax, other fees, deposit, withdrawal)
  - [x] Parse both Transfers (deposit, withdrawal)
  - [x] Reject invalid/malformed XML
  - [x] Reject empty XML
  - [x] Handle XML with no Trades/CashTransactions/Transfers

**Verification:** `go test ./internal/domain/ibkrimport/...` passes. Parser correctly extracts all records from the sample XML.

---

### Task 2: Duplicate Detection Query + Repository Method [PRIORITY: HIGH]

**Corresponds to:** Scenario "Duplicate detection prevents re-import"

**Description:** Add a database query to check if a transaction with a given `external_system` + `external_reference` combination already exists, and expose it through the transaction repository.

- [x] Add SQL query to `internal/data/queries/transaction.sql`: `HasExternalReference` — `SELECT 1 FROM transactions WHERE external_system = ? AND external_reference = ? LIMIT 1`
- [x] Run `sqlc generate` to regenerate Go code
- [x] Add `ExternalReferenceExists(ctx, externalSystem, externalReference string) bool` method to `TransactionRepository`
- [x] Add `ExternalReferenceChecker` interface to `internal/domain/transaction/` (or define in `ibkrimport` package)
- [x] Write unit tests for the repository method (using mock or in-memory SQLite)

**Verification:** `go test ./internal/data/...` passes. Method correctly returns true for existing references and false for non-existing ones.

---

### Task 3: Import Domain Service [PRIORITY: HIGH]

**Corresponds to:** Scenario "Upload and parse an IBKR Flex XML file", "Preview shows importable and skipped transactions", "Confirm import creates transactions (all-or-nothing)", "Duplicate detection prevents re-import", "FX trades are recorded as currency conversion transactions", "CashTransactions are classified by type", "Transfers between IBKR accounts", "Unsupported instrument types are skipped"

**Description:** Create the import domain service that orchestrates preview generation (symbol resolution, duplicate detection, classification) and transactional import. This is the core business logic layer.

- [x] Define `internal/domain/ibkrimport/models.go` with:
  - [x] `PreviewResponse` — contains `Importable []PreviewTransaction`, `Skipped []SkippedTransaction`, `Errored []ErroredTransaction`, counts
  - [x] `PreviewTransaction` — fields for display: date, type, symbol, quantity, price, currency, netCash, externalReference, description
  - [x] `SkippedTransaction` — externalReference, reason (e.g., "unsupported instrument type", "duplicate", "unmapped symbol")
  - [x] `ErroredTransaction` — externalReference, error message
  - [x] `ImportRequest` — DTO for confirm endpoint: XML data ([]byte), account ID
  - [x] `ImportResult` — summary: created count, skipped count, any errors
- [x] Define service interfaces (minimal, for testability):
  - [x] `SymbolResolver` — resolves broker symbol to internal symbol, checks symbol existence
  - [x] `DuplicateChecker` — checks if external reference exists
  - [x] `TransactionCreator` — creates transactions (supports batch/transactional creation)
  - [x] `AccountChecker` — checks account existence (reuse existing `transaction.AccountChecker`)
  - [x] `SymbolCreator` — creates new symbol mappings (reuse existing `transaction.SymbolCreator`)
  - [x] `BrokerSymbolAdder` — adds broker symbol mappings
- [x] Implement `Service` struct with dependencies injected via constructor
- [x] Implement `Preview(ctx, xmlData []byte, accountID int64) (*PreviewResponse, error)`:
  - [x] Parse XML using parser from Task 1
  - [x] For each Trade: classify instrument type (STK COMMON/ETF = supported, others = skipped), resolve symbol via broker symbol map, check duplicate, build preview entry
  - [x] For each CashTransaction: classify by type (Dividends → dividend, Withholding Tax → tax, Broker Interest Received → interest, Other Fees → fee, positive amount → deposit, negative → withdrawal), resolve symbol for dividends, check duplicate, build preview entry
  - [x] For each FX Trade (assetCategory=CASH): generate two preview entries (withdrawal in source currency, deposit in target currency)
  - [x] For each Transfer: generate one preview entry (deposit if cashTransfer > 0, withdrawal if < 0)
  - [x] Handle negative quantities on sells (IBKR reports sells with negative quantity — use absolute value for display, keep as-is for import)
  - [x] Handle zero netCash on FX trades
  - [x] Return categorized results (importable, skipped, errored)
- [x] Implement `ConfirmImport(ctx, xmlData []byte, accountID int64) (*ImportResult, error)`:
  - [x] Parse XML again (stateless — no server-side session)
  - [x] Re-resolve symbols and re-check duplicates (data may have changed since preview)
  - [x] Build `transaction.CreateRequest` for each importable transaction
  - [x] Set `ExternalSystem = "IBKR"` and `ExternalReference = transactionID` on each
  - [x] Create all transactions within a single SQLite transaction (BEGIN/COMMIT)
  - [x] If any creation fails, rollback — none are committed
  - [x] Return summary of created/skipped counts
- [x] Implement `CreateSymbol(ctx, internalSymbol, marketDataSymbol string) error` — delegates to symbol service
- [x] Implement `AddBrokerSymbolMapping(ctx, brokerName, brokerSymbol, internalSymbol string) error` — resolves internal symbol to mapping ID, adds broker symbol
- [x] Write comprehensive unit tests:
  - [x] Preview with sample XML — correct counts for importable/skipped/errored
  - [x] Preview with all duplicates — all skipped
  - [x] Preview with unmapped symbols — skipped with reason
  - [x] Preview with unsupported instrument types — skipped
  - [x] Preview with FX trades — generates 2 entries per FX trade
  - [x] Preview with transfers — correct deposit/withdrawal classification
  - [x] Preview with cash transactions — correct type classification
  - [x] Confirm import creates transactions atomically
  - [x] Confirm import rolls back on failure
  - [x] Confirm import detects new duplicates (added between preview and confirm)
  - [x] Reject preview for invalid XML
  - [x] Reject preview for non-existent account
  - [x] Reject confirm for non-existent account

**Verification:** `go test ./internal/domain/ibkrimport/...` passes. All scenarios from the spec are covered by unit tests.

---

### Task 4: API Handlers [PRIORITY: HIGH]

**Corresponds to:** Scenario "Import via API", "Upload and parse an IBKR Flex XML file", "Upload invalid or empty XML file"

**Description:** Create API handlers for the import workflow: upload XML → get preview → confirm import. Also handles inline symbol creation and broker symbol mapping. Routes are under `/api/transactions/import/ibkr/` to live within the transactions section.

- [x] Create `internal/api/handlers/ibkr_import.go`:
  - [x] `ImportHandler` struct with dependencies (import service, symbol service, account service)
  - [x] `HandlePreview(w, r)` — POST `/api/transactions/import/ibkr/preview`:
    - [x] Parse multipart form with XML file and account_id
    - [x] Call import service Preview
    - [x] Return JSON preview response with importable/skipped/errored transactions
    - [x] Handle errors: invalid XML → 400, account not found → 404
  - [x] `HandleConfirm(w, r)` — POST `/api/transactions/import/ibkr/confirm`:
    - [x] Parse multipart form with XML file and account_id
    - [x] Call import service ConfirmImport
    - [x] Return JSON import result summary
    - [x] Handle errors: account not found → 404, creation failure → 500
  - [x] `HandleCreateSymbol(w, r)` — POST `/api/transactions/import/ibkr/symbols`:
    - [x] Accept JSON body with internal_symbol and market_data_symbol
    - [x] Call symbol service Create
    - [x] Return created symbol mapping
  - [x] `HandleAddBrokerSymbol(w, r)` — POST `/api/transactions/import/ibkr/broker-symbols`:
    - [x] Accept JSON body with broker_name, broker_symbol, internal_symbol
    - [x] Resolve internal symbol to mapping ID
    - [x] Add broker symbol via service
    - [x] Return success
  - [x] `RegisterRoutes(r)` — mount routes on chi.Mux
- [x] Write unit tests for handlers:
  - [x] Preview with valid XML → 200 with preview data
  - [x] Preview with invalid XML → 400
  - [x] Preview with missing account → 404
  - [x] Confirm with valid XML → 200 with result
  - [x] Confirm with all duplicates → 200 with zero created
  - [x] Create symbol → 201
  - [x] Add broker symbol → 200

**Verification:** `go test ./internal/api/handlers/... -run Import` passes.

---

### Task 5: Web UI [PRIORITY: HIGH]

**Corresponds to:** Scenario "Upload and parse an IBKR Flex XML file", "Create new internal symbol during import preview", "Add broker symbol mapping during import preview", "Preview shows importable and skipped transactions"

**Description:** Create server-rendered web pages for the import workflow with inline symbol management. Accessible from the Transactions page header via an "Import" dropdown (extensible for future brokers).

- [x] Create `internal/api/handlers/ibkr_import_web.go`:
  - [x] `ImportWebHandler` struct with dependencies
  - [x] `HandleImportPage(w, r)` — GET `/transactions/import/ibkr`:
    - [x] Render import page with file upload form, account dropdown, supported types info panel
  - [x] `HandleImportPost(w, r)` — POST `/transactions/import/ibkr`:
    - [x] Parse multipart form (XML file + account_id)
    - [x] Call import service Preview
    - [x] Render preview page showing importable/skipped/errored transactions with filter tabs
    - [x] Include inline forms for creating symbols and adding broker symbol mappings (via AJAX to API endpoints)
    - [x] Include confirm button (POST with base64-encoded XML data)
  - [x] `HandleConfirmPost(w, r)` — POST `/transactions/import/ibkr/confirm`:
    - [x] Re-submit XML data (base64-encoded) and account_id
    - [x] Call import service ConfirmImport
    - [x] Redirect to /transactions with flash message (no separate result page)
  - [x] `RegisterRoutes(r)` — mount web routes
- [x] Create `templates/transaction/import.html` — upload page:
  - [x] File input for XML upload
  - [x] Account dropdown (populated from account service)
  - [x] Info panel showing supported transaction types (Trades: stocks/ETFs, CashTransactions: dividends/interest/fees/taxes/deposits/withdrawals, FX Trades, Transfers)
  - [x] Submit button
- [x] Create `templates/transaction/import_preview.html` — preview page:
  - [x] Summary counts (importable, skipped, errored)
  - [x] Filter tabs (all, importable, skipped, errored)
  - [x] Table of transactions with full details
  - [x] Skipped items show reason
  - [x] Unmapped symbols show "Resolve Symbol" button (modal with AJAX to API endpoints)
  - [x] Confirm import button (disabled if zero importable)
  - [x] Cancel link
- [x] Write tests for web handlers:
  - [x] GET /transactions/import/ibkr → 200, renders upload page
  - [x] POST /transactions/import/ibkr with valid XML → 200, renders preview
  - [x] POST /transactions/import/ibkr with invalid XML → 200, shows error
  - [x] POST /transactions/import/ibkr with account not found → 200, shows error
  - [x] POST /transactions/import/ibkr with missing account → 200, shows error
  - [x] POST /transactions/import/ibkr/confirm → redirects with flash
  - [x] POST /transactions/import/ibkr/confirm with missing XML data → 400
  - [x] POST /transactions/import/ibkr/confirm with missing account → 400
  - [x] RegisterRoutes — all routes wired

**Verification:** `go test ./internal/api/handlers/... -run ImportWeb` passes. Pages render correctly with `go run cmd/server/main.go`.

---

### Task 6: Router Wiring + Navigation [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (enables access to the feature)

**Description:** Wire the import handlers into the application router and add an "Import" dropdown to the Transactions page header (next to "Add Transaction" button), structured to support multiple brokers.

- [ ] Update `internal/api/router.go`:
  - [ ] Create import service instance with dependencies
  - [ ] Create `ImportHandler` and register API routes
  - [ ] Create `ImportWebHandler` and register web routes (inside the renderer block)
- [ ] Update `templates/transaction/list.html`:
  - [ ] Add an "Import" dropdown next to "Add Transaction" in the page header, with "IBKR Flex XML" link to `/transactions/import/ibkr` (structured for future broker additions)
- [ ] Verify the full flow end-to-end:
  - [ ] Navigate to /transactions, click Import → IBKR Flex XML
  - [ ] Upload sample XML, see preview
  - [ ] Confirm import, see result
  - [ ] Verify transactions appear in /transactions list

**Verification:** Server starts without errors. Full import flow works end-to-end with sample XML.

---

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| XML parsing library | `encoding/xml` (stdlib) | No external dependency needed; IBKR Flex XML is flat (self-closing tags with attributes), maps cleanly to Go structs with xml tags |
| Preview state management | Stateless — re-parse on confirm | Follows existing REST patterns; avoids server-side session state; XML re-parsing is fast (< 100ms for typical files) |
| Confirm endpoint input | Multipart form (XML file + account_id) | Same input as preview; client re-sends the file; consistent with upload pattern |
| Package location | `internal/domain/ibkrimport/` | Follows `internal/domain/<feature>/` convention; IBKR-specific parser is isolated from generic import workflow |
| Transactional import | SQLite BEGIN/COMMIT via `sql.DB` | Uses existing `*sql.DB` connection; `db.BeginTx()` provides native transaction support |
| Symbol resolution during import | Check broker symbol map first, then direct internal symbol match | Follows existing symbol map design; broker symbols are the primary resolution path for imported data |
| Cash transaction symbol | `$CASH-{currency}` for non-dividend cash txns | Follows existing convention; dividends use the actual symbol |
| FX trade handling | Split into withdrawal + deposit (two transactions) | Matches Rust reference implementation; preserves original currencies |
| Negative quantity on sells | Use as-is (negative) for quantity field | Existing transaction model supports negative quantities; IBKR reports sells with negative quantity |
| File upload size limit | 50 MB (chi default) | Sufficient for large IBKR reports; can be tuned later if needed |
| Duplicate detection | Check `external_system = "IBKR"` + `external_reference = transactionID` | Unique per-broker; allows same transactionID across different brokers (unlikely but safe) |
| Import entry point | Transactions page header — "Import" dropdown next to "Add Transaction" | Keeps import context within transactions; dropdown extensible for future brokers |

## Risks

- **Large XML files** — Very large reports (10,000+ transactions) could slow down preview/confirm. Mitigation: Go's `encoding/xml` is fast; 50 MB limit prevents runaway files. If needed, add a timeout context.
- **Symbol resolution changes between preview and confirm** — User might create a symbol after preview but before confirm. Mitigation: Confirm re-resolves symbols; if a symbol was unmapped at preview but resolved at confirm, it's now importable (correct behavior).
- **Race condition on duplicate detection** — Two concurrent imports could both pass the duplicate check before either commits. Mitigation: Add a unique index on `(external_system, external_reference)` in a future migration; for now, rely on SQLite's single-connection serialization.
- **XML attribute name mismatch** — IBKR Flex XML uses `tradePrice` instead of `price` and `ibOrderID` instead of `orderID`. Mitigation: Preprocess XML string before parsing (simple string replacement), documented in the Rust reference.
