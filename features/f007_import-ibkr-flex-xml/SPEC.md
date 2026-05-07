# Feature: Import IBKR Flex XML

## Description

Allow users to import transaction history from Interactive Brokers Flex Web Report (XML format) into their portfolio. The feature parses Trades (buy/sell of stocks and ETFs), CashTransactions (deposits, withdrawals, dividends, interest, fees, taxes), and Transfers from the XML, maps broker symbols to internal symbols, detects duplicates, and presents a preview of what will be imported before the user confirms. Unsupported instrument types (e.g., options, futures) are skipped with a reason.

The import page clearly indicates which transaction types are supported, with details in a collapsible panel to avoid overwhelming the user. During the preview, the user can create new internal symbols or add broker symbol mappings inline — without navigating away from the import page.

The import operation is transactional: all importable transactions are created together or none are. Symbol creations and mappings made during the preview are committed immediately and are not part of the import transaction.

This feature establishes the import workflow (upload → preview → map → confirm) that will be reused by future broker import features (e.g., Trading 212). The XML parser itself is IBKR-specific.

## User Stories

- **As a user, I want to upload an IBKR Flex XML report so that my transactions are automatically parsed and ready for import into my portfolio.**

- **As a user, I want to create new internal symbols or add broker symbol mappings directly from the import preview, so that I don't need to navigate to a separate page before importing.**

- **As a user, I want to review the full details of transactions to be imported and see which are skipped, so that I can verify the data before committing.**

- **As a user, I want to select which of my existing accounts the imported transactions belong to, so that account information in the XML file is ignored and transactions are attributed correctly.**

- **As a user, I want duplicate transactions (same IBKR transactionID already imported) to be detected and reported as skipped, so that re-importing the same file doesn't create duplicate records.**

## Scenarios

### Scenario: Upload and parse an IBKR Flex XML file
**Given** the user has at least one account set up in the system
**When** the user uploads a valid IBKR Flex XML file and selects a target account
**Then** the system parses the XML and extracts Trades (buy/sell of stocks and ETFs), CashTransactions, and Transfers
**And** unsupported instrument types (e.g., options, futures) are excluded and marked as skipped
**And** the system returns a preview showing the parsed transactions with full details, filterable by status (valid, skipped, error)

### Scenario: Create new internal symbol during import preview
**Given** the XML contains trades with a broker symbol that has no internal symbol mapping (e.g., "COIN")
**When** the system parses the file
**Then** the symbol is flagged as unmapped in the preview
**And** the user can create a new internal symbol directly from the preview, including the internal symbol name and market data provider symbol
**And** the creation is committed immediately (not part of the import transaction)
**And** the trade is now resolvable and appears as valid in the preview

### Scenario: Add broker symbol mapping during import preview
**Given** an internal symbol already exists (e.g., "BRK.B") but has no broker symbol mapping for IBKR
**And** the XML contains trades with an IBKR broker symbol (e.g., "BRK B")
**When** the system parses the file
**Then** the broker symbol is flagged as unmapped in the preview
**And** the user can create a broker symbol mapping (linking "BRK B" from broker "IBKR" to the existing internal symbol "BRK.B") directly from the preview
**And** the mapping is committed immediately (not part of the import transaction)
**And** the trade is now resolvable and appears as valid in the preview

### Scenario: Preview shows importable and skipped transactions
**Given** the system has parsed the XML and resolved symbol mappings
**When** the user views the import preview
**Then** the preview shows a summary count of transactions to import, transactions skipped, and transactions with errors
**And** the preview shows full details of each transaction, filterable by status (valid, skipped, error)
**And** skipped transactions show their IBKR transactionID and the reason (e.g., "duplicate — already imported", "unsupported instrument type")
**And** errored transactions show their IBKR transactionID and the error reason

### Scenario: Confirm import creates transactions (all-or-nothing)
**Given** the user has reviewed the import preview and resolved any unmapped symbols
**When** the user confirms the import
**Then** all importable transactions are created as a single transaction — if any transaction fails to create, none are committed
**And** each transaction records the IBKR transactionID as its external reference
**And** the system returns a summary of how many transactions were created and how many were skipped
**And** symbol creations and mappings made during the preview are not rolled back (they were committed immediately)

### Scenario: Duplicate detection prevents re-import
**Given** the user has previously imported a Flex XML file with transactionID "33035505490"
**When** the user uploads the same file (or a file containing the same transactionID)
**Then** the transaction with that ID is marked as skipped in the preview
**And** it is not created again when the import is confirmed

### Scenario: FX trades are recorded as currency conversion transactions
**Given** the XML contains FX trades (e.g., GBP to USD conversion)
**When** the system parses the file
**Then** each FX trade is recorded as transactions representing the currency conversion (withdrawal in source currency and deposit in target currency)
**And** each transaction preserves its original currency from the XML

### Scenario: CashTransactions are classified by type
**Given** the XML contains CashTransactions of various types (Dividends, Broker Interest Received, Deposits/Withdrawals, etc.)
**When** the system parses the file
**Then** each CashTransaction is classified into an appropriate transaction type (e.g., dividends become dividend transactions, interest becomes interest transactions, fees become fee transactions, tax withholdings become tax transactions, and generic positive/negative amounts become deposit/withdrawal transactions)

### Scenario: Transfers between IBKR accounts
**Given** the XML contains Transfers between IBKR accounts
**When** the system parses the file
**Then** each transfer is recorded as a deposit or withdrawal transaction attributed to the user-selected account
**And** all IBKR account information in the transfer (source and destination accounts) is ignored

### Scenario: Upload invalid or empty XML file
**Given** the user uploads a file that is not valid IBKR Flex XML
**When** the system attempts to parse the file
**Then** the system returns an error describing the parsing failure
**And** no transactions are created

### Scenario: Upload XML with no matching transactions
**Given** the user uploads a valid Flex XML file
**When** all trades have unmapped symbols, all transactions are duplicates, or the file contains no Trades/CashTransactions/Transfers
**Then** the preview shows zero importable transactions
**And** the user can choose not to proceed with the import

### Scenario: Unsupported instrument types are skipped
**Given** the XML contains trades for derivatives (e.g., options, futures)
**When** the system parses the file
**Then** those trades are marked as skipped with the reason "unsupported instrument type"
**And** they are not included in the importable count
**And** they are shown in the preview under the skipped filter

### Scenario: Import via API
**Given** a client (including the web UI) wants to import an IBKR Flex XML report
**When** the client uploads the XML file along with the target account
**Then** the system parses the file and returns a preview with importable, skipped, and errored transactions
**And** the client can confirm the import via a subsequent request
**And** the system returns a summary of the import result

## Edge Cases

- **Empty XML file or malformed XML** — system returns a clear parsing error, no partial data created
- **XML with no Trades, no CashTransactions, no Transfers** — preview shows zero importable transactions
- **All symbols unmapped** — preview shows all trades as requiring symbol mapping; import can proceed once mappings are provided inline
- **All transactions are duplicates** — preview shows all as skipped with reasons
- **Mixed currencies in trades** — each transaction preserves its original currency from the XML
- **Negative quantities on sells** — IBKR reports sells with negative quantity; system handles this correctly
- **Zero netCash on FX trades** — FX trades are constructed from non-zero withdrawal/deposit amounts in the XML, so netCash is always non-zero in practice. The import bypasses the transaction validator (which rejects zero netCash) and uses BatchCreate directly.
- **Very large XML file with thousands of transactions** — system processes the file without timing out
- **Re-importing a previously imported file** — all duplicates detected and skipped, no new records created
- **XML containing derivatives (options, futures)** — unsupported instrument types are skipped with reason "unsupported instrument type"
- **Same IBKR transactionID across different report files** — duplicate detected via external_reference, second import skips it
- **User closes browser during preview** — no server-side state to clean up; any temporary data is discarded

## Constraints

- Only one XML file per import operation
- Target account must already exist in the system (no auto-creation of accounts)
- Single account per import — the user selects one target account for all transactions in the file
- IBKR account information from the XML is ignored entirely; user selects the target account
- The XML parser is IBKR-specific; the import workflow (upload → preview → map → confirm) is designed to be reusable for other brokers
- The import operation is transactional: all importable transactions are created together or none are
- Symbol creations and mappings made during the preview are committed immediately and are not part of the import transaction
- Symbol creation, broker symbol mapping, and market data preview are all inline on the import page — no navigation to other pages required
- Web UI is required for the import flow; the same API is used by the web UI
- Supported transaction types: Trades (buy/sell of stocks and ETFs), CashTransactions (deposits, withdrawals, dividends, interest, fees, taxes), Transfers
- Unsupported instrument types (options, futures, etc.) are skipped with a reason

## Non-Goals

- Support for brokers other than IBKR (e.g., Trading 212 is a separate feature)
- Auto-creation of accounts from the XML
- Real-time API integration with IBKR (file upload only)
- Import of positions or portfolio snapshots (transactions only)
- Import of derivatives (options, futures, etc.) — unsupported instrument types are skipped
- Support for CorporateActions section of the XML
- Batch import of multiple files at once
- Scheduling or automation of imports
- Importing into multiple accounts from a single file — one target account per import operation

## Reference Materials

- **`ibkr_sample_redacted.xml`** — Sample IBKR Flex Web Report (AF type) with redacted account details. Contains Trades (STK COMMON, STK ETF), CashTransactions, and Transfers. Use as the primary reference for XML structure, available fields, and real-world data patterns.
- **`ibkrmapping.txt`** — Reference implementation (Rust) showing the parsing and mapping logic for Trades, CashTransactions, FX trades, and Transfers. Useful for understanding field extraction and type classification.

## Dependencies

- f002 (Account CRUD) — target account must exist
- f003 (Symbol Map) — symbol resolution uses the existing symbol map
- f004 (Transaction CRUD) — imported transactions are created via the transaction service
