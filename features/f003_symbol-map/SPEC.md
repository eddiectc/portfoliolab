# Feature: symbol-map

## Description

A symbol mapping system that normalizes ticker symbols across broker imports and maps them to external market data providers. The default market data provider is Yahoo Finance. This ensures positions aggregate correctly regardless of how a broker reports a symbol, and enables market data lookups using a consistent convention.

This feature provides the symbol mapping infrastructure consumed by future features (transaction CRUD, broker imports). Transaction creation and import are **not** part of this feature.

## User Stories

- As a user creating a transaction, I want to define the market data provider mapping for a new symbol, so that prices can be fetched for P&L calculations.
- As a user importing transactions from a broker, I want to map broker-specific symbols to internal canonical symbols, so that my positions aggregate correctly across accounts.
- As a user managing my portfolio, I want to view and edit all symbol mappings in a dedicated page, so that I can maintain correct mappings over time.
- As a user, I want symbols to follow a consistent convention (Yahoo Finance format), so that I can understand and manage them without ambiguity.

## Scenarios

### Scenario: Create symbol mapping via CRUD page
**Given** I navigate to the symbol mappings page
**When** I create a new mapping with an internal symbol and a market data provider symbol
**Then** the mapping is saved
**And** I can optionally associate one or more broker symbols with the mapping

### Scenario: Market data preview during symbol creation
**Given** I am creating a new symbol mapping and typing a market data provider symbol
**When** I pause typing for a short duration (e.g., 2 seconds)
**Then** a preview appears showing the instrument's name, exchange, currency, and latest price
**And** if the symbol is invalid or the data fetch fails, the preview shows a warning but does not block creation
**And** I can still proceed to save the mapping without a successful preview

### Scenario: Update market data provider symbol
**Given** a symbol mapping already exists (e.g., internal symbol `AAPL` → Yahoo Finance `AAPL`)
**When** I edit the mapping to change the market data provider symbol
**Then** the mapping is updated
**And** existing transactions referencing this symbol are not affected

### Scenario: Update internal symbol
**Given** a symbol mapping already exists (e.g., internal symbol `APPL` → Yahoo Finance `AAPL`)
**When** I edit the mapping to change the internal symbol to `AAPL`
**And** `AAPL` does not already exist as an internal symbol
**Then** the mapping is updated with the new internal symbol
**And** existing transactions referencing the old internal symbol continue to work

### Scenario: Update internal symbol to an existing one
**Given** a symbol mapping exists with internal symbol `APPL`
**And** another mapping already exists with internal symbol `AAPL`
**When** I attempt to change the internal symbol from `APPL` to `AAPL`
**Then** the update is rejected with an error indicating the internal symbol already exists

### Scenario: Delete symbol mapping
**Given** a symbol mapping exists and is referenced by one or more transactions
**When** I attempt to delete the mapping
**Then** I am warned that the mapping is in use
**And** the deletion is blocked until the referencing transactions are handled

### Scenario: Delete unused symbol mapping
**Given** a symbol mapping exists and is not referenced by any transaction
**When** I delete the mapping
**Then** the mapping is removed

### Scenario: Add broker symbol to existing mapping
**Given** a symbol mapping already exists (e.g., internal symbol `AAPL` → Yahoo Finance `AAPL`)
**When** I add a broker source (e.g., broker "IBKR", broker symbol `AAPL.US`) to the mapping
**Then** the broker symbol is associated with the existing internal symbol
**And** the market data provider symbol is unchanged

### Scenario: Broker symbol already mapped to a different internal symbol
**Given** broker symbol `AAPL.US` (from broker "IBKR") is already mapped to internal symbol `AAPL`
**When** I attempt to add the same broker symbol from the same broker to a different internal symbol `AAPL-ALT`
**Then** the addition is rejected with an error indicating the broker symbol is already in use

### Scenario: View symbol mappings list
**Given** I have multiple symbol mappings configured
**When** I navigate to the symbol mappings page
**Then** I see a list of all mappings showing internal symbol, broker source(s), and market data provider symbol

> **Deferred**: Search/filter by internal symbol, broker, or market data provider symbol. Existing list pages (portfolios, accounts) don't have search either. Deferred to a future UX improvement feature.

### Scenario: Market data preview during symbol update
**Given** I am editing an existing symbol mapping and change the market data provider symbol
**When** I pause typing for a short duration
**Then** a preview appears showing the instrument's name, exchange, currency, and latest price for the new symbol
**And** if the preview fails, the update is not blocked

### Scenario: Duplicate internal symbol prevented
**Given** an internal symbol `AAPL` already exists in the symbol mappings
**When** I attempt to create another mapping with the same internal symbol
**Then** the creation is rejected with an error

## Edge Cases

- **Empty or whitespace-only symbol**: Rejected during creation/update.
- **Symbol used in transactions but deleted**: Deletion blocked; user must reassign or delete referencing transactions first.
- **Broker symbol identical to internal symbol**: Allowed — the mapping still records the broker source explicitly.
- **Multiple brokers, same internal symbol**: Allowed — e.g., IBKR's `AAPL.US` and Trading212's `AAPLU` both map to internal symbol `AAPL`.
- **Same internal symbol, different market data provider symbols**: Not allowed — each internal symbol has exactly one market data provider symbol (e.g., `AAPL` maps to one Yahoo Finance symbol only).
- **Market data preview fetch fails or times out**: Preview area shows a warning (e.g., "could not fetch data"); symbol creation/update is not blocked.

## Constraints

- No automatic symbol fetching or verification from external APIs.
- No symbol resolution heuristics — all mappings are user-defined.
- Market data provider symbol convention follows Yahoo Finance format (e.g., `AAPL`, `AAPL.LON`, `0700.HK`).
- Each internal symbol maps to exactly one market data provider symbol.
- Multiple broker symbols can map to the same internal symbol.

## Non-Goals

- Automatic symbol detection or suggestion from external data sources.
- Support for market data providers other than Yahoo Finance (future feature).
- Symbol verification (checking whether a Yahoo Finance symbol is valid).
- Bulk import of symbol mappings from a file.
- Historical symbol mapping (e.g., handling corporate actions like ticker changes).
- Transaction CRUD (creating/editing individual transactions) — future feature that consumes this.
- Broker import workflow (upload, preview, map, submit) — future feature that consumes this.

## Dependencies

- **Market data layer (soft dependency)**: The market data preview requires the ability to fetch a quote from Yahoo Finance. If the market data layer is unavailable, the preview simply shows no data and the feature continues to work. This is a soft dependency — the core symbol mapping functionality does not require market data fetching.
