# RESEARCH.md — BlackRock/iShares Data Sources

## Test Fund

**iShares Edge MSCI World Momentum Factor UCITS ETF (IWMO)**
- URL: `https://www.ishares.com/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf`
- Portfolio ID: `270051`
- Bloomberg Ticker: `IWMO LN`
- ISIN: `IE00BP3QZ825`

## Product Page

### URL Pattern

```
https://www.ishares.com/uk/individual/en/products/{portfolioId}/{seo-slug}?switchLocale=y&siteEntryPassthrough=true
```

The `?switchLocale=y&siteEntryPassthrough=true` parameter is **required** to bypass the investor type selection screen (Individual vs Professional). Without it, the page returns a generic landing page with no fund data.

### Server-Rendered Data

The product page (with switchLocale param) includes the following sections as server-rendered HTML:

#### Key Facts

| Field | Example Value |
|---|---|
| Net Assets | USD 5,181,115,355 (as of 29/May/2026) |
| Inception Date | 03/Oct/2014 |
| Asset Class | Equity |
| SFDR Classification | Other |
| Total Expense Ratio | 0.25% |
| Use of Income | Accumulating |
| Domicile | Ireland |
| Rebalance Frequency | Quarterly |
| UCITS | Yes |
| Fund Manager | BlackRock Asset Management Ireland Limited |
| Custodian | State Street Custodial Services (Ireland) Limited |
| Bloomberg Ticker | IWMO LN |
| ISA Eligibility | Yes |
| Base Currency | USD |
| Benchmark Index | MSCI World Momentum index (Net) |
| ISIN | IE00BP3QZ825 |
| Securities Lending Return | 0.01% (as of 31/Mar/2026) |
| Product Structure | Physical |
| Methodology | Optimised |
| Issuing Company | iShares IV plc |
| Administrator | State Street Fund Services (Ireland) Limited |
| Fiscal Year End | 31 May |
| SIPP Available | Yes |
| UK Reporting Status | Yes |

#### Portfolio Characteristics (Equity ETFs)

| Field | Example Value |
|---|---|
| Number of Holdings | 352 (as of 29/May/2026) |
| P/E Ratio | 29.16 (as of 29/May/2026) |
| P/B Ratio | 3.89 (as of 29/May/2026) |
| 3y Beta | 0.999 (as of 30/Apr/2026) |
| Standard Deviation (3y) | 16.62% (as of 30/Apr/2026) |
| Benchmark Level | USD 6,825.90 (as of 29/May/2026) |
| Benchmark Ticker | — |

#### Portfolio Characteristics (Bond ETFs/Funds)

From a corporate bond fund (portfolio 261492):

| Field | Example Value |
|---|---|
| Number of Holdings | 1224 (as of 30/Apr/2026) |
| Standard Deviation (3y) | 4.96% (as of 30/Apr/2026) |
| Yield to Maturity | 5.59 (as of 30/Apr/2026) |
| Weighted Average YTM | 5.49% (as of 30/Apr/2026) |
| Weighted Avg Maturity | 7.63 (as of 30/Apr/2026) |
| Modified Duration | 5.13 (as of 30/Apr/2026) |
| Effective Duration | 5.17 (as of 30/Apr/2026) |
| WAL to Worst | 7.63 (as of 30/Apr/2026) |
| 3y Beta | 0.997 (as of 30/Apr/2026) |
| 12 Month Trailing Dividend Distribution Yield | 4.30% (as of 30/Apr/2026) |

#### Exchange Listings (server-rendered table)

| Exchange | Ticker | Currency | Listing Date | SEDOL | Bloomberg Ticker | RIC |
|---|---|---|---|---|---|---|
| London Stock Exchange | IWMO | USD | 06/Oct/2014 | BP3QZ82 | IWMO LN | IWMO.L |
| Deutsche Boerse Xetra | IS3R | EUR | 07/Oct/2014 | BVFZJ10 | IS3R GY | IS3R.DE |
| London Stock Exchange | IWFM | GBP | 06/Oct/2014 | BP3QZ93 | IWFM LN | IWFM.L |
| Bolsa Mexicana De Valores | IWMO | MXN | 08/Dec/2017 | BYVJ1H7 | IWMON MM | — |
| Borsa Italiana | IWMO | EUR | 16/Feb/2015 | BVDPH30 | IWMO IM | IWMO.MI |
| SIX Swiss Exchange | IWMO | USD | 15/Dec/2014 | BRKWGB7 | IWMO SW | WMO.S |

### Component ID

The product page HTML contains a component ID embedded in the holdings download link:

```html
<a href="/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf/1506575576011.ajax?fileType=csv&fileName=IWMO_holdings&dataType=fund">
```

The component ID (`1506575576011`) is used to construct the holdings data URLs. This ID is specific to the page and may change if BlackRock redesigns the site.

## Holdings Data

### JSON API (preferred)

```
https://www.ishares.com/uk/individual/en/products/{portfolioId}/{seo-slug}/{componentId}.ajax?tab=all&fileType=json&asOfDate={yyyymmdd}
```

**Example**: `https://www.ishares.com/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf/1506575576011.ajax?tab=all&fileType=json&asOfDate=20260529`

**Response format**: JSON with UTF-8 BOM. Root object contains `aaData` array:

```json
{
  "aaData": [
    [
      "MU",                              // [0] Ticker
      "MICRON TECHNOLOGY INC",            // [1] Name
      "Information Technology",           // [2] Sector
      "Equity",                           // [3] Asset Class
      {"display": "USD 340,433,571.00", "raw": 340433571},  // [4] Market Value
      {"display": "6.57", "raw": 6.57076},                   // [5] Weight (%)
      {"display": "340,433,571.00", "raw": 340433571},      // [6] Notional Value
      {"display": "350,601.00", "raw": 350601},             // [7] Shares
      "US5951121038",                          // [8] CUSIP/ISIN (or "-" for cash)
      {"display": "971.00", "raw": 971},                 // [9] Price
      "United States",                         // [10] Location (country)
      "NASDAQ",                                // [11] Exchange
      "USD"                                     // [12] Market Currency
    ],
    // ... 383 total items for IWMO
  ]
}
```

**Notes**:
- 383 items for IWMO (352 equity holdings + cash/derivative positions)
- Last item is typically USD CASH with negative weight (cash position)
- Numeric fields have both `display` (formatted string) and `raw` (number) values
- File includes UTF-8 BOM — must decode as `utf-8-sig`

### CSV Download (fallback)

```
https://www.ishares.com/uk/individual/en/products/{portfolioId}/{seo-slug}/{componentId}.ajax?fileType=csv&fileName={ticker}_holdings&dataType=fund
```

**Response format**: CSV with UTF-8 BOM.

```csv
Fund Holdings as of,"29/May/2026"

Ticker,Name,Sector,Asset Class,Market Value,Weight (%),Notional Value,Shares,Price,Location,Exchange,Market Currency
"MU","MICRON TECHNOLOGY INC","Information Technology","Equity","340,433,571.00","6.57",...
```

**Notes**:
- CSV has a header row with "as of" date, then a blank line, then column headers, then data
- Same data as JSON but less structured (all values are strings)
- The `fileName` parameter is ignored — always returns holdings regardless of value tried (e.g. `IWMO_sector`, `IWMO_geography` all return the same holdings CSV)

### Date Parameter

The `asOfDate` parameter in the JSON API accepts dates in `YYYYMMDD` format. It's unclear if this is required or optional, and whether it filters to a specific date or just validates. The as-of date from the page (e.g. 29/May/2026) should be used.

## Sector / Geography Data

### Status: Not Available via Simple HTTP

The sector and geography breakdowns are **not** available in the server-rendered HTML (shows "Sorry, no data available" placeholder). They are loaded via JavaScript at runtime.

### Attempted Approaches

1. **Direct AJAX with tab=sector/tab=geography**: Returns HTTP 500 error:
   ```
   .../1506575576011.ajax?tab=sector&fileType=json&asOfDate=20260529
   .../1506575576011.ajax?tab=geography&fileType=json&asOfDate=20260529
   ```
   Both return 500 Internal Server Error.

2. **Product data API**: `https://www.ishares.com/uk/individual/en/product-data.jsn?portfolioId=270051`
   Returns minimal data: `{"siteContext":"ishares-uk","locale":"en_GB","timestamp":...,"data":{"270051":{"unlinked":false}}}`

3. **Product screener API**: `https://www.ishares.com/uk/individual/en/product-screener/product-screener-v3.jsn?portfolioId=270051`
   Returns HTTP 500 Internal Server Error.

4. **BlackRock API Gateway**: `https://www.blackrock.com/api-gateway/ishares-uk/product/270051/holdings`
   Returns HTTP 404 Not Found.

5. **BlackRock Apigee**: `https://api.blackrock.com`
   Referenced in page JS but no working endpoints discovered.

### Conclusion

The sector/geography data requires either:
- **Browser automation** (playwright/chromedp) to render the JavaScript and extract the data from the DOM
- **Reverse-engineering the JS API** by inspecting the minified JavaScript bundles to find the actual XHR/fetch calls
- **Deriving sector from holdings CSV/JSON** — the holdings data includes a "Sector" column per security, so sector allocation can be computed by aggregating weights by sector

### Recommended Approach

**Derive sector allocation from holdings data** (the JSON API includes sector per security). For geography, the holdings data includes "Location" (country) per security — aggregate weights by country for geography allocation. This avoids the need for browser automation or JS API reverse-engineering.

**Trade-off**: The computed sector/geography from holdings may not match iShares' published breakdown exactly (they may use different classification standards or include derivatives/cash differently). However, it provides a good approximation and is fully extractable via the JSON API.

## JavaScript API Endpoints (from page source)

The page source references the following API URLs (may be useful for future investigation):

```javascript
BLK.apiGatewayUrl = "https://www.blackrock.com/api-gateway";
BLK.apigeePath = "https://api.blackrock.com";

BLK["urlMap"] = {
  "productScreenerV3Api": "https://www.ishares.com/uk/individual/en/product-screener/product-screener-v3.jsn",
  "productPage": "https://www.ishares.com/uk/individual/en/products",
  "cwpScreenerApi": "https://www.ishares.com/uk/individual/en/product-data.jsn",
  "compareToolPage": "https://www.ishares.com/uk/individual/en/products/compare-funds",
  "compareEsgApi": "https://www.ishares.com/uk/individual/en/esg-product-data.jsn"
};
```

## Non-ETF Fund Example

**BlackRock Corporate Bond Tracker Fund** (portfolio 261492)

URL: `https://www.ishares.com/uk/individual/en/products/261492/ishares-core-msci-world-ucits-etf`

This is a non-ETF fund (tracker fund) with a different page structure:
- Key Facts: AUM, inception, benchmark, OCF, ISIN, minimum investment, use of income, regulatory structure, Morningstar category, dealing frequency, SEDOL
- Portfolio Characteristics: Number of holdings, std dev, YTM, weighted avg YTM, weighted avg maturity, trailing dividend yield, 3y beta, modified duration, effective duration, WAL to worst
- Risk Indicator: 1-7 scale
- Ratings: Morningstar rating (3 stars), Morningstar Medalist (Bronze)
- Holdings: Top 10 only (not full holdings)
- Pricing & Exchange: NAV per share class (Class H, D, L, S with accumulating/distributing variants)

**Not in scope for this feature** — ETFs only.

## Rate Limiting

No explicit rate limiting observed during testing. Apply 1-2 second delays between requests as per project convention.
