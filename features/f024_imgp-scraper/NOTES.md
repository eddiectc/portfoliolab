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
- Task 3: `ParseFundFacts` returns `(*extractor.FundProfile, error)` instead of `(*extractor.FundProfile, *extractor.FundInfo, error)` as originally planned — FundInfo is not in the PDF.
- Task 3: Risk measures for the sample fund (LU2951555585) only has Volatility (9.16%) and Sharpe Ratio (2.52) populated; InfoRatio, Beta, Correlation, and TrackingError are absent because the fund is too new (< 1 year). The parser handles this gracefully — `FieldsPresent` bitmask tracks which fields were actually parsed.
- Task 3: `RiskMeasures` struct gained a `FieldsPresent` field (not in original spec). Added after silent fallback audit revealed consumers couldn't distinguish zero from missing.
- Task 3: `extractShareClass` return type changed from `string` to `(string, error)` (not in original spec). Added after silent fallback audit.
- Task 4: Optional section parser errors are silently discarded (not returned). The chart parsers (asset class, equity derivatives, currency derivatives) return partial data + error on label/percentage mismatch. The extractor accepts the partial data and discards the error, since these are optional sections. This matches the plan's atomicity rule: only Fund Facts + Reference Date are required.

## Future Improvements
- None yet.

## Known Issues
- Pre-existing: `internal/domain/symbols/service_test.go` has compilation errors (decimal.Decimal type mismatches, wrong argument count) — not caused by this feature.
- Pre-existing: `internal/domain/extractor/wisdomtree/parsers_test.go` has unused imports — not caused by this feature.
