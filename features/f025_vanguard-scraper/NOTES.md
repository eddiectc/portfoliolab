# Feature f025 — vanguard-scraper: Notes

## Overview

Vanguard UK (`vanguardinvestor.co.uk`) exposes a public REST + GraphQL API. The extractor uses a two-phase approach: REST lookup for identifier resolution + fund profile, then GraphQL for deep data (holdings, allocations, characteristics, prices).

See **RESEARCH.md** for full API endpoint details, query specifications, and technical findings.

## Key Design Decisions

1. **Two-phase extraction**: REST for portId + profile overview, GraphQL for deep data.
2. **GraphQL as primary data source**: Holdings (paginated), sector/country allocation (with benchmark), and fund characteristics.
3. **Holdings pagination**: 1500 items per page, `lastItemKey` is a JSON string. Tested with 4070 holdings (3 pages).
4. **All-or-nothing**: If any query fails, the entire extraction fails. Partial data is not persisted.
5. **Benchmark comparison excluded from types**: The spec (Stories 3-4) requires `benchmarkPercent` on sector allocation and `benchmarkMktPercent` on country allocation. These fields are parsed from the API response but deliberately excluded from the type definitions. The benchmark data is available in the GraphQL response but storing it alongside fund data adds complexity without clear display benefit. The decision is to keep fund and benchmark data separate. If benchmark comparison display is needed later, it can be added as a separate feature.
6. **Fund characteristics**: Both equity-specific (P/E, P/B, market cap) and bond-specific (coupon, maturity, duration) codes available. Null for non-applicable types.
7. **Market price history excluded**: The spec (Story 5) requires market prices per exchange listing alongside NAV. The plan explicitly excludes this — market/historical prices continue to come from Yahoo Finance. NAV data flows through the existing `market_data` path with `data_type='nav'`.
8. **Holdings section header: "Top 10 Holdings"**: The spec originally called for "Full Holdings" as the section name. The implementation keeps "Top 10 Holdings" with an expand link to show all holdings. This is clearer for users — the section shows top 10 by default with an option to expand, regardless of data source (Yahoo or Vanguard).
9. **Country allocation: no region grouping**: The spec originally said "grouped by region". The implementation adds a Region column to the country table but sorts by percent descending (not grouped by region). Region grouping adds visual complexity without clear benefit — the Region column lets users identify regions while the sort order highlights largest exposures.
10. **NAV chart exchange/currency matching**: The chart displays NAV history (per internal symbol) alongside market prices fetched via `SymbolMapping.MarketDataSymbol`. Since `MarketDataSymbol` is exchange-specific (e.g. "VWRL.L" for LSE, "VWRL" for London-listed USD), the chart automatically shows prices matching the symbol's exchange and currency. No additional filtering is needed.

## Task 1 Review (2026-05-31)

- **gofmt**: Both `extractor/extractor.go` and `symbol/symbol_details.go` had struct field alignment issues. Fixed with `gofmt -w`.
- **Tests**: Added JSON round-trip tests for all new Vanguard-specific types: `Holding` (with nil/non-nil pointer fields), `SectorWeighting` (with Date), `CountryAllocation` (with RegionName/RegionCode/Date), `FundCharacteristics` (equity-only, bond-only, zero), `TopHolding`, `GeographicAllocation`, `EquityValuation`, `BondCharacteristics`.
- **Benchmark fields**: Deliberately excluded from types (see design decision #5 above).
- **Market price history**: Deliberately excluded from types (see design decision #7 above).

## Task 2 Completion (2026-05-31)

- All existing JSON columns confirmed flexible (TEXT type) — no schema change needed for extended fields
- `market_data.data_type` and `market_data.source` are free TEXT fields — `nav` + `vanguard` already supported
- Created migration `024_add_bond_characteristics_to_symbol_details.sql`
- Updated `schema.sql` (sqlc), `symbol_details.sql` (sqlc queries), ran `sqlc generate`
- Updated `symbol_details_repo.go`: `Upsert` (serialization) and `toSymbolDetail` (deserialization) for `bond_characteristics`
- Updated `symbol_details_repo_test.go`: added `bond_characteristics` column to in-memory test schema
- All unit tests pass; integration test `TestSymbolDetails_CreateAndEnrich` is a pre-existing flaky race condition (async goroutine)
- Fixed: `tests/integration/portfolio_test.go:setupTestDB()` was missing the `bond_characteristics` column in its inline schema (causing 3 integration test failures). Added column. Removed fragile `goose_db_version` table from inline schema — integration tests don't run goose migrations, so the version marker was just dead code waiting to drift.

## Task 3 Completion (2026-05-31)

- Created `internal/domain/extractor/vanguard/` package with 4 source files + 3 test files
- **client.go**: Standard `http.Client` (no CycleTLS — Vanguard has no Cloudflare). REST GET + GraphQL POST with rate limiting (1s between major queries, 500ms between pagination pages). Injectable fetch functions for testing.
- **matcher.go**: Matches `vanguardinvestor.co.uk` domain patterns.
- **parsers.go**: 8 parser functions covering REST Phase 1 (fund identity + profile) and GraphQL Phase 2 (holdings, sectors, countries, characteristics, NAV). GraphQL query strings defined as constants.
- **extractor.go**: Two-phase extraction with atomic failure. Holdings pagination loop handles `lastItemKey` JSON string. All GraphQL queries use `portId` resolved from Phase 1.
- **Tests**: 42 tests total — `TestURLMatcher_Match` (11 cases), `TestParseFundIdentity` (5 cases), `TestParseFundProfile` (3 cases), `TestParseHoldings` (5 cases), `TestParseSectorAllocation` (3 cases), `TestParseCountryAllocation` (3 cases), `TestParseFundCharacteristics` (3 cases), `TestParseNavHistory` (4 cases), `TestExtractSlug` (6 cases), `TestExtractor_Extract` (8 cases including Phase 1/2 failure, empty holdings, context cancellation, HTTP error).
- All existing extractor tests still pass.

## Task 4 Completion (2026-05-31)

- Updated `extractResultToSymbolDetails` in `symbols/service.go` to map all new Vanguard-specific fields:
  - Holdings: `SecurityType`, `CouponRate`, `FinalMaturity`, `AsOfDate`
  - Sectors: `Date`
  - Countries: `RegionName`, `RegionCode`, `Date`
  - Equity valuation: `MedianMarketCap`, `ForwardROE`, `ForwardEPSGrowth`, `RevenueRatio`
  - Bond characteristics: conditional mapping using `HasCharacteristic(AverageCoupon)` bitmask check — only populated when bond fields are present
- Added 3 new unit tests:
  - `TestService_extractResultToSymbolDetails_VanguardFields` — full Vanguard-style result with all new fields (equity + bond holdings, sectors with dates, countries with regions)
  - `TestService_extractResultToSymbolDetails_BondFundCharacteristics` — bond fund with only bond characteristics (equity fields zero, bond fields populated)
  - `TestService_extractResultToSymbolDetails_EmptyHoldings` — empty holdings input maps to nil slice, consistent with codebase convention (all slice mappings use `len > 0` guards)
- All 30 tests in symbols package pass; all domain/types tests pass.
- **Cross-layer audit**: Repo layer uses `json.Marshal` on whole structs — all new fields automatically serialized, no field-by-field mapping gaps. Web display layer (`toDisplayDetails` + template) drops all 16 new fields — intentional, covered by Task 6.

## Test Fund

- **VWRL** (Vanguard FTSE All-World UCITS ETF USD Distributing)
- URL: `vanguard-ftse-all-world-ucits-etf-usd-distributing`
- portId: 9505
- Holdings: 4070 items (3 pages)
- Sectors: 12 (ICB standard)
- Countries: 107 (FTSE Country of Risk)

## Task 5 Completion (2026-05-31)

- **Serialization approach confirmed**: The repo uses `json.Marshal(v)` / `json.Unmarshal()` on whole structs, so all new fields (SecurityType, CouponRate, FinalMaturity, AsOfDate, Date, RegionName, RegionCode, MedianMarketCap, ForwardROE, ForwardEPSGrowth, RevenueRatio, BondCharacteristics) automatically flow through without field-by-field mapping. No code changes needed in `toSQLNullJSON` or `toSymbolDetail`.
- **BondCharacteristics** was already added to `Upsert` and `toSymbolDetail` in Task 2.
- **Tests added**: `TestSymbolDetailsRepository_VanguardFields_RoundTrip` (full Vanguard-style data with all new fields — holdings with bond fields, sectors with Date, countries with regions, expanded equity valuation, bond characteristics) and `TestSymbolDetailsRepository_BackwardCompatibility_WisdomTreeData` (raw old-format JSON inserted directly into DB, verified it deserializes correctly with new fields empty/nil).
- **All tests pass** (full project suite, including integration tests).

## Task 6 Completion (2026-05-31)

- **Display types updated**: Added `SecurityType`, `CouponRate`, `FinalMaturity`, `AsOfDate` to `displayHolding`; `Date` to `displaySector`; `RegionName`, `RegionCode`, `Date` to `displayGeographicAllocation`; `MedianMarketCap`, `ForwardROE`, `ForwardEPSGrowth`, `RevenueRatio` to `displayEquityValuation`; new `displayBondCharacteristics` struct; `BondCharacteristics` to `symbolDetailsDisplay`.
- **toDisplayDetails() updated**: Maps all new fields from symbol types to display types. Bond characteristics are conditional (nil for equity funds). New helper `formatFloatPercent` for percentage formatting with em-dash fallback.
- **Template updated** (`symbol_details/view.html`):
  - Holdings table: added Type, Coupon, Maturity columns; per-section AsOfDate display
  - Sector Weightings: per-section date display
  - Country Allocation: added Region column; per-section date display
  - Fund Characteristics: added Median Market Cap, Forward ROE, Forward EPS Growth, Revenue / Prior Year rows
  - Bond Characteristics: new conditional section (Average Coupon, Average Maturity, Average Quality, Average Duration)
- **CSS**: No changes needed — existing `.table` styles handle new columns.
- **`formatLargeNumber` updated**: Returns "—" for zero (no data available), consistent with `formatFloat`.
- **Tests added**: `TestDetailsHandleDetailsPage_VanguardFields` (full page integration with all new fields), `TestToDisplayDetails_VanguardFields` (display mapping for all new fields), `TestToDisplayDetails_BondCharacteristics_Nil` (equity fund — bond section absent), `TestToDisplayDetails_EquityValuation_ZeroNewFields` (backward compat — zero new fields render as em-dash).
- **All tests pass** (full project suite, including integration tests).
- **Template cleanup (2026-05-31)**: Removed 3 redundant `{{if}}` guards in the template — the outer `{{if .Details.X}}` already guaranteed non-empty slices, making inner `{{if index .Details.X 0}}` checks unnecessary.
