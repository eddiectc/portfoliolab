# Feature: Performance Benchmark

## Description
Allow the investor to compare their portfolio's performance against a set of pre-defined market benchmarks. The comparison appears across three views: the performance chart (overlay line), the monthly return heatmap (side-by-side toggle), and the summary statistics (MWR percentage). This gives the investor immediate context on whether their portfolio is beating or lagging the market.

## User Stories

- As an investor, I want to overlay a benchmark line on my portfolio performance chart, so I can visually compare my returns against the market at a glance.
- As an investor, I want to see my portfolio's MWR alongside the benchmark's MWR on the performance page, so I can quantify the difference.
- As an investor, I want to toggle a benchmark comparison in the monthly return heatmap, so I can see which months I outperformed or underperformed on a month-by-month basis.

## Scenarios

### Scenario: View portfolio performance without a benchmark
**Given** I am on the portfolio performance page
**When** no benchmark is selected
**Then** I see only my portfolio's performance chart, monthly heatmap, and MWR statistic as before

### Scenario: Select a benchmark for the first time
**Given** I am on the portfolio performance page with no benchmark selected
**When** I open the benchmark selector and choose a benchmark (e.g., S&P 500)
**Then** the benchmark line appears on the performance chart
**And** the benchmark's MWR percentage appears alongside my portfolio's MWR percentage
**And** the monthly heatmap updates to show the benchmark comparison

### Scenario: Overlay a benchmark on the performance chart
**Given** I am on the portfolio performance page with a date range selected
**When** I select a benchmark (e.g., S&P 500)
**Then** a second line appears on the performance chart representing the benchmark's price over the same period
**And** the portfolio line uses the left Y-axis (portfolio value scale)
**And** the benchmark line uses a separate right Y-axis showing the benchmark's actual price

### Scenario: Switch between different benchmarks
**Given** a benchmark is currently selected on the performance chart
**When** I select a different benchmark (e.g., switch from S&P 500 to NASDAQ)
**Then** the benchmark line updates to reflect the new benchmark's cumulative return over the currently selected date range

### Scenario: Clear the benchmark selection
**Given** a benchmark is currently selected
**When** I deselect the benchmark (e.g., choose "None" or a clear option)
**Then** the benchmark line is removed from the chart
**And** the benchmark's MWR percentage is hidden
**And** the monthly heatmap reverts to showing only my portfolio's monthly returns
**And** the view returns to showing only my portfolio performance

### Scenario: Benchmark changes with date range
**Given** a benchmark is selected and a date range is applied
**When** I change the date range (e.g., from YTD to 1 Year)
**Then** the benchmark line updates to the cumulative return for the new date range

### Scenario: View MWR comparison on the performance page
**Given** I am on the portfolio performance page with a benchmark selected
**When** I view the summary statistics
**Then** I see my portfolio's MWR percentage alongside the benchmark's MWR percentage for the selected date range

### Scenario: MWR disappears when benchmark is cleared
**Given** a benchmark is selected and MWR comparison is visible
**When** I deselect the benchmark
**Then** the benchmark MWR statistic is hidden and only my portfolio's MWR remains

### Scenario: Monthly heatmap without benchmark
**Given** I am viewing the monthly return heatmap
**When** no benchmark is selected
**Then** I see only my portfolio's monthly returns color-coded in the heatmap

### Scenario: Monthly heatmap with benchmark comparison
**Given** I am viewing the monthly return heatmap
**When** I select a benchmark
**Then** each cell shows my portfolio's monthly return compared to the benchmark's monthly return for that same month
**And** the coloring reflects whether I outperformed (warmer color) or underperformed (cooler color) the benchmark that month

### Scenario: Heatmap updates when benchmark changes
**Given** a benchmark is selected in the heatmap
**When** I switch to a different benchmark
**Then** the heatmap recalculates the monthly comparison against the new benchmark

### Scenario: Heatmap updates when date range changes
**Given** a benchmark is selected and a date range is applied to the heatmap
**When** I change the date range
**Then** the heatmap shows the monthly comparison for the new date range

### Scenario: Benchmark selection is shared across views
**Given** I select a benchmark in one view (e.g., the performance chart)
**When** I look at the monthly heatmap or the MWR statistics section on the same page
**Then** the same benchmark selection is reflected in those views without needing to re-select it

## Edge Cases
- **Benchmark data unavailable for selected date range:** If the benchmark has no price data for part or all of the selected period, the benchmark line/stat gracefully degrades (e.g., line stops where data ends, stat shows a warning)
- **Benchmark data fetch fails:** If market data for the benchmark cannot be fetched, the UI shows the portfolio data alone with a warning badge next to the benchmark name indicating the data is unavailable
- **Partial data overlap:** If the benchmark and portfolio have different data availability windows (e.g., benchmark starts in 2010 but portfolio started in 2024), the comparison is shown only for the overlapping period; the benchmark line/clipping adjusts accordingly
- **Benchmark data has gaps:** If the benchmark price data has discontinuities (e.g., delisting, split adjustments, missing days), the chart handles gaps gracefully (e.g., line breaks at discontinuities rather than interpolating across them)
- **Date range exceeds benchmark history:** If the selected date range extends before the benchmark's earliest available data, the benchmark line starts from its first available data point within the range rather than from the range boundary
- **Benchmark currency differs from portfolio:** The three UK-domiciled benchmarks (VWRP.L, VUSA.L, XNAQ.L) are GBP-denominated while the portfolio may be multi-currency — the benchmark return is shown as a pure percentage (currency-agnostic), so no conversion is needed for the comparison
- **Empty portfolio (no transactions):** The benchmark can still be shown independently; portfolio line and statistics are empty or zero while the benchmark data displays normally
- **Very short date range (e.g., single day):** The comparison still works but may show minimal or zero return difference

## Constraints
- Only five pre-defined benchmarks are supported:
  - S&P 500 (`^GSPC`)
  - NASDAQ Composite (`^IXIC`)
  - Vanguard FTSE All-World UCITS (`VWRP.L`)
  - Vanguard S&P 500 UCITS (`VUSA.L`)
  - iShares NASDAQ 100 UCITS (`XNAQ.L`)
- Benchmark data uses the existing market data fetch and caching infrastructure
- The comparison is return-based (percentage) — no absolute value comparison
- Only one benchmark can be selected at a time (no multi-benchmark overlay)
- The feature uses the existing performance page; no new page is added

## Non-Goals
- Risk-adjusted metrics (alpha, beta, Sharpe ratio, etc.)
- Benchmark rebalancing or buy-and-hold simulation
- Custom user-defined benchmarks (users cannot add their own tickers)
- Multiple simultaneous benchmarks (only one benchmark at a time)
- Tax-adjusted returns
- Currency conversion for benchmark values (returns are expressed as percentages)
- Real-time benchmark data streaming (uses cached/fetched data like existing market data)

## Dependencies
- **f010 (Portfolio Performance)** — provides the performance chart, MWR/TWR stats, and date range selection
- **f011 (Historical Market Data Caching)** — provides the mechanism to fetch and cache benchmark price data (already complete)
