# Notes: BlackRock/iShares Data Extractor

## Decisions
- 2026-06-01: Changed `CharacteristicsFieldsMask` from `uint16` to `uint32` — the original 14-bit mask would overflow when adding 3 new BlackRock fields (17 total entries, `1 << 16` overflows `uint16`).
- 2026-06-01: `jsonNode` uses a custom `UnmarshalJSON` to handle mixed string/object cells in the holdings JSON API (some cells are plain strings like ticker/name, others are `{"display":..., "raw":...}` objects).
- 2026-06-01: `parseFloatValue` strips parenthetical date suffixes BEFORE stripping `%` — the `%` can appear mid-string (e.g. `16.62% (as of 30/Apr/2026)`), so `TrimSuffix("%")` alone would miss it.
- 2026-08-29: `AnnualExpenseRatio` convention confirmed as a **fraction** (0.0025 = 0.25%) — the web display (`symbol_details_web.go`) renders `value * 100`, and vanguard/wisdomtree store fractions. BlackRock's `ParseFundProfile` stored the raw percent number (displayed 25.00% for a 0.25% fund); now divides by 100. Existing DB rows keep the old value until the symbol is re-refreshed.

## Deviations from Plan
- Task 3: Test `TestExtractor_Extract_MissingAsOfDate` renamed to `TestExtractor_Extract_Cancellation` (context cancellation) — the as-of date test is covered by `TestParseAsOfDate` in parsers_test.go. The extractor-level test focuses on context cancellation instead.

## Future Improvements
- None yet.

## Task 6 Notes
- Holdings table: only the **Sector** column was added to the visible table (10 new fields available in display struct but not all shown). Adding AssetClass, MarketValue, Exchange, etc. would overcrowd the table for 350+ holding lists. The fields are available for future expandable-row or detail-popup features.

## Known Issues
- 2026-08-29 (resolved): iShares website redesign broke all `blackrock` extractor refreshes. Investigation + fix options in `FINDINGS-2026-08-29-website-redesign.md`. Fixed the same day via **Option 2 — full JSON API migration** (see 2026-08-29 implementation notes below).
- 2026-08-29 (open, separate extractor): `imgp` parser's `extractPercent` returns the raw percent number (e.g. `0.55` for "0.55%") and stores it directly in `AnnualExpenseRatio`, while the convention is a fraction (`0.0055`). Web display multiplies by 100, so imgp symbols likely show TER 100x too high (e.g. 55.00% instead of 0.55%). Not fixed — flag for follow-up; verify by refreshing an imgp symbol.

## 2026-08-29 Implementation Notes (website-redesign fix)

**Decision: Option 2 (full JSON API migration), per user.** The 2026-08 page redesign removed the embedded profile tables and the `.ajax` CSV holdings endpoint. Fund data now lives in a Varnish-backed product data JSON API whose base URL (`apiHost`) and request params (`productDataParams`) are embedded in the product page HTML; the portfolio ID comes from the product URL.

**Changes:**
- `json_parsers.go` (new): `ParseProductDataConfig` (page → API config; page is HTML-entity-escaped, so it is unescaped first), `ParsePortfolioID` (`/products/{id}/` URL segment), `BuildProductDataURL` (config + portfolio ID + component → API URL), `ParseFundProfileFromJSON` (`keyFundFacts`), `ParseHoldingsFromJSON` (`holdings`, column-oriented arrays + `asOfDate`). Shared helpers `dataPointString`/`dataPointStrings` handle scalar (string | object{display,value} | null) and column (array) formatted values.
- `extractor.go`: Phase 1 fetches the product page and parses identity, API config, portfolio ID, characteristics, page as-of date. Phase 2 fetches `keyFundFacts` JSON → fund profile; TER is filled from the page HTML (data-item row) when missing from JSON (it isn't in `keyFundFacts`). Phase 3 fetches `holdings` JSON → holdings; the holdings `asOfDate` wins over the page as-of date. All fetches go through the injectable `Client` (tests stub `SetFetchFunc`), so extraction remains testable offline.
- `parsers.go`: `parseKeyValue` now tries old `<td>` table → **data-item table row** (new `parseKeyValueFromDataRow` + `findClassToken` with strict class-token boundaries — `col-totalNetAssets` must not match `col-totalNetAssetsFundLevel`; React SSR comments `<!-- -->` stripped inside rows) → div-based layout. Added missing `Total Expense Ratio → emeaMgt` mapping. `ParseAsOfDate` allows extra classes on `as-of-date` elements. `ParseComponentID`/`ParseHoldings` (CSV) marked `Deprecated` (kept for resilience; still used as fallback path shape only in tests).

**Tests (all offline, hand-written fixtures modeled on live page/API captures from 2026-08-29):**
- `extractor_test.go`: rewritten to the three-fetch flow — `TestExtractor_Extract` (happy path incl. holdings as-of date override, benchmark ticker → symbol), `TestExtractor_Extract_AsOfDatePageFallback` (page date fallback when JSON has no as-of), `TestExtractor_Extract_MissingProductDataConfig` (page without API config → `phase 1 — parse product data config`), plus phase 1/2/3 failure, empty-holdings, and cancellation tests.
- `parsers_test.go`: added JSON API parser tests (profile from `keyFundFacts`, holdings incl. cash row + as-of, error cases), data-item row parser tests (span-wrapped and plain values, prefix-collision guard), `ParseAsOfDate` extra-class test.
- `go test ./internal/domain/extractor/blackrock/` green; BlackRock integration suite (14 tests) green. Pre-existing unrelated failure: `tests/integration TestBenchmarkChartAllPeriods` (fails on clean main too).
