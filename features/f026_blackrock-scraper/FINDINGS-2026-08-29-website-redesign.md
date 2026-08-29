# FINDINGS — 2026-08-29: iShares Website Redesign Breaks BlackRock Extractor

Status: **Resolved 2026-08-29** — Option 2 (full JSON API migration) approved by user and implemented. See `NOTES.md` → “2026-08-29 Implementation Notes”. Open questions below remain open for follow-up.

## Symptom

All `blackrock` extractor refreshes fail at Phase 1 (atomic rejection → no data stored):

```
level=WARN msg="failed to refresh symbol details" symbol=IWMO.L marketDataSymbol=IWMO.L
error="fetch symbol details for IWMO.L: extract from https://www.ishares.com/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf:
extract from \"...\" (blackrock): phase 1 — parse fund profile: fund profile data missing (no ISIN or AUM found)"
```

Last successful IWMO.L refresh: **2026-07-26** (as-of date 2026-07-23). The page layout changed sometime between 2026-07-26 and 2026-08-29. **Every iShares symbol in the portfolio is affected**, not just IWMO.L.

## Root Cause

iShares re-released their product pages on a new Astro-based build ("onedes" design system). The server-rendered HTML now uses a different table structure for Key Fund Facts, and the holdings section is fully client-side rendered (the old CSV download link/endpoint is gone).

## What Changed on the Page (verified against live HTML, 2026-08-29)

Test page: `https://www.ishares.com/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf` (fetches fine with plain curl + browser UA, HTTP 200, ~2.9 MB — no bot blocking).

### Per-parser status

| Parser | Old format | New format | Status |
|---|---|---|---|
| `ParseFundIdentity` | `<h1>` tag | `<h1>` still present ("iShares Edge MSCI World Momentum Factor UCITS ETF") | ✅ still works |
| `ParseFundProfile` / `ParseFundCharacteristics` | legacy `<td>Key</td><td>Val</td>`, then div-based `<div class="product-data-item col-xxx">…<div class="data">VAL</div>` | table rows: `<tr class="data-item … col-xxx" data-id="keyFundFacts-row-xxx">…<th class="caption">…<span class="label">KEY</span>…</th><td class="data …">VAL</td>` | ❌ **the reported error** — the `col-xxx` marker still exists, but the value regex only matches `<div class="data">` (exact class) |
| `ParseAsOfDate` | `class="as-of-date"` (exact class match) | `class="as-of-date oneds-body-s-compact"` (extra classes) | ❌ exact-match regex fails; the dates themselves are present (e.g. `as of 28/Aug/2026`) |
| `ParseComponentID` + Phase 2 holdings CSV | `/{componentId}.ajax?fileType=csv&fileName=…` link in HTML | Holdings section is **client-side rendered** (`data-componentname="holdings"` container is empty server-side). No `.ajax` link in HTML. Probing the old component ID (`1506575576011.ajax?fileType=csv…`) now returns the product page HTML (HTTP 200) instead of CSV | ❌ Phase 2 (holdings, sector/country allocation, CSV as-of date) is dead |

### New Key Fund Facts row structure (sample)

```html
<tr class="data-item oneds-body-m-compact col-isin" data-id="keyFundFacts-row-isin">
  <th class="caption" tabindex="-1">
    <div class="caption-wrapper">
      <span class="label" data-id="keyFundFacts-isin-label" webqc-datapoint="keyFundFacts-isin-label">ISIN</span>
    </div>
  </th>
  <td class="data oneds-body-l-bold" webqc-datapoint="keyFundFacts-isin" tabindex="-1" data-id="keyFundFacts-isin-data">IE00BP3QZ825</td>
</tr>
```

- Same `col-xxx` class names as before (35 rows on this page), e.g. `col-isin`, `col-totalNetAssets`, `col-inceptionDate`, `col-bbeqtick`, `col-indexSeriesName`, `col-sfdr`, `col-numHoldings`, `col-priceEarnings`, …
- Values now in `<td class="data …">` (with extra classes), labels in `<span class="label">`.
- Values observed: Net Assets `USD 6,022,456,334`, Inception `03/Oct/2014`, Bloomberg Ticker `IWMO LN`, Benchmark `MSCI World Momentum index (Net)`, SFDR `Other`.
- ⚠️ Subtlety for any marker-based parse: `col-totalNetAssets` is a string prefix of `col-totalNetAssetsFundLevel` — marker lookups must match the full class token (e.g. `col-totalNetAssets"` with the closing quote), not a bare `strings.Index` on `col-<name>`.

## New Data Source Discovered: Product Data JSON API

The site's JS (Astro chunks under `/_astro/…`) loads all fund data from a Varnish-backed API. Reverse-engineered from the page's embedded JSON config (`apiHost`, `product_data_api` keys) and the JS chunks (`ProductPageAjaxUtils`, `useFetchDataV2`, `useIsProductPagesApiUrl`). **Verified working on 2026-08-29.**

### Endpoint

```
GET {apiHost}?slice=emea&appSubType=ISHARES&appType=PRODUCT_PAGE&component={component}
       &locale=en_GB&portfolioId={portfolioId}&targetSite=ishares-uk&userType=individual
       &excludeContent=true&asOfDate=&includeConfig=true

Headers (required):
  Accept: application/json
  x-application-id: pp-ui-csr
  (Referer: product page URL — used in verification; not confirmed mandatory)
```

- `apiHost` is embedded per-page in the product page HTML. On this page:
  `https://www.blackrock.com/varnish-api/uk-retail01-product-data/product-data/api/v2/get-product-data`
  — the site JS rewrites `blackrock.com/` → `ishares.com/` when running on ishares.com, so call the `www.ishares.com` variant.
  The `uk-retail01-product-data` path segment is environment-specific → **parse `apiHost` from the page HTML rather than hardcoding**; this keeps the approach market-agnostic (UK/IE/NL/…).
- `locale=en_GB`, `targetSite=ishares-uk`, `userType=individual`, `appSubType=ISHARES` all taken from the page config. `userType` matches the `/individual/` segment in the product URL. `slice` (`emea`/`us`) did not change the response for this product but is part of the standard request.
- `portfolioId` = the numeric ID in the product URL (`270051`).
- `locale=en` (without region) is rejected with a 400 ("should only contain alphabetical characters" — quirk; use `en_GB`).

### Components / response shape

Response is a JSON envelope: `componentsByNameMap → {component} → containersByNameMap → {container} → dataPointsByNameMap → {field} → {formattedValue, value, …}`.

| component | container | payload |
|---|---|---|
| `keyFundFacts` | `default` | 29 datapoints, one `formattedValue` each. Covers **every** field we currently scrape from HTML: `isin`, `totalNetAssets`, `inceptionDate`, `bbeqtick`, `indexSeriesName`, `sfdr`, `assetClass`, `useOfProfitsCode`, `domicile`, `rebalanceFrequency`, `fundmanager`, `fundCustodian`, `productStructure`, `fundMethodologyTypeCode`, `issuingCompany`, plus extras (`sharesOutstanding`, `baseCurrencyCode`, `fiscalYearEndDate`, `fundAdministrator`, `esmaBenchmarksByDataPointName`, …) |
| `holdings` | `all` | **373 holdings** in column-oriented form — each field is an array aligned by row index: `ticker`, `issueName`, `holdingPercent`, `sectorName`, `countryOfRisk`, `marketValue`, `notionalValue`, `unitsHeld`, `unitPrice`, `exchange`, `isin`, `assetClass`, `marketCurrencyCode`, plus `asOfDate` (`formattedValue` = `27/Aug/2026`, `value` = `20260827`) |

`holdings` also has `metadata` / `documents` / `download` containers (the `download` container had no datapoints at time of verification).

### Notes on data quality

- Holdings JSON is **richer than the old CSV**: it includes per-holding ISIN (CSV had `"-"`), and `asOfDate` is authoritative for the holdings snapshot.
- The characteristics fields (`numHoldings`, `priceEarnings`, `priceBook`, `threeYrBetaFund`, `volatilitySourced3YrAnnualized`) are NOT in `keyFundFacts`; they appear in the HTML as data-item rows (likely served by a separate component, e.g. the "fundamentals-and-risks" JS chunk). If we migrate to full JSON, either keep HTML parsing for those few fields or identify their component name.

## What Is NOT a Problem

- **ISIN semantics**: the page shows `IE00BP3QZ825` (fund-level primary / EUR A12ATF series) rather than the LSE-listed USD series `IE00B03XCT69`. The app **already stores `IE00BP3QZ825`** (set during the last successful refresh on 2026-07-26), and RESEARCH.md documents the same ISIN — pre-existing, not a regression. No action required unless we want to special-case the LSE-listed series.
- No bot blocking / auth on the page or API as of 2026-08-29.
- `ParseFundIdentity` (h1) unaffected.

## Impact

- Symbol detail refresh is fully broken for all iShares/BlackRock symbols (profile + holdings + allocations + as-of date).
- Stale data continues to be served from the last good snapshot (IWMO.L: fetched 2026-07-26).
- Market prices for these symbols still refresh via yfinance — only the *details* are stale.

## Proposed Fix Options

1. **Minimal patch** — keep HTML parsing for Phase 1:
   - Extend the key-value parser to match the new `<tr class="data-item col-xxx">…<td class="data …">VAL</td>` layout (keep legacy `<td>/<div>` fallbacks; beware the `col-totalNetAssets` prefix collision).
   - Fix `ParseAsOfDate` regex to allow extra classes (`class="as-of-date[ "]`).
   - Replace only the CSV Phase 2 with the JSON API `component=holdings`.
2. **Full JSON migration (recommended)** —
   - Keep the HTML fetch only for the fund name (`<h1>`) + discovery of `apiHost` (and the characteristics fields, if we can't find their JSON component).
   - Parse the profile from `component=keyFundFacts` and holdings from `component=holdings`.
   - Pros: far less regex fragility against future re-designs, richer data (per-holding ISIN), one JSON fetch instead of the dead CSV. Cons: larger change, must parse `apiHost` from page HTML per request.

Both options: add parser tests for the new shapes; keep existing old-layout tests (other markets or cached pages may still use them).

## Open Questions (for user)

- Option 1 or Option 2? (Recommendation: **Option 2**.)
- Keep the characteristics fields (`numHoldings`, P/E, P/B, beta, volatility) — HTML-parse them or find their JSON component?
- Any interest in fixing the LSE-series ISIN (`IE00B03XCT69` vs page's `IE00BP3QZ825`)? (Currently out of scope; pre-existing.)

## Verification Notes (reproducible)

```bash
# Product page (HTML, ~2.9 MB, HTTP 200)
curl -sL -A "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36" \
  "https://www.ishares.com/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf"

# JSON API — key fund facts
curl -s -H "Accept: application/json" -H "x-application-id: pp-ui-csr" \
  "https://www.ishares.com/varnish-api/uk-retail01-product-data/product-data/api/v2/get-product-data?slice=emea&appSubType=ISHARES&appType=PRODUCT_PAGE&component=keyFundFacts&locale=en_GB&portfolioId=270051&targetSite=ishares-uk&userType=individual&excludeContent=true&asOfDate=&includeConfig=true"

# JSON API — holdings (same params, component=holdings)
```

JS chunks inspected (2026-08-29): `/_astro/product-pages-*.js` → `holdings-clientonly-v2/v3-*.js` → `useFetchDataV2-*.js` → `ProductPageAjaxUtils-*.js` (URL builder + `x-application-id: pp-ui-csr` header), `useIsProductPagesApiUrl-*.js` (path suffix `/product-data/api`).
