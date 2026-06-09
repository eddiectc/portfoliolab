# WisdomTree ETF Scraper — Progress & Findings

**Last updated**: 2026-06-09

## Update 2026-06-09: Test data refreshed

The test HTML file (`testdata/wmgt_page_cycletls.html`) was stale and missing `fundSectorsData`. The WisdomTree website had restructured — the old scrape was from a page version that didn't embed the sectors CSV variable. Fresh scrape confirms:

- `fundSectorsData` IS present on the live page (12 sectors for WMGT as of 2026-06-08)
- All other data variables (`fundInfo`, `fundMarketData`, `fundHoldingsData`, `fundThemeData`, `fundBenchmarks`) still present
- All parsers work correctly against the fresh HTML
- Test HTML file updated with fresh 477KB page content
- Added `TestParseSectors_Real` to validate sectors against real HTML

> **WORKFLOW RULE**: Update this file AFTER EVERY finding or failure, not at the end. This prevents repeating work when context window resets.
**Target URL**: `https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt---wisdomtree-megatrends-ucits-etf---usd-acc`
**Fund**: WMGT — WisdomTree Megatrends UCITS ETF - USD Acc
**ISIN**: IE0000902GT6

---

## Status: ✅ Data extraction complete. ALL data extractable via CycleTLS + HTML parsing. Holdings with ticker/symbol data available via the all-holdings modal page. No headless browser needed.

---

## How the Data Works

**All data is embedded inline in the HTML page** as CSV strings inside JavaScript variables. No separate API calls needed. The page is server-rendered (Sitecore CMS) and injects the data directly into `<script>` blocks.

### Page Structure

The HTML page contains **four data variables**, each a CSV string with `\n` as row separator:

| Variable | Data Type | Format |
|----------|-----------|--------|
| `var fundHoldingsData` | Top holdings (weights + security names, **no ticker**) | `date,Weight,Security Description` |
| `var fundMarketData<HASH>` | NAV history + benchmark index | `date,fund_ticker,close_price_adj,volume_adj,nav,...,uv10KMP,uv10KNAV` |
| `var fundThemeData` | Theme allocation breakdown | `date,Weight,Security Description` |
| `var fundSectorsData` | Sector allocation (per-security + totals) | `date,securityName,weight,Sector,wgtSector` |

Plus one JSON object:
| Variable | Data Type | Format |
|----------|-----------|--------|
| `var fundInfo<HASH>` | Fund identifier | `{'symbol':'WMGT', 'name':'WisdomTree Megatrends UCITS ETF - USD Acc'}` |

The `<HASH>` suffix (e.g. `657FFD51CE524D86BA76544F895FF0FE`) is a Sitecore item ID unique to each fund page. For `fundHoldingsData`, `fundThemeData`, and `fundSectorsData`, the variable name has **no hash suffix**.

**Additionally**, the raw HTML contains **server-rendered tables** with:
- **NAV table**: NAV, Daily Change, Daily Return, Total AUM of fund, Issuer AUM (with "as of" date in header)
- **Product Overview table**: Inception Date, TER, Exchange Ticker, Index Name (key-value rows with `class="key"` and `class="value"`)

**All-holdings modal** (separate page fetched via `data-href` link):
- Contains an embedded JSON array (`var source = [...]`) with full holdings data including `IdentifierTicker` (e.g. `"NVDA UQ"`) and `IdentifierName`
- URL pattern: `https://www.wisdomtree.eu/{locale}/global/etf-details/modals/all-holdings?id={GUID}`
- The GUID is extracted from a `data-href` attribute on the main page
- Modal page also bypassed by CycleTLS (same Cloudflare protection)

---

## Extraction Patterns (Regex for Go)

### 1. Fund Info
```regex
var fundInfo\w+\s*=\s*\{([^}]+)\}
```
Extract the JSON-like object, then parse `symbol` and `name` fields.

### 2. Holdings Data (Main Page CSV — No Ticker)
```regex
var fundHoldingsData = '([^']+)'\s*;
```
The captured group is CSV with literal `\n` as row separator. Replace `\\n` with actual newlines, then parse as CSV.

**CSV Columns**: `date,Weight,Security Description`
- `date`: M/D/YYYY format (e.g. "5/22/2026")
- `Weight`: Decimal as string (e.g. "0.0143063") — represents portfolio weight
- `Security Description`: Quoted string, may contain `\u0026` for `&`

**Note**: The last ~25 rows are cash/currency positions (e.g. "CASH W-O", "STERLING POUND", "JAPANESE YEN") — these are not equity holdings.

**Note**: This CSV has **no ticker/symbol column**. Use the all-holdings modal (Section 2b) for ticker data.

### 2b. Holdings Data (All-Holdings Modal — With Ticker)

The main page contains a link to an "all-holdings" modal page:
```html
<a data-href="https://www.wisdomtree.eu/en-gb/global/etf-details/modals/all-holdings?id={8B845B79-F55C-4B6A-8D67-CA84E1C19C5B}">
```

**Extract modal URL** with regex:
```regex
data-href="([^"]*all-holdings[^"]*)"
```

**Fetch the modal page** via the same CycleTLS client. The modal contains an embedded JSON array:
```regex
var source = (\[\s*\{[^\]]*\}\s*\])
```

**JSON structure per holding**:
```json
{
  "CountryCode": "US ",
  "Weight": 0.1426250,
  "COBDate": "2026-06-02T00:00:00",
  "IdentifierName": "Nvidia Corp",
  "IdentifierTicker": "NVDA UQ",
  "SharesPar": "26576",
  "MarketValue": 5921664.32
}
```

**Ticker format**: Bloomberg-style with market suffix (e.g. `"NVDA UQ"`, `"MSFT US"`, `"LLY UN"`). Strip the suffix (everything after the first space) to get the clean ticker. Some entries have CUSIP instead of ticker (e.g. `"US5128073062"`) — keep as-is.

**Weight format**: Fraction (e.g. `0.1426250` = 14.26%). Multiply by 100 for percentage.

**Cash positions**: Same filtering rules apply — entries with `IdentifierName` containing "CASH W-O", currency names, etc. should be filtered out.

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

### 6. Country Allocation (In Raw HTML — HTML Table)
**IS embedded in the raw HTML** as a standard `<table>` inside `id="country-allocation-section"`. Previously thought to be JS-rendered — this was incorrect. The table has a `<thead>` with "Country"/"Weight" headers and a `<tbody>` with numbered rows.

**Format in raw HTML**:
```html
<table class="table table-striped-customized">
    <thead>
        <tr>
            <th class="key">Country</th>
            <th class="value">Weight</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td class="key">1. United States</td>
            <td class="value"><span class="value percent positive">40.54%</span></td>
        </tr>
        ...
    </tbody>
</table>
```

**Parsing**: Regex on `<td class="key">` for country name (strip leading "N. "), regex on `<span class="value percent` for weight (strip `%`).

**As of date**: Present in a `<span class="info-component">As of 22 May 2026</span>` above the table.

### 7. Market Capitalization (In Raw HTML — HTML Table)
**IS embedded in the raw HTML** as a standard `<table>` inside `id="fund-facts-section"`. Previously thought to be JS-rendered — this was incorrect.

**Format in raw HTML**:
```html
<table class="table table-striped-customized">
    <thead>
        <tr>
            <th class="key">Market Capitalization</th>
            <th class="value">As of 22 May 2026</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td class="key">Total Market Capitalization ($ Trillion)</td>
            <td class="value">58.68</td>
        </tr>
        <tr>
            <td colspan="2"><strong>Fund MarketCap Breakdown</strong></td>
        </tr>
        <tr>
            <td class="key shifted">Large Cap (&gt; $10 Billion)</td>
            <td class="value">64.42%</td>
        </tr>
        ...
    </tbody>
</table>
```

**Parsing**: Extract key-value pairs from table rows. Skip the "Fund MarketCap Breakdown" header row (colspan=2). Parse numeric values and percentages. HTML entities like `&gt;` are used.

### 8. Fund Characteristics (In Raw HTML — HTML Table)
**IS embedded in the raw HTML** as a standard `<table>` inside `id="fund-facts-section"` (same section as Market Cap, in a separate column). Previously thought to be JS-rendered — this was incorrect.

**Format in raw HTML**:
```html
<table class="table table-striped-customized">
    <thead>
        <tr>
            <th class="key">Fund Characteristics</th>
            <th class="value">As of 22 May 2026</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td class="key">*Dividend Yield</td>
            <td class="value">0.94</td>
        </tr>
        <tr>
            <td class="key">Price/Earnings</td>
            <td class="value">69.64</td>
        </tr>
        <tr>
            <td class="key">Estimated Price/Earnings</td>
            <td class="value">34.56</td>
        </tr>
        <tr>
            <td class="key">Price/Book</td>
            <td class="value">4.06</td>
        </tr>
        <tr>
            <td class="key">Price/Sales</td>
            <td class="value">2.61</td>
        </tr>
        <tr>
            <td class="key">Price/Cash Flow</td>
            <td class="value">28.69</td>
        </tr>
        <tr>
            <td class="key">Gross Buyback Yield</td>
            <td class="value">0.65</td>
        </tr>
        <tr>
            <td class="key">Net Buyback Yield</td>
            <td class="value">-1.21</td>
        </tr>
    </tbody>
</table>
```

**Parsing**: Extract key-value pairs from table rows. Strip leading "*" from keys. Parse numeric values (may be negative).

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

### 10. AUM, TER, Inception Date (In Raw HTML)

**All three are embedded in the raw HTML** as key-value rows in server-rendered tables — no dynamic loading required.

#### Inception Date & TER — Product Overview Table

**Location**: A `<table class="table table-striped-customized">` in the "Product Overview" section, before the NAV table. Each row is:
```html
<tr>
    <td class="key">Inception Date</td>
    <td class="value">05 Dec 2023</td>
</tr>
<tr>
    <td class="key">TER</td>
    <td class="value">0.50%</td>
</tr>
```

**Extraction regex** (for each field):
```regex
<td class="key">\s*Inception Date\s*</td>\s*<td class="value">\s*([^<]+)\s*</td>
<td class="key">\s*TER\s*</td>\s*<td class="value">\s*([^<]+)\s*</td>
```

**Parsing**:
- **Inception Date**: Trim whitespace, parse as `"02 Jan 2006"` (Go reference time). E.g. `"05 Dec 2023"` → `time.Date(2023, 12, 5, 0, 0, 0, 0, time.UTC)`
- **TER**: Trim whitespace, strip trailing `%`, parse as float (e.g. `"0.50%"` → `0.50`). Represents annual total expense ratio as a percentage.

#### AUM — NAV Table

**Location**: Same table as NAV (the one with the "Net Asset Value" header), below the Daily Return row:
```html
<tr>
    <td>Total AUM of fund</td>
    <td><span class="value currency positive">$60,368,055</span></td>
</tr>
<tr>
    <td>Issuer AUM</td>
    <td><span class="value currency positive">$17,226,851,369</span></td>
</tr>
```

**Extraction regex**:
```regex
<td>Total AUM of fund</td>\s*<td><span class="value[^>]*">([^<]+)</span></td>
<td>Issuer AUM</td>\s*<td><span class="value[^>]*">([^<]+)</span></td>
```

**Parsing**:
- **Total AUM of fund**: Trim, strip `$` and commas, parse as float (e.g. `"$60,368,055"` → `60368055.00`). Represents total assets under management for this specific fund in USD.
- **Issuer AUM**: Same pattern. Represents total AUM across all WisdomTree products (not just this fund). E.g. `"$17,226,851,369"` → `17226851369.00`.

**Note**: The AUM values share the same "as of" date as the NAV (from the NAV table header, see Section 9).

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

## Cloudflare Protection — RESOLVED

**WisdomTree.eu is behind Cloudflare bot protection.** Plain `curl` / Go `net/http` requests return a Cloudflare challenge page ("Just a moment...") instead of the actual HTML.

**CycleTLS successfully bypasses Cloudflare** — tested and confirmed (2026-05-26). Returns 200 OK with full page content including all data sections.

**Test configuration**:
```go
client := cycletls.Init()
resp, err := client.Do(url, cycletls.Options{
    Ja3:       "771,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-13-18-51-45-43-27-21,29-23-24,0",
    UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
}, "GET")
```

**No headless browser needed.** CycleTLS (already a transitive dependency via go-yfinance) is sufficient for all data extraction.

## Implementation Notes for Go Scraper

1. **HTTP Client**: Use CycleTLS (already a transitive dependency via go-yfinance) for browser-grade TLS fingerprinting. **Confirmed working** — returns 200 OK with full page content.

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

## CycleTLS — Confirmed Working

Country allocation, market cap, and fund characteristics **ARE in the raw HTML** (not JS-rendered as previously believed). CycleTLS successfully bypasses Cloudflare and returns the full page with all data.

**No headless browser needed.** All data is extractable from raw HTML using:
- Regex + CSV parsing for inline JS variables (holdings, NAV, themes, sectors)
- Regex on HTML tables for structured data (country allocation, market cap, fund characteristics, AUM, TER, inception date)

**Verdict**: **CycleTLS + HTML parsing** is the complete solution. No chromedp, no external service, no additional dependencies.

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
| AUM, TER, Inception Date extraction | ✅ Found in raw HTML — server-rendered key-value table rows, same pattern as NAV |
| `curl` on WisdomTree URL | ❌ Cloudflare challenge page ("Just a moment...") — plain HTTP blocked |
| `curl` on dataspanapi.wisdomtree.com | ❌ Cloudflare blocked — same protection as main site |
| dataspanapi `/funddetails/country_allocation` | ❌ 404 endpoint doesn't exist (Cloudflare blocked anyway) |
| dataspanapi `/funddetails/market_capitalization` | ❌ 404 endpoint doesn't exist (Cloudflare blocked anyway) |
| Check bundle.js for API endpoints | ❌ No fetch calls, no API URLs (only XML namespace constants) |
| Check chunk.js for API endpoints | ❌ No fetch calls, no API URLs, no country/allocation references |
| Check main.js for API endpoints | ✅ Found `dataspanapi.wisdomtree.com/funddetails/calendar_year_performance` but Cloudflare blocked |
| Check script.js for country allocation loading | ❌ No references to country-allocation-section or info-component |
| Check for Sitecore rendering API | ❌ No evidence of JSS/rendering API endpoints |
| Check for hidden JSON data blocks | ❌ Only ld+json is schema.org Organization (not fund data) |
| Check data- attributes on country section | ❌ No data-href or data-content attributes (unlike modal sections) |
| Check existing dependencies for TLS fingerprinting | ✅ CycleTLS + utls already in go.mod (via go-yfinance) |
| CycleTLS against WisdomTree (Cloudflare test) | ✅ CONFIRMED — 200 OK, full page, all data sections present |
| Country allocation in raw HTML (CycleTLS response) | ✅ CONFIRMED — HTML table with actual data, not empty |
| Market cap in raw HTML (CycleTLS response) | ✅ CONFIRMED — HTML table with actual data |
| Fund characteristics in raw HTML (CycleTLS response) | ✅ CONFIRMED — HTML table with actual data |
| Check main page CSV for ticker column | ❌ Only 3 columns: date, Weight, Security Description — no ticker |
| Check "all-holdings" modal for ticker data | ✅ CONFIRMED — embedded JSON with `IdentifierTicker` (e.g. "NVDA UQ") |
| `curl` on modal URL | ❌ Cloudflare blocked — same protection as main site |
| CycleTLS on modal URL | ✅ CONFIRMED — 200 OK, full JSON with ticker data |

---

## Key Findings Summary

1. **Cloudflare protection**: All WisdomTree domains (wisdomtree.eu, dataspanapi.wisdomtree.com) are behind Cloudflare. Plain `net/http` or `curl` requests are blocked. **CycleTLS bypasses Cloudflare successfully** — confirmed 2026-05-26.

2. **Inline data**: Holdings, NAV, sectors, themes, fund info, AUM, TER, inception date are all embedded in the raw HTML as CSV strings in JavaScript variables. Extractable with regex + CSV parsing.

3. ~~JS-rendered data~~: Country allocation, market cap, and fund characteristics **ARE in raw HTML** as standard HTML tables. Previously misidentified as JS-rendered. Extractable with regex on HTML tables.

4. **No dataspanapi endpoints for country/market cap/characteristics**: The only known dataspanapi endpoint is `/funddetails/calendar_year_performance`. Country allocation, market cap, and fund characteristics have no dedicated API.

5. **Existing infrastructure**: The project already uses CycleTLS (via go-yfinance) for Yahoo Finance. The same approach works for WisdomTree — **all data**, not just inline CSV variables.

6. **Holdings ticker data**: The main page CSV has no ticker column. Ticker/symbol data is available only via the all-holdings modal page (separate URL extracted from `data-href` on main page). The modal contains an embedded JSON array with `IdentifierTicker` (Bloomberg-style, e.g. `"NVDA UQ"`), `IdentifierName`, `Weight`, `CountryCode`, etc. CycleTLS bypasses Cloudflare on the modal page as well.

## Next Steps for Go Implementation

### Phase 1: All Data (CycleTLS + regex/CSV + HTML table parsing)

1. ~~Test CycleTLS against WisdomTree~~ — **✅ CONFIRMED: CycleTLS bypasses Cloudflare successfully**
2. ~~Implement HTTP fetcher using CycleTLS~~ — ✅ Done
3. ~~Implement regex extraction for each data variable~~ — ✅ Done
4. ~~Implement regex on HTML tables for country allocation, market cap, fund characteristics~~ — ✅ Done
5. ~~Write CSV parser for each data type~~ — ✅ Done
6. ~~Add unit tests~~ — ✅ Done
7. ~~Fetch all-holdings modal for ticker data~~ — ✅ Done (extract modal URL, fetch via CycleTLS, parse embedded JSON)
8. ~~Strip Bloomberg market suffix from tickers~~ — ✅ Done (e.g. "NVDA UQ" → "NVDA")

### Decision Point

**CycleTLS + HTML parsing is the complete solution.** No chromedp, no headless browser, no external service needed. All data (including country allocation, market cap, fund characteristics, and holdings tickers) is extractable.
