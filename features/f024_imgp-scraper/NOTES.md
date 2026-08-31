# Notes: iM Global Partner (iMGP) Scraper

## Decisions
- 2026-05-30: Added `github.com/ledongthuc/pdf` v0.0.0-20250511090121-5959a4027728 as the PDF parsing dependency. Pure Go, MIT license, consistent with the project's no-CGO approach.
- 2026-05-30: New types use `float64` for percentages, matching the existing extractor pattern (`Holding.Percent`, `SectorWeighting.Percent` etc.) — not `govalues/decimal`. This keeps the extractor layer consistent; `decimal` is used only in the transaction/position domain for money.
- 2026-05-30: Types are duplicated between `extractor` and `symbol` packages (identical field names/structure), following the existing pattern (`FundProfile`, `MarketCapBreakdown`). Task 6 service mapping will be straightforward field-by-field.
- 2026-05-30: `extractPDFURL` uses HTML parsing only (regex on `href="...FACTSHEETS_EN.pdf"`) — no URL construction fallback. If the factsheet link is absent from the page HTML, extraction fails explicitly. Verified against live page `https://www.imgp.com/fund/LU2951555585`.
- 2026-05-30: `ParseFundFacts` signature changed from `(*extractor.FundProfile, *extractor.FundInfo, error)` to `(*extractor.FundProfile, error)` — FundInfo comes from the HTML page (not PDF), so the parser returns only FundProfile. The extractor wiring (Task 4) will populate FundInfo separately.
- 2026-05-30: Added `Isin`, `ShareClassName`, `OngoingCharges` fields to both `extractor.FundProfile` and `symbol.FundProfile`, following the existing pattern of duplicating types between the two packages.
- 2026-05-30: **Silent fallback audit** — fixed three silent fallbacks:
  - `RiskMeasures` now has a `FieldsPresent` bitmask (`RiskFieldsMask`) + `HasField()`/`AllFieldsPresent()` helpers so consumers can distinguish "field = 0" from "field absent in source".
  - Chart parsers (`AssetClassAllocation`, `EquityDerivativesByRegion`, `CurrencyDerivativesAllocation`) now return partial paired data + explicit error when label/percentage counts mismatch, instead of silently dropping extras.
  - `extractShareClass` now returns `(string, error)` instead of `string`, returning explicit error if the regex doesn't match.
- 2026-05-30: The fixture PDF (LU2951555585) has a genuine data quality issue: the Currency Derivatives Allocation section lists 10 currency labels but only 9 percentage values (EUR has no value). The parser handles this gracefully — returns 9 paired entries + error. This is expected behavior, not a bug.

## Deviations from Plan
- Task 1: `github.com/ledongthuc/pdf` dependency stabilized in Task 3 via `go mod tidy` (was deferred from Task 1).
- Task 6: **Silent fallback caught** — `extractResultToSymbolDetails` was missing the `Isin`, `ShareClassName`, and `OngoingCharges` fields on `FundProfile`. These fields were added to both `extractor.FundProfile` and `symbol.FundProfile` in Task 1, but the service mapping was never updated. Data parsed by the iMGP extractor would have been silently dropped. Fixed.
- Task 3: `ParseFundFacts` returns `(*extractor.FundProfile, error)` instead of `(*extractor.FundProfile, *extractor.FundInfo, error)` as originally planned — FundInfo is not in the PDF.
- Task 3: Risk measures for the sample fund (LU2951555585) only has Volatility (9.16%) and Sharpe Ratio (2.52) populated; InfoRatio, Beta, Correlation, and TrackingError are absent because the fund is too new (< 1 year). The parser handles this gracefully — `FieldsPresent` bitmask tracks which fields were actually parsed.
- Task 3: `RiskMeasures` struct gained a `FieldsPresent` field (not in original spec). Added after silent fallback audit revealed consumers couldn't distinguish zero from missing.
- Task 3: `extractShareClass` return type changed from `string` to `(string, error)` (not in original spec). Added after silent fallback audit.
- Task 4: Optional section parser errors are silently discarded (not returned). The chart parsers (asset class, equity derivatives, currency derivatives) return partial data + error on label/percentage mismatch. The extractor accepts the partial data and discards the error, since these are optional sections. This matches the plan's atomicity rule: only Fund Facts + Reference Date are required.

## 2026-08-29 — AnnualExpenseRatio stored as raw percent instead of fraction

Discovered during f026 (BlackRock) TER display work: `extractPercent` returned the raw percentage
number ("0.55%" -> 0.55) which was stored directly in `AnnualExpenseRatio`. The web display
multiplies by 100 (`%.2f%%` of `value * 100`), so funds showed 55.00% TER. Fixed in
`ParseFundFacts` (`parsers.go`): `profile.AnnualExpenseRatio = mgmtFees / 100`, matching the
fraction convention used by vanguard, wisdomtree and (after the f026 fix) blackrock. The
integration test `TestIMGPSymbolDetails_FullStackRoundTrip` already used a fraction
(0.015), confirming the parser was the outlier.

**Not changed:** `OngoingCharges` still stores the raw percentage — its display path
(`symbol_details_web.go`) formats it *without* `*100`, so its convention is the raw number.
`extractPercent` itself is unchanged (shared by both fields). Parser unit test updated to
expect 0.0055 (epsilon compare — 0.55/100 is not exactly representable in float64).

## 2026-08-31 — US share class pages (revision)
iMGP now also hosts US-listed fund share classes (e.g. `DBMF`, IMGP DBi Managed Futures
Strategy ETF) under `https://www.imgp.com/us/fund/{ISIN}-{slug}` pages. The old extractor
failed on them with `parse fund facts: parse ISIN: ISIN not found` — the US factsheet PDF
lists the CUSIP instead of the ISIN, omits the share class name and ongoing charges, labels
the fee "Gross Expense Ratio", and uses month-first dates (`05/07/2019` = 7 May 2019). The
page itself embeds a `const fund = {...}` JSON object with the structured identity (`isin`,
`sub_fund_name`, `cusip_code`, `management_fee_us`) — present on both the EU and US page
variants.

Decisions:
- **Fund identity comes from the page JSON, not the URL** (`parseFundPageJSON` in `client.go`;
  `parseFundInfoFromHTML` in `extractor.go` uses it first, URL path / `<title>` as fallbacks).
  This also fixes fund names with the `| iM Global Partner EN/US` title suffix. The URL-based
  `extractISINFromURL` now handles the `/us/fund/{ISIN}-{slug}` layout via `isISINLike`
  (letters allowed in the 10-char body — the EU regex was digits-only and could never match
  `US53700T8273`).
- **Date layout is selected from the URL region**: `/us/` prefix → month-first, otherwise
  day-first (`dateLayoutFor`, `DateLayout` type). US inception `05/07/2019` parses as
  7 May 2019; EU dates unchanged. A US fund with a day-first layout would yield an impossible
  month > 12 → explicit error, per the no-silent-fallback convention.
- **ISIN is optional in the PDF**: `ParseFundFacts` no longer requires it in the factsheet
  (US lists CUSIP instead). The extractor back-fills `profile.Isin` from the resolved
  `fundInfo.Symbol`; if both sources lack an ISIN, extraction fails explicitly.
- **Fee label fallback**: `extractPercentAny(section, "Management Fees", "Gross Expense Ratio")`
  — the US label matches before the EU one, so a factsheet containing both would report the
  gross ratio. Still a fraction (`/100`) per the 2026-08-29 convention.
- **Volatility label fallback**: `Fund Volatility` (EU) → `Volatility` (US).
- **Share class / ongoing charges** are optional (US factsheets omit both) — no longer
  required fields in `ParseFundFacts`.
- **Sample fixture**: `samples/DBMF_FACTSHEETS_EN.pdf` (real US factsheet, Jul 2026) +
  `testdata/dbmf_factsheet.txt` (extracted text) + `testdata/us_fund_page_snippet.html`
  (synthetic `const fund` JSON snippet — the real page embeds a 2 MB blob). Full-flow test:
  `TestExtractor_Extract_US` (`extractor_test.go`) — mock client serving the page HTML plus the
  real sample PDF bytes, asserts identity, back-filled ISIN, month-first inception, fee fraction
  and risk measures. Skipped when the sample PDF is absent.
- **Documented empties (US factsheet vs EU)**: no ISIN (CUSIP 56170L828 instead), no share
  class name, no ongoing charges, no currency suffix on `Fund Size` ("3.9 Bn" — regex already
  tolerated this). Verified against the real PDF.

Deviations from plan (revision): none — implemented directly after user-reported failure.

## Future Improvements
- None yet.

## Known Issues
- (Resolved 2026-08-31: both pre-existing test-compilation issues listed here — `symbols/service_test.go` and `wisdomtree/parsers_test.go` — no longer reproduce; full `go test ./...` is green.)
