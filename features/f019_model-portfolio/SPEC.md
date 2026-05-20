# Feature: Model Portfolio

## Description

A **Model Portfolio** is a named, hypothetical portfolio definition consisting of symbols and target weight percentages (summing to 100%). Unlike real portfolios (which track actual transactions and positions), model portfolios are allocation blueprints used for analysis and comparison.

This feature enables the investor to:
- Create and manage named model portfolios (symbol + % definitions)
- View and browse their collection of model portfolios
- Apply a model portfolio as the target allocation on a real portfolio with a single action

## User Stories

### US-1: Create a Model Portfolio
As an investor, I want to create a named model portfolio by specifying symbols and their target weight percentages, so that I can save allocation blueprints for later use.

### US-2: Edit or Delete a Model Portfolio
As an investor, I want to edit the name, symbols, or weights of an existing model portfolio, or delete one I no longer need, so that my model portfolio collection stays current.

### US-3: Add New Symbols Inline
As an investor creating a model portfolio, I want to add symbols that aren't yet in the system without navigating to another page, so that I can define my model portfolio in a single flow.

### US-4: List and Browse Model Portfolios
As an investor, I want to see a list of all my model portfolios with key details, so that I can find and select one for comparison or application.

### US-5: Apply Model Portfolio as Target Allocation from Allocation Page
As an investor, I want to select a saved model portfolio from the Allocation page and apply it as my target allocation with a single action, so that I can quickly set my rebalancing targets from a blueprint I've already analyzed.

## Scenarios

### Scenario: Create a model portfolio (happy path)
**Given** I am on the model portfolio creation page
**When** I enter a unique name (e.g., "60/40 Classic")
**And** I add symbols with target percentages that sum to 100% (e.g., VWRP 60%, VGIT 40%)
**And** I save the model portfolio
**Then** the model portfolio is stored with its name, symbols, and weights
**And** I can view it in the model portfolio list

### Scenario: Create model portfolio with inline symbol creation
**Given** I am creating a model portfolio
**And** I enter a ticker that is not yet in the system
**When** I add it to the model portfolio
**Then** the symbol is created and becomes available in the system
**And** the symbol appears in the model portfolio definition
**And** market data fetching is triggered for the new symbol

### Scenario: Reject model portfolio with invalid weights
**Given** I am creating or editing a model portfolio
**When** the target percentages do not sum to 100%
**And** I attempt to save
**Then** the save is rejected
**And** an error message shows the current total and how far it is from 100%

### Scenario: Reject model portfolio with negative or zero weights
**Given** I am creating or editing a model portfolio
**When** I enter a weight of 0% or a negative percentage for a symbol
**And** I attempt to save
**Then** the save is rejected
**And** an error message indicates that each symbol must have a positive weight

### Scenario: Reject model portfolio with duplicate name
**Given** a model portfolio named "60/40 Classic" already exists
**When** I attempt to create another model portfolio with the same name
**Then** the save is rejected
**And** an error message indicates the name is already in use

### Scenario: Reject rename to an existing name
**Given** two model portfolios "Portfolio A" and "Portfolio B" exist
**When** I rename "Portfolio A" to "Portfolio B"
**Then** the rename is rejected
**And** an error message indicates the name is already in use

### Scenario: Edit an existing model portfolio
**Given** a model portfolio "60/40 Classic" exists with VWRP 60%, VGIT 40%
**When** I edit it to change weights to VWRP 70%, VGIT 30%
**And** I save the changes
**Then** the updated weights are stored
**And** the model portfolio retains its original name and creation date

### Scenario: Edit model portfolio — add a new symbol
**Given** a model portfolio with VWRP 60%, VGIT 40%
**When** I add a third symbol VUSA 20% and adjust to VWRP 40%, VUSA 20%, VGIT 40%
**And** I save
**Then** the model portfolio now has three symbols with the updated weights

### Scenario: Edit model portfolio — remove a symbol
**Given** a model portfolio with VWRP 40%, VUSA 20%, VGIT 40%
**When** I remove VUSA and adjust to VWRP 50%, VGIT 50%
**And** I save
**Then** the model portfolio now has two symbols with the updated weights

### Scenario: Delete a model portfolio
**Given** a model portfolio "Test Portfolio" exists
**When** I delete it
**Then** the model portfolio is removed
**And** it no longer appears in the model portfolio list
**And** it is no longer available as a comparison or application option

### Scenario: List model portfolios
**Given** multiple model portfolios exist
**When** I navigate to the model portfolios page
**Then** I see a list of all model portfolios showing name, number of symbols, and creation date
**And** I can click on a model portfolio to view its details (name, symbols, weights)

### Scenario: Empty model portfolio list
**Given** no model portfolios exist
**When** I navigate to the model portfolios page
**Then** I see an empty state message
**And** a prompt to create the first model portfolio is shown

### Scenario: Apply model portfolio as target allocation from Allocation page
**Given** a model portfolio "60/40 Classic" exists with VWRP 60%, VGIT 40%
**And** I have a real portfolio with an existing target allocation
**When** I navigate to the Allocation page
**And** I select "60/40 Classic" from the model portfolio selector
**And** I confirm the apply action
**Then** the existing target allocation is replaced with the model portfolio's symbols and weights
**And** the allocation page reflects the new target
**And** a confirmation message indicates the target allocation was updated from the model portfolio

### Scenario: Apply model portfolio when no target allocation exists
**Given** a model portfolio "60/40 Classic" exists with VWRP 60%, VGIT 40%
**And** I have a real portfolio with no target allocation configured
**When** I navigate to the Allocation page
**And** I select "60/40 Classic" from the model portfolio selector
**And** I confirm the apply action
**Then** the target allocation is created with the model portfolio's symbols and weights
**And** the allocation page reflects the new target
**And** a confirmation message indicates the target allocation was set from the model portfolio

### Scenario: Apply model portfolio with symbols not in real portfolio
**Given** a model portfolio contains symbols that are not currently held in the real portfolio
**When** I select it from the Allocation page and apply it as the target allocation
**Then** the target allocation is set with all symbols from the model portfolio
**And** symbols not yet held appear in the target with 0% actual and a full buy suggestion in the rebalancing view

## Edge Cases

- **Empty model portfolio list** — No model portfolios exist; the page shows an empty state with a prompt to create one
- **Single symbol model portfolio** — A model portfolio with only one symbol (100% weight) is valid
- **Weight rounding** — Weights are stored with sufficient precision; display rounds to 1 decimal place. The sum validation uses stored precision (tolerance of 0.01% for floating-point safety)
- **Model portfolio name with special characters** — Names can contain letters, numbers, spaces, and common punctuation (e.g., "60/40 Classic", "My Portfolio #2")
- **Deleting a model portfolio referenced in an active comparison** — If a model portfolio is deleted, it is removed from any active comparison selection
- **Symbol removed from market / delisted** — If a symbol in a model portfolio is no longer tradeable, the model portfolio still exists but a warning is shown when attempting to use it (compare or apply)

## Constraints

- Model portfolio weights must sum to exactly 100% (with 0.01% tolerance for floating-point)
- Each symbol in a model portfolio must have a positive weight (> 0%)
- Model portfolio names must be unique
- Model portfolio names are limited to 100 characters
- Applying a model portfolio replaces the single existing target allocation (consistent with f018)

## Non-Goals

- **Portfolio comparison** — Comparing portfolios (model vs model, model vs real, real vs real) is covered by f020
- **Real-time paper trading** — Model portfolios are allocation blueprints only, not live tracking
- **Rebalancing simulation** — No periodic rebalancing of weights
- **Transaction cost modeling** — No fees, spreads, or slippage are simulated
- **Tax lot simulation** — No tax implications are considered
- **Margin or leverage** — The simulation assumes full cash purchase
- **Dividend reinvestment** — Dividends are not explicitly modeled
- **Multiple target allocation profiles** — Applying a model portfolio replaces the single existing target allocation
- **Scheduling automatic comparisons or applications** — All actions are manual and on demand
- **Concurrent edit conflict resolution** — Last write wins; no optimistic locking

## Dependencies

- **f003_symbol-map** — Symbol management and mapping
- **f009_positions** — Real portfolio position data for applying model portfolios
- **f011_historical-market-data-caching** — Market prices and FX rates for newly added symbols
- **f018_allocation** — Target allocation storage and "apply as target" functionality
- **Inline symbol creation** — Existing mechanism from broker import (f007/f008) for adding new symbols during model portfolio creation
