# Notes: iM Global Partner (iMGP) Scraper

## Decisions
- 2026-05-30: Added `github.com/ledongthuc/pdf` v0.0.0-20250511090121-5959a4027728 as the PDF parsing dependency. Pure Go, MIT license, consistent with the project's no-CGO approach.
- 2026-05-30: New types use `float64` for percentages, matching the existing extractor pattern (`Holding.Percent`, `SectorWeighting.Percent` etc.) — not `govalues/decimal`. This keeps the extractor layer consistent; `decimal` is used only in the transaction/position domain for money.
- 2026-05-30: Types are duplicated between `extractor` and `symbol` packages (identical field names/structure), following the existing pattern (`FundProfile`, `MarketCapBreakdown`). Task 6 service mapping will be straightforward field-by-field.

## Deviations from Plan
- Task 1: `github.com/ledongthuc/pdf` is in go.mod but marked `// indirect` since no code imports it yet. `go mod tidy` will strip it. The dependency will be stabilized when Task 3 adds the first import in `parsers.go`. PLAN.md checkbox for this sub-item is left unchecked until then.

## Future Improvements
- None yet.

## Known Issues
- Pre-existing: `internal/domain/symbols/service_test.go` has compilation errors (decimal.Decimal type mismatches, wrong argument count) — not caused by this feature.
- Pre-existing: `internal/domain/extractor/wisdomtree/parsers_test.go` has unused imports — not caused by this feature.
