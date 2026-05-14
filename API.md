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
GET /api/positions/summary?account_id=&portfolio_id=&account_ids=
```

Returns aggregated totals across all open positions (not paginated).

**Query params:** Same filters as list open positions.

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
GET /api/positions/closed/summary?account_id=&portfolio_id=&account_ids=
```

Returns aggregated totals across all closed positions (not paginated).

**Query params:** Same filters as list closed positions.

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

---

## Architecture Notes

- **API-first**: The API is the single source of truth for all data computation. The web UI delegates to the same service methods.
- **Fields filtering**: The `/api/performance` endpoint supports a `fields` query parameter to request subsets of data, reducing payload for clients that don't need everything.
- **No authentication**: Access control is assumed to be handled by a reverse proxy.
- **Decimal precision**: All monetary values use string-encoded decimals (e.g., `"150.00"`) to preserve precision.
