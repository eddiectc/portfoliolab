# Feature: Import Trading 212 CSV

## Description

Allow users to import transaction history from Trading 212 CSV exports into their portfolio. The feature parses the CSV file, classifies rows into transaction types (buys, sells, deposits, withdrawals, interest), maps Trading 212 broker tickers to internal symbols, detects duplicates by the CSV's `ID` column, and presents a preview of what will be imported before the user confirms.

The import page clearly indicates which transaction types are supported. During the preview, the user can create new internal symbols or add broker symbol mappings inline — without navigating away from the import page. The import operation is transactional: all importable transactions are created together or none are.

This feature follows the same import workflow established by the IBKR Flex XML import (f007): upload → preview → map → confirm.

## User Stories

- **As a user, I want to upload a Trading 212 CSV export so that my transactions are automatically parsed and ready for import into my portfolio.**

- **As a user, I want to create new internal symbols or add broker symbol mappings directly from the import preview, so that I don't need to navigate to a separate page before importing.**

- **As a user, I want to review the full details of transactions to be imported and see which are skipped, so that I can verify the data before committing.**

- **As a user, I want to select which of my existing accounts the imported transactions belong to, so that the transactions are attributed correctly.**

- **As a user, I want duplicate transactions (same Trading 212 ID already imported) to be detected and reported as skipped, so that re-importing the same file doesn't create duplicate records.**

## Scenarios

### Scenario: No accounts exist
**Given** the user has no accounts set up in the system
**When** the user navigates to the Trading 212 import page
**Then** the system indicates that at least one account is required before importing

### Scenario: Upload and parse a Trading 212 CSV file
**Given** the user has at least one account set up in the system
**When** the user uploads a valid Trading 212 CSV file and selects a target account
**Then** the system parses the CSV and classifies each row into a transaction type (buy, sell, deposit, withdrawal, interest)
**And** the system returns a preview showing the parsed transactions with full details, filterable by status (valid, skipped, error)

### Scenario: Create new internal symbol during import preview
**Given** the CSV contains trades with a Trading 212 ticker that has no internal symbol mapping (e.g., "QGRP")
**When** the system parses the file
**Then** the ticker is flagged as unmapped in the preview
**And** the user can create a new internal symbol directly from the preview, including the internal symbol name and market data provider symbol
**And** the creation is committed immediately (not part of the import transaction)
**And** the trade is now resolvable and appears as valid in the preview

### Scenario: Add broker symbol mapping during import preview
**Given** an internal symbol already exists (e.g., "QGRP") but has no broker symbol mapping for Trading 212
**And** the CSV contains trades with a Trading 212 ticker (e.g., "QGRP")
**When** the system parses the file
**Then** the broker ticker is flagged as unmapped in the preview
**And** the user can create a broker symbol mapping (linking the Trading 212 ticker to the existing internal symbol) directly from the preview
**And** the mapping is committed immediately (not part of the import transaction)
**And** the trade is now resolvable and appears as valid in the preview

### Scenario: Preview shows importable and skipped transactions
**Given** the system has parsed the CSV and resolved symbol mappings
**When** the user views the import preview
**Then** the preview shows a summary count of transactions to import, transactions skipped, and transactions with errors
**And** the preview shows full details of each transaction, filterable by status (valid, skipped, error)
**And** skipped transactions show their Trading 212 ID and the reason (e.g., "duplicate — already imported")
**And** errored transactions show their Trading 212 ID and the error reason

### Scenario: Confirm import creates transactions (all-or-nothing)
**Given** the user has reviewed the import preview and resolved any unmapped symbols
**When** the user confirms the import
**Then** all importable transactions are created as a single transaction — if any transaction fails to create, none are committed
**And** each transaction records the Trading 212 ID for duplicate detection
**And** the system returns a summary of how many transactions were created and how many were skipped
**And** symbol creations and mappings made during the preview are not rolled back (they were committed immediately)

### Scenario: Duplicate detection prevents re-import
**Given** the user has previously imported a Trading 212 CSV file containing transaction ID "EOF48468690772"
**When** the user uploads the same file (or a file containing the same ID)
**Then** the transaction with that ID is marked as skipped in the preview
**And** it is not created again when the import is confirmed

### Scenario: Buy transactions are classified correctly
**Given** the CSV contains rows with Action "Limit buy" or "Market buy"
**When** the system parses the file
**Then** each row is classified as a buy transaction
**And** the quantity, price, and currency are extracted from the row
**And** net cash is set to the negative of the total amount (money leaving the account)

### Scenario: Sell transactions are classified correctly
**Given** the CSV contains rows with Action "Limit sell" or "Market sell"
**When** the system parses the file
**Then** each row is classified as a sell transaction
**And** the quantity, price, and currency are extracted from the row
**And** net cash is set to the total amount (money entering the account)

### Scenario: Deposit and withdrawal transactions are classified correctly
**Given** the CSV contains rows with Action "Deposit" or "Withdrawal"
**When** the system parses the file
**Then** "Deposit" rows are classified as deposit transactions and "Withdrawal" rows are classified as withdrawal transactions
**And** the total amount and currency are extracted from the row
**And** the symbol is set to `$CASH-{currency}` (e.g., `$CASH-GBP`) with price of 1
**And** net cash for deposits is positive (money entering) and for withdrawals is negative (money leaving)

### Scenario: Interest on cash transactions are classified correctly
**Given** the CSV contains rows with Action "Interest on cash"
**When** the system parses the file
**Then** each row is classified as an interest transaction
**And** the total amount and currency are extracted from the row
**And** the symbol is set to `$CASH-{currency}` (e.g., `$CASH-GBP`) with price of 1
**And** net cash is positive (money entering the account)

### Scenario: Share price in pence (GBX) is converted to GBP
**Given** the CSV contains trades where the price currency is GBX (pence)
**When** the system parses the file
**Then** the price per share is converted from pence to pounds (divided by 100)
**And** the currency is set to GBP for the resulting transaction
**And** the total amount (already in the total currency column) is used as-is

### Scenario: Unsupported action types are skipped
**Given** the CSV contains rows with action types not supported by the importer (e.g., "Dividend", "Fee", "Tax")
**When** the system parses the file
**Then** those rows are marked as skipped with the reason "unsupported action type"
**And** they are not included in the importable count
**And** they are shown in the preview under the skipped filter

### Scenario: Upload invalid or empty CSV file
**Given** the user uploads a file that is not a valid Trading 212 CSV
**When** the system attempts to parse the file
**Then** the system returns an error describing the parsing failure
**And** no transactions are created

### Scenario: Upload CSV with no matching transactions
**Given** the user uploads a valid Trading 212 CSV file
**When** all trades have unmapped tickers, all transactions are duplicates, or the file contains no valid rows
**Then** the preview shows zero importable transactions
**And** the user can choose not to proceed with the import

### Scenario: Import via API
**Given** a client (web UI or future mobile app) wants to import a Trading 212 CSV export
**When** the client uploads the CSV file along with the target account
**Then** the system parses the file and returns a preview with importable, skipped, and errored transactions
**And** the client can confirm the import via a subsequent request
**And** the system returns a summary of the import result

The import API is designed to be broker-agnostic at the workflow level (upload → preview → map → confirm), enabling reuse by future clients such as a mobile app.

## Edge Cases

- **Empty CSV file or malformed CSV** — system returns a clear parsing error, no partial data created
- **CSV with only header row and no data** — preview shows zero importable transactions
- **All tickers unmapped** — preview shows all trades as requiring symbol mapping; import can proceed once mappings are provided inline
- **All transactions are duplicates** — preview shows all as skipped with reasons
- **Price in GBX (pence)** — price is converted to GBP; total amount in the CSV is already in the correct currency
- **Very large CSV file with thousands of rows** — system processes the file without timing out
- **Re-importing a previously imported file** — all duplicates detected and skipped, no new records created
- **Same Trading 212 ID across different report files** — duplicate detected via external_reference, second import skips it
- **User closes browser during preview** — no server-side state to clean up; any temporary data is discarded
- **CSV with unexpected Action values** — rows with unrecognized Action values are marked as skipped with reason "unsupported action type"

## Constraints

- Only one CSV file per import operation
- Target account must already exist in the system (no auto-creation of accounts)
- Single account per import — the user selects one target account for all transactions in the file
- The CSV parser is Trading 212-specific; the import workflow (upload → preview → map → confirm) follows the pattern established by f007
- The import operation is transactional: all importable transactions are created together or none are
- Symbol creations and mappings made during the preview are committed immediately and are not part of the import transaction
- Symbol creation, broker symbol mapping, and market data preview are all inline on the import page — no navigation to other pages required
- Web UI is required for the import flow; the same API is used by the web UI
- Supported transaction types: Limit buy, Market buy, Limit sell, Market sell, Deposit, Withdrawal, Interest on cash
- Unsupported transaction types (skipped with reason "unsupported action type"): Dividend, Fee, Tax, and any other unrecognized Action values
- Duplicate detection uses the `ID` column from the CSV

## Non-Goals

- Support for brokers other than Trading 212 (e.g., IBKR is a separate feature, f007)
- Auto-creation of accounts from the CSV
- Real-time API integration with Trading 212 (file upload only)
- Import of positions or portfolio snapshots (transactions only)
- Batch import of multiple files at once
- Scheduling or automation of imports
- Importing into multiple accounts from a single file — one target account per import operation
- Support for Trading 212 "Smart Portfolio" or other non-standard account types beyond the CSV data

## Reference Materials

- **`tc_isa_20260101_20260424.csv`** — Sample Trading 212 CSV export (ISA account). Contains deposits, limit buys, market buys, and daily interest on cash. Use as the primary reference for CSV structure, available columns, and real-world data patterns.
- **`trading212_sample.csv`** — Generated sample CSV with additional transaction types (sells, withdrawals) for comprehensive test coverage.
- **`trading212mapping.txt`** — Reference Rust implementation showing CSV parsing, action classification, net cash calculation, and currency conversion logic.

## Dependencies

- f002 (Account CRUD) — target account must exist
- f003 (Symbol Map) — symbol resolution uses the existing symbol map
- f004 (Transaction CRUD) — imported transactions are created via the transaction service
