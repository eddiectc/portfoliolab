# Feature: Benchmark Selection

## Description

Replace the hardcoded list of five pre-defined benchmarks with a user-driven approach: any symbol in the user's symbol mappings can be marked as a benchmark via a checkbox on the symbol create/edit forms. When a symbol is marked as a benchmark, it triggers the same historical price caching (from 2000 to present) that the current predefined benchmark system uses. The performance comparison UI (chart overlay, MWR stats, monthly heatmap) continues to work but draws from user-selected benchmarks instead of the hardcoded list.

Multiple symbols can be marked as benchmarks simultaneously. The user selects which one(s) to compare against on the performance page.

## User Stories

### US-1: Mark a Symbol as Benchmark on Creation
As a user creating a new symbol mapping, I want to optionally mark it as a benchmark so that it's available for portfolio performance comparison.

### US-2: Mark an Existing Symbol as Benchmark
As a user editing an existing symbol mapping, I want to toggle the benchmark flag so that I can add or remove it from the benchmark set.

### US-3: Historical Price Caching for Benchmarks
As a user, when I mark a symbol as a benchmark, I want its historical prices (from 2000 to present) to be fetched and cached automatically so that comparison data is available.

### US-4: Select Benchmark(s) on Performance Page
As a user viewing portfolio performance, I want to select from my benchmark symbols so that I can compare my portfolio against them.

### US-5: Background Refresh of Benchmark Prices
As a user, I want benchmark historical prices to be refreshed by the background job so that the data stays current without manual intervention.

### US-6: Remove Benchmark Flag
As a user, I want to unmark a symbol as a benchmark so that it's no longer available for comparison and no longer refreshed as a benchmark.

## Scenarios

### Scenario: Create symbol with benchmark flag
**Given** I am creating a new symbol mapping
**When** I fill in the internal symbol and market data provider symbol and check the "Use as benchmark" checkbox
**Then** the symbol mapping is saved with the benchmark flag set
**And** historical price fetching (2000 to present) is triggered in the background
**And** if the historical fetch fails, the symbol is still created and marked as benchmark (fetch failure is non-blocking)

### Scenario: Create symbol without benchmark flag
**Given** I am creating a new symbol mapping
**When** I leave the "Use as benchmark" checkbox unchecked
**Then** the symbol mapping is saved without the benchmark flag
**And** no benchmark-specific historical fetching is triggered

### Scenario: Toggle benchmark flag on existing symbol (enable)
**Given** symbol `WMGG.L` exists and is not marked as a benchmark
**When** I edit the symbol and check the "Use as benchmark" checkbox
**Then** the symbol is updated with the benchmark flag set
**And** historical price fetching (2000 to present) is triggered in the background

### Scenario: Toggle benchmark flag on existing symbol (disable)
**Given** symbol `^GSPC` exists and is marked as a benchmark
**When** I edit the symbol and uncheck the "Use as benchmark" checkbox
**Then** the symbol is updated with the benchmark flag cleared
**And** it is no longer included in benchmark background refreshes
**And** existing cached price data is preserved (not deleted)

### Scenario: Multiple benchmarks selected
**Given** symbols `^GSPC`, `WMGG.L`, and `VWRP.L` are all marked as benchmarks
**When** I view the portfolio performance page
**Then** all three appear as available benchmark options
**And** I can select one to compare against my portfolio

### Scenario: No benchmarks configured
**Given** no symbols are marked as benchmarks
**When** I view the portfolio performance page
**Then** the benchmark selector shows a message that no benchmarks are configured
**And** I cannot select a benchmark for comparison

### Scenario: Benchmark historical data unavailable
**Given** symbol `XYZ` is marked as a benchmark
**And** no historical price data is available for `XYZ`
**When** I select `XYZ` as the comparison benchmark on the performance page
**Then** the portfolio performance data is shown normally
**And** the benchmark comparison shows a warning that no data is available
**And** the benchmark line is not rendered on the chart

### Scenario: Background refresh includes benchmark symbols
**Given** symbols `^GSPC` and `WMGG.L` are marked as benchmarks
**When** the background market data refresh job runs
**Then** historical prices for both benchmark symbols are gap-filled (latest cached date to present)
**And** if data is current, the fetch is skipped

### Scenario: Background refresh excludes non-benchmark symbols
**Given** symbol `AAPL` is not marked as a benchmark
**When** the background market data refresh job runs
**Then** `AAPL` is not included in the benchmark refresh cycle
**And** `AAPL` is only refreshed if it has active positions (normal symbol refresh)

### Scenario: Switch benchmark on performance page
**Given** benchmarks `^GSPC` and `WMGG.L` are configured
**And** `^GSPC` is currently selected on the performance page
**When** I switch the selection to `WMGG.L`
**Then** the benchmark line updates to show `WMGG.L`'s cumulative return
**And** the benchmark MWR stat updates
**And** the monthly heatmap comparison updates

## Edge Cases

- **Symbol marked as benchmark but not found on Yahoo**: Historical fetch fails — symbol remains marked as benchmark, cached data is empty, warning shown on performance page
- **Benchmark symbol deleted**: If a symbol marked as benchmark is deleted, the benchmark flag is removed (cascading)
- **Benchmark symbol's market data provider symbol changed**: Historical data is still keyed by the provider symbol — existing cached data may become stale; next background refresh fetches with the new provider symbol
- **Index symbols (e.g., `^GSPC`, `^IXIC`)**: Treated the same as any other symbol — historical prices fetched and cached normally
- **Very old benchmark (data from 2000)**: Large initial fetch — handled in background, non-blocking
- **Concurrent benchmark toggles**: Rapidly enabling and disabling the benchmark flag — last write wins, fetch is idempotent
- **Currency mismatch**: Benchmark in different currency from portfolio — comparison is return-based (percentage), so no conversion needed

## Constraints

- **Benchmark flag**: A boolean field on the symbol mapping model (`is_benchmark`)
- **Multiple benchmarks**: User can mark any number of symbols as benchmarks
- **Selection on performance page**: User selects which benchmark(s) to display; the existing single-benchmark selection UI is preserved (one at a time), but the source of available benchmarks changes from hardcoded to user-defined
- **Historical fetch**: When a symbol is marked as benchmark, historical prices are fetched from 2000-01-01 to present, using the existing market data fetch and caching infrastructure (same as current predefined benchmarks)
- **Background refresh**: Benchmark symbols are included in the periodic gap-fill cycle (same logic as current `gapFillBenchmarks`), checking latest cached date and fetching gaps
- **Removal**: Unchecking the benchmark flag stops future refreshes but preserves existing cached data
- **Error responses**: `{"error": "message", "code": "ERROR_CODE"}`

## Non-Goals

- Deleting cached benchmark price data when the benchmark flag is removed (data preserved)
- Multiple simultaneous benchmark overlays on the chart (one at a time, as current)
- Risk-adjusted metrics (alpha, beta, Sharpe ratio)
- Custom benchmark configuration (e.g., buy-and-hold simulation, rebalancing)
- Automatic benchmark suggestion based on portfolio composition
- Historical benchmark flag changes (audit trail)

## Dependencies

- **f003_symbol-map** (done) — benchmark flag added to symbol mapping model
- **f012_performance-benchmark** (done) — comparison logic, chart overlay, MWR stats, monthly heatmap reused; source of benchmarks changes from hardcoded to user-defined
- **f011_historical-market-data-caching** (done) — market cache background refresh extended to include user-defined benchmarks
