# Yahoo Finance ETF Holdings — Research Findings

## Overview

This document captures research findings on fetching ETF holdings data from Yahoo Finance, including API endpoints, authentication, data structure, and library limitations.

## API Endpoint

Yahoo's **quoteSummary** API supports a `topHoldings` module that returns ETF composition data:

```
GET https://query2.finance.yahoo.com/v10/finance/quoteSummary/{symbol}?modules=topHoldings&corsDomain=finance.yahoo.com&formatted=false&crumb={crumb}
```

The module can be combined with others: `modules=topHoldings,fundProfile,assetProfile`.

## Authentication

The same crumb/cookie flow used for existing market data fetching:

1. **Get cookie**: `GET https://fc.yahoo.com` → extract `Set-Cookie` header
2. **Get crumb**: `GET https://query2.finance.yahoo.com/v1/test/getcrumb` (with cookie) → body is the crumb string
3. **Make request**: Include cookie in `Cookie` header and crumb as query param

The existing `go-yfinance` library handles this internally via its `AuthManager`. The crumb is valid for ~1 hour.

### Alternative: CSRF Consent Flow

For EU/regulated regions, Yahoo may require the consent flow:

1. `GET https://guce.yahoo.com/consent` → extract CSRF token + session ID
2. `POST https://consent.yahoo.com/v2/collectConsent?sessionId=...` → submit consent
3. `GET https://guce.yahoo.com/copyConsent?sessionId=...` → copy consent
4. `GET https://query2.finance.yahoo.com/v1/test/getcrumb` → get crumb

The `go-yfinance` library falls back to this automatically if the basic flow fails.

## Response Structure

### Top Holdings

```json
{
  "quoteSummary": {
    "result": [{
      "topHoldings": {
        "holdings": [
          {
            "symbol": "BE",
            "holdingName": "Bloom Energy Corp Class A",
            "holdingPercent": 0.013997001
          }
        ],
        "stockPosition": 0.9929,
        "bondPosition": 0,
        "cashPosition": 0.006,
        "convertiblePosition": 0,
        "preferredPosition": 0,
        "otherPosition": 0.001,
        "bondHoldings": {},
        "bondRatings": [{"us_government": 0}],
        "equityHoldings": {
          "priceToEarnings": 0.03516,
          "priceToBook": 0.27355,
          "priceToCashflow": 0.06053,
          "priceToSales": 0.40772
        },
        "sectorWeightings": [
          {"realestate": 0.059699997},
          {"consumer_cyclical": 0.036},
          {"basic_materials": 0.1148},
          {"consumer_defensive": 0.0042},
          {"technology": 0.21209998},
          {"communication_services": 0.0318},
          {"financial_services": 0.0241},
          {"utilities": 0.0337},
          {"industrials": 0.3637},
          {"energy": 0.0446},
          {"healthcare": 0.0754}
        ],
        "maxAge": 1
      }
    }]
  }
}
```

### Field Descriptions

| Field | Type | Description |
|---|---|---|
| `holdings` | array | Top 10 individual holdings |
| `holdings[].symbol` | string | Ticker symbol (may be international, e.g., `ABBN.SW`, `7011.T`) |
| `holdings[].holdingName` | string | Full company name |
| `holdings[].holdingPercent` | float | Allocation as decimal (0.01399 = 1.399%) |
| `stockPosition` | float | Total equity allocation as decimal |
| `bondPosition` | float | Total bond allocation as decimal |
| `cashPosition` | float | Cash allocation as decimal |
| `convertiblePosition` | float | Convertible securities allocation |
| `preferredPosition` | float | Preferred stock allocation |
| `otherPosition` | float | Other assets allocation |
| `equityHoldings` | object | Aggregate valuation ratios of equity holdings |
| `equityHoldings.priceToEarnings` | float | Blended P/E of holdings |
| `equityHoldings.priceToBook` | float | Blended P/B of holdings |
| `equityHoldings.priceToCashflow` | float | Blended P/CF of holdings |
| `equityHoldings.priceToSales` | float | Blended P/S of holdings |
| `sectorWeightings` | array | Sector allocation (up to 11 sectors) |
| `bondHoldings` | object | Bond-specific details (maturity, duration — often empty for equity ETFs) |
| `bondRatings` | array | Credit rating breakdown (e.g., AAA, AA, A, BBB) |

### Fund Profile Module

The `fundProfile` module provides additional metadata:

```json
{
  "fundProfile": {
    "family": "WisdomTree Management Limited",
    "legalType": "Exchange Traded Fund",
    "categoryName": null,
    "feesExpensesInvestment": {
      "totalNetAssets": 21526.37,
      "annualReportExpenseRatio": 0,
      "annualHoldingsTurnover": 0
    },
    "managementInfo": {
      "managerName": null,
      "managerBio": null
    }
  }
}
```

| Field | Description |
|---|---|
| `family` | Fund family / provider name |
| `legalType` | "Exchange Traded Fund", "Mutual Fund", etc. |
| `categoryName` | Morningstar-style category (often null for non-US ETFs) |
| `totalNetAssets` | Net assets (units vary — may be in thousands/millions) |
| `annualReportExpenseRatio` | Expense ratio as decimal |
| `annualHoldingsTurnover` | Turnover rate as decimal |
| `managerName` | Fund manager name (often null for passive ETFs) |

## go-yfinance Library Limitations

The `go-yfinance` v1.3.0 library does **not** expose the `topHoldings` module. Available methods on `*ticker.Ticker`:

| Method | Data Source | Available |
|---|---|---|
| `Quote()` | v7 quote API | ✅ |
| `Info()` | quoteSummary (assetProfile, summaryDetail, etc.) | ✅ |
| `History()` | chart API | ✅ |
| `MajorHolders()` | quoteSummary (majorHoldersBreakdown) | ✅ |
| `InstitutionalHolders()` | quoteSummary (institutionOwnership) | ✅ |
| `MutualFundHolders()` | quoteSummary (fundOwnership) | ✅ |
| `InsiderTransactions()` | quoteSummary (insiderTransactions) | ✅ |
| `Financials()` | fundamentals API | ✅ |
| `Analysis()` | quoteSummary (earnings, recommendations) | ✅ |
| `Options()` | options API | ✅ |
| `News()` | news API | ✅ |
| `Calendar()` | visualization API | ✅ |
| **`TopHoldings()`** | **quoteSummary (topHoldings)** | **❌ Not implemented** |

### Key Distinction

The existing holders methods (`InstitutionalHolders`, `MutualFundHolders`) answer **"who owns shares of this ETF?"** — they return institutional investors and mutual funds that hold the ETF. The `topHoldings` module answers **"what does this ETF own?"** — it returns the underlying positions of the ETF itself.

### Internal Implementation

The library's `fetchQuoteSummary` method (unexported, in `holders.go`) supports arbitrary modules. The `topHoldings` module is simply not wired up. The internal `QuoteSummaryModules` list in `endpoints.go` also omits it.

## Symbol Support

### US ETFs

US-listed ETFs (e.g., `SPY`, `VTI`, `QQQ`) return complete holdings data reliably.

### International ETFs

LSE-listed ETFs (e.g., `WMGG.L`) also return holdings data via the same endpoint. Key findings:

- **WMGG.L** (WisdomTree Megatrends UCITS ETF) — ✅ Returns full topHoldings + fundProfile
- **WGMM.L** — ❌ Not found on Yahoo Finance (ticker doesn't exist)

The v7 quote endpoint (`/v7/finance/quote`) also works for international tickers and returns basic quote data (price, volume, market cap).

### Non-ETF Symbols

Stocks (e.g., `AAPL`) return the `topHoldings` module but with empty or minimal data — the module is designed for funds/ETFs.

## Error Handling

| Scenario | HTTP Status | Response |
|---|---|---|
| Symbol not found | 404 | `{"quoteSummary":{"result":null,"error":{"code":"Not Found","description":"Quote not found for symbol: X"}}}` |
| Missing crumb | 401 | `{"finance":{"result":null,"error":{"code":"Unauthorized","description":"Invalid Crumb"}}}` |
| Rate limited | 429 | Rate limit response |
| Invalid module | 200 | Module omitted from result (no error) |

## Implementation Approach

To fetch ETF holdings, we need to replicate the crumb/cookie auth and call the quoteSummary endpoint directly, since go-yfinance doesn't expose it. Options:

1. **Direct HTTP calls** — Replicate the auth flow (cookie + crumb) and call `quoteSummary` with `modules=topHoldings`. This is what the existing `YahooFinanceFetcher` does for quotes, just with a different endpoint.

2. **Extend go-yfinance** — Fork or patch the library to add `TopHoldings()`. Adds dependency on library internals.

3. **Use go-yfinance's auth + raw HTTP** — Create a `*yf.Ticker` to get the auth (crumb/cookie) warmed up, then make raw HTTP calls to the quoteSummary endpoint. However, the auth fields are unexported.

**Recommendation**: Option 1 — add a method to `YahooFinanceFetcher` that handles the auth flow and fetches topHoldings. This follows the existing pattern (the fetcher already manages its own HTTP client and auth) and keeps the dependency on go-yfinance minimal (only for the existing quote/history methods).

## Data Freshness Considerations

- ETF holdings typically change quarterly (rebalancing)
- Yahoo doesn't include a "last updated" timestamp in the response
- The `maxAge: 1` field in the response is Yahoo's cache directive (1 day)
- Recommended caching: store with a `fetched_at` timestamp, show stale indicator after 7 days
- No need for aggressive refresh — quarterly data doesn't change daily

## Rate Limiting

Yahoo Finance rate limits unauthenticated and crumb-authenticated requests. Observed behavior:

- Single requests work reliably
- Batch requests (multiple symbols in quick succession) may trigger 429
- Recommendation: serialize requests with a small delay (500ms–1s) between symbols during bulk refresh
