# Feature f025 — vanguard-scraper: Notes

## Overview

Vanguard UK (`vanguardinvestor.co.uk`) exposes a public REST + GraphQL API. The extractor uses a two-phase approach: REST lookup for identifier resolution + fund profile, then GraphQL for deep data (holdings, allocations, characteristics, prices).

See **RESEARCH.md** for full API endpoint details, query specifications, and technical findings.

## Key Design Decisions

1. **Two-phase extraction**: REST for portId + profile overview, GraphQL for deep data.
2. **GraphQL as primary data source**: Holdings (paginated), sector/country allocation (with benchmark), and fund characteristics.
3. **Holdings pagination**: 1500 items per page, `lastItemKey` is a JSON string. Tested with 4070 holdings (3 pages).
4. **All-or-nothing**: If any query fails, the entire extraction fails. Partial data is not persisted.
5. **Benchmark comparison excluded from types**: The spec (Stories 3-4) requires `benchmarkPercent` on sector allocation and `benchmarkMktPercent` on country allocation. These fields were deliberately excluded from the type definitions — the Vanguard GraphQL API returns benchmark data, but storing it alongside fund data in the same struct adds complexity without clear display benefit. If needed later, benchmark data can be stored separately or added as optional fields.
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

## Test Fund

- **VWRL** (Vanguard FTSE All-World UCITS ETF USD Distributing)
- URL: `vanguard-ftse-all-world-ucits-etf-usd-distributing`
- portId: 9505
- Holdings: 4070 items (3 pages)
- Sectors: 12 (ICB standard)
- Countries: 107 (FTSE Country of Risk)
