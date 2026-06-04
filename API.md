# API Reference

All API endpoints are prefixed with `/api/`. Responses are JSON. Errors return `{"error": "message", "code": "ERROR_CODE"}`.

Pagination is supported on list endpoints via `?limit=&offset=` query parameters (default: limit=50, offset=0).

## Error Response

```json
{
  "error": "portfolio not found",
  "code": "PORTFOLIO_NOT_FOUND"
}
```

Common error codes: `INVALID_REQUEST`, `INVALID_ID`, `INTERNAL_ERROR`, `*_NOT_FOUND`, `*_EXISTS`.

---

## Portfolios

### List Portfolios

```
GET /api/portfolios?limit=&offset=
```

**Response:** `200 OK` — `Portfolio[]`

### Create Portfolio

```
POST /api/portfolios
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Portfolio name (unique) |
| `base_currency` | string | yes | 3-letter ISO currency code |

**Response:** `201 Created` — `Portfolio`

### Get Portfolio

```
GET /api/portfolios/{id}
```

**Response:** `200 OK` — `Portfolio` | `404` — not found

### Update Portfolio

```
PATCH /api/portfolios/{id}
```

**Request body:** (all fields optional)

| Field | Type | Description |
|---|---|---|
| `name` | string | New name |
| `base_currency` | string | New base currency |

**Response:** `200 OK` — `Portfolio` | `404` — not found

### Delete Portfolio

```
DELETE /api/portfolios/{id}
```

**Response:** `204 No Content` | `404` — not found

---

## Accounts

### List Accounts

```
GET /api/accounts?portfolio_id=&limit=&offset=
```

**Query params:**

| Param | Type | Description |
|---|---|---|
| `portfolio_id` | int64 | Filter by portfolio |
| `limit` | int | Max results (default 50) |
| `offset` | int | Skip N results |

**Response:** `200 OK` — `Account[]`

### Create Account

```
POST /api/accounts
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Account name (unique within portfolio) |
| `portfolio_id` | int64 | yes | Parent portfolio ID |
| `base_currency` | string | yes | 3-letter ISO currency code |

**Response:** `201 Created` — `Account`

### Get Account

```
GET /api/accounts/{id}
```

**Response:** `200 OK` — `Account` | `404` — not found

### Update Account

```
PATCH /api/accounts/{id}
```

**Request body:** (all fields optional)

| Field | Type | Description |
|---|---|---|
| `name` | string | New name |
| `base_currency` | string | New base currency |

**Response:** `200 OK` — `Account` | `404` — not found

### Delete Account

```
DELETE /api/accounts/{id}
```

**Response:** `204 No Content` | `404` — not found

---

## Transactions

### List Transactions

```
GET /api/transactions?account_id=&symbol=&type=&date_from=&date_to=&limit=&offset=
```

**Query params:**

| Param | Type | Description |
|---|---|---|
| `account_id` | int64 | Filter by account |
| `symbol` | string | Filter by symbol |
| `type` | string | Filter by type (`buy`, `sell`, `deposit`, `withdrawal`, `dividend`, `fee`, `interest`) |
| `date_from` | string | Filter from date (`YYYY-MM-DD`) |
| `date_to` | string | Filter to date (`YYYY-MM-DD`) |
| `limit` | int | Max results (default 50) |
| `offset` | int | Skip N results |

**Response:** `200 OK` — `Transaction[]`

### Create Transaction

```
POST /api/transactions
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `account_id` | int64 | yes | Account ID |
| `date` | string | yes | Transaction date (`YYYY-MM-DD`) |
| `type` | string | yes | `buy`, `sell`, `deposit`, `withdrawal`, `dividend`, `fee`, `interest` |
| `symbol` | string | yes | Symbol (e.g. `AAPL`, `$CASH-USD` for cash) |
| `quantity` | number | conditional | Shares (positive for buy/deposit, negative for sell/withdrawal); omitted for cash-only |
| `price` | number | conditional | Price per share; omitted for cash-only |
| `currency` | string | yes | Transaction currency |
| `net_cash` | number | conditional | Net cash flow (positive = money in, negative = money out); auto-computed for buys/sells |
| `description` | string | no | Optional description |

**Response:** `201 Created` — `Transaction`

### Get Transaction

```
GET /api/transactions/{id}
```

**Response:** `200 OK` — `Transaction` | `404` — not found

### Update Transaction

```
PATCH /api/transactions/{id}
```

**Request body:** (all fields optional, same as create)

**Response:** `200 OK` — `Transaction` | `404` — not found

### Delete Transaction

```
DELETE /api/transactions/{id}
```

**Response:** `204 No Content` | `404` — not found

---

## Positions

### List Open Positions

```
GET /api/positions?account_id=&portfolio_id=&account_ids=&limit=&offset=
```

Returns positions enriched with current market data (price, market value, unrealized P&L).

**Query params:**

| Param | Type | Description |
|---|---|---|
| `account_id` | int64 | Filter by account |
| `portfolio_id` | int64 | Filter by portfolio |
| `account_ids` | int64[] | Filter by multiple accounts (repeat param) |
| `limit` | int | Max results (default 50) |
| `offset` | int | Skip N results |

**Response:** `200 OK` — `PositionWithMarket[]`

### Open Positions Summary

```
GET /api/positions/summary?account_id=&portfolio_id=&account_ids=&base_currency=
```

Returns aggregated totals across all open positions (not paginated).

**Query params:**

| Param | Type | Description |
|---|---|---|
| `account_id` | int64 | Filter by account |
| `portfolio_id` | int64 | Filter by portfolio |
| `account_ids` | int64[] | Filter by multiple accounts |
| `base_currency` | string | Override base currency (auto-resolves from first portfolio if omitted) |

**Error:** `400 Bad Request` — `NO_BASE_CURRENCY` if no portfolios exist to resolve currency from.

**Response:** `200 OK` — `OpenPositionSummary`

```json
{
  "total_cost_basis_base": 50000,
  "total_mkt_value_base": 55000,
  "total_unrealized_pnl_base": 5000
}
```

### List Closed Positions

```
GET /api/positions/closed?account_id=&portfolio_id=&account_ids=&limit=&offset=
```

**Query params:** Same as open positions.

**Response:** `200 OK` — `Position[]`

### Closed Positions Summary

```
GET /api/positions/closed/summary?account_id=&portfolio_id=&account_ids=&base_currency=
```

Returns aggregated totals across all closed positions (not paginated).

**Query params:**

| Param | Type | Description |
|---|---|---|
| `account_id` | int64 | Filter by account |
| `portfolio_id` | int64 | Filter by portfolio |
| `account_ids` | int64[] | Filter by multiple accounts |
| `base_currency` | string | Override base currency (auto-resolves from first portfolio if omitted) |

**Error:** `400 Bad Request` — `NO_BASE_CURRENCY` if no portfolios exist to resolve currency from.

**Response:** `200 OK` — `ClosedPositionSummary`

```json
{
  "total_realized_pnl_base": 3500
}
```

### Get Lot Details

```
GET /api/lots/{lot_id}
```

**Response:** `200 OK` — `LotDetails` | `404` — not found

### Recalculate Positions

```
POST /api/positions/recalculate?account_id=&portfolio_id=
```

Recalculates positions from scratch. If neither param is provided, recalculates all accounts.

**Query params:**

| Param | Type | Description |
|---|---|---|
| `account_id` | int64 | Recalculate specific account |
| `portfolio_id` | int64 | Recalculate all accounts in portfolio |

**Response:** `200 OK` — `{"status": "recalculated", "account_id/portfolio_id/scope": "..."}`

---

## Performance

### Get Performance

```
GET /api/performance?portfolio_id=&period=&date_from=&date_to=&benchmark=&mode=&fields=
```

Returns portfolio performance analytics. Computation is done once in the service layer; use `fields` to request subsets.

**Query params:**

| Param | Type | Description |
|---|---|---|
| `portfolio_id` | int64 | Filter by portfolio (omit = all portfolios) |
| `period` | string | Preset period: `1W`, `1M`, `3M`, `1Y`, `3Y`, `5Y`, `YTD`, `All` (default) |
| `date_from` | string | Custom start date (`YYYY-MM-DD`) |
| `date_to` | string | Custom end date (`YYYY-MM-DD`) |
| `benchmark` | string | Benchmark ticker (e.g. `^GSPC`, `^IXIC`, `SPY`) |
| `mode` | string | `equity` (default, total return) or `nav` (NAV per unit) |
| `fields` | string | Comma-separated field groups: `equity_curve`, `metrics`, `risk`, `drawdown`, `yearly`, `monthly`, `benchmark`, `nav`. Omit = all. |

**Response:** `200 OK` — `PerformanceResult`

**Field groups:**

| Group | Fields included |
|---|---|
| `equity_curve` | `equity_curve` |
| `metrics` | `return_metrics`, `base_currency`, `warnings` |
| `risk` | `risk_metrics` |
| `drawdown` | `drawdown_analysis` |
| `yearly` | `yearly_returns` |
| `monthly` | `monthly_returns` |
| `benchmark` | `benchmark_ticker`, `benchmark_prices`, `benchmark_mwr_pct`, `benchmark_currency`, `benchmark_warning` |
| `nav` | `nav_summary` |

### Refresh Performance Data

```
POST /api/performance/refresh?portfolio_id=&period=
```

Refreshes current market data (prices + FX rates) for the visible period.

**Query params:**

| Param | Type | Description |
|---|---|---|
| `portfolio_id` | int64 | Filter by portfolio |
| `period` | string | Preset period |

**Response:** `200 OK` — `RefreshResult`

---

## Market Data

### Refresh Market Data

```
POST /api/market-data/refresh
```

Triggers a full background refresh of all cached market data. Returns immediately.

**Response:** `202 Accepted` — `{"message": "market data refresh started"}`

### Get Cache Status

```
GET /api/market-data/status
```

**Response:** `200 OK` — `CacheStatus`

---

## Symbol Mappings

### List Symbol Mappings

```
GET /api/symbol-mappings?limit=&offset=
```

**Response:** `200 OK` — `SymbolMapping[]`

### Create Symbol Mapping

```
POST /api/symbol-mappings
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `internal_symbol` | string | yes | Internal symbol (e.g. `AAPL`) |
| `market_data_symbol` | string | yes | Market data symbol (e.g. `AAPL.US`) |
| `is_benchmark` | bool | no | Mark as benchmark (default `false`) |
| `data_source_url` | string | no | Provider source URL (e.g. WisdomTree ETF page). NULL/empty = Yahoo Finance (default) |

**Response:** `201 Created` — `SymbolMapping`

### Get Symbol Mapping

```
GET /api/symbol-mappings/{id}
```

**Response:** `200 OK` — `SymbolMapping` | `404` — not found

### Update Symbol Mapping

```
PATCH /api/symbol-mappings/{id}
```

**Request body:** (all fields optional)

| Field | Type | Description |
|---|---|---|
| `internal_symbol` | string | New internal symbol |
| `market_data_symbol` | string | New market data symbol |
| `is_benchmark` | bool | Benchmark flag |
| `data_source_url` | string | Provider source URL. Empty string = clear (revert to Yahoo) |

**Response:** `200 OK` — `SymbolMapping` | `404` — not found

### Delete Symbol Mapping

```
DELETE /api/symbol-mappings/{id}
```

**Response:** `204 No Content` | `404` — not found | `409` — in use

### Add Broker Symbol

```
POST /api/symbol-mappings/{id}/broker-symbols
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `broker_symbol` | string | yes | Broker's symbol representation |

**Response:** `204 No Content` | `409` — already exists

### Preview Symbol

```
GET /api/symbol-mappings/preview?symbol=
```

Fetch a market data quote for a symbol, with auto-correction detection.

**Query params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `symbol` | string | yes | Symbol to preview |

**Response:** `200 OK` — `PreviewResponse`

---

## Imports

### IBKR Import

#### Preview IBKR Import

```
POST /api/transactions/import/ibkr/preview
```

**Request:** `multipart/form-data`

| Field | Type | Required | Description |
|---|---|---|---|
| `xml_file` | file | yes | IBKR Flex XML statement |
| `account_id` | string | yes | Target account ID |

**Response:** `200 OK` — `PreviewResponse`

#### Confirm IBKR Import

```
POST /api/transactions/import/ibkr/confirm
```

Same request as preview. Actually persists the transactions.

**Response:** `200 OK` — `ImportResult`

#### Create Symbol (IBKR)

```
POST /api/transactions/import/ibkr/symbols
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `internal_symbol` | string | yes | Internal symbol |
| `market_data_symbol` | string | yes | Market data symbol |

**Response:** `201 Created` — `SymbolMapping`

#### Add Broker Symbol (IBKR)

```
POST /api/transactions/import/ibkr/broker-symbols
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `broker_name` | string | yes | Broker name (e.g. `ibkr`) |
| `broker_symbol` | string | yes | Broker's symbol |
| `internal_symbol` | string | yes | Internal symbol |

**Response:** `200 OK` — `{"status": "ok"}`

---

### Trading 212 Import

#### Preview Trading 212 Import

```
POST /api/transactions/import/trading212/preview
```

**Request:** `multipart/form-data`

| Field | Type | Required | Description |
|---|---|---|---|
| `csv_file` | file | yes | Trading 212 CSV statement |
| `account_id` | string | yes | Target account ID |

**Response:** `200 OK` — `PreviewResponse`

#### Confirm Trading 212 Import

```
POST /api/transactions/import/trading212/confirm
```

Same request as preview. Actually persists the transactions.

**Response:** `200 OK` — `ImportResult`

#### Create Symbol (Trading 212)

```
POST /api/transactions/import/trading212/symbols
```

Same as IBKR create symbol.

**Response:** `201 Created` — `SymbolMapping`

#### Add Broker Symbol (Trading 212)

```
POST /api/transactions/import/trading212/broker-symbols
```

Same as IBKR add broker symbol.

**Response:** `200 OK` — `{"status": "ok"}`

---

## Allocation

Shows how portfolio capital is distributed across symbols, with support for target allocation, drift tracking, and rebalancing suggestions.

### Get Current Allocation

```
GET /api/allocation?portfolio_ids=
```

Returns the current allocation breakdown for the selected portfolio(s). Cash is aggregated to the portfolio base currency using current FX rates.

**Query params:**

| Param | Type | Description |
|---|---|---|
| `portfolio_ids` | string | Comma-separated portfolio IDs (e.g. `1,2`). Omit = all portfolios. |

**Response:** `200 OK` — `AllocationResult`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `ZERO_TOTAL_VALUE` | 400 | Portfolio has zero or negative total value |
| `MIXED_CURRENCIES` | 400 | Selected portfolios have conflicting base currencies |

### Get Target Allocation

```
GET /api/allocation/target?portfolio_id=
```

Returns saved target allocations for a portfolio.

**Query params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `portfolio_id` | int64 | yes | Portfolio ID |

**Response:** `200 OK` — `TargetAllocation[]`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `MISSING_PORTFOLIO_ID` | 400 | `portfolio_id` is required |

### Save Target Allocation

```
POST /api/allocation/target?portfolio_id=
```

Saves or updates target allocation weights for a portfolio. Percentages must each be in [0, 100] and sum to exactly 100.

**Query params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `portfolio_id` | int64 | yes | Portfolio ID |

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `symbol` | string | yes | Symbol (e.g. `AAPL`, `CASH`) |
| `target_pct` | number | yes | Target weight percentage (0–100) |

**Response:** `200 OK` — `{"status": "saved"}`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `MISSING_PORTFOLIO_ID` | 400 | `portfolio_id` is required |
| `INVALID_REQUEST` | 400 | Malformed request body |
| `INVALID_TARGET_PCT` | 400 | A percentage is outside [0, 100] |
| `TARGET_SUM_NOT_100` | 400 | Percentages do not sum to 100 (includes delta in message) |
| `DUPLICATE_SYMBOL` | 400 | Symbol appears more than once |

### Delete Target Allocation

```
DELETE /api/allocation/target?portfolio_id=&symbol=
```

Deletes target allocation(s) for a portfolio. If `symbol` is omitted, deletes all targets for the portfolio.

**Query params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `portfolio_id` | int64 | yes | Portfolio ID |
| `symbol` | string | no | Symbol to delete (omit = delete all) |

**Response:** `200 OK` — `{"status": "deleted", "scope": "AAPL"}` or `{"status": "deleted", "scope": "all"}`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `MISSING_PORTFOLIO_ID` | 400 | `portfolio_id` is required |

### Get Drift Comparison

```
GET /api/allocation/drift?portfolio_id=
```

Returns drift comparison between actual and target allocation. Symbols with |drift| ≤ 5% are marked as balanced.

**Query params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `portfolio_id` | int64 | yes | Portfolio ID |

**Response:** `200 OK` — `DriftResult`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `MISSING_PORTFOLIO_ID` | 400 | `portfolio_id` is required |
| `ZERO_TOTAL_VALUE` | 400 | Portfolio has zero or negative total value |

### Get Rebalancing Suggestions

```
GET /api/allocation/rebalance?portfolio_id=
```

Returns suggested trades to close the gap between actual and target allocation. Symbols within 5% drift tolerance are excluded. Suggestions are ordered by drift magnitude (largest first).

**Query params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `portfolio_id` | int64 | yes | Portfolio ID |

**Response:** `200 OK` — `RebalanceResult`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `MISSING_PORTFOLIO_ID` | 400 | `portfolio_id` is required |
| `ZERO_TOTAL_VALUE` | 400 | Portfolio has zero or negative total value |

---

## Model Portfolios

Named allocation blueprints (symbol + weight %) that can be created, managed, and applied as target allocations on real portfolios.

### List Model Portfolios

```
GET /api/model-portfolios?limit=&offset=
```

**Response:** `200 OK` — `ModelPortfolio[]`

### Create Model Portfolio

```
POST /api/model-portfolios
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Portfolio name (unique, 1–100 chars) |
| `entries` | array | yes | Array of `{symbol, weight_pct}` entries |

**Entries fields:**

| Field | Type | Required | Description |
|---|---|---|---|
| `symbol` | string | yes | Symbol (e.g. `AAPL`) |
| `weight_pct` | number | yes | Weight percentage (> 0, all must sum to 100) |

**Response:** `201 Created` — `ModelPortfolio`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `INVALID_NAME` | 400 | Name is empty or exceeds 100 characters |
| `MODEL_PORTFOLIO_NAME_EXISTS` | 409 | A model portfolio with this name already exists |
| `EMPTY_ENTRIES` | 400 | At least one entry is required |
| `INVALID_WEIGHT` | 400 | Each weight must be greater than 0% |
| `DUPLICATE_SYMBOL` | 400 | Entries contain duplicate symbols |
| `weight_sum_not_100` | 400 | Weights must sum to exactly 100% (includes delta in message) |

### Get Model Portfolio

```
GET /api/model-portfolios/{id}
```

**Response:** `200 OK` — `ModelPortfolio` | `404` — not found

### Update Model Portfolio

```
PATCH /api/model-portfolios/{id}
```

**Request body:** (all fields optional except `entries`)

| Field | Type | Description |
|---|---|---|
| `name` | string | New name |
| `entries` | array | Updated entries array |

**Response:** `200 OK` — `ModelPortfolio` | `404` — not found

### Delete Model Portfolio

```
DELETE /api/model-portfolios/{id}
```

**Response:** `204 No Content` | `404` — not found

---

## Efficient Frontier

Portfolio optimization via the efficient frontier method. Select candidate symbols, configure optimization parameters, and compute the efficient frontier — the set of allocations maximizing expected return for a given level of risk.

### Compute Efficient Frontier

```
POST /api/efficient-frontier/compute
```

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `symbols` | string[] | yes | Candidate symbols (2–10) |
| `period` | string | no | Lookback period: `1Y`, `3Y`, `5Y` (default: `1Y`) |
| `risk_free_rate` | number | no | Annualized risk-free rate as decimal (default: `0.045`) |

**Response:** `200 OK` — `ComputeFrontierResponse`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `INVALID_REQUEST` | 400 | Malformed request body |
| `INSUFFICIENT_SYMBOLS` | 400 | Fewer than 2 symbols provided |
| `TOO_MANY_SYMBOLS` | 400 | More than 10 symbols provided |
| `INVALID_PERIOD` | 400 | Period not one of `1Y`, `3Y`, `5Y` |
| `INSUFFICIENT_DATA` | 400 | Insufficient price data for computation |
| `SINGULAR_MATRIX` | 400 | Covariance matrix is singular |
| `NUMERICAL_FAILURE` | 400 | Optimization failed numerically |
| `INTERNAL_ERROR` | 500 | Unexpected computation error |

### Get Candidate Symbols

```
GET /api/efficient-frontier/symbols
```

Returns all known internal symbols for autocomplete.

**Response:** `200 OK` — `{"symbols": ["AAPL", "MSFT", ...]}`

### Get Portfolio Symbols

```
GET /api/efficient-frontier/portfolio/{id}/symbols
```

Returns the distinct symbols held in a real portfolio.

**Response:** `200 OK` — `{"symbols": ["AAPL", "MSFT", ...]}`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `INVALID_ID` | 400 | Invalid portfolio ID |
| `INTERNAL_ERROR` | 500 | Failed to retrieve symbols |

### Get Model Portfolio Symbols

```
GET /api/efficient-frontier/model-portfolio/{id}/symbols
```

Returns the symbols in a model portfolio.

**Response:** `200 OK` — `{"symbols": ["AAPL", "BND", ...]}`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `INVALID_ID` | 400 | Invalid model portfolio ID |
| `INTERNAL_ERROR` | 500 | Failed to retrieve symbols |

### Save as Model Portfolio

```
POST /api/efficient-frontier/save
```

Saves an optimized allocation from the frontier as a model portfolio.

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Model portfolio name |
| `entries` | array | yes | Array of `{symbol, weight}` entries |

**Entries fields:**

| Field | Type | Required | Description |
|---|---|---|---|
| `symbol` | string | yes | Symbol |
| `weight` | number | yes | Weight as fraction (0.0–1.0) |

**Response:** `201 Created` — `ModelPortfolio`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `NOT_CONFIGURED` | 500 | Model portfolio creator not configured |
| `INVALID_REQUEST` | 400 | Malformed request body |
| `INVALID_NAME` | 400 | Name is empty |
| `EMPTY_ENTRIES` | 400 | No entries provided |
| `NAME_EXISTS` | 409 | Name already exists |
| `WEIGHT_SUM_NOT_100` | 400 | Weights don't sum to 100% |
| `INVALID_WEIGHT` | 400 | Invalid weight value |
| `DUPLICATE_SYMBOL` | 400 | Duplicate symbol in entries |

### ComputeFrontierResponse

```json
{
  "result": {
    "frontier_points": [
      {"return_pct": 8.0, "volatility_pct": 10.0, "sharpe_ratio": 0.5, "weights": [0.6, 0.4]}
    ],
    "max_sharpe": {"name": "Max Sharpe", "return_pct": 8.0, "volatility_pct": 10.0, "sharpe_ratio": 0.5, "weights": [0.6, 0.4]},
    "min_variance": {"name": "Min Variance", "return_pct": 5.0, "volatility_pct": 5.0, "sharpe_ratio": 0.3, "weights": [0.3, 0.7]},
    "highest_return": {"name": "Highest Return", "return_pct": 12.0, "volatility_pct": 15.0, "sharpe_ratio": 0.6, "weights": [0.8, 0.1, 0.1]},
    "symbols": ["SPY", "EFA", "BND"],
    "trading_days": 252,
    "computed_at": "2024-01-15T12:00:00Z"
  },
  "warnings": ["Symbol X: limited data available"],
  "excluded_symbols": ["DELETED"]
}
```

---

## Type Reference

### Portfolio

```json
{
  "id": 1,
  "name": "Personal",
  "base_currency": "USD",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

### Account

```json
{
  "id": 1,
  "name": "Brokerage",
  "portfolio_id": 1,
  "base_currency": "USD",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

### Transaction

```json
{
  "id": 1,
  "account_id": 1,
  "date": "2024-01-15T00:00:00Z",
  "type": "buy",
  "symbol": "AAPL",
  "quantity": 10.00,
  "price": 150.00,
  "currency": "USD",
  "net_cash": -1500.00,
  "description": null,
  "lot_id": null,
  "external_system": null,
  "external_reference": null,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

> **Sign convention:** Quantity is positive for buys/deposits/dividends, negative for sells/withdrawals. Net cash is positive for money in (deposits, sells), negative for money out (buys, withdrawals).

### PerformanceResult

```json
{
  "equity_curve": [{"date": "2024-01-15T00:00:00Z", "portfolio_value": 1000000, "net_deposit": 1000000}, ...],
  "return_metrics": {
    "profit_loss": 255000,
    "twr_pct": 24.80,
    "annualized_twr_pct": 12.75,
    "mwr_pct": 23.90,
    "holding_period_mwr_pct": 23.90,
    "simple_return_pct": 25.50,
    "annualized_simple_return_pct": 12.75,
    "has_insufficient_data": false
  },
  "base_currency": "USD",
  "warnings": [],
  "risk_metrics": {
    "annualized_volatility_pct": 15.20,
    "sharpe_ratio": 0.84,
    "sortino_ratio": 1.23
  },
  "drawdown_analysis": {
    "max_drawdown_pct": 12.50,
    "current_drawdown_pct": 2.30,
    "drawdown_duration_days": 45
  },
  "yearly_returns": [
    {"year": 2024, "return_pct": 25.50}
  ],
  "monthly_returns": [
    {
      "year": 2024,
      "months": {
        "1": {"return_pct": 2.50, "benchmark_return_pct": 1.80, "diff_pct": 0.70},
        "2": {"return_pct": -1.20, "benchmark_return_pct": -0.80, "diff_pct": -0.40}
      }
    }
  ],
  "benchmark_ticker": "^GSPC",
  "benchmark_mwr_pct": 18.50,
  "benchmark_currency": "USD",
  "nav_summary": {
    "nav_per_unit": 1.255,
    "total_units": 10000,
    "total_value": 12550,
    "inception_date": "2024-01-15T00:00:00Z"
  }
}
```

### SymbolMapping

```json
{
  "id": 1,
  "internal_symbol": "AAPL",
  "market_data_symbol": "AAPL",
  "is_benchmark": false,
  "data_source_url": "",
  "broker_symbols": [],
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

> `data_source_url` — optional provider source URL (e.g. WisdomTree ETF page). When set, symbol details fetching is routed to the matching extractor instead of Yahoo Finance. Empty/NULL = Yahoo Finance.

### SymbolDetailsResponse

Embedded in `SymbolGetResponse.symbol_details`. Includes standard Yahoo fields (holdings, sectors, geographic allocations, fund profile, equity valuation) plus extractor-specific fields:

```json
{
  "internal_symbol": "WMGT",
  "short_name": "WisdomTree Mid Cap Growth",
  "long_name": "WisdomTree Mid Cap Growth UCITS ETF",
  "exchange": "ILS",
  "currency": "GBP",
  "quote_type": "ETF",
  "top_holdings": [...],
  "sector_weightings": [...],
  "geographic_allocations": [...],
  "fund_profile": {...},
  "equity_valuation": {...},
  "bond_characteristics": {...},
  "market_cap_breakdown": {
    "total": 100.0,
    "large": 15.2,
    "mid": 68.5,
    "small": 16.3
  },
  "theme_breakdown": [
    {"name": "Technology", "percent": 42.5},
    {"name": "Consumer Discretionary", "percent": 28.3}
  ],
  "extractor_as_of_date": "2026-05-22T00:00:00Z",
  "fetched_at": "2026-05-26T12:00:00Z"
}
```

> `market_cap_breakdown` — total market cap + large/mid/small cap split (extractor data only).  
> `theme_breakdown` — theme allocation percentages (extractor data only).  
> `equity_valuation` — equity-specific fund characteristics (P/E, P/B, market cap, ROE, etc.). Present for equity funds; omitted for bond funds.  
> `bond_characteristics` — bond-specific fund characteristics (average coupon, maturity, quality, duration). Present for bond funds; omitted for equity funds.  
> `extractor_as_of_date` — provider's reference date (not the fetch date). Present only when data comes from an extractor; omitted for Yahoo-sourced data.

### PreviewResponse (Symbol Mapping)

```json
{
  "symbol": "AAPL",
  "name": "",
  "exchange": "",
  "currency": "USD",
  "latest_price": 150.00,
  "corrected_symbol": ""
}
```

> `corrected_symbol` is populated when the market data provider returns data for a different symbol than requested (auto-correction).

### AllocationResult

```json
{
  "rows": [
    {
      "symbol": "AAPL",
      "market_value": 60000.00,
      "market_value_base": 60000.00,
      "allocation_pct": 60.0,
      "currency": "USD",
      "has_market_data": true,
      "account_breakdown": [
        {
          "account_id": 1,
          "account_name": "Brokerage",
          "quantity": 100.00,
          "market_value": 60000.00,
          "market_value_base": 60000.00,
          "pct_of_symbol": 100.0
        }
      ]
    }
  ],
  "total_value_base": 100000.00,
  "base_currency": "USD",
  "cash_row": null,
  "last_updated": "2024-01-15T12:00:00Z",
  "market_data_available": true,
  "warnings": []
}
```

### TargetAllocation

```json
{
  "portfolio_id": 1,
  "symbol": "AAPL",
  "target_pct": 60.0
}
```

### DriftResult

```json
{
  "rows": [
    {
      "symbol": "AAPL",
      "actual_pct": 65.0,
      "target_pct": 60.0,
      "drift_pct": 5.0,
      "is_balanced": false
    }
  ],
  "base_currency": "USD",
  "has_target": true,
  "warnings": []
}
```

### RebalanceResult

```json
{
  "suggestions": [
    {
      "symbol": "AAPL",
      "direction": "sell",
      "shares": 5.00,
      "dollar_value": 750.00,
      "drift_reduction": 3.2
    }
  ],
  "base_currency": "USD",
  "total_dollar_value": 750.00,
  "warnings": [],
  "is_balanced": false
}
```

### ModelPortfolio

```json
{
  "id": 1,
  "name": "60/40 Portfolio",
  "entries": [
    {"symbol": "AAPL", "weight_pct": "60.00"},
    {"symbol": "BND", "weight_pct": "40.00"}
  ],
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

### Get Portfolio Comparison

```
GET /api/comparison
```

Compare any two portfolios (model vs model, model vs real, or real vs real) on performance, risk, drawdown, distribution, overlap, and correlation metrics. Model portfolios use a buy-and-hold simulation starting from `starting_value`. Real portfolios use actual transaction history.

**Query params:**

| Param | Type | Required | Description |
|---|---|---|---|
| `portfolio_a_id` | int64 | yes | Portfolio A ID (model or real) |
| `portfolio_a_type` | string | yes | Portfolio A type: `model` or `real` |
| `portfolio_b_id` | int64 | yes | Portfolio B ID (model or real) |
| `portfolio_b_type` | string | yes | Portfolio B type: `model` or `real` |
| `period` | string | no | Fixed period: `1W`, `1M`, `3M`, `1Y`, `3Y`, `5Y`, `YTD`, `All` |
| `date_from` | string | no | Custom start date (YYYY-MM-DD). Overrides `period`. |
| `date_to` | string | no | Custom end date (YYYY-MM-DD). Overrides `period`. |
| `base_currency` | string | no | Base currency for all values (e.g. `USD`, `EUR`). |
| `starting_value` | string | no | Initial investment for model portfolio simulation. Default: `10000`. |

**Response:** `200 OK` — `ComparisonResult`

**Errors:**

| Code | Status | Description |
|---|---|---|
| `MISSING_PORTFOLIO_A_ID` | 400 | `portfolio_a_id` is required |
| `INVALID_PORTFOLIO_A_ID` | 400 | `portfolio_a_id` is not a valid integer |
| `MISSING_PORTFOLIO_A_TYPE` | 400 | `portfolio_a_type` is required |
| `INVALID_PORTFOLIO_A_TYPE` | 400 | `portfolio_a_type` must be `model` or `real` |
| `MISSING_PORTFOLIO_B_ID` | 400 | `portfolio_b_id` is required |
| `INVALID_PORTFOLIO_B_ID` | 400 | `portfolio_b_id` is not a valid integer |
| `MISSING_PORTFOLIO_B_TYPE` | 400 | `portfolio_b_type` is required |
| `INVALID_PORTFOLIO_B_TYPE` | 400 | `portfolio_b_type` must be `model` or `real` |
| `INVALID_PERIOD` | 400 | `period` is not a valid fixed period |
| `INVALID_DATE_FROM` | 400 | `date_from` is not a valid date (YYYY-MM-DD) |
| `INVALID_DATE_TO` | 400 | `date_to` is not a valid date (YYYY-MM-DD) |
| `INVALID_STARTING_VALUE` | 400 | `starting_value` is not a valid number or is ≤ 0 |
| `INTERNAL_ERROR` | 500 | Comparison computation failed |

### ComparisonResult

```json
{
  "computed_at": "2024-01-15T12:00:00Z",
  "portfolio_a": {
    "id": 1,
    "name": "Growth Portfolio",
    "type": "model",
    "return_metrics": {
      "twr_pct": 15.50,
      "annualized_twr_pct": 7.75,
      "simple_return_pct": 15.00,
      "annualized_simple_pct": 7.50,
      "cagr_pct": 7.50,
      "days_elapsed": 365,
      "has_insufficient_data": false
    },
    "risk_metrics": {
      "annualized_volatility_pct": 12.30,
      "sharpe_ratio": 0.65,
      "sortino_ratio": 0.92
    },
    "drawdown": {
      "max_drawdown_pct": -8.50,
      "current_drawdown_pct": -2.30,
      "drawdown_duration_days": 45
    },
    "drawdown_series": [
      {"date": "2024-01-02", "pct": 0.00},
      {"date": "2024-01-03", "pct": 1.50}
    ],
    "yearly_returns": [
      {"year": 2024, "return_pct": 15.50}
    ],
    "period_extremes": {
      "best_month": 8.50,
      "best_month_label": "2024-03",
      "worst_month": -6.20,
      "worst_month_label": "2024-09",
      "best_year": 15.50,
      "best_year_label": "2024",
      "worst_year": -3.20,
      "worst_year_label": "2023",
      "win_rate_pct": 66.67
    },
    "return_distribution": {
      "annual": [
        {"label": "2024: +15.50%", "count": 1}
      ],
      "annual_binned": [
        {"label": "10% to 15%", "count": 1},
        {"label": "15% to 20%", "count": 1}
      ],
      "monthly": [
        {"label": "-5% to 0%", "count": 3},
        {"label": "0% to 5%", "count": 5},
        {"label": "5% to 10%", "count": 4}
      ]
    },
    "intra_correlation": {
      "symbols": ["AAPL", "GOOG", "MSFT"],
      "matrix": [
        [1.000, 0.850, 0.720],
        [0.850, 1.000, 0.910],
        [0.720, 0.910, 1.000]
      ]
    },
    "warnings": []
  },
  "portfolio_b": {
    "id": 2,
    "name": "Value Portfolio",
    "type": "real",
    "return_metrics": {
      "twr_pct": 12.00,
      "simple_return_pct": 11.50,
      "cagr_pct": 6.00,
      "days_elapsed": 365,
      "has_insufficient_data": false
    },
    "risk_metrics": {
      "annualized_volatility_pct": 10.50,
      "sharpe_ratio": 0.78,
      "sortino_ratio": 1.10
    },
    "drawdown": {
      "max_drawdown_pct": -6.00,
      "current_drawdown_pct": -1.50,
      "drawdown_duration_days": 30
    },
    "drawdown_series": [
      {"date": "2024-01-02", "pct": 0.00},
      {"date": "2024-01-03", "pct": 0.80}
    ],
    "intra_correlation": {
      "symbols": ["JNJ", "PG", "KO"],
      "matrix": [
        [1.000, 0.620, 0.550],
        [0.620, 1.000, 0.780],
        [0.550, 0.780, 1.000]
      ]
    }
  },
  "cross_metrics": {
    "beta_alpha": {
      "beta": 1.05,
      "alpha": 2.50,
      "overlap_days": 252
    },
    "correlation": {
      "correlation": 0.87,
      "overlap_days": 252
    },
    "overlap": {
      "top_holdings_a": [
        {"symbol": "AAPL", "weight": 0.30, "name": "Apple Inc."},
        {"symbol": "GOOG", "weight": 0.20}
      ],
      "top_holdings_b": [
        {"symbol": "AAPL", "weight": 0.25, "name": "Apple Inc."},
        {"symbol": "MSFT", "weight": 0.20}
      ],
      "overlap_pct": 45.00,
      "sector_allocation_a": {
        "breakdown": {"Technology": 0.58, "Healthcare": 0.15, "Financials": 0.12},
        "unknown_weight_pct": 0.05,
        "warnings": [],
        "missing_symbols": []
      },
      "sector_allocation_b": {
        "breakdown": {"Technology": 0.45, "Healthcare": 0.20, "Financials": 0.18},
        "unknown_weight_pct": 0.02,
        "warnings": [],
        "missing_symbols": []
      },
      "country_allocation_a": {
        "breakdown": {"United States": 0.72, "China": 0.08, "Japan": 0.05},
        "unknown_weight_pct": 0.03,
        "warnings": [],
        "missing_symbols": []
      },
      "country_allocation_b": {
        "breakdown": {"United States": 0.65, "China": 0.12, "Japan": 0.07},
        "unknown_weight_pct": 0.01,
        "warnings": [],
        "missing_symbols": []
      },
      "merged_holdings": [
        {"symbol": "AAPL", "name": "Apple Inc.", "weight_a": 0.30, "weight_b": 0.25, "overlap_pct": 25.00},
        {"symbol": "MSFT", "name": "Microsoft Corp.", "weight_a": 0.20, "weight_b": 0.00, "overlap_pct": 0.00}
      ],
      "overweight_holdings": [
        {"symbol": "AAPL", "name": "Apple Inc.", "weight_a": 0.30, "weight_b": 0.25, "difference": 5.00}
      ],
      "underweight_holdings": [
        {"symbol": "GOOG", "name": "Alphabet Inc.", "weight_a": 0.10, "weight_b": 0.20, "difference": -10.00}
      ],
      "neutral_holdings": [],
      "warnings": []
    }
  },
  "warnings": ["symbol data clipped for GOOG"]
}
```

---

## Architecture Notes

- **API-first**: The API is the single source of truth for all data computation. The web UI delegates to the same service methods.
- **Fields filtering**: The `/api/performance` endpoint supports a `fields` query parameter to request subsets of data, reducing payload for clients that don't need everything.
- **No authentication**: Access control is assumed to be handled by a reverse proxy.
- **Decimal precision**: All monetary values use string-encoded decimals (e.g., `"150.00"`) to preserve precision.
