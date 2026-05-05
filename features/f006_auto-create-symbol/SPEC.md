# Feature: auto-create-symbol

## Description

When creating or editing a transaction in the web UI, the user can type a symbol that doesn't yet exist in the symbol map. The form shows a live market data preview (instrument name, exchange, currency, latest price) and lets the user confirm creation of the symbol mapping directly from the transaction form — without navigating away to the symbol mappings page. If the preview fetch fails, the user can still create the symbol by filling in the required details manually. This eliminates the extra step of pre-creating every symbol before recording transactions.

The `$CASH-{currency}` symbols remain as a separate, silent auto-create convention: they are created automatically when first used in a transaction, with no preview and no user confirmation needed. This is distinct from the inline symbol creation workflow, which requires explicit user confirmation for all non-cash symbols.

## User Stories

- As a user recording a transaction, I want to type a symbol I haven't set up yet and see a market data preview, so that I can verify it's the right instrument before creating it.
- As a user recording a transaction, I want to create a new symbol mapping directly from the transaction form, so that I don't have to navigate to a separate page first.
- As a user recording a transaction, I want to create a symbol manually if the market data preview fails, so that I'm not blocked by external API issues.
- As a user editing a transaction, I want the same symbol preview and inline creation when changing to a new symbol, so that the workflow is consistent.

## Scenarios

### Scenario: Preview existing symbol while typing
**Given** a symbol mapping for `AAPL` already exists
**When** I type `AAPL` in the symbol field on the transaction form
**And** I pause typing briefly
**Then** a preview appears showing the instrument's name, exchange, currency, and latest price
**And** the preview is sourced from the existing symbol mapping's market data

### Scenario: Preview new symbol while typing
**Given** no symbol mapping for `MSFT` exists
**When** I type `MSFT` in the symbol field on the transaction form
**And** I pause typing briefly
**Then** a preview appears showing the instrument's name, exchange, currency, and latest price
**And** the preview indicates that this symbol does not yet exist in the symbol map
**And** an option to create the symbol mapping is shown

### Scenario: Create new symbol from preview and submit transaction
**Given** no symbol mapping for `MSFT` exists
**When** I type `MSFT` in the symbol field on the transaction form
**And** the preview appears with market data details
**And** I confirm creation of the symbol mapping from the preview
**Then** the symbol mapping is created immediately with the internal symbol `MSFT` and the market data provider symbol `MSFT`
**And** the symbol field is now populated with the created symbol
**And** I can continue filling out and submitting the transaction

### Scenario: Preview shows auto-corrected symbol
**Given** no symbol mapping for `AAPL` exists
**And** I type `AAP` in the symbol field on the transaction form
**And** I pause typing briefly
**And** the market data fetch returns data for `AAPL` (corrected symbol)
**Then** the preview shows the details for `AAPL` with an indicator that the symbol was corrected
**And** the preview does not auto-accept the corrected symbol
**When** I explicitly accept `AAPL` from the preview
**And** I confirm creation of the symbol mapping
**Then** the symbol mapping is created with the internal symbol `AAPL`
**And** the symbol field is populated with `AAPL`
**When** I do not accept the corrected symbol
**Then** I can clear the field or type a different symbol

### Scenario: Preview fails — create symbol manually
**Given** no symbol mapping for `UNKNOWN` exists
**And** the market data fetch for `UNKNOWN` fails or times out
**When** I type `UNKNOWN` in the symbol field on the transaction form
**And** I pause typing briefly
**Then** the preview area shows a warning that market data could not be fetched
**And** I am presented with fields to manually enter all symbol details: name, exchange, currency, and market data provider symbol
**When** I fill in all required fields and confirm creation
**Then** the symbol mapping is created with the details I provided
**And** the symbol field is now populated with the created symbol
**And** I can continue filling out and submitting the transaction

### Scenario: Preview fails — user abandons symbol creation
**Given** no symbol mapping for `UNKNOWN` exists
**And** the market data fetch fails
**When** I type `UNKNOWN` and the preview shows a failure
**Then** I can clear the symbol field or choose a different existing symbol from the list
**And** the transaction form is not blocked

### Scenario: Cash symbol auto-created silently
**Given** no `$CASH-GBP` symbol mapping exists
**When** I create a deposit transaction with symbol `$CASH-GBP` and currency `GBP`
**Then** the `$CASH-GBP` symbol mapping is created automatically and silently
**And** no preview is shown for cash symbols
**And** no user confirmation is required
**And** the transaction is created successfully
**And** the symbol mapping uses the market data provider symbol `$CASH-GBP`

### Scenario: Cash symbol already exists
**Given** a `$CASH-USD` symbol mapping already exists
**When** I create a deposit transaction with symbol `$CASH-USD` and currency `USD`
**Then** no preview is shown
**And** the transaction is created successfully using the existing `$CASH-USD` symbol mapping

### Scenario: Cash symbol with mismatched currency rejected
**Given** no `$CASH-GBP` symbol mapping exists
**When** I create a transaction with symbol `$CASH-GBP` but currency `USD`
**Then** the form shows an error indicating the currency mismatch
**And** the symbol is not auto-created

### Scenario: Malformed cash symbol rejected
**Given** I type `$CASH-` (missing currency) in the symbol field
**Then** the symbol is rejected with an error indicating an invalid cash symbol format
**And** no preview is shown
**And** the transaction form is not blocked

### Scenario: Edit transaction — change to existing symbol
**Given** a transaction with symbol `AAPL` exists
**And** a symbol mapping for `MSFT` already exists
**When** I edit the transaction and change the symbol to `MSFT`
**Then** the preview for `MSFT` appears
**And** the transaction is updated successfully with the new symbol

### Scenario: Edit transaction — change to new symbol, create inline
**Given** a transaction with symbol `AAPL` exists
**And** no symbol mapping for `TSLA` exists
**When** I edit the transaction and type `TSLA` as the new symbol
**And** the preview appears with market data details
**And** I confirm creation of the symbol mapping
**And** I save the transaction
**Then** the symbol mapping for `TSLA` is created
**And** the transaction is updated with symbol `TSLA`

### Scenario: Edit transaction — change to new symbol, preview fails, create manually
**Given** a transaction with symbol `AAPL` exists
**And** no symbol mapping for `XYZ` exists
**And** the market data fetch for `XYZ` fails
**When** I edit the transaction and type `XYZ` as the new symbol
**And** the preview shows a failure
**And** I manually fill in the symbol details and confirm creation
**And** I save the transaction
**Then** the symbol mapping for `XYZ` is created with the manually provided details
**And** the transaction is updated with symbol `XYZ`

### Scenario: Duplicate symbol creation — concurrent conflict
**Given** no symbol mapping for `NVDA` exists
**When** I attempt to create the `NVDA` symbol mapping from the transaction form
**And** the symbol mapping was already created by another request (e.g., another tab) between the preview and the creation
**Then** the creation is rejected with an error indicating the symbol already exists
**And** the symbol field is populated with the now-existing symbol
**And** I can continue and submit the transaction using the existing symbol

### Scenario: Preview clears when typing a different symbol
**Given** a preview for `AAPL` is showing
**When** I start typing a different symbol (e.g., `MSFT`)
**Then** the previous preview is cleared
**And** after pausing briefly, a new preview for `MSFT` appears

### Scenario: Preview does not show for cash symbols
**Given** I type `$CASH-USD` in the symbol field
**Then** no market data preview is shown
**And** the symbol is accepted as a valid cash symbol if the currency matches the transaction's currency field

## Edge Cases

- **Empty or whitespace-only symbol**: Rejected as invalid (same as existing behavior).
- **Preview fetch fails or times out**: Preview area shows a warning; user can fill in details manually or choose an existing symbol.
- **Symbol created between preview and submission**: The creation step detects the conflict, treats the symbol as already available, and allows the transaction to proceed.
- **Cash symbol (`$CASH-{currency}`) auto-created**: No preview, no manual entry — created silently on first use if the currency matches.
- **Cash symbol with mismatched currency**: Rejected — the currency portion of `$CASH-{currency}` must match the transaction's currency field.
- **Very long symbol**: Handled by existing symbol validation (max length enforced by the symbol map).
- **Special characters in symbol**: Handled by existing symbol validation (Yahoo Finance format convention).
- **Malformed cash symbol** (`$CASH-`, `$CASH-USD-EXTRA`): Rejected with an error indicating invalid cash symbol format.
- **Symbol auto-corrected by market data provider** (e.g., `AAP` → `AAPL`): Preview shows the corrected symbol; user must explicitly accept it before proceeding.

## Constraints

- **Scope**: This feature only affects the web UI transaction form. The API still requires non-cash symbols to exist; inline creation is a UI workflow that creates the symbol mapping before submitting the transaction.
- **Debounce**: The preview fetch is debounced to avoid excessive API calls while typing.
- **Market data is optional**: The feature works even when market data fetching is unavailable — users fall back to manual entry.
- **No special marking**: Symbols created via the inline workflow or the cash auto-create are not distinguished from manually created ones in the UI or data model.
- **Cash convention**: `$CASH-{currency}` symbols follow the existing convention (quantity = cash value, price = 1, netCash = quantity) and are created automatically and silently on first use — separate from the inline creation workflow. All other symbols require explicit user confirmation via the inline creation workflow.
- **Immediate creation**: When the user confirms symbol creation from the preview, the symbol is created immediately. If the user later picks a different symbol, the previously created symbol remains in the system (no deletion).
- **Manual entry requires all fields**: When the market data preview fails, the user must manually provide all symbol fields: name, exchange, currency, and market data provider symbol.
- **Symbol auto-correction**: If the market data provider returns data for a different symbol than typed (e.g., `AAP` → `AAPL`), the preview shows the corrected symbol and the user must explicitly accept it.

## Non-Goals

- Automatic symbol verification or suggestion beyond the market data preview (symbol auto-correction from the market data provider is supported but requires explicit user acceptance).
- Support for market data providers other than Yahoo Finance (future feature).
- Bulk symbol creation.
- Symbol creation during broker import (future feature that reuses the same API).
- API-level auto-creation of non-cash symbols during transaction creation (the API still requires non-cash symbols to exist; inline creation is a UI workflow that creates the symbol mapping before submitting the transaction). The `$CASH-{currency}` exception remains at the API level.
- Symbol mapping editing from the transaction form (only creation).
- Historical symbol mapping or corporate action handling.

## Dependencies

- **f003_symbol-map** (done) — the symbol map system must exist; this feature creates entries in it.
- **f004_transaction-crud** (done) — the transaction form and API must exist; this feature enhances the form.
- **Market data layer** (soft dependency) — the preview requires fetching quotes from Yahoo Finance. If unavailable, the preview shows a warning and manual entry is offered.
