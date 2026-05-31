# Feature f025 — vanguard-scraper: API Research

## Discovery Method

Reverse-engineered from the Angular SPA JavaScript bundle (`/ukm/main.abb203f6fcc68a89.js`) on `vanguardinvestor.co.uk`. The SPA embeds a `CLIENT_CONFIG` JSON block with internal API URLs. The public-facing REST and GraphQL APIs are proxied through the same domain.

## Public API Endpoints

| Endpoint | Method | Auth | Description |
|---|---|---|---|
| `/api/funds/{fundSlug}` | GET | None | Fund profile, pricing, distributions, returns, risk, AUM |
| `/api/productList` | GET | None | All 172 UK funds with basic metadata |
| `/gpx/graphql` | POST | None | Holdings, sector allocation, country allocation, characteristics, price history |

## REST API Response (`/api/funds/{fundSlug}`)

Example: `vanguard-ftse-all-world-ucits-etf-usd-distributing` (VWRL, portId 9505)

Key fields: `name`, `ticker`, `sedol`, `portId`, `inceptionDate`, `OCF`, `benchmark`, `currencyCode`, `fundType`, `managementType`, `distributionStrategyType`, `assetClass`, `region`, `navPrice`, `marketPrice`, `totalNetAssets`, `risk`, `assetAllocations[]`, `fundData.distributionHistory`, `fundData.annualNAVReturns`, `siblings[]`.

## GraphQL Queries

All queries use `portId` (resolved from REST API) as the primary identifier. The GraphQL endpoint requires `Content-Type: application/json` with `operationName`, `variables`, and `query` fields.

### HoldingDetailsQuery (paginated, 1500/page)

```json
{
  "operationName": "HoldingDetailsQuery",
  "variables": {
    "portIds": ["9505"],
    "securityTypes": ["MF.MF","FI.ABS","FI.CONV","FI.CORP","FI.IP","FI.LOAN","FI.MBS","FI.MUNI","FI.NONUS_GOV","FI.US_GOV","MM.AGC","MM.BACC","MM.CD","MM.CP","MM.MCP","MM.RE","MM.TBILL","MM.TD","MM.TFN","EQ.DRCPT","EQ.ETF","EQ.FSH","EQ.PREF","EQ.PSH","EQ.REIT","EQ.STOCK","EQ.RIGHT","EQ.WRT"],
    "lastItemKey": null
  },
  "query": "query HoldingDetailsQuery($portIds: [String!], $securityTypes: [String!], $lastItemKey: String) {\n  borHoldings(portIds: $portIds) {\n    holdings(limit: 1500, securityTypes: $securityTypes, lastItemKey: $lastItemKey) {\n      totalHoldings\n      lastItemKey\n      items {\n        effectiveDate\n        marketValuePercentage\n        issuerName\n        securityLongDescription\n        couponRate\n        securityType\n        finalMaturity\n      }\n    }\n  }\n}"
}
```

**Pagination**: `lastItemKey` is a JSON string (not an object). Example:
```json
{"skey":0.00702,"pkey":"fundBorHoldings-9505-70b78789ad8c5f427ae2993436015aa22053b2d6","portIdKey":"fundBorHoldings-9505-2026-04-30"}
```

Pass this string as the `lastItemKey` variable in the next request. When `lastItemKey` is `null`, all pages have been fetched.

**Test results** (VWRL, 4070 total holdings):
- Page 1: 1500 items, lastItemKey set
- Page 2: 1500 items, lastItemKey set
- Page 3: 798 items, lastItemKey = null

**Limit**: Cannot exceed 1500. Attempts with 5000/7000 return "GraphQL request failed".

### getSectorDiversification

```json
{
  "operationName": "getSectorDiversification",
  "variables": { "portIds": ["9505"] },
  "query": "query getSectorDiversification($portIds: [String!]!) {\n  funds(portIds: $portIds) {\n    profile { primarySectorEquityClassification }\n    sectorDiversification { sectorCode date sectorName fundPercent benchmarkPercent }\n  }\n}"
}
```

Returns 12 ICB sectors with fund vs benchmark percentages. Classification: "ICB Sectors".

### MarketAllocationGqlQuery

```json
{
  "operationName": "MarketAllocationGqlQuery",
  "variables": { "portIds": ["9505"] },
  "query": "query MarketAllocationGqlQuery($portIds: [String!]!) {\n  funds(portIds: $portIds) {\n    profile { fundFullName primaryMarketEquityClassification polarisPdtTypeIndicator marketOfDomicile }\n    marketAllocation { portId date countryCode countryName fundMktPercent holdingStatCode benchmarkMktPercent regionCode regionName }\n  }\n}"
}
```

Returns 107 countries with fund vs benchmark percentages, grouped by region (Emerging Markets, Europe, North America, Pacific, etc.). Classification: "FTSE Country of Risk".

### FundCharacteristicsQuery

```json
{
  "operationName": "FundCharacteristicsQuery",
  "variables": { "portIds": ["9505"] },
  "query": "query FundCharacteristicsQuery($portIds: [String!]!) {\n  polarisAnalyticsHistory(portIds: $portIds) {\n    portId\n    monthly {\n      analytics {\n        fund(getLatest: true) {\n          items {\n            codes {\n              PBRATIO { analyticValue effectiveDate __typename }\n              PERATIO { analyticValue effectiveDate __typename }\n              AVGCPN { analyticValue effectiveDate __typename }\n              MKTCAPMEDN { analyticValue effectiveDate __typename }\n              FRC5YRROE { analyticValue effectiveDate __typename }\n              EPSFRC5YR { analyticValue effectiveDate __typename }\n              TRNVRRPTR { analyticValue effectiveDate __typename }\n              AVGWTDMTY { analyticValue effectiveDate __typename }\n              AVGQLYTFTO { analyticValue effectiveDate __typename }\n              AVGDURADJ { analyticValue effectiveDate __typename }\n              __typename\n            }\n            __typename\n          }\n          __typename\n        }\n        __typename\n      }\n      __typename\n    }\n    __typename\n  }\n}"
}
```

**Important**: Each code field must specify its sub-fields (`analyticValue`, `effectiveDate`, `__typename`). A bare code name (e.g. `PERATIO` without sub-fields) returns HTTP 500. The response wraps each code as `{analyticValue: string, effectiveDate: string, __typename: string}` or `null` for non-applicable types.

Returns fund characteristics (P/E, P/B, market cap, ROE, EPS growth, etc.). Equity-specific codes (PERATIO, PBRATIO, MKTCAPMEDN) and bond-specific codes (AVGCPN, AVGWTDMTY, AVGDURADJ) are both available — null values for non-applicable types.

### PriceDetailsQuery

```json
{
  "operationName": "PriceDetailsQuery",
  "variables": { "portIds": ["9505"], "startDate": "2026-04-30", "endDate": "2026-05-30", "limit": 0 },
  "query": "query PriceDetailsQuery($portIds: [String!]!, $startDate: String!, $endDate: String!, $limit: Float) {\n  funds(portIds: $portIds) {\n    pricingDetails {\n      navPrices(startDate: $startDate, endDate: $endDate, limit: $limit) { items { price asOfDate currencyCode } }\n      marketPrices(startDate: $startDate, endDate: $endDate, limit: $limit) { items { portId items { price asOfDate currencyCode } } }\n    }\n  }\n}"
}
```

Returns NAV prices and market prices per exchange listing. `limit: 0` means unlimited. Returns ~22 trading days for a month range. Market prices include multiple exchange listings (L005=USD LSE, L006=GBP LSE, L013=CHF SIX, etc.).

## URL Pattern

- Fund page: `https://www.vanguardinvestor.co.uk/investments/{fundSlug}`
- REST API: `https://www.vanguardinvestor.co.uk/api/funds/{fundSlug}`
- GraphQL: `https://www.vanguardinvestor.co.uk/gpx/graphql` (POST with portIds)

## Rate Limiting Observations

- No explicit rate limiting observed during testing (multiple rapid requests succeeded)
- Recommendation: 1-2s delay between major queries, 0.5-1s between pagination pages

## Key Design Decisions

1. **Two-phase extraction**: REST API for portId resolution + fund profile overview, then GraphQL for deep data (holdings, allocations, characteristics).

2. **GraphQL as primary data source**: The GraphQL API provides holdings (with pagination), sector/country allocation (with benchmark), and fund characteristics — data not available from the REST API alone.

3. **Holdings pagination**: 1500 items per page, `lastItemKey` is a JSON string. The extractor paginates until `lastItemKey` is null.

4. **All-or-nothing**: If any query fails, the entire extraction fails. Partial data is not persisted.

5. **Benchmark comparison**: Sector and country allocations include benchmark percentages. Stored alongside fund data for comparison display.

6. **Fund characteristics**: Both equity-specific (P/E, P/B, market cap) and bond-specific (coupon, maturity, duration) codes are available. Null for non-applicable fund types.

## Data Mapping

### Fund Profile (from Phase 1 + Phase 2)

| Source Field | Portfolio Lab Domain Field | Logic |
|---|---|---|
| `name` / `displayName` | `FundInfo.Name` | Direct mapping. |
| `ticker` | `FundInfo.Symbol` | Direct mapping. |
| `sedol` | Stored in fund profile | SEDOL identifier. |
| `portId` | Stored in fund profile | Internal Vanguard portfolio ID. |
| `currencyCode` | Stored in fund profile | Base currency (e.g. "USD"). |
| `inceptionDate` | `FundProfile.InceptionDate` | Parse date string. |
| `OCF` | `FundProfile.AnnualExpenseRatio` | Parse percentage string to float. |
| `benchmark` | Stored in fund profile | Benchmark name. |
| `managementType` | Stored in fund profile | e.g. "Index", "Active". |
| `assetClass` | Stored in fund profile | e.g. "Equity", "Fixed Income". |
| `fundType` | Stored in fund profile | e.g. "etf", "fund". |
| `distributionStrategyType` | Stored in fund profile | e.g. "INCM" (distributing), "ACCM" (accumulating). |
| `region` | Stored in fund profile | e.g. "Global", "UK", "US". |
| `fundFullName` | Stored in fund profile | Full fund name. |
| `ISIN` | Stored in fund profile | ISIN identifier. |
| `TOTEXPRTPC` | Stored in fund profile | Expense ratio as numeric value. |

### Holdings

| Source Field | Portfolio Lab Domain Field | Logic |
|---|---|---|
| `issuerName` | Holding name | Direct mapping. |
| `securityLongDescription` | Holding description | Direct mapping. |
| `marketValuePercentage` | Holding weight | Parse as float percentage. |
| `securityType` | Holding type | e.g. "EQ.STOCK", "FI.CORP". |
| `couponRate` | Holding coupon | For fixed income. |
| `finalMaturity` | Holding maturity | For fixed income. |
| `effectiveDate` | `ExtractResult.AsOfDate` | Reference date for holdings. |

### Sector Allocation

| Source Field | Portfolio Lab Domain Field | Logic |
|---|---|---|
| `sectorName` | Sector name | ICB 12-sector standard. |
| `sectorCode` | Sector code | ICB sector code. |
| `fundPercent` | Fund allocation % | Parse as float. |
| `benchmarkPercent` | Benchmark allocation % | Parse as float. |
| `date` | Reference date | Allocation as-of date. |

### Country Allocation

| Source Field | Portfolio Lab Domain Field | Logic |
|---|---|---|
| `countryName` | Country name | Direct mapping. |
| `countryCode` | Country code | ISO country code. |
| `fundMktPercent` | Fund allocation % | Parse as float. |
| `benchmarkMktPercent` | Benchmark allocation % | Parse as float. |
| `regionName` | Region name | e.g. "Emerging Markets", "Europe". |
| `regionCode` | Region code | Region identifier. |
| `date` | Reference date | Allocation as-of date. |

### Fund Characteristics

| Source Code | Description | Logic |
|---|---|---|
| `PERATIO` | P/E ratio | Parse as float. |
| `PBRATIO` | P/B ratio | Parse as float. |
| `MKTCAPMEDN` | Median market cap | Parse as float. |
| `FRC5YRROE` | Forward 5Y ROE | Parse as float. |
| `EPSFRC5YR` | Forward 5Y EPS growth | Parse as float. |
| `TRNVRRPTR` | Revenue / Revenue prior year | Parse as float. |
| `AVGCPN` | Average coupon (bonds) | Parse as float. |
| `AVGWTDMTY` | Average weighted maturity (bonds) | Parse as float. |
| `AVGQLYTFTO` | Average quality (bonds) | Parse as float. |
| `AVGDURADJ` | Average duration adjusted (bonds) | Parse as float. |
