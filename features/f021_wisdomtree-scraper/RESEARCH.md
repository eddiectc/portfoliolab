# WisdomTree New Site — Research Findings (2026-08-30)

**Scope**: Full investigation of the restructured WisdomTree website and the data needed to rebuild the extractor.
**Companion doc**: `v1/RESEARCH.md` documents the **old** (Sitecore) site, archived after the 2026-08-30 relaunch. Everything below is for the **current** site.
**Samples**: raw captures in `samples/` (pages, API responses, extracted tables).

## TL;DR

WisdomTree replaced its Sitecore site (embedded `var fundInfo = ...` JS variables + CSV strings) with a
**Next.js (App Router) site**. All existing parsers are broken — none of the old data variables exist anymore.
The rebuild needs:

1. **`wtClassID`** (numeric fund-class ID, e.g. QGRW = `49567173`) — extract from the page, then
2. **The JSON APIs** for everything an API exists for (API-first — user preference):
   - `GET /api/fund-holdings/{id}` — holdings (works for UCITS **and** US funds)
   - `GET /api/fund-history/{id}` — NAV + AUM + shares history (UCITS **and** US)
   - `GET /api/product-charts/{id}/growth-10k` / `premium-discount` — performance charts (US funds **only**)
3. **The React Flight payload** embedded in the page (`self.__next_f.push(...)`) for the remaining section tables
   (Product Overview, NAV/AUM, Fees, Country, Market Cap, Fund Characteristics, Sector/Theme breakdowns, Listings)
   — there is **no API** for these; the flight payload is the only source (see §5.4).

No headless browser needed. The existing CycleTLS client passes Cloudflare for both the page and the APIs.

**URL matching decision (user, 2026-08-30)**: the matcher accepts **only the new URL format**
(`wisdomtree.com/{region}/products/{asset-class}/{slug}`, §1a). The old `.eu` URLs are dropped — the user will
update stored `data_source_url` values from the portal before the extractor change ships.

## 1. What changed

| | Old site | New site |
|---|---|---|
| URLs | `wisdomtree.eu/en-gb/etfs/thematic/wmgt---...` | **`wisdomtree.com/{region}/products/{asset-class}/{slug}`** — two flavors, both verified against the official sitemaps (§1a) |
| Stack | Sitecore CMS, server-rendered HTML | Next.js App Router, React Server Components (RSC) + client hydration |
| Data in page | JS variables: `fundInfo`, `fundHoldingsData`, `fundSectorsData`, `fundThemeData`, `fundMarketData`, `fundBenchmarks` (CSV strings) | React Flight payload chunks: `self.__next_f.push([1,"..."])` — JS-escaped RSC arrays containing structured JSON-like props |
| Holdings with tickers | separate all-holdings modal page (CSV) | `GET /api/fund-holdings/{wtClassID}` (JSON) |
| NAV history | `fundMarketData` CSV in page | `GET /api/fund-history/{wtClassID}` (JSON, incl. AUM) |
| Page size | ~480 KB | ~4.2–5 MB (flight data is verbose) |

### 1a. New URL format (verified via official sitemaps, 2026-08-30)

Sitemaps (all pass CycleTLS): `wisdomtree.com/sitemaps/products/us/sitemap.xml` (210 URLs) and
`/sitemaps/products/eu/sitemap.xml` (6 688 URLs).

| Region group | Pattern | Example | Slug type |
|---|---|---|---|
| US | `/us/products/{asset-class}/{ticker}` | `https://www.wisdomtree.com/us/products/equity/ezm` | **ticker** |
| EU/GB | `/{region}/products/{asset-class}/{name-slug}` | `https://www.wisdomtree.com/gb/products/equities/wisdomtree-us-quality-growth-ucits-etf---usd-acc` | **name slug** (lowercase, `---` = currency/variant suffix) |

Asset-class vocabulary differs per region (observed in sitemaps):
- US: `equity`, `fixed-income`, `crypto`, `currency`, `alternative`, `capital-efficient`
- EU: `equities`, `fixed-income`, `commodities`, `currencies`, `alternatives`, `digital-assets`

Proposed matcher regex (new format **only**):

```go
wisdomtreeURLRe = regexp.MustCompile(`^https?://(?:www\.)?wisdomtree\.com/[a-z]{2}/products/[a-z-]+/[a-z0-9-]+/?$`)
```

Notes:
- Sitemap URLs have no trailing slash; browsers append one — allow both.
- Region is any 2-letter code (`us`, `gb`, `de`, …) and is irrelevant to parsing (same flight data).
- **Old `.eu` URLs**: verified on 2026-08-30 that `wisdomtree.eu/en-gb/...` still serves the new site
  with identical data (same `wtClassID`, same tables). They keep working technically, but per the user decision the
  extractor will **not** match them; the user updates stored URLs from the portal before the change ships.
  (`.com/etfs/...` without a region is dead → connection failure.)

## 2. Cloudflare

- Plain `curl` (any UA): **403 challenge** on `wisdomtree.com` pages **and** `/api/*` endpoints, and on old `.eu` URLs.
- Static assets (`/_next/static/...`): not challenged — plain curl works (used to map the JS chunks).
- **The project's CycleTLS client (Chrome 129 JA3, HTTP/1, browser headers) passes everything**: fund pages,
  `.eu` legacy pages, and both JSON APIs. No changes to `client.go` needed.

## 3. `wtClassID` — the numeric fund-class ID

Appears hundreds of times in the flight payload, in the form `\"wtClassID\":49567173` (backslash-escaped quotes,
because it sits inside a JS string literal in `self.__next_f.push`).

Go regex (works on the raw fetched body):

```go
wtClassIDRe = regexp.MustCompile(`wtClassID\\?":(\d{6,10})`)
```

Notes:
- QGRW = `49567173`, WMGT = `46987205`, EZM (US) = `1000518`.
- It is **per share class**, not per fund — the same fund with multiple share classes (Acc/Dist, currencies) has
  different IDs. The stored `data_source_url` therefore pins exactly one class, which is what we want.
- First match is fine; all occurrences on a single page are the same ID (verified: 2027 identical matches on WMGT).

## 4. Page structure (new site)

The HTML is one giant document where all meaningful data lives in **React Flight payload** string chunks:

```js
self.__next_f.push([1,"0:{\"id\":\"9c\",\"chunk\":\"[\\\"$\\\",\\\"div\\\",null,{...}]\"}"])
// ... many chunks; each is a JS string that after unescaping contains RSC arrays
```

Decoding procedure (verified with Python, same applies in Go):
1. Regex all `self\.__next_f\.push\(\[1,"((?:[^"\\]|\\.)*)"\]\)` chunks.
2. JS-unescape each string (standard `\uXXXX`, `\"`, `\\`, `\/`).
3. Decode the resulting RSC arrays. Data of interest appears as plain JSON objects inside component props.

### 4a. Section tables (the workhorse)

Every data section is a table object:

```json
{"columns":[{"id":"0","name":"Product Overview","isRowHeader":true},{"id":"1","name":"As of 28/08/2026","isRowHeader":true}],
 "rows":[{"0":"ISIN","1":"IE000YGEAK03","id":1,...},{"0":"Base Currency","1":"USD",...}],
 "ariaLabel":"Overview Table","variant":"product"}
```

`rows[].{0}` = label, `rows[].{1}` = value (numeric string keys). Tables are identified by their `ariaLabel`
(e.g. `"Overview Table"`, `"Country Allocation Table"`) and/or first column name — **not** by position, because the
table set varies by fund and by region. UCITS pages (QGRW, WMGT) carry 12 tables:

| Table | Content |
|---|---|
| Product Overview | ISIN, Asset Class, Base Currency, Inception Date, Dividend Frequency, Use of Income (Acc/Dist) |
| **Net Asset Value** | NAV (e.g. `US$42.974`), Daily Change, Daily return, **Total AUM of fund** (`US$47,442,965`), Issuer AUM |
| Structure | Legal Form, Listing Status, etc. |
| Further Legal and Tax Information | ISA, UCITS, etc. |
| Key Service Providers | Custodian, Administrator, etc. |
| Fees | Total expense ratio (TER), e.g. `0.33%` |
| Market Capitalisation | Fund-level market cap breakdown (e.g. `Total Market Capitalisation ($ Trillion)`) |
| Fund characteristic | Dividend Yield, P/E, P/B, P/S, P/CF (5 fields, same as old site) |
| Country | **Country allocation**: `United States` → `99.54%` (percent strings) |
| (second) "Country" | **Listings & codes**: flag + Exchange, Ccy, Ticker, Class, Full Ticker, MIC, ISIN, SEDOL, inception |
| Ex-Dividend Date | single row of recent ex-div dates + amounts |
| Index Details | Index Name, etc. |

US pages (EZM) carry a **superset/different set** — additionally: `Trading Information Table`, `Closing Market
Values Table`, `Total Returns History Table`, `Month End Performance (Sell Hold) Table`, `Quarter End Performance
(Sell Hold) Table`, `All Hedge Ratios Table`, `Recent Distributions Table`, plus embedded chart datasets
(`Historical AUM area chart`). Date format also differs by region: UCITS `As of 28/08/2026` vs US `As of 8/27/2026`.
The parser must select tables by name and parse both date formats.

The "As of" date for the whole page is the second column header of these tables (`As of 28/08/2026`).

### 4b. Section components (Sector / Theme breakdowns)

Server-rendered as flight components (`$L99` / `$L98`) with props:

```json
{"sectionId":"Sector Breakdown","sectionKey":"sector-breakdown","sectionTitle":"Sector Breakdown",
 "ranking":[{"constituentName":"Information Technology","pctWeight":0.584,...,"constituentRanking":1,
            "rankClassification":"Sector","dt":"...","wtClassID":49567173,"wgtSumCheck":1}],
 "holdings":{"Information Technology":[{...per-sector holdings...}, ...]}}
```

- `pctWeight` is a **fraction** (0.584 = 58.4%) — same convention as old-site sectors.
- `rankClassification` is `"Sector"` for the Sector Breakdown section and `"EU Thematic Bucket"` for the Theme
  Breakdown section (same component shape).
- **Presence varies by fund**: QGRW has Sector Breakdown but **no** Theme Breakdown / Market Cap sections of its
  own (fund has no theme data); WMGT has both. Parsers must keep the existing *optional-section → `nil, nil`*
  convention.

### 4c. Embedded holdings

The page also embeds the holdings list in flight data (QGRW: 109 rows) — but the **API is better** (see below),
so use the API.

## 5. JSON APIs (all pass CycleTLS, no auth)

Base: `https://www.wisdomtree.com/api/...`

### 5.1 `GET /api/fund-holdings/{wtClassID}` — holdings (replaces holdings modal page)

JSON array, newest as-of date (single `dt` per response). Fields:

| field | type | notes |
|---|---|---|
| `dt` | date | e.g. `2026-08-27` (typically 1 day behind page "as of") |
| `wtClassID` | int | |
| `fundTicker` | string | e.g. `QGRW LN` |
| `sectorName` | string | **sector per holding** — can aggregate into sector weights |
| `assetGroup` | string | e.g. `EQUITY` / cash |
| `securityTicker` | string | **exchange ticker** (old site only had this on the modal page) |
| `securityName` | string | |
| `shares` | int | |
| `marketValueBase` | number | market value in fund base currency |
| `wgt` | number | **fraction** (0.00044 = 0.044%); sums to 1.0000 |
| `figi` | string | FIGI identifier |
| `checkSumWgtAssetGroup`, `checkSumWgtEntity` | number | internal checksums, ignore |
| `extraDataJSON` | null/string | |
| `descriptorA` | string | |

Verified: QGRW 101 records, Σ`wgt` = 1.0000, includes a `CASH W-O` row. WMGT response 350 KB (same schema).
Cash filtering by name keywords (existing convention) still applies.

Verified for UCITS (QGRW 101, WMGT 920) **and** US (EZM) funds — same schema.

### 5.2 `GET /api/fund-history/{wtClassID}` — NAV + AUM history (replaces `fundMarketData`)

**Default view (no params)** — 601 records for QGRW, **ascending** by date, since inception:

```json
{"aum":47442.9648,"dt":"2026-08-28T00:00:00.000Z","name":"WisdomTree US Quality Growth UCITS ETF - USD Acc",
 "nav":42.9737,"navDelta":0,"navDeltaPCT":0,"navPrevious":42.9737,"relatedTicker":null,
 "sharesOutstanding":1104000,"ticker":"QGRW LN"}
```

`aum` is in **millions** (47442.9648 → $47,442,965 — exactly matches the Net Asset Value table "Total AUM of fund").
One call gives NAV history **and** AUM history **and** shares outstanding.

**`?view=navHistoryModal`** — slim variant, **descending**: `{dt, nav, closePrice, premiumDiscountToNav, pdIndicator}`
(`closePrice`/`premiumDiscountToNav` are null for thinly-traded classes like QGRW; populated where liquid).

**`?dataset=premiumDiscount&format=json`** — returns `[]` for QGRW (no P/D data). Used by the performance chart;
skip for now.

Also present in the JS (untested, not needed): `?${params}` chart view and `/historical-data-export` CSV endpoint.

Verified for UCITS (QGRW, WMGT) **and** US (EZM, history since 2007) funds — same schema.

### 5.3 `GET /api/product-charts/{wtClassID}/{type}` — performance charts (US funds only)

Type values (from the performance-chart JS): **`growth-10k`** and **`premium-discount`**.

Response: `{"chartString":"<CSV>"}` — CSV rows embedded in a JSON string:

- `growth-10k`: `date,fund_ticker,close_price_adj,volume_adj,nav,bmk_ticker_A,price_level_A,bmk_ticker_B,price_level_B,bmk_ticker_C,price_level_C,bmk_ticker_D,price_level_D,uv10KMP,uv10KNAV` (daily since inception; benchmarks pre-baked per fund)
- `premium-discount`: `date,BpsDiff,BpsDiffPct`

Availability matrix (live, 2026-08-30):

| Fund | growth-10k | premium-discount |
|---|---|---|
| QGRW (UCITS) | 404 `Product chart not found.` | 404 |
| WMGT (UCITS) | 404 | 404 |
| EZM (US) | ✅ 1 MB | ✅ 150 KB |

Semantics: **400** = unknown type value; **404** = valid type, no chart for this share class. Treat 404 as the
*optional section absent* case (`nil, nil`), not an error. These charts exist for US-domiciled funds only; UCITS
pages render performance differently (Total Returns table in flight). Whether we parse these is a spec decision —
they carry strictly more data than the old site ever exposed.

### 5.4 Complete API surface (exhaustive)

All `/api/` endpoints referenced anywhere in the site's JS chunks:

| Endpoint | Verdict |
|---|---|
| `/api/fund-holdings/{id}` | **use** — holdings |
| `/api/fund-history/{id}` (+ `?dataset=`, `?view=`, `?${params}`, `/historical-data-export`) | **use** — default view (NAV+AUM) |
| `/api/product-charts/{id}/growth-10k` / `premium-discount` | **use** (US funds; optional) |
| `/api/download-file?id=` | not needed (document downloads) |
| `/api/media-library/video-playback-info` | not needed |

There is **no API** for the section tables (Overview, Fees, NAV, Country, Market Cap, Characteristics, Listings)
or for the Sector/Theme breakdowns — those exist only in the flight payload. API-first is therefore applied to the
maximum extent the site allows: 3 API calls per extraction for UCITS, +2 for US funds, flight payload for the rest.

## 6. Data map: old parser → new source

| Old parser (old site) | New source |
|---|---|
| `ParseFundInfo` (ISIN, TER, inception, base ccy, …) | **Product Overview** + **Fees** + **Structure** tables (flight) |
| `ParseHoldings` (names + weights only) | **`GET /api/fund-holdings/{wtClassID}`** — strictly better: adds `securityTicker`, `figi`, `sectorName`, `shares`, `marketValueBase` |
| `ParseNavHistory` (`fundMarketData` CSV) | **`GET /api/fund-history/{wtClassID}`** default view (nav, aum, sharesOutstanding) |
| `ParseSectors` (`fundSectorsData` CSV) | **Sector Breakdown** `ranking` (pctWeight fraction, flight — no API) — or aggregate `sectorName` from the holdings API |
| `ParseThemes` (`fundThemeData` CSV) | **Theme Breakdown** `ranking` (same component, `rankClassification="EU Thematic Bucket"`, flight — no API); absent on some funds → `nil, nil` |
| `ParseCountryAllocation` (HTML table) | **Country (Weight %)** table (flight — no API) — cleaner than HTML |
| `ParseMarketCap` (HTML table) | **Market Capitalisation** table (flight — no API) |
| `ParseFundCharacteristics` (HTML table) | **Fund characteristic** table (flight — no API) — same 5 fields |
| `ParseAsOfDate` (HTML "as of") | Table column header `As of …` (UCITS `28/08/2026` / US `8/27/2026` — two formats) and/or holdings API `dt` |
| — (AUM was scraped from HTML NAV table) | latest `aum` from **fund-history API** (preferred) or **Net Asset Value** table (`Total AUM of fund`, flight) |
| — (new) | **Listings & codes** table: exchange, ccy, ticker, class, MIC, ISIN, SEDOL, inception (flight — no API) |
| — (new, spec decision) | **Performance** (`growth-10k`) + **Premium/Discount** history via **`/api/product-charts`** — US funds only; 404 → `nil, nil` |

Legend: **API** = JSON endpoint (preferred), **flight** = React Flight payload (only available source).

## 7. Cross-checks (live, 2026-08-30)

| Data point | Source A | Source B | Consistent |
|---|---|---|---|
| QGRW AUM | NAV table `US$47,442,965` | fund-history latest `aum=47442.9648` (millions) | ✅ |
| QGRW NAV | NAV table `US$42.974` | fund-history latest `nav=42.9737` | ✅ |
| QGRW holdings | page-embedded 109 rows | API 101 records, Σwgt=1.0 | ✅ (page embeds extra aggregate rows) |
| QGRW top sector | Sector Breakdown `Information Technology 58.40%` | (aggregating API `sectorName`) | ✅ |
| WMGT AUM | NAV table `US$66,226,762` | fund-history latest `aum=66226.7618` (millions) | ✅ |
| WMGT holdings | — | API 920 records, Σwgt = 1.0000 | ✅ |
| WMGT wtClassID | old `.eu` URL page | new `/gb/products/...` page | ✅ same ID, same data |

Fund-level sanity values: QGRW — TER 0.33%, inception 16 Apr 2024, base USD, ICAV, 101 holdings, top country
US 99.54%. WMGT — NAV US$42.156, AUM US$66,226,762, TER 0.50%.

## 8. Sample files (`samples/`)

| File | Content |
|---|---|
| `qgrw_new.html` / `wmgt_new.html` | Full new-site pages (`.com/gb/products/...`), fetched with CycleTLS |
| `qgrw_gb.html` | Same QGRW page re-fetched (user's exact URL) |
| `wmgt_oldurl.html` | New-site WMGT page served at the **old** `.eu` URL (proves legacy URLs work) |
| `eu_listing_library_entry_{qgrw,wmgt}.json` | Extracted Listings & codes tables |
| `holdings_api_meta.txt`, `holdings_api_qgrw_first3.json` | `/api/fund-holdings` schema + sample records |
| `holdings_api_{qgrw,wmgt}_full.json` | **Full** `/api/fund-holdings` responses (QGRW 101 recs / WMGT 920 recs) — test fixtures |
| `fund_history_default_meta.txt`, `fund_history_default_qgrw_sample.json` | `/api/fund-history` default view schema + sample |
| `nav_history_api_meta.txt`, `nav_history_api_qgrw_first5.json` | `?view=navHistoryModal` variant |
| `fund_history_default_{qgrw,wmgt}_full.json` | **Full** `/api/fund-history` default-view responses (601 / 691 recs) — test fixtures |
| `sector_section_{qgrw,wmgt}.json` | Sector Breakdown flight components |
| `theme_section_wmgt.json` | Theme Breakdown flight component (WMGT only) |
| `qgrw_table_*.json` / `wmgt_table_*.json` | Extracted flight tables: product_overview, net_asset_value, fees, market_capitalisation, fund_characteristic, country |
| `ezm_page.html` | Full **US** fund page (flight-payload structure differs — superset of tables, `8/27/2026` date format) |
| `holdings_api_ezm_full.json`, `fund_history_default_ezm_full.json` | Core APIs for a US fund (same schema as UCITS) — test fixtures |
| `product_charts_ezm_{growth-10k,premium-discount}.json` | `/api/product-charts` responses (US fund; UCITS returns 404) — test fixtures |

## 9. Open questions / risks

1. **Undocumented APIs** — `/api/fund-holdings` and `/api/fund-history` are reverse-engineered from minified JS.
   They can change without notice. Mitigation: keep flight-embedded data (holdings are in the page too) as a
   fallback path, and surface API failures distinctly in errors/logs.
2. **Fund-to-fund section variance** — confirmed: QGRW lacks Theme Breakdown. Keep `nil, nil` for optional
   sections (existing convention) and do not hard-fail on missing sections.
3. **`wtClassID` is per share class** — if a user's fund page later changes class (e.g. re-list), the stored URL
   changes too; extraction re-derives the ID from the page each time, so this self-heals.
4. **Flight payload format** — depends on Next.js internals. The table/section shapes are stable within the app;
   the push-chunk mechanism is standard Next.js App Router and low-risk, but parsing must be tolerant
   (scan all chunks, match by known table/section names, ignore unknowns).
5. **URL matching (decided)** — matcher accepts **only** the new format, any 2-letter region, both slug types
   (ticker / name-slug); old `.eu` URLs no longer matched. User updates stored `data_source_url` values from the
   portal before the extractor change is deployed. Region is irrelevant to parsing; US and EU pages differ in
   available tables (handled by name-based table selection, §4a).
6. **Performance/P-D scope (spec decision)** — `/api/product-charts` data exists for US funds only. Options: (a)
   include as new optional sections, (b) defer. They are additive — old site had nothing equivalent.
7. **Rate limiting** — each extraction now needs page + 2–4 API calls. The client's 1 s min-delay already exists;
   consider whether background bulk refresh of many symbols needs a longer delay.
8. **`dt` mismatch** — holdings API `dt` (2026-08-27) lags the page "As of" (2026-08-28) by one day. Decide: use
   page "As of" for `extractor_as_of_date` (recommended) and keep holdings `dt` as the holdings section's own date.

## 10. Recommended rebuild approach (for the spec/plan phase)

1. **URL matcher** → new format only (§1a regex); drop `.eu` support. User updates stored URLs from the portal
   first (order matters: update URLs → deploy new matcher, or extractors of not-yet-updated symbols break).
2. **Fetch page** with existing CycleTLS client → extract `wtClassID` (regex on raw body).
3. **API-first** (same client, same rate limiting): `fund-holdings` + `fund-history` (always), then
   `product-charts/growth-10k` + `premium-discount` where relevant (US funds; 404 → `nil, nil`).
4. **Decode flight payload** (chunk regex + JS unescape + name-based table/section selection) for everything
   without an API: Overview, Fees, NAV/AUM, Country, Market Cap, Characteristics, Listings, Sector/Theme.
5. **Rewrite parsers** against the new shapes; keep the existing `extractor.ExtractResult` contract and the
   optional-section `nil, nil` semantics so the service layer (Task 4 of the original plan) is untouched.
6. **Tests**: unit tests with saved samples from `samples/` (table JSON, section JSON, API JSON,
   flight-chunk fixtures, UCITS + US variants); keep the `SetFetchFunc` injection for the page + add injection
   points for API bodies so integration tests stay hermetic.
