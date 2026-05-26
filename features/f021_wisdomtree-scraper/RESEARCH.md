# WisdomTree ETF Scraper — Progress & Findings

**Last updated**: 2026-05-26

> **WORKFLOW RULE**: Update this file AFTER EVERY finding or failure, not at the end. This prevents repeating work when context window resets.
**Target URL**: `https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt---wisdomtree-megatrends-ucits-etf---usd-acc`
**Fund**: WMGT — WisdomTree Megatrends UCITS ETF - USD Acc
**ISIN**: IE0000902GT6

---

## Status: ✅ Data extraction patterns fully identified, samples saved (including country allocation, market cap, fund characteristics)

---

## How the Data Works

**All data is embedded inline in the HTML page** as CSV strings inside JavaScript variables. No separate API calls needed. The page is server-rendered (Sitecore CMS) and injects the data directly into `<script>` blocks.

### Page Structure

The HTML page contains **four data variables**, each a CSV string with `\n` as row separator:

| Variable | Data Type | Format |
|----------|-----------|--------|
| `var fundHoldingsData` | Top holdings (weights + security names) | `date,Weight,Security Description` |
| `var fundMarketData<HASH>` | NAV history + benchmark index | `date,fund_ticker,close_price_adj,volume_adj,nav,...,uv10KMP,uv10KNAV` |
| `var fundThemeData` | Theme allocation breakdown | `date,Weight,Security Description` |
| `var fundSectorsData` | Sector allocation (per-security + totals) | `date,securityName,weight,Sector,wgtSector` |

Plus one JSON object:
| Variable | Data Type | Format |
|----------|-----------|--------|
| `var fundInfo<HASH>` | Fund identifier | `{'symbol':'WMGT', 'name':'WisdomTree Megatrends UCITS ETF - USD Acc'}` |

The `<HASH>` suffix (e.g. `657FFD51CE524D86BA76544F895FF0FE`) is a Sitecore item ID unique to each fund page. For `fundHoldingsData`, `fundThemeData`, and `fundSectorsData`, the variable name has **no hash suffix**.

---

## Extraction Patterns (Regex for Go)

### 1. Fund Info
```regex
var fundInfo\w+\s*=\s*\{([^}]+)\}
```
Extract the JSON-like object, then parse `symbol` and `name` fields.

### 2. Holdings Data
```regex
var fundHoldingsData = '([^']+)'\s*;
```
The captured group is CSV with literal `\n` as row separator. Replace `\\n` with actual newlines, then parse as CSV.

**CSV Columns**: `date,Weight,Security Description`
- `date`: M/D/YYYY format (e.g. "5/22/2026")
- `Weight`: Decimal as string (e.g. "0.0143063") — represents portfolio weight
- `Security Description`: Quoted string, may contain `\u0026` for `&`

**Note**: The last ~25 rows are cash/currency positions (e.g. "CASH W-O", "STERLING POUND", "JAPANESE YEN") — these are not equity holdings.

### 3. NAV/Market Data
```regex
var fundMarketData\w+\s*=\s*'([^']+)'\s*;
```
Same pattern — CSV with literal `\n`.

**CSV Columns**: `date,fund_ticker,close_price_adj,volume_adj,nav,bmk_ticker_A,price_level_A,...,uv10KMP,uv10KNAV`
- `date`: M/D/YYYY format
- `fund_ticker`: e.g. "WMGT LN"
- `close_price_adj`: Usually empty
- `volume_adj`: Usually empty
- `nav`: NAV value (e.g. "25.0617")
- `uv10KMP`: Market price index (base 10000, e.g. "10024")
- `uv10KNAV`: NAV index (base 10000, e.g. "10024")

### 4. Theme Data
```regex
var fundThemeData = '([^']+)'\s*;
```
Same pattern — CSV with literal `\n`.

**CSV Columns**: `date,Weight,Security Description`
- `date`: M/D/YYYY format
- `Weight`: Decimal as string (e.g. "0.0857710")
- `Security Description`: Quoted theme name (e.g. "Grid Infrastructure", "AI Infrastructure")

### 5. Sectors Data
```regex
var fundSectorsData = '([^']+)'\s*;
```
Same pattern — CSV with literal `\n`.

**CSV Columns**: `date,securityName,weight,Sector,wgtSector`
- `date`: M/D/YYYY format
- `securityName`: Quoted security name
- `weight`: Individual security weight
- `Sector`: Sector name (e.g. "Industrials", "Information Technology")
- `wgtSector`: Sector total weight (repeated for each security in that sector)

**To get sector totals**: Take unique `(Sector, wgtSector)` pairs. The `wgtSector` column is the same for all securities within a sector.

### 6. Country Allocation (Dynamically Loaded)
**NOT embedded as a JavaScript variable.** The country allocation `<tbody>` is empty in the raw HTML — data is loaded dynamically via JavaScript after page load.

**How it works**: The section has `id="country-allocation-section"` with an empty `<tbody>`. No corresponding `var fundCountryData` variable exists. No `data-href` modal link exists for this section (unlike nav-history, all-holdings, distribuition-history). The data is loaded by an unknown JS mechanism — possibly a Sitecore component that fetches data server-side and injects it into the DOM.

**Extraction method**: Use `web_fetch` (Defuddle article extractor) which returns the fully-rendered page content including the dynamically-loaded country table. The raw HTML (`web_scrape format=raw`) has an empty `<tbody>`.

**Format in extracted content**: Markdown table:
```
| Country | Weight |
| --- | --- |
| 1. United States | 40.54% |
| 2. China | 11.62% |
| 3. South Korea | 6.65% |
...
```

**Parsing**: Strip leading "N. " from country names, parse percentage from weight column.

**For Go implementation**: Either use a headless browser (chromedp/playwright) to render the page then extract the table from DOM, OR use an article extraction library similar to Defuddle/Readability.

### 7. Market Capitalization (Dynamically Loaded)
**NOT in raw HTML at all.** No `id="market-cap"` section or `var fundMarketCapData` variable exists. The entire Market Capitalization table is injected by JavaScript after page load.

**Extraction method**: Same as country allocation — use `web_fetch` (Defuddle article extractor) to get the fully-rendered page.

**Format in extracted content**: HTML table:
```
| Market Capitalization | As of 22 May 2026 |
| Total Market Capitalization ($ Trillion) | 58.68 |
| Fund MarketCap Breakdown | |
| Large Cap (> $10 Billion) | 64.42% |
| Mid Cap (≥ $2 Billion and ≤ $10 Billion) | 26.55% |
| Small Cap (< $2 Billion) | 9.02% |
```

**Parsing**: Extract key-value pairs from table rows. The "Fund MarketCap Breakdown" row is a section header (colspan=2) — skip it.

### 8. Fund Characteristics (Dynamically Loaded)
**NOT in raw HTML at all.** Same dynamic loading pattern as Market Capitalization.

**Extraction method**: Same as country allocation — use `web_fetch` (Defuddle article extractor).

**Format in extracted content**: Markdown table:
```
| Fund Characteristics | As of 22 May 2026 |
| *Dividend Yield | 0.94 |
| Price/Earnings | 69.64 |
| Estimated Price/Earnings | 34.56 |
| Price/Book | 4.06 |
| Price/Sales | 2.61 |
| Price/Cash Flow | 28.69 |
| Gross Buyback Yield | 0.65 |
| Net Buyback Yield | -1.21 |
```

**Parsing**: Extract key-value pairs from table rows. Strip leading "*" from keys. The first row is a section header — skip it.

### 9. As of Date (In Raw HTML)

The "As of" date that appears in Market Capitalization and Fund Characteristics ("As of 22 May 2026") is **embedded in the raw HTML** in the NAV table header — not dynamically loaded.

**Location**: Second `<th>` of the "Net Asset Value" table:
```html
<table class="table table-striped-customized">
    <thead>
        <tr>
            <th>Net Asset Value</th>
            <th> 22 May 2026</th>
        </tr>
    </thead>
```

**Extraction regex**:
```regex
<th>\s*Net Asset Value\s*</th>\s*<th>\s*([^<]+)\s*</th>
```

The captured group is the date string (e.g. "22 May 2026"). Trim whitespace and parse as `time.Time` with format `"02 Jan 2006"` (Go reference time).

**Note**: The holdings/themes/sectors CSV data have their own dates embedded in each row (e.g. "5/11/2026", "5/8/2026") which may differ from the NAV "as of" date. Use the NAV table header date as the primary fund data timestamp. The country allocation section has a placeholder `<span class="info-component">As of 01 Jan 001</span>` in raw HTML — ignore it; use the NAV table date instead.

Saved in `wisdomtree-samples/`:

| File | Size | Description |
|------|------|-------------|
| `holdings_sample.csv` | 39KB | Full holdings (~800 securities + currencies) |
| `nav_sample.csv` | 1.2KB | NAV history (first 20 + last 5 rows of ~500 rows) |
| `themes_sample.csv` | 872B | 20 themes with weights |
| `sectors_sample.csv` | 60KB | Full sector data (837 rows, per-security + sector totals) |
| `fund_info.json` | 69B | Fund symbol + name |
| `country_allocation.csv` | 564B | 36 countries with weights (from web_fetch) |
| `market_cap.csv` | 168B | Market cap breakdown (from web_fetch) |
| `fund_characteristics.csv` | 198B | Fund characteristics (from web_fetch) |

---

## URL Pattern

The URL structure is:
```
https://www.wisdomtree.eu/{locale}/etfs/{category}/{ticker}---{name-slug}
```

Examples:
- `https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt---wisdomtree-megatrends-ucits-etf---usd-acc`
- `https://www.wisdomtree.eu/en-gb/etfs/equity/wrde---wisdomtree-us-large-cap-dividend-fund`

The locale can be: `en-gb`, `pl-pl`, `fr-lu`, `da-dk`, `de-at`, etc. The data is the same regardless of locale.

---

## Implementation Notes for Go Scraper

1. **HTTP Client**: Use a browser-like User-Agent. The page may have Cloudflare protection. Test with `net/http` first; if blocked, try `chromedp` or `playwright-go`.

2. **Parsing Approach**:
   - Fetch the HTML page
   - Use regex to extract each `var fundXxxData = '...'` block
   - Replace `\\n` with `\n` in the captured CSV string
   - Replace HTML entities like `\u0026` → `&`
   - Parse the resulting CSV with `encoding/csv`

3. **Error Handling**:
   - Check for Cloudflare challenge pages (look for `cf-chl` or `turnstile` in the HTML)
   - Validate that the expected variables exist before parsing
   - Handle empty/missing fields gracefully

4. **Rate Limiting**: WisdomTree is a static CMS — but add a 1-2 second delay between requests to be safe.

---

## What Was Tried (Chronological)

| Attempt | Result |
|---------|--------|
| `web_fetch` with default settings | ✅ Works — returns full HTML |
| `web_scrape` with `format=raw` | ✅ Works — saves raw HTML to file |
| `web_extract` with regex | ❌ Tool requires `selectors` object, not raw regex |
| `grep` on saved HTML file | ✅ Works — extracts all data patterns |
| Look for separate API endpoints | ❌ No API — all data is inline |
| Look for country/geography breakdown | ❌ No `var fundCountryData` — `<tbody>` empty in raw HTML |
| `web_fetch` (Defuddle extractor) on full page | ✅ Returns rendered content with country allocation table populated |
| Try modal URL `/modals/country-allocation` | ❌ 404 Not Found |
| Try modal URL `/modals/geographic-exposure` | ❌ 404 Not Found |
| Try modal URL `/modals/countries` | ❌ 404 Not Found |
| Check bundle.js (etf-charts) for country view | ❌ No "country" or "allocation" references in bundle.js |
| Check perfchart.wisdomtree.eu JS | ❌ Just html-to-react parser + web vitals, no country data |
| Country allocation extraction | ✅ Found via `web_fetch` article extraction — data is JS-rendered, not in raw HTML |
| Market Capitalization extraction | ✅ Found via `web_fetch` article extraction — same dynamic loading pattern as country allocation |
| Fund Characteristics extraction | ✅ Found via `web_fetch` article extraction — same dynamic loading pattern |

---

## Next Steps for Go Implementation

1. Write HTTP fetcher with proper User-Agent
2. Implement regex extraction for each data variable
3. Write CSV parser for each data type
4. Add unit tests using the sample files in `wisdomtree-samples/`
5. Handle Cloudflare/bot detection (if needed)
6. **Country allocation, Market Cap, Fund Characteristics**: Use headless browser (chromedp/playwright) OR article extraction library (go-readability) to get JS-rendered content, then parse the tables from the DOM. All three use the same dynamic loading pattern.
