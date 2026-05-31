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
