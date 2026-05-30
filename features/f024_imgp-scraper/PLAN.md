# Implementation Plan: iM Global Partner (iMGP) Scraper

## Overview

Build an iMGP factsheet PDF extractor that registers with the existing dispatcher framework. The extractor fetches the fund page HTML, resolves the factsheet PDF URL, downloads the PDF, parses structured data (Fund Facts, Risk Measures, Portfolio Composition), and stores it in new database columns. The symbol details page displays the new data sections.

## Task Dependencies

```
Task 1 (PDF parsing + types) → Task 2 (matcher + client) → Task 3 (parsers) → Task 4 (extractor wiring)
                                                                        ↓
Task 5 (DB schema) → Task 6 (repo + service mapping) → Task 7 (UI display)
                                                                        ↓
                                         Task 8 (registration + integration)
```

Tasks 1–3 can be developed in parallel with Task 5. Task 4 depends on 1–3. Task 6 depends on 5 + 4. Task 7 depends on 6. Task 8 depends on 4 + 6.

## Tasks

### Task 1: PDF parsing library + iMGP-specific domain types [PRIORITY: HIGH]
**Corresponds to:** Story 1 (data extraction)
**Description:** Add a PDF text extraction library and define the Go types for iMGP-specific data that don't fit existing extractor types.

- [x] Add `github.com/ledongthuc/pdf` dependency (pure Go, MIT license, text extraction from PDFs) — stabilized in Task 3 via `go mod tidy`
- [x] Extend `extractor.ExtractResult` with new fields: `RiskMeasures`, `AssetClassAllocation`, `EquityDerivativesByRegion`, `CurrencyDerivativesAllocation`
- [x] Add corresponding types to `extractor` package: `RiskMeasures` struct (volatility, Sharpe, info ratio, beta, correlation, tracking error), `AssetClassEntry`, `RegionDerivativeEntry`, `CurrencyDerivativeEntry`
- [x] Add corresponding types to `symbol` package: `RiskMeasures`, `AssetClassEntry`, `RegionDerivativeEntry`, `CurrencyDerivativeEntry`
- [x] Write unit tests for type definitions (trivial — verify JSON serialization)

**Verification:** `go build ./...` succeeds; new types serialize to JSON correctly.

### Task 2: iMGP matcher + client [PRIORITY: HIGH]
**Corresponds to:** Story 2 (dispatcher integration)
**Description:** Create the URL matcher and HTTP client for iMGP, following the WisdomTree/DWS pattern.

- [x] Create `internal/domain/extractor/imgp/matcher.go` — matches `*.imgp.com` domains
- [x] Create `internal/domain/extractor/imgp/matcher_test.go` — table-driven tests for matching/non-matching URLs
- [x] Create `internal/domain/extractor/imgp/client.go` — HTTP client with CycleTLS (same pattern as WisdomTree), rate limiting (1s min delay), and two methods: `FetchPage(url) (string, error)` for HTML and `FetchPDF(url) ([]byte, error)` for PDF bytes
- [x] Create `internal/domain/extractor/imgp/client_test.go` — tests with mock fetch function
- [x] Implement `extractPDFURL(html) (string, error)` helper that finds the factsheet PDF download link in the fund page HTML

**Verification:** Matcher correctly identifies iMGP URLs; client fetches HTML and PDF with mock; `extractPDFURL` finds the PDF link from sample HTML.

### Task 3: PDF parsers [PRIORITY: HIGH]
**Corresponds to:** Story 1 (data extraction)
**Description:** Parse structured data from the factsheet PDF text. Each parser extracts one section.

- [x] Create `internal/domain/extractor/imgp/parsers.go` with:
  - `ParseFundFacts(pdfText) (*extractor.FundProfile, error)` — AUM, inception date, ISIN, share class name, management fees, ongoing charges
  - `ParseRiskMeasures(pdfText) (*extractor.RiskMeasures, error)` — volatility, Sharpe ratio, information ratio, beta, correlation, tracking error
  - `ParseAssetClassAllocation(pdfText) ([]extractor.AssetClassEntry, error)` — equities, bonds, gold, oil (can be negative, don't sum to 100%)
  - `ParseEquityDerivativesByRegion(pdfText) ([]extractor.RegionDerivativeEntry, error)` — North America, Europe, Asia, Emerging Countries (sum to ~100%, can be negative)
  - `ParseCurrencyDerivativesAllocation(pdfText) ([]extractor.CurrencyDerivativeEntry, error)` — USD, EUR, JPY, etc. (sum to ~100%, can be negative)
  - `ParseReferenceDate(pdfText) (time.Time, error)` — "as of" date from factsheet header
- [x] Create `internal/domain/extractor/imgp/parsers_test.go` — table-driven tests using the sample PDF text
- [x] Extract text from the sample PDF (`features/f024_imgp-scraper/samples/LU2951555585_FACTSHEETS_EN.pdf`) and save as a test fixture in `testdata/`
- [x] Each parser returns `nil, nil` for optional sections not present; returns explicit error for required sections (Fund Facts, Reference Date)

**Verification:** All parsers extract correct values from the sample PDF fixture; optional section absence returns `nil, nil`; missing required section returns error.

### Task 4: Extractor wiring [PRIORITY: HIGH]
**Corresponds to:** Story 1 + Story 2
**Description:** Wire the client and parsers into the Extractor interface, implementing atomic extraction.

- [x] Create `internal/domain/extractor/imgp/extractor.go` implementing `extractor.Extractor` + `extractor.URLMatcher`
- [x] `Extract(ctx, sourceURL)` flow: fetch HTML page → extract PDF URL → download PDF → extract text → parse all sections → return `ExtractResult` or error
- [x] Atomicity: if Fund Facts or Reference Date parsing fails, return error (no partial data). Optional sections (risk, asset class, derivatives) returning nil is acceptable.
- [x] Create `internal/domain/extractor/imgp/extractor_test.go` — tests with mock client (success path, fund facts failure, reference date failure, optional section absence)
- [x] Context cancellation test

**Verification:** Extractor returns complete `ExtractResult` with mock data; fails atomically on required section errors; succeeds with partial optional sections.

### Task 5: Database schema migration [PRIORITY: HIGH]
**Corresponds to:** Story 1 (data storage)
**Description:** Add new columns to `symbol_details` for iMGP-specific data.

- [x] Create `migrations/023_add_imgp_columns.sql` with:
  - `risk_measures TEXT DEFAULT NULL`
  - `asset_class_allocation TEXT DEFAULT NULL`
  - `equity_derivatives_by_region TEXT DEFAULT NULL`
  - `currency_derivatives_allocation TEXT DEFAULT NULL`
- [x] Update `internal/data/queries/schema.sql` to include new columns
- [x] Update `internal/data/queries/symbol_details.sql` to include new columns in INSERT and SELECT
- [x] Run `sqlc generate` to regenerate types and queries
- [x] Update `symbol_details.sql.go` — verify generated code includes new fields

**Verification:** `goose sqlite3 data/portfoliolab.db up` succeeds; `sqlc generate` succeeds; generated types include new columns.

### Task 6: Repository + service mapping [PRIORITY: HIGH]
**Corresponds to:** Story 1 (data persistence)
**Description:** Update the repository and service to handle new data types.

- [x] Update `symbol_details_repo.go` `toSymbolDetail()` to deserialize new JSON columns into `symbol.SymbolDetails`
- [x] Update `symbol_details_repo.go` `Upsert()` to serialize new fields via `toSQLNullJSON`
- [x] Update `symbols/service.go` `extractResultToSymbolDetails()` to map new `ExtractResult` fields to `symbol.SymbolDetails`
- [x] Update `symbol_details_repo_test.go` with tests for new fields (serialization round-trip)
- [x] Update `symbols/service_test.go` if needed for the mapping

**Verification:** Repository round-trips new fields correctly; service maps ExtractResult to SymbolDetails for new types.

### Task 7: Symbol details page — UI display [PRIORITY: MEDIUM]
**Corresponds to:** Story 3 (web UI)
**Description:** Add new display sections to the symbol details template and handler.

- [x] Add display types to `symbol_details_web.go`: `displayRiskMeasures`, `displayAssetClassEntry`, `displayRegionDerivativeEntry`, `displayCurrencyDerivativeEntry`
- [x] Add corresponding fields to `symbolDetailsDisplay` struct
- [x] Update `toDisplayDetails()` to populate new fields from `symbol.SymbolDetails`
- [x] Update `templates/symbol_details/view.html` with new card sections:
  - **Risk Measures** — table with volatility, Sharpe ratio, info ratio, beta, correlation, tracking error; shows reference date
  - **Asset Class Allocation** — table (NOT pie chart) with asset class and percentage; values can be negative; shows reference date
  - **Equity Derivatives by Region** — table with region and percentage; shows reference date
  - **Currency Derivatives Allocation** — table with currency and percentage; shows reference date
- [x] Each section renders only when data is present (non-nil/non-empty)
- [x] Update `symbol_details_web_test.go` with tests for new display formatting

**Verification:** Template renders new sections with test data; negative values display correctly; reference date shown per section.

### Task 8: Registration + integration [PRIORITY: MEDIUM]
**Corresponds to:** Story 2 (dispatcher registration)
**Description:** Register the iMGP extractor with the dispatcher and verify end-to-end flow.

- [x] Add `import "codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/imgp"` to `internal/api/router.go`
- [x] Add `extractorReg.Register(imgp.NewExtractor())` after existing registrations
- [x] Verify `go build ./...` succeeds
- [x] Run existing extractor tests to ensure no regression
- [x] Add a real integration test (`real_test.go`) that extracts from the sample PDF (skipped in short mode)

**Verification:** Binary builds; existing tests pass; iMGP extractor is discoverable via `FindByURL` for `imgp.com` URLs.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| **PDF parsing library** | `github.com/ledongthuc/pdf` | Pure Go, MIT license, simple text extraction API. No CGO dependency (consistent with `modernc.org/sqlite` choice). Alternative `pdfcpu` is heavier and focused on manipulation, not extraction. |
| **PDF URL discovery** | Extract from HTML fund page | The iMGP fund page (`/fund/{ISIN}`) is HTML with the PDF factsheet linked. The PDF URL is not in a predictable pattern (WordPress uploads change paths). Fetching HTML first and extracting the PDF link is more resilient than guessing a URL pattern. |
| **New DB columns vs. JSON extension** | New columns in `symbol_details` | Follows existing pattern: each data section gets its own TEXT column (e.g. `fund_profile`, `equity_valuation`, `themes`). Consistent with sqlc schema and migration approach. |
| **iMGP types in shared `extractor` package** | Add to shared types | Risk measures and allocation entries are generic enough that future extractors (DWS, Dimensional) may produce similar data. Sharing types avoids duplication. |
| **Asset class allocation as table, not chart** | Table in template | Per spec: values can be negative (short positions) and don't sum to 100%. Pie charts and stacked bars are misleading for this data. |
| **CycleTLS for iMGP client** | Same as WisdomTree/DWS | Reuses existing pattern. iMGP may have bot protection. The CycleTLS dependency is already in the project. |
| **Atomic extraction** | Required sections: Fund Facts + Reference Date | Per spec: "Fund Facts including reference date must be present." Optional sections (risk, allocations) may be absent without failure. |

## Risks

- **PDF layout changes**: iMGP may restructure factsheets. Mitigation: parsers are text-based with flexible regex; test against sample PDF; explicit error on parse failure.
- **PDF URL extraction**: The HTML page structure may change, breaking the PDF link extraction. Mitigation: try multiple selectors (href patterns, data attributes); explicit error logged.
- **Cloudflare/bot detection**: iMGP may block non-browser requests. Mitigation: CycleTLS with browser fingerprint; rate limiting; explicit error if blocked.
- **Multiple fund types**: Equity funds may have traditional holdings instead of derivatives allocation. Mitigation: parsers handle whatever sections are present; optional sections return nil.
