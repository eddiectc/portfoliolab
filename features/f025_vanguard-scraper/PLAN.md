# Implementation Plan: Vanguard Data Extractor

## Overview

Build a Vanguard-specific data extractor that integrates with the existing extractor framework (f021). The Vanguard extractor uses a two-phase approach: REST API for fund identifier resolution + profile, then GraphQL API for deep data (holdings with pagination, sector/country allocation, fund characteristics, and NAV history). Requires extending existing types to support per-section "as of" dates (stored in JSON), expanded fund characteristics (equity + bond metrics), and optional holding fields. NAV history flows through the existing `NavPoint` → `market_data` path. Market/historical prices continue to come from Yahoo Finance.

**Key difference from existing extractors**: WisdomTree/DWS scrape HTML or call simple GET endpoints. Vanguard uses a GraphQL API (POST with structured queries) and requires pagination for holdings. This is the first extractor to use GraphQL.

## Task Dependencies

```
Task 1 (Extend Types) ──────────────────────────────────────────► Task 3 (Vanguard Package)
                                                                      │
Task 2 (DB migration + sqlc) ────────────────────────────────────┤
                                                                      │
                                                               Task 4 (Service Mapping)
                                                                      │
                                                               Task 5 (Repository Serialization)
                                                                      │
                                                               Task 6 (Web Display)
                                                                      │
                                                               Task 7 (Registration + Integration Tests)
```

All tasks are sequential. Task 1 must complete before Task 3 (new types are used by parsers). Task 2 must complete before Task 5 (sqlc types needed for repo layer). Tasks 4-7 depend on the extractor being functional.

## Tasks

### Task 1: Extend extractor and symbol types [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundation — new data shapes required by Vanguard)

**Description:** Extend the shared `extractor` and `symbol` package types to accommodate Vanguard-specific data. Per-section "as of" dates are stored in JSON alongside the data (not as new SymbolDetails columns). Expanded fund characteristics cover both equity and bond metrics.

- [x] **extractor.SectorWeighting**: Add `Date string` field (per-section "as of" date, serialized in `sector_weightings` JSON column)
- [x] **extractor.CountryAllocation**: Add `RegionName string`, `RegionCode string`, and `Date string` fields (serialized in `geographic_allocations` JSON column)
- [x] **extractor.FundCharacteristics**: Add equity-specific fields (`MedianMarketCap`, `ForwardROE`, `ForwardEPSGrowth`, `RevenueRatio`) and bond-specific fields (`AverageCoupon`, `AverageMaturity`, `AverageQuality`, `AverageDuration`)
- [x] **extractor.Holding**: Add `SecurityType string`, `CouponRate *float64`, `FinalMaturity *string`, and `AsOfDate string` fields
- [x] **symbol.GeographicAllocation**: Add `RegionName string`, `RegionCode string`, `Date string` fields
- [x] **symbol.EquityValuation**: Add `MedianMarketCap`, `ForwardROE`, `ForwardEPSGrowth`, `RevenueRatio` fields
- [x] **symbol.SymbolDetails**: Add `BondCharacteristics *BondCharacteristics` type (AverageCoupon, AverageMaturity, AverageQuality, AverageDuration)
- [x] **symbol.SectorWeighting**: Add `Date string` field (required for Task 4 mapping)
- [x] **symbol.TopHolding**: Add `SecurityType`, `CouponRate`, `FinalMaturity`, `AsOfDate` fields (required for Task 4 mapping)
- [x] Verify all existing tests still compile and pass (types are backward-compatible additions)

**Verification:** All types compile; existing tests pass; new fields are present in both extractor and symbol packages.

---

### Task 2: Database / Schema — Add `bond_characteristics` column [PRIORITY: MEDIUM]

**Corresponds to:** Story 7 (Integration with extractor framework)

**Description:** Most new fields flow through existing JSON columns (top_holdings, sector_weightings, geographic_allocations, equity_valuation) — no schema change needed for those. But `BondCharacteristics` is a new top-level field on `SymbolDetails` with no existing column, so a new JSON column is required.

- [x] Confirm: `symbol_details.top_holdings` JSON column can store extended holding fields (SecurityType, CouponRate, FinalMaturity, AsOfDate) — JSON is flexible, no change
- [x] Confirm: `symbol_details.sector_weightings` JSON column can store Date field — JSON is flexible, no change
- [x] Confirm: `symbol_details.geographic_allocations` JSON column can store Region + Date fields — JSON is flexible, no change
- [x] Confirm: `symbol_details.equity_valuation` JSON column can store expanded fields — JSON is flexible, no change
- [x] Confirm: `market_data` table already supports `data_type='nav'` and `source='vanguard'` — confirmed from f021
- [x] Create migration `024_add_bond_characteristics_to_symbol_details.sql`: `ALTER TABLE symbol_details ADD COLUMN bond_characteristics TEXT` (nullable JSON, same pattern as `equity_valuation`)
- [x] Run `sqlc generate` to regenerate types with the new column
- [x] Update sqlc queries (`InsertSymbolDetails`, `GetSymbolDetailsByInternalSymbol`) to include `bond_characteristics`

**Verification:** All existing JSON columns can accommodate new fields without schema changes.

---

### Task 3: Vanguard extractor package [PRIORITY: HIGH]

**Corresponds to:** Stories 1-5 (Vanguard data extraction)

**Description:** Create the `internal/domain/extractor/vanguard/` package implementing the two-phase extraction: REST API for fund identity/portId resolution, then GraphQL for deep data. Uses Go's standard `http.Client` (no CycleTLS needed — Vanguard's API has no Cloudflare protection). GraphQL queries use structured POST requests with `operationName`, `variables`, and `query` fields.

**Data sources** (from RESEARCH.md):

| Phase | Data | Endpoint | Method |
|-------|------|----------|--------|
| 1 | Fund identity + profile | `/api/funds/{fundSlug}` | GET (REST) |
| 2 | Holdings (paginated, 1500/page) | `/gpx/graphql` | POST (GraphQL) |
| 2 | Sector allocation | `/gpx/graphql` | POST (GraphQL) |
| 2 | Country allocation | `/gpx/graphql` | POST (GraphQL) |
| 2 | Fund characteristics | `/gpx/graphql` | POST (GraphQL) |
| 2 | NAV history | `/gpx/graphql` | POST (GraphQL) |

- [x] Create `internal/domain/extractor/vanguard/` package
- [x] Implement `client.go`: HTTP client with standard `http.Client`, 1-2s rate limiting between major queries, 0.5-1s between pagination pages, GraphQL POST support (`queryGraphQL(portId, operationName, variables, query) (string, error)`)
- [x] Implement `matcher.go`: `URLMatcher` matching `vanguardinvestor.co.uk` domain patterns
- [x] Implement `parsers.go` — Phase 1 (REST API):
  - `ParseFundIdentity(json) (*FundIdentity, error)` — extracts name, ticker, sedol, portId, inceptionDate, ISIN, currencyCode from REST response; portId is required for Phase 2
  - `ParseFundProfile(json) (*extractor.FundProfile, error)` — extracts OCF (→ AnnualExpenseRatio), benchmark, managementType, assetClass, fundType, distributionStrategyType, region
- [x] Implement `parsers.go` — Phase 2 (GraphQL):
  - `ParseHoldings(json) ([]extractor.Holding, string, error)` — parses paginated holdings items (issuerName, securityLongDescription, marketValuePercentage, securityType, couponRate, finalMaturity); returns holdings + effectiveDate; handles pagination transparently
  - `ParseSectorAllocation(json) ([]extractor.SectorWeighting, string, error)` — parses sectorDiversification (sectorName, sectorCode, fundPercent, date); returns sectors + date
  - `ParseCountryAllocation(json) ([]extractor.CountryAllocation, string, error)` — parses marketAllocation (countryName, countryCode, fundMktPercent, regionName, regionCode, date); returns countries + date
  - `ParseFundCharacteristics(json) (*extractor.FundCharacteristics, error)` — parses polarisAnalyticsHistory analytics (PERATIO, PBRATIO, MKTCAPMEDN, FRC5YRROE, EPSFRC5YR, TRNVRRPTR, AVGCPN, AVGWTDMTY, AVGQLYTFTO, AVGDURADJ); handles both equity and bond fund types
  - `ParseNavHistory(json) ([]extractor.NavPoint, error)` — parses navPrices from pricingDetails (price, asOfDate, currencyCode); maps to existing `extractor.NavPoint` type
- [x] Implement `extractor.go`:
  - `Extractor` struct with `*URLMatcher` and `*Client`
  - `Extract(ctx, sourceURL)` — two-phase extraction:
    1. Extract fund slug from source URL → REST API call → resolve portId + fund identity + profile
    2. GraphQL queries for holdings (with pagination loop), sectors, countries, characteristics, NAV
    3. Atomic: if Phase 1 fails → entire extraction fails; if any Phase 2 query fails → entire extraction fails
  - `Name()` returns `"vanguard"`
  - `Match(rawURL)` delegates to URLMatcher
- [x] Write unit tests:
  - `TestURLMatcher_Match` — vanguardinvestor.co.uk matches, other domains don't
  - `TestParseFundIdentity` — table-driven with sample JSON; validates portId, ticker, name extraction
  - `TestParseFundProfile` — validates OCF → AnnualExpenseRatio, benchmark, etc.
  - `TestParseHoldings` — validates item parsing, optional bond fields (couponRate, finalMaturity), effectiveDate extraction
  - `TestParseSectorAllocation` — validates fundPercent + date
  - `TestParseCountryAllocation` — validates country + region fields
  - `TestParseFundCharacteristics` — equity fund (P/E, P/B populated, bond fields null) and bond fund (coupon, maturity populated, equity fields null)
  - `TestParseNavHistory` — validates NAV price points with dates
  - `TestExtractor_Extract` — full extraction with mocked HTTP server (Phase 1 + Phase 2)
  - `TestExtractor_Extract_Phase1Failure` — invalid fund slug → entire extraction fails
  - `TestExtractor_Extract_Phase2Failure` — Phase 1 succeeds but holdings query fails → entire extraction fails
  - `TestExtractor_Extract_EmptyHoldings` — zero holdings → stored as empty (not a failure)
  - `TestExtractor_Extract_MissingEffectiveDate` — holdings without effectiveDate → fails

**Verification:** All parsers return correct structs from sample JSON; two-phase extraction works end-to-end; atomic failure on Phase 1 or Phase 2 error; pagination handled transparently.

---

### Task 4: Service layer — extractResultToSymbolDetails mapping [PRIORITY: HIGH]

**Corresponds to:** Story 7 (Integration with extractor framework)

**Description:** Update the `extractResultToSymbolDetails` function in `symbols/service.go` to map the new Vanguard-specific fields. The existing routing logic (dispatcher → extractor) already works — no changes needed there. Only the field mapping requires updates.

- [x] Update sector mapping: include `Date` field when mapping `extractor.SectorWeighting` → `symbol.SectorWeighting`
- [x] Update country mapping: include `RegionName`, `RegionCode` when mapping `extractor.CountryAllocation` → `symbol.GeographicAllocation`
- [x] Update equity valuation mapping: include new fields (`MedianMarketCap`, `ForwardROE`, `ForwardEPSGrowth`, `RevenueRatio`) when mapping `extractor.FundCharacteristics` → `symbol.EquityValuation`
- [x] Add bond characteristics mapping: map bond-specific fields from `extractor.FundCharacteristics` → new `symbol.BondCharacteristics`
- [x] Update holdings mapping: include `SecurityType`, `CouponRate`, `FinalMaturity`, `AsOfDate` when mapping `extractor.Holding` → `symbol.TopHolding`
- [x] NAV history flows through existing path (`result.NavHistory` → `storeNavHistory` → `market_data` table) — no changes needed
- [x] Write unit tests: extractResultToSymbolDetails correctly maps all new fields (table-driven with Vanguard-specific ExtractResult)
- [x] Write unit tests: bond fund characteristics map correctly (equity fields null, bond fields populated)
- [x] Write unit tests: empty holdings map to nil slice (consistent with codebase convention — all slice mappings use `len > 0` guards)

**Verification:** All new fields flow from ExtractResult through to SymbolDetails; existing field mapping unchanged; NAV flows through existing path.

---

### Task 5: Repository JSON serialization/deserialization [PRIORITY: MEDIUM]

**Corresponds to:** Story 7 (Integration with extractor framework)

**Description:** Update the symbol repository to serialize/deserialize the new fields. The existing JSON columns are flexible for extended fields on existing types. `bond_characteristics` uses the new column added in Task 2. The serialization functions must include all new fields.

- [x] Update `toSQLNullJSON` / JSON serialization for holdings: include `SecurityType`, `CouponRate`, `FinalMaturity`, `AsOfDate`
- [x] Update `toSQLNullJSON` / JSON serialization for sectors: include `Date`
- [x] Update `toSQLNullJSON` / JSON serialization for countries: include `RegionName`, `RegionCode`, `Date`
- [x] Update `toSQLNullJSON` / JSON serialization for equity valuation: include new fields
- [x] Add bond characteristics serialization/deserialization
- [x] Add characteristics date serialization/deserialization
- [x] Write integration test: repo round-trip for all new fields (Vanguard-style data inserted → retrieved → fields match)
- [x] Write integration test: backward compatibility — existing WisdomTree/DWS data still deserializes correctly (new fields are optional in JSON)

**Verification:** All new fields survive a full round-trip through the repository; existing data not affected.

---

### Task 6: Web display [PRIORITY: MEDIUM]

**Corresponds to:** Stories 1-5 (user-facing display of Vanguard data)

**Description:** Update the web display to render the new data. Per-section dates displayed alongside their sections. Expanded characteristics include both equity and bond metrics. Holdings table shows security type and bond-specific columns. NAV chart uses existing price chart (NAV points flow through `market_data` as `data_type='nav'`).

- [x] Update `toDisplayDetails()` in web layer to include:
  - `BondCharacteristics` (conditional, only if non-nil)
  - New equity valuation fields
  - Region fields on country allocations
- [x] Update `symbol_details.html` template:
  - Holdings table: add columns for SecurityType, CouponRate, FinalMaturity (bond columns only visible when relevant)
  - Sectors section: display per-section date
  - Countries section: display per-section date, add region grouping
  - Characteristics section: display expanded equity fields + conditional bond characteristics section
  - NAV chart: no changes needed (existing chart works with `data_type='nav'` via `market_data`)
- [x] Update CSS if needed for new columns/sections
- [x] Write integration test: Vanguard-style symbol details page renders with all new fields

**Verification:** All new fields visible in web UI; per-section dates displayed; bond characteristics conditional; existing WisdomTree/DWS pages unchanged.

---

### Task 7: Register extractor + integration tests [PRIORITY: MEDIUM]

**Corresponds to:** Story 7 (Integration with extractor framework)

**Description:** Register the Vanguard extractor in the dispatcher and write integration tests covering the full stack.

- [x] Register `vanguard` extractor in `internal/api/router.go` (added to extractor registry)
- [x] Write integration test: full extraction pipeline (dispatcher routing, extractor registration, full stack round-trip)
- [x] Write integration test: API endpoint `/api/symbols/{id}` returns Vanguard data with new fields (securityType, regionName, bondCharacteristics, expanded equity valuation)
- [x] Write integration test: web page `/symbols/{id}/details` renders correctly with symbol name and data
- [x] Write integration test: NAV data stored in `market_data` with correct `source='vanguard'` and `data_type='nav'`
- [x] Write integration test: data source URL round-trip through stale query
- [x] Write integration test: bond fund with only BondCharacteristics (equity_valuation NULL)
- [x] Write integration test: equity fund with only EquityValuation (bond_characteristics NULL)
- [x] Update `features/README.md` to mark f025 as complete
- [x] Add `BondCharacteristics` to `SymbolDetailsResponse` API handler (was missing from response struct)

**Verification:** Vanguard extractor registered and functional; full stack integration tests pass; existing extractors unaffected.

## Technical Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | **HTTP client: Standard `http.Client`** | Vanguard's API has no Cloudflare protection. Simpler than importing CycleTLS. |
| 2 | **Per-section dates stored in JSON** | Dates are display-only, not queried. Adding `Date` fields to existing types (SectorWeighting, CountryAllocation) keeps them serialized through existing JSON columns. No new SymbolDetails columns needed. |
| 3 | **Fund characteristics split: EquityValuation + BondCharacteristics** | Equity and bond metrics are mutually exclusive. Keeping them separate avoids null pollution. BondCharacteristics is optional (`*BondCharacteristics`) — nil for equity funds. |
| 4 | **NAV only (no market price history)** | Vanguard provides NAV history via GraphQL. Market/historical prices continue to come from Yahoo Finance. NAV points flow through existing `NavPoint` → `market_data` path. |
| 5 | **One new column: `bond_characteristics`** | Extended fields on existing types (holdings, sectors, countries, equity valuation) reuse existing JSON columns. `BondCharacteristics` is a new top-level field requiring its own JSON column (`ALTER TABLE symbol_details ADD COLUMN bond_characteristics TEXT`). |
| 6 | **Atomic two-phase extraction** | Phase 1 (REST) is required for Phase 2 (GraphQL). If Phase 1 fails, the entire extraction fails. If any Phase 2 query fails, the entire extraction fails. No partial data. |
| 7 | **Pagination handled in extractor** | Holdings pagination (1500/page) is handled internally by the extractor. The caller sees a flat list. Rate limiting between pages is built into the client. |
