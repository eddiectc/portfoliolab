# Feature: Overlap Enhancement

## Description

Enhance the **Holdings Overlap** section on the portfolio comparison page with deeper allocation analysis. The current overlap section shows only top underlying holdings per portfolio and a weighted overlap percentage. This enhancement adds sector overlap, country allocation comparison, and a richer holdings analysis with overweight/underweight breakdowns.

The goal is to give the investor a complete picture of how two portfolios differ in their exposure — not just which stocks overlap, but where the sector and geographic allocations diverge and which holdings drive the difference.

## User Stories

### US-1: Sector Overlap Analysis
As an investor, I want to see the combined sector allocation for each portfolio side-by-side, so that I can understand how each portfolio is positioned across sectors and spot concentration differences.

### US-2: Sector Drift Visualization
As an investor, I want to see a drift chart showing the sector allocation difference between the two portfolios, so that I can quickly identify which sectors one portfolio is overweight or underweight relative to the other.

### US-3: Country Allocation Analysis
As an investor, I want to see the combined country allocation for each portfolio side-by-side, so that I can understand geographic exposure differences between the two portfolios.

### US-4: Country Drift Visualization
As an investor, I want to see a drift chart showing the country allocation difference between the two portfolios, so that I can quickly identify geographic exposure divergence.

### US-5: Merged Holdings Table
As an investor, I want to see a single table of top holdings from both portfolios, ordered by overlap, so that I can compare weightings of shared and unique holdings at a glance.

### US-6: Overweight/Underweight Holdings
As an investor, I want to see the top 10 holdings where the first portfolio is overweight or underweight relative to the second portfolio, so that I can understand the key drivers of portfolio divergence.

## Scenarios

### Scenario: View sector allocation for both portfolios
**Given** two portfolios are selected for comparison
**And** at least one portfolio contains ETFs with sector data available
**When** I view the sector allocation section
**Then** I see a table showing each sector's combined weight (%) for the first portfolio and the second portfolio side-by-side
**And** sectors are sorted by combined weight descending
**And** sectors present in only one portfolio show 0% for the other

### Scenario: View sector drift chart
**Given** two portfolios are selected for comparison
**And** sector allocation data is available for at least one portfolio
**When** I view the sector drift visualization
**Then** I see a diverging bar chart showing the difference (first portfolio minus second portfolio) for each sector
**And** positive bars indicate sectors where the first portfolio is overweight relative to the second portfolio
**And** negative bars indicate sectors where the first portfolio is underweight relative to the second portfolio
**And** sectors are sorted by absolute difference descending

### Scenario: View country allocation for both portfolios
**Given** two portfolios are selected for comparison
**And** at least one portfolio contains ETFs with country data available
**When** I view the country allocation section
**Then** I see a table showing each country's combined weight (%) for the first portfolio and the second portfolio side-by-side
**And** countries are sorted by combined weight descending
**And** countries present in only one portfolio show 0% for the other

### Scenario: View country drift chart
**Given** two portfolios are selected for comparison
**And** country allocation data is available for at least one portfolio
**When** I view the country drift visualization
**Then** I see a diverging bar chart showing the difference (first portfolio minus second portfolio) for each country
**And** positive bars indicate countries where the first portfolio is overweight relative to the second portfolio
**And** negative bars indicate countries where the first portfolio is underweight relative to the second portfolio
**And** countries are sorted by absolute difference descending

### Scenario: View merged holdings table
**Given** two portfolios are selected for comparison
**And** underlying holdings data is available for at least one portfolio
**When** I view the holdings analysis section
**Then** I see a single table showing top holdings from both portfolios merged
**And** the table includes columns: Name, Weight in the first portfolio, Weight in the second portfolio, Overlap %
**And** holdings appearing in both portfolios are listed first, sorted by Overlap % descending
**And** holdings appearing in only one portfolio follow, sorted by their weight in that portfolio descending (holdings unique to either portfolio are interleaved, not separated into subsections)
**And** Overlap % is the absolute percentage points of the smaller weight (min of the two weights) for shared holdings
**And** Overlap % is 0% for holdings present in only one portfolio
**And** the table is limited to the top 10 from the first portfolio plus the top 10 from the second portfolio (deduplicated by underlying symbol)

### Scenario: View overweight holdings table
**Given** two portfolios are selected for comparison
**And** underlying holdings data is available for both portfolios
**When** I view the overweight section
**Then** I see a table of the top 10 holdings where the first portfolio weight exceeds the second portfolio weight
**And** holdings are sorted by the difference (first minus second) descending
**And** the table includes columns: Name, Weight in the first portfolio, Weight in the second portfolio, Difference
**And** holdings where the second portfolio weight is 0 (not present in the second portfolio) are included

### Scenario: View underweight holdings table
**Given** two portfolios are selected for comparison
**And** underlying holdings data is available for both portfolios
**When** I view the underweight section
**Then** I see a table of the top 10 holdings where the second portfolio weight exceeds the first portfolio weight
**And** holdings are sorted by the difference (second minus first) descending
**And** the table includes columns: Name, Weight in the first portfolio, Weight in the second portfolio, Difference
**And** holdings where the first portfolio weight is 0 (not present in the first portfolio) are included

### Scenario: Sector allocation with missing data
**Given** two portfolios are selected for comparison
**And** some ETFs in the portfolios have no cached sector data
**When** I view the sector allocation section
**Then** I see the sector breakdown for symbols that have data
**And** a warning indicates which symbols are missing sector data
**And** missing data is accumulated into an "Unknown" bucket

### Scenario: Country allocation with missing data
**Given** two portfolios are selected for comparison
**And** some ETFs in the portfolios have no cached country data
**When** I view the country allocation section
**Then** I see the country breakdown for symbols that have data
**And** a warning indicates which symbols are missing country data
**And** missing data is accumulated into an "Unknown" bucket

### Scenario: Holdings analysis with no underlying data
**Given** two portfolios are selected for comparison
**And** neither portfolio contains ETFs with holdings data (e.g., all direct stock holdings)
**When** I view the holdings analysis section
**Then** I see the direct holdings treated as their own underlying holdings
**And** the merged table, overweight, and underweight tables are computed from direct positions

### Scenario: Portfolio names used throughout
**Given** two portfolios are selected for comparison
**When** I view any section of the enhanced overlap analysis
**Then** all labels, table headers, and chart legends use the actual portfolio names instead of generic positional labels

### Scenario: View complete enhanced overlap page
**Given** two portfolios are selected for comparison
**And** both portfolios contain ETFs with sector, country, and holdings data available
**When** I view the enhanced overlap section of the comparison page
**Then** I see the sector allocation table and drift chart
**And** I see the country allocation table and drift chart
**And** I see the merged holdings table
**And** I see the overweight and underweight holdings tables
**And** all sections are presented in a single scrollable view

### Scenario: One portfolio is empty
**Given** two portfolios are selected for comparison
**And** one portfolio has no positions
**When** I view the enhanced overlap analysis
**Then** all tables show 0% for the empty portfolio's columns
**And** a message indicates the empty portfolio has no holdings to compare
**And** drift charts show the full allocation of the non-empty portfolio as positive difference

### Scenario: Both portfolios identical
**Given** two identical portfolios are selected for comparison
**When** I view the enhanced overlap analysis
**Then** the sector drift chart shows zero difference for all sectors
**And** the country drift chart shows zero difference for all countries
**And** the overweight and underweight tables are empty
**And** the merged holdings table shows 100% overlap for all shared holdings

## Edge Cases

- **No sector data for any symbol** — Sector allocation section shows "Unknown" bucket with total weight and a message explaining no sector data is available
- **No country data for any symbol** — Country allocation section shows "Unknown" bucket with total weight and a message explaining no country data is available
- **No ETF holdings data** — Holdings analysis falls back to direct positions (symbols themselves are treated as underlying holdings)
- **One portfolio has data, the other has none** — Drift charts show the full allocation of the portfolio with data as the difference; tables show 0% for the empty portfolio
- **All holdings are direct stocks (no ETFs)** — Sector/country data comes from the stock's primary sector and country fields; holdings analysis uses direct positions
- **Model portfolio vs real portfolio** — Both portfolio types are treated the same way; model portfolio weights are used directly, real portfolio uses current positions
- **Very large number of unique countries** — Country drift chart limits display to top N countries by absolute difference (e.g., top 15), with a note about remaining countries
- **Very large number of sectors** — Sector drift chart shows all sectors (typically 11 GICS sectors, so no truncation needed)
- **Zero-weight holdings** — Holdings with 0% weight in both portfolios are excluded from all tables
- **Negative drift values** — Diverging bar chart correctly renders negative values below the axis with distinct coloring

## Constraints

- Sector allocation uses the same methodology already available in the portfolio analysis feature (ETF look-through to constituents, primary sector for direct stock holdings)
- Country allocation uses the same methodology already available in the portfolio analysis feature (ETF look-through to geographic breakdown)
- Holdings analysis uses underlying holdings (ETFs expanded to constituents), same as the existing overlap computation
- Overlap % for shared holdings is the smaller of the two weights (min), shown as absolute percentage points
- Merged holdings table is limited to top 10 from the first portfolio + top 10 from the second portfolio (deduplicated)
- Overweight/underweight tables are limited to top 10 each
- Drift charts show the difference (first portfolio minus second portfolio) as diverging bars
- Portfolio names (not positional labels) are used in all labels, headers, and legends
- Maximum of 2 portfolios in a single comparison (existing constraint)
- Sector/country data must be available in cached symbol details; no live fetching during comparison

## Non-Goals

- **Industry allocation** — Only sectors and countries; no GICS industry-level breakdown
- **Asset class allocation** — Not part of this enhancement
- **Currency exposure analysis** — Not part of this enhancement
- **More than 2 portfolios** — Comparisons remain limited to 2 portfolios
- **Export / download** — No export functionality for the enhanced analysis
- **Interactive filtering** — No ability to filter sectors/countries dynamically; all data is shown at once
- **Time-series allocation history** — Allocation is computed at current positions; no historical allocation tracking
- **Custom drift direction** — Drift is always the first portfolio minus the second portfolio (selection order); no toggle to reverse

## Dependencies

- **f020_portfolio-comparison** — Extends the existing comparison page and overlap section
- **f015_symbol-details** — Requires sector weightings, geographic allocations, and top holdings data cached in symbol details
- **f017_portfolio-analysis** — Reuses the existing sector and geographic allocation computation logic
