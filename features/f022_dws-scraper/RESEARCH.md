# DWS Xtrackers ETF Scraper — Research & Findings

**Last updated**: 2026-05-28

## Status: ✅ API endpoints identified. No browser required.

Unlike WisdomTree, DWS uses a fully decoupled JSON API for its Product Detail Pages (PDP). The website is a Nuxt.js SPA that fetches data from a REST API. We can bypass the frontend entirely and query the API directly using standard HTTP requests.

---

## How the Data Works

The data is retrieved via a structured API. The URL pattern for all requests is:
`https://etf.dws.com/api/pdp/{locale}/etf/{identifier}/{endpoint}`

### Parameters
- `{locale}`: `en-gb` (for English/UK)
- `{identifier}`: The unique fund slug found in the URL (e.g., `IE00BGV5VN51-artificial-intelligence-big-data-ucits-etf-1c`)
- `{endpoint}`: The specific data requested.

### API Endpoints

| Endpoint | Data Type | Key Information |
|----------|-----------|-----------------|
| `pdpSettings` | Fund Meta | Product type, internal identifier, URL |
| `holdings` | Portfolio | ISIN, Security Name, Weight, Market Value, Country, Industry |
| `performancechart` | Time Series | NAV history, Benchmark history, As-of date |
| `pdpMetaTagsTealium` | Analytics | Metadata for tracking (likely not needed) |

---

## Extraction Patterns (for Go)

### 1. Holdings & Composition Data (`/holdings`)
**Format**: JSON
**Structure**: 
The data is split into two main parts: the **Portfolio Table** and **Fund Metadata**.

#### A. Portfolio Table (for Holdings, Country, and Sector)
Located in `tables[0].values`. Each entry is a map.

**Mapping**:
- `header`: ISIN
- `column_0`: Security Name
- `column_1`: Weight (Use `sortValue` for raw float, e.g., `9.01827819`)
- `column_3`: **Country** $\rightarrow$ Use this to aggregate **Country Allocation**.
- `column_4`: **Industry** $\rightarrow$ Use this to aggregate **Sector Weighting**.

**Aggregation Logic (Go)**:
Create a `map[string]float64` for countries and sectors. Iterate through all holdings and add the `sortValue` of `column_1` to the corresponding map key.

#### B. Fund Facts & Costs (TER)
Located in the `costAndFees.data` array.

**Extraction**:
- **TER**: Find the object where `key == "Total ongoing costs of the product"`. The `value` contains the percentage (e.g., `"0.381% p.a."`).

#### C. MiFID II / Risk Data
Located in `mifid.targetMarket.data`.
- Contains risk indicators (e.g., "Risk indicator (PRIIPS methodology)") and target client profiles.


### 2. Performance Data (`/performancechart`)
**Format**: JSON
**Structure**:
- `asOfDate`: Date string (e.g., `"27/05/2026"`)
- `seriesConfiguration`: Describes the series (0 = NAV, 1 = Index)
- `values`: An array of arrays: `[timestamp, [nav_data, index_data]]`
    - `timestamp`: Unix milliseconds.
    - `nav_data`: `[value, adjusted_value, flag]`

---

## Sample Data
Samples are located in `features/f022_dws-scraper/samples/`:
- `holdings_sample.json`: Full JSON response for the Artificial Intelligence ETF holdings.
- `performance_sample.json`: Full JSON response for NAV/Index history.

---

## Go Implementation Plan

### 1. Client Setup
Since the API is public and does not currently exhibit aggressive bot protection for API endpoints (unlike the main HTML pages), a standard `http.Client` with a browser-like `User-Agent` is sufficient.

```go
client := &http.Client{Timeout: 10 * time.Second}
req, _ := http.NewRequest("GET", url, nil)
req.Header.Set("User-Agent", "Mozilla/5.0 ...")
```

### 2. Data Models
Define structs to mirror the API response:

```go
type HoldingsResponse struct {
    Tables []struct {
        Columns []Column `json:"columns"`
        Values  []map[string]ValueCell `json:"values"`
    } `json:"tables"`
}

type ValueCell struct {
    Value     string  `json:"value"`
    SortValue float64 `json:"sortValue"`
}
```

### 3. Parsing Logic
- **Holdings**: Iterate through `values`, map the `column_X` keys to the correct domain fields (Name, Weight, etc.) using the `columns` definition.
- **Performance**: Convert Unix milliseconds to `time.Time` and extract the first element of the inner array for the NAV value.

## Verdict
**Lightweight HTTP + JSON parsing is the complete solution.** No headless browser (Puppeteer/Playwright/chromedp) is necessary for this source.
