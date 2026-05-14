# Implementation Plan: Portfolio Performance

## Overview

Build portfolio-level performance analytics: an equity curve chart (portfolio market value vs. cumulative net deposits over time) and summary return metrics (TWR, MWR, simple return, and their annualized counterparts). Supports single-portfolio and all-portfolio views, multi-currency FX conversion, period selection, and manual data refresh. Data is computed on-demand from transactions and cached market data.

## Task Dependencies

```
Task 1 (domain models)
    ↓
Task 2 (historical price fetching) ← Task 3 (equity curve computation)
    ↓                                 ↓
Task 4 (return metrics)        Task 5 (data refresh)
    ↓                                 ↓
Task 6 (API handler) ←──────── Task 7 (web handler + template)
                                            ↓
                                      Task 8 (router + nav wiring)
```

Tasks 2 and 3 can be worked in parallel after Task 1. Tasks 4 and 5 are independent of each other but both depend on earlier tasks. Tasks 6-8 are sequential integration.

## Tasks

### Task 1: Domain models for performance [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundational types)

**Description:** Define the domain types used by the performance computation layer — equity curve data points, performance result envelope, and refresh result.

- [x] Create `internal/domain/position/performance.go` with domain types:
  - `EquityCurvePoint` — Date, PortfolioValue, NetDeposit
  - `PerformanceResult` — EquityCurve ([]EquityCurvePoint), ReturnMetrics, BaseCurrency, Warnings
  - `ReturnMetrics` — TotalReturnPct, AnnualizedReturnPct (CAGR), HasInsufficientData
  - `RefreshResult` — SymbolsRefreshed, FxPairsRefreshed, FailedSymbols
- [x] Add `PerformanceFilters` struct (PortfolioID *int64, Period string, DateFrom/DateTo *time.Time)
- [x] Write unit tests for model serialization (JSON marshal/unmarshal of decimal fields)

**Verification:** Types compile, JSON serialization round-trips correctly for all field combinations.

---

### Task 2: Historical price fetching [PRIORITY: HIGH]

**Corresponds to:** Scenario: Multi-currency portfolio performance, Scenario: Manually refresh historical market data

**Description:** Extend the market data infrastructure to fetch and cache historical daily prices for multiple symbols over a date range. Uses go-yfinance's `multi.Download` with a shared HTTP client, `AutoAdjust: false` (unadjusted prices for accurate portfolio valuation), and `Interval: "1d"`.

- [x] Add `FetchHistoricalPricesBatch(ctx context.Context, symbols []string, start, end time.Time) (map[string][]HistoricalPrice, []string)` to `market.MarketDataFetcher` interface
  - `HistoricalPrice` struct: Date (time.Time), Close (decimal.Decimal), Currency (string)
  - Returns a map of symbol → prices (sorted by date ASC) and a slice of failed symbol names
- [x] Implement `FetchHistoricalPricesBatch` on `YahooFinanceFetcher` using `multi.NewTickers` + per-ticker `History()` with `AutoAdjust: false`
  - Convert `models.Bar.Close` (float64) to `decimal.Decimal` via `decimal.NewFromFloat64`
  - Filter bars to only include dates within [start, end]
  - Currency retrieved from `GetHistoryMetadata()` (cached by `History()` call)
- [x] Add `UpsertHistoricalPrices(ctx context.Context, symbol string, prices []HistoricalPrice) error` to `MarketDataRepository`
  - Uses existing `InsertMarketData` sqlc query (ON CONFLICT(symbol, source, date) upsert)
  - Loop over prices and upsert each; returns first error encountered
- [x] Write unit tests for HistoricalPrice struct fields and sorted-by-date behavior
- [x] Write unit tests for batch upsert into market_data repository

**Verification:** Historical prices for multiple symbols over a date range can be fetched from Yahoo in a single batch call and cached in the market_data table. `AutoAdjust: false` ensures unadjusted (actual market) prices are stored.

**Deviation from plan:** Used `multi.NewTickers` + per-ticker `History()` instead of `multi.Download` because `multi.Download` doesn't return currency information from the bars. The `History()` call on each ticker caches `ChartMeta` (including currency) via `GetHistoryMetadata()`. Same shared HTTP client is used.

---

### Task 3: Equity curve computation [PRIORITY: HIGH]

**Corresponds to:** Scenario: View portfolio equity curve (happy path), Scenario: View single portfolio performance, Scenario: View portfolio equity curve with period selector, Scenario: Portfolio with only deposits, Scenario: Portfolio with negative net deposit

**Description:** Core computation that walks transactions chronologically and produces the equity curve (portfolio market value + cumulative net deposit per date). This is the heart of the feature.

- [x] Add `ComputeEquityCurve(ctx context.Context, filters PerformanceFilters) (*PerformanceResult, error)` method on `position.Service`
- [x] Implement date-range determination: earliest transaction date → today (or date-to from filters)
- [x] Fetch all transactions for the filtered accounts within the date range, sorted by date ASC
- [x] Walk transactions chronologically, maintaining:
  - Position quantities per symbol (increment on buy, decrement on sell)
  - Running cash balance per currency (from net_cash of all cash-affecting types)
  - Cumulative net deposit (sum of deposit/withdrawal net_cash only)
- [x] Collect all unique symbols held during the period and batch-fetch historical prices via `FetchHistoricalPricesBatch` (uses `multi.Download` with shared HTTP client, `AutoAdjust: false`, `Interval: "1d"`)
  - Upsert all fetched prices into market_data table
  - Track failed symbols for warnings
- [x] At each unique transaction date, compute portfolio market value:
  - Sum of (quantity × historical price) for each open symbol position
  - Plus cash balances, converted to base currency
  - Use cached historical prices from the batch fetch
  - FX-convert foreign currency values using `FxConverter.GetRateForDate`
- [x] Build `EquityCurvePoint` for each date with PortfolioValue and NetDeposit in base currency
- [x] Implement daily interpolation: carry forward last known value for non-transaction days
- [x] Handle "all portfolios" mode: check base currency consistency, return error if mismatched
- [x] Handle empty state: no transactions → empty result with appropriate flag
- [x] Write comprehensive unit tests with hand-written mocks:
  - Happy path: deposits + buys + sells across multiple months
  - Single-currency portfolio
  - Multi-currency portfolio with FX conversion
  - Portfolio with only deposits (no buys/sells)
  - Negative net deposit (withdrawals > deposits)
  - No transactions (empty state)
  - Missing market data for a symbol (excluded from value, warning added)
  - FX rate gaps (fallback to spot rate)
  - All portfolios with matching currencies
  - All portfolios with mismatched currencies (error)
  - Period filtering (1W, 1M, 1Y, YTD, All)

**Verification:** Given a known set of transactions and prices, the equity curve matches expected portfolio values and net deposits at each date.

**Technical Decision B — Where to put performance logic:**

| Option | Pros | Cons |
|--------|------|------|
| **A: Add to `position.Service`** (chosen) | Reuses existing dependencies (FxConverter, AccountLister, TransactionRepository); co-located with related position logic | Service already large (~400 lines) |
| B: New `performance` package | Clean separation of concerns | Duplicates dependency wiring; needs its own service + interfaces |
| C: Add to `portfolio.Service` | Portfolio-level concept | Portfolio service is thin CRUD; would bloat it with analytics logic |

---

### Task 4: Return metrics computation [PRIORITY: HIGH]

**Corresponds to:** Scenario: View summary return metrics

**Description:** Compute return metrics from the equity curve data: Time-Weighted Return (TWR), Money-Weighted Return (MWR), and Simple Return (profit / net deposit), plus annualized versions.

- [x] Add `ComputePeriodReturn` function (pure function, no dependencies) — computes TWR, MWR, and simple return from equity curve and TWR breakpoints
- [x] TWR: geometrically links sub-period returns between cash flow breakpoints
  - No cash flows → degenerates to simple value return `(end / begin) - 1`
- [x] MWR: bisection-based IRR solver on cash flows derived from NetDeposit column
  - Added `MWRPct` (annualized) and `HoldingPeriodMWRPct` (period return)
- [x] Simple return: `(end_pv - end_nd) / end_nd × 100` — "for every unit deposited, how much profit was made?"
  - Added `SimpleReturnPct` and `AnnualizedSimpleReturnPct`
  - Handle zero net deposit → nil
- [x] Annualized versions use `math.Pow(1+r, 365/days) - 1` for all metrics
  - Handle < 2 data points → insufficient data flag
  - Handle zero days → nil annualized
- [x] Write table-driven unit tests:
  - Standard case: positive return over 1+ years
  - Less than 1 year (still annualized)
  - Zero net deposit (simple return nil)
  - Fewer than 2 data points (insufficient data)
  - Zero return (value == deposit)
  - Negative return (value < deposit)
  - Multiple cash flows with known TWR
  - MWR with known IRR
  - Simple return with intermediate deposits

**Verification:** Return metrics are correctly computed for all edge cases; decimal precision is maintained.

**Deviation from plan:** The plan specified `TotalReturnPct = (current_value - net_deposit) / net_deposit` and `AnnualizedReturnPct` (CAGR). During implementation, TWR was chosen as the primary metric because it isolates investment performance from the timing and magnitude of deposits/withdrawals. MWR was added post-retro to complement TWR. Simple return was added and later corrected (2026-05-14) from raw value-based return to profit-over-deposits.

**Technical Decision C — CAGR calculation with decimal.Decimal:**

| Option | Pros | Cons |
|--------|------|------|
| **A: ln/exp approximation** (chosen) | Stays in decimal domain; arbitrary precision | More complex; need to implement ln and exp for decimal |
| B: Convert to float64 for pow, back to decimal | Simple; uses math.Pow | Loses precision at extreme values |
| C: Use integer exponent only (skip fractional) | Simple | Inaccurate for non-integer year spans |

Given that CAGR is a display metric (not used for financial decisions), **Option B** (float64 conversion for the power operation) is actually simpler and sufficient. The decimal values are only used for the final formatting. I'll use `math.Pow` for the exponentiation and convert back to decimal.

---

### Task 5: Historical data refresh [PRIORITY: MEDIUM]

**Corresponds to:** Scenario: Manually refresh historical market data

**Description:** Allow the user to manually trigger a refresh of historical market data (prices + FX rates) for the visible period. Fetches current prices/FX rates, upserts them, and returns a summary.

- [x] Add `RefreshMarketData(ctx context.Context, filters PerformanceFilters) (*RefreshResult, error)` method on `position.Service`
- [x] Determine the set of symbols held during the period (from open positions + transactions)
- [x] For each symbol, fetch current quote via `FetchQuotesBatch` and upsert into market_data
- [x] For each currency pair needed, fetch current FX rate via `FxConverter.GetCurrentRate` (which already caches)
- [x] Collect failed symbols and return in RefreshResult
- [x] Write unit tests with mocks:
  - Successful refresh for multiple symbols
  - Partial failure (some symbols fail, others succeed)
  - Empty portfolio (no symbols to refresh)
  - Multi-currency with FX pair refresh
  - Open positions only (no transactions in range)
  - No market fetcher configured
  - Period filtering
  - Portfolio filtering

**Verification:** Clicking refresh fetches current data for all symbols and returns a summary of what was refreshed and what failed.

**Technical Decision D — Refresh scope:**

| Option | Pros | Cons |
|--------|------|------|
| **A: Refresh current prices only** (chosen) | Simple; reuses existing `FetchQuote`/`GetCurrentRate`; sufficient for most cases | Doesn't update historical prices for past dates |
| B: Refresh full historical range | Most accurate equity curve | Many more API calls; slower; rate limit risk |
| C: Hybrid — current + transaction dates | Balance of accuracy and speed | More complex logic |

For MVP, Option A is sufficient. The equity curve uses current prices for the most recent date anyway. Historical price accuracy can be improved in a follow-up feature.

---

### Task 6: API handler [PRIORITY: HIGH]

**Corresponds to:** All scenarios (API layer)

**Description:** REST API endpoint for retrieving performance data. Returns equity curve data points and return metrics as JSON.

- [x] Create `internal/api/handlers/performance.go`
- [x] Define `PerformanceHandler` struct with `*position.Service` and `*portfolio.Service` dependencies
- [x] Register route: `GET /api/performance` with query params: `portfolio_id`, `period`
- [x] Parse query params into `PerformanceFilters`
- [x] Call `service.ComputeEquityCurve(ctx, filters)` and return JSON response
- [x] Define response DTO: `PerformanceResponse` with EquityCurve, ReturnMetrics, BaseCurrency, Warnings
- [x] Register route: `POST /api/performance/refresh` with same query params
- [x] Call `service.RefreshMarketData(ctx, filters)` and return JSON response
- [x] Handle errors: portfolio not found, mismatched currencies, internal error
- [x] Write unit tests (mock service, verify request/response mapping)
- [x] Write integration-style tests with hand-written service mock

**Verification:** API returns correct JSON for valid requests; returns appropriate error codes for invalid inputs.

---

### Task 7: Web handler + template [PRIORITY: HIGH]

**Corresponds to:** All scenarios (web UI layer)

**Description:** Server-rendered performance page with equity curve chart (ECharts), period selector, portfolio selector, summary metrics, and refresh button.

- [x] Create `internal/api/handlers/performance_web.go`
- [x] Define `PerformanceWebHandler` with `*position.Service`, `*portfolio.Service`, `*web.Renderer`
- [x] Register routes: `GET /performance`, `POST /performance/refresh`
- [x] `HandlePerformance` renders the page:
  - Parse portfolio_id and period from query params
  - Call `ComputeEquityCurve` to get data
  - Pass data to template
- [x] `HandleRefresh` triggers refresh, sets flash message, redirects to `/performance`
- [x] Create `templates/performance/index.html`:
  - Portfolio selector dropdown (all portfolios + individual)
  - Period selector buttons (1W, 1M, 3M, 1Y, 3Y, 5Y, YTD, All)
  - ECharts line chart with two series: Portfolio Value, Net Deposit
  - Summary metrics cards: Total Return %, CAGR, Current Value, Net Deposit
  - Refresh button (form POST)
  - Empty state message (no transactions)
  - Error state (mismatched currencies)
  - Warning indicators (missing market data, FX fallback)
- [x] Pass chart data as JSON in `<script>` tag (following existing pattern)
- [x] Write unit tests for handler (mock service, verify template rendering)

**Verification:** Performance page renders correctly with chart, metrics, and controls. Period selector updates the chart. Refresh button triggers data refresh and shows flash message.

**Deviation from plan:** Pre-built URLs in handler (`RefreshURL`, `PeriodURLs`) instead of inline template expressions, because Go's html/template is strict about expressions inside href/action attributes ("ambiguous context within a URL").

---

### Task 8: Router wiring + navigation [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (integration)

**Description:** Wire the performance handlers into the router and enable the Analytics navigation link.

- [x] Update `internal/api/router.go`:
  - Create `performanceHandler` and `performanceWebHandler` instances
  - Wire dependencies (positionSvc, portfolioSvc, renderer)
  - Call `RegisterRoutes(r)` for both handlers
- [x] Update `templates/partials/nav.html`:
  - Change `<a href="#" class="disabled">Analytics</a>` to `<a href="/performance">Performance</a>`
- [x] Run `go build` and verify no compilation errors
- [x] Run `go test ./...` and verify all tests pass

**Verification:** Application builds and runs; navigation link works; performance page is accessible at `/performance`.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Historical price fetching | `multi.NewTickers` + per-ticker `History()` with `AutoAdjust: false` | Per-ticker `History()` caches `ChartMeta` (including currency) via `GetHistoryMetadata()`, which `multi.Download` doesn't return |
| Performance logic location | Methods on `position.Service` | Reuses existing deps (FxConverter, AccountLister, TransactionRepository); co-located with position analytics |
| Return metrics | TWR + MWR + simple return | TWR isolates investment performance; MWR shows actual investor experience; simple return answers "profit per unit deposited" |
| Simple return formula | `(end_pv - end_nd) / end_nd × 100` | Profit over total net deposit; always defined when deposits > 0; distinct from TWR/MWR |
| Annualization | `math.Pow` for exponentiation, convert back to decimal | Simpler; sufficient precision for display metrics |
| Refresh scope | Current prices + FX rates only | Simple MVP; historical accuracy can be improved later |
| Equity curve interpolation | Carry forward last known value | Matches spec; simple; no speculative interpolation |
| Chart library | ECharts (client-side) | Consistent with existing project; interactive tooltips |
| Period filtering | Server-side (compute full curve, filter by date range) | Single computation; client just changes query params |
| Multi-portfolio currency check | Reject if base currencies differ | Matches spec; prevents misleading aggregation |

## Risks

- **Yahoo Finance rate limits**: Fetching historical data for many symbols/dates could hit rate limits. Mitigation: batch requests via `multi.NewTickers`; cache aggressively; serve stale data on failure.
- **Decimal precision for CAGR**: Using `math.Pow` (float64) for the exponentiation step. Mitigation: CAGR is a display metric only; precision loss is negligible for typical portfolio values.
- **Large date ranges**: Computing equity curves for portfolios with 5+ years of daily data could be slow. Mitigation: compute on-demand (no pre-computation); add timeout to HTTP handler; consider caching results in a future iteration.
- **Historical price availability**: Yahoo may not have historical data for all symbols (especially obscure or delisted ones). Mitigation: exclude missing symbols from calculation; show warning to user.
