# Implementation Plan: BlackRock/iShares Data Extractor

## Overview

Build a BlackRock/iShares-specific data extractor that integrates with the existing extractor framework (f021). The extractor uses a two-phase approach: product page (server-rendered HTML) for fund identifier resolution + fund profile + portfolio characteristics, then the holdings JSON API for deep data (full holdings list, from which sector and geography allocations are derived by aggregating holding weights). The sector/geography data is not available via direct API (returns 500), so it is computed from the holdings data.

**Key characteristics**: The iShares product page requires `?switchLocale=y&siteEntryPassthrough=true` to bypass investor type selection. The holdings data is available via a JSON API endpoint. The component ID (embedded in the page HTML) is needed to construct the holdings URL.

## Task Dependencies

```
Task 1 (Extend Types) ──────────────────────────────────────────────► Task 3 (BlackRock Package)
                                                                          │
                                                               Task 4 (Service Mapping)
                                                                          │
                                                               Task 5 (Repository Serialization)
                                                                          │
                                                               Task 6 (Web Display)
                                                                          │
                                                               Task 7 (Registration + Integration Tests)
```

All tasks are sequential. Task 1 must complete before Task 3 (new types are used by parsers). Tasks 4-7 depend on the extractor being functional.

## Tasks

### Task 1: Extend extractor and symbol types [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundation — new data shapes required by BlackRock extractor)

**Description:** Extend the shared `extractor` and `symbol` package types to accommodate iShares-specific data. The iShares Key Facts include many fields not in the existing types (SFDR classification, domicile, rebalance frequency, fund manager, custodian, etc.). The portfolio characteristics include beta and standard deviation not in the existing `FundCharacteristics`.

- [x] **extractor.FundProfile**: Add iShares-specific fields:
  - `SFDRClassification string` (e.g. "Other", "Article 6", "Article 8", "Article 9")
  - `Domicile string` (e.g. "Ireland", "Luxembourg")
  - `RebalanceFrequency string` (e.g. "Quarterly", "Semi-Annually")
  - `ProductStructure string` (e.g. "Physical", "Synthetic")
  - `Methodology string` (e.g. "Optimised", "Representative")
  - `FundManager string` (e.g. "BlackRock Asset Management Ireland Limited")
  - `Custodian string` (e.g. "State Street Custodial Services (Ireland) Limited")
  - `IssuingCompany string` (e.g. "iShares IV plc")
  - `BenchmarkTicker string` (e.g. Bloomberg ticker of the benchmark)

- [x] **symbol.FundProfile**: Add matching fields:
  - `SFDRClassification string`
  - `Domicile string`
  - `RebalanceFrequency string`
  - `ProductStructure string`
  - `Methodology string`
  - `FundManager string`
  - `Custodian string`
  - `IssuingCompany string`
  - `BenchmarkTicker string`

- [x] **extractor.FundCharacteristics**: Add iShares-specific fields:
  - `Beta3Y float64` (3-year beta)
  - `StandardDeviation3Y float64` (3-year standard deviation)
  - `NumberOfHoldings int` (number of holdings)
  - Add corresponding bitmask entries in `CharacteristicsFieldsMask`

- [x] **symbol.EquityValuation**: Add matching fields:
  - `Beta3Y float64`
  - `StandardDeviation3Y float64`
  - `NumberOfHoldings int`

- [x] **extractor.Holding**: Add iShares-specific fields:
  - `Sector string` (e.g. "Information Technology")
  - `AssetClass string` (e.g. "Equity", "Cash")
  - `MarketValue float64` (market value in base currency)
  - `NotionalValue float64` (notional value)
  - `Shares float64` (number of shares/units)
  - `Price float64` (price per share)
  - `Identifier string` (CUSIP/ISIN, or "-" for cash)
  - `Location string` (country, e.g. "United States")
  - `Exchange string` (e.g. "NASDAQ")
  - `MarketCurrency string` (e.g. "USD")

- [x] **symbol.TopHolding**: Add matching fields:
  - `Sector string`
  - `AssetClass string`
  - `MarketValue float64`
  - `NotionalValue float64`
  - `Shares float64`
  - `Price float64`
  - `Identifier string`
  - `Location string`
  - `Exchange string`
  - `MarketCurrency string`

- [x] Verify all existing tests still compile and pass (types are backward-compatible additions)

**Verification:** All types compile; existing tests pass; new fields are present in both extractor and symbol packages.

---

### Task 2: No migration needed [PRIORITY: LOW]

**Corresponds to:** All scenarios (data compatibility — confirms no schema change required)

**Description:** All new fields flow through existing JSON columns (`fund_profile`, `equity_valuation`, `top_holdings`) — no schema change needed. JSON is flexible and automatically accommodates new fields.

- [ ] Confirm: `symbol_details.fund_profile` JSON column can store extended FundProfile fields — JSON is flexible, no change needed
- [ ] Confirm: `symbol_details.equity_valuation` JSON column can store expanded EquityValuation fields — JSON is flexible, no change needed
- [ ] Confirm: `symbol_details.top_holdings` JSON column can store extended holding fields — JSON is flexible, no change needed
- [ ] Confirm: `market_data` table already supports `data_type='nav'` — confirmed from f021

**Verification:** All existing JSON columns can accommodate new fields without schema changes.

---

### Task 3: BlackRock extractor package [PRIORITY: HIGH]

**Corresponds to:** Story 2 (BlackRock data extraction)

**Description:** Create the `internal/domain/extractor/blackrock/` package implementing the two-phase extraction: product page HTML for fund profile + characteristics, then JSON API for holdings (from which sector/geography are derived). Uses Go's standard `http.Client` with TLS fingerprinting (CycleTLS) for the product page.

**Data sources** (from RESEARCH.md):

| Phase | Data | Source | Method |
|-------|------|--------|--------|
| 1 | Fund identity + profile | Product page HTML (`?switchLocale=y&siteEntryPassthrough=true`) | HTTP GET + HTML parsing |
| 1 | Portfolio characteristics | Product page HTML (P/E, P/B, beta, std dev, etc.) | HTTP GET + HTML parsing |
| 1 | Component ID | Product page HTML (embedded in holdings download link) | Regex on page HTML |
| 2 | Holdings (full list) | JSON API (`{componentId}.ajax?tab=all&fileType=json&asOfDate={yyyymmdd}`) | HTTP GET + JSON parsing |
| 2 | Sector allocation | Derived from holdings data (aggregate by sector) | Computed |
| 2 | Geography allocation | Derived from holdings data (aggregate by location) | Computed |

- [ ] Create `internal/domain/extractor/blackrock/` package
- [ ] Implement `client.go`: HTTP client with CycleTLS for all requests (both product page and JSON API are behind the same Cloudflare/WAF on `www.ishares.com`), 1-2s rate limiting between requests, error handling for non-200 responses
- [ ] Implement `matcher.go`: `URLMatcher` matching `*.ishares.com/uk/*` domain patterns
- [ ] Implement `parsers.go` — Phase 1 (product page HTML):
  - `ParseFundIdentity(html) (*extractor.FundInfo, error)` — extracts name from page title / h1 tag
  - `ParseFundProfile(html) (*extractor.FundProfile, error)` — extracts Key Facts table: AUM, inception date, asset class, SFDR, TER, distribution strategy, domicile, rebalance frequency, UCITS, fund manager, custodian, benchmark, ISIN, product structure, methodology, issuing company, benchmark ticker
  - `ParseFundCharacteristics(html) (*extractor.FundCharacteristics, error)` — extracts portfolio characteristics table: P/E, P/B, beta, std dev, number of holdings; handles both equity and bond fund types
  - `ParseComponentID(html) (string, error)` — extracts component ID from holdings download link (regex on `<a href=".../.ajax?fileType=csv...">`)
  - `ParseAsOfDate(html) (string, error)` — extracts the "as of" date from holdings section header
- [ ] Implement `parsers.go` — Phase 2 (JSON API):
  - `ParseHoldings(json) ([]extractor.Holding, string, error)` — parses `aaData` array (13-field arrays with ticker, name, sector, asset class, market value, weight, notional value, shares, identifier, price, location, exchange, currency); returns holdings + asOfDate; handles UTF-8 BOM
  - `DeriveSectorAllocation(holdings) ([]extractor.SectorWeighting, error)` — aggregates holdings by sector (sum weights per sector, sort descending)
  - `DeriveCountryAllocation(holdings) ([]extractor.CountryAllocation, error)` — aggregates holdings by location (sum weights per country, sort descending)
- [ ] Implement `extractor.go`:
  - `Extractor` struct with `*URLMatcher` and `*Client`
  - `Extract(ctx, sourceURL)` — two-phase extraction:
    1. Parse source URL for portfolio ID → fetch product page → extract fund identity, profile, characteristics, component ID, as-of date
    2. Construct holdings JSON URL → fetch holdings → derive sector + country allocations
    3. Atomic: if Phase 1 fails → entire extraction fails; if Phase 2 fails → entire extraction fails
  - `Name()` returns `"blackrock"`
  - `Match(rawURL)` delegates to URLMatcher
- [ ] Write unit tests:
  - `TestURLMatcher_Match` — ishares.com/uk matches, other domains don't
  - `TestParseFundIdentity` — table-driven with sample HTML; validates name extraction
  - `TestParseFundProfile` — validates Key Facts extraction (AUM, ISIN, benchmark, new fields)
  - `TestParseFundCharacteristics` — equity fund (P/E, P/B, beta populated) and bond fund (YTM, duration populated)
  - `TestParseComponentID` — extracts component ID from page HTML
  - `TestParseHoldings` — validates 13-field array parsing, UTF-8 BOM handling, asOfDate extraction
  - `TestDeriveSectorAllocation` — aggregates holdings by sector, correct percentages
  - `TestDeriveCountryAllocation` — aggregates holdings by location, correct percentages
  - `TestExtractor_Extract` — full extraction with mocked HTTP server (Phase 1 + Phase 2)
  - `TestExtractor_Extract_Phase1Failure` — invalid fund URL → entire extraction fails
  - `TestExtractor_Extract_Phase2Failure` — Phase 1 succeeds but holdings query fails → entire extraction fails
  - `TestExtractor_Extract_EmptyHoldings` — zero holdings → stored as empty (not a failure)
  - `TestExtractor_Extract_MissingAsOfDate` — holdings without as-of date → fails

**Verification:** All parsers return correct structs from sample HTML/JSON; two-phase extraction works end-to-end; atomic failure on Phase 1 or Phase 2 error; sector/geography derived correctly from holdings.

---

### Task 4: Service layer — extractResultToSymbolDetails mapping [PRIORITY: HIGH]

**Corresponds to:** Stories 2, 3 (extraction output mapped to display types)

**Description:** Update the `extractResultToSymbolDetails` function in `symbols/service.go` to map the new iShares-specific fields. The existing routing logic already works — only the field mapping requires updates.

- [ ] Update FundProfile mapping: include new fields (`SFDRClassification`, `Domicile`, `RebalanceFrequency`, `ProductStructure`, `Methodology`, `FundManager`, `Custodian`, `IssuingCompany`, `BenchmarkTicker`)
- [ ] Update EquityValuation mapping: include new fields (`Beta3Y`, `StandardDeviation3Y`, `NumberOfHoldings`)
- [ ] Update holdings mapping: include new fields (`Sector`, `AssetClass`, `MarketValue`, `NotionalValue`, `Shares`, `Price`, `Identifier`, `Location`, `Exchange`, `MarketCurrency`)
- [ ] Sector and country allocation mapping already works (existing code maps `result.Sectors` and `result.CountryAllocation`) — no changes needed
- [ ] Write unit tests: extractResultToSymbolDetails correctly maps all new fields (table-driven with BlackRock-specific ExtractResult)
- [ ] Write unit tests: bond fund characteristics map correctly (equity fields null, bond fields populated)

**Verification:** All new fields flow from ExtractResult through to SymbolDetails; existing field mapping unchanged.

---

### Task 5: Repository JSON serialization/deserialization [PRIORITY: MEDIUM]

**Corresponds to:** Stories 2, 3 (data persistence round-trip)

**Description:** The existing `toSQLNullJSON` function marshals entire structs to JSON. Since JSON is flexible, new fields are automatically included. However, the deserialization (`toSymbolDetail`) must handle the new fields correctly.

- [ ] Confirm `toSQLNullJSON` automatically includes new FundProfile fields (marshals entire struct) — no changes needed
- [ ] Confirm `toSQLNullJSON` automatically includes new EquityValuation fields — no changes needed
- [ ] Confirm `toSQLNullJSON` automatically includes new TopHolding fields — no changes needed
- [ ] Update `toSymbolDetail` deserialization: ensure new fields are populated from JSON (unmarshal entire struct — should work automatically)
- [ ] Write integration test: repo round-trip for all new fields (BlackRock-style data inserted → retrieved → fields match)
- [ ] Write integration test: backward compatibility — existing WisdomTree/Vanguard data still deserializes correctly (new fields are optional in JSON)

**Verification:** All new fields survive a full round-trip through the repository; existing data not affected.

---

### Task 6: Web display [PRIORITY: MEDIUM]

**Corresponds to:** Story 3 (Symbol details page display)

**Description:** Update the web display to render the new data. Extended FundProfile fields shown in the fund profile section. Extended characteristics include beta and std dev. Holdings table shows additional columns (sector, exchange, etc.).

- [ ] Update `toDisplayDetails()` in web layer to include:
  - Extended FundProfile fields (SFDR, domicile, rebalance frequency, product structure, methodology, fund manager, custodian, issuing company, benchmark ticker)
  - Extended EquityValuation fields (beta, std dev, number of holdings)
  - Extended holding fields (sector, asset class, market value, exchange, etc.)

- [ ] Update `symbol_details.html` template:
  - Fund Profile section: add new fields (SFDR, domicile, rebalance frequency, product structure, methodology, fund manager, custodian, issuing company, benchmark ticker)
  - Characteristics section: display beta, std dev, number of holdings
  - Holdings table: add columns for Sector, AssetClass, MarketValue, Exchange (consider conditional visibility or expandable rows for large tables)
  - Sectors section: display per-section date (already supported from Vanguard)
  - Countries section: display per-section date (already supported from Vanguard)

- [ ] Update CSS if needed for new columns/sections
- [ ] Write integration test: BlackRock-style symbol details page renders with all new fields

**Verification:** All new fields visible in web UI; per-section dates displayed; existing WisdomTree/Vanguard pages unchanged.

---

### Task 7: Register extractor + integration tests [PRIORITY: MEDIUM]

**Corresponds to:** Stories 1, 3, 4 (routing, display, background refresh)

**Description:** Register the BlackRock extractor in the dispatcher and write integration tests covering the full stack.

- [ ] Register `blackrock` extractor in `internal/api/router.go` (added to extractor registry)
- [ ] Write integration test: full extraction pipeline (dispatcher routing, extractor registration, full stack round-trip)
- [ ] Write integration test: API endpoint `/api/symbols/{id}` returns BlackRock data with new fields (SFDR, domicile, beta, extended holdings)
- [ ] Write integration test: web page `/symbols/{id}/details` renders correctly with symbol name and data
- [ ] Write integration test: data source URL round-trip through stale query
- [ ] Write integration test: equity fund with P/E, P/B, beta characteristics
- [ ] Write integration test: bond fund with YTM, duration characteristics
- [ ] Write integration test: sector allocation derived from holdings
- [ ] Write integration test: country allocation derived from holdings
- [ ] Write integration test: background refresh triggers both BlackRock details and Yahoo market data
- [ ] Write integration test: extraction failure during background refresh marks symbol as failed, preserves previous data
- [ ] Update `features/README.md` to mark f026 as in-progress

**Verification:** BlackRock extractor registered and functional; full stack integration tests pass; existing extractors unaffected; background refresh integration works.

---

## Technical Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | **HTTP client: CycleTLS for all requests** | Both product page and JSON API are on `www.ishares.com` behind the same Cloudflare/WAF. Follows existing pattern (DWS, Dimensional, iMGP, WisdomTree all use CycleTLS for everything). Simpler code — one client. |
| 2 | **Sector/geography derived from holdings** | Direct API for sector/geography returns HTTP 500. Holdings JSON includes sector and location per security — aggregate weights to derive allocations. Trade-off: computed values may not match iShares' published breakdown exactly. |
| 3 | **No database migration** | All new fields flow through existing JSON columns (`fund_profile`, `equity_valuation`, `top_holdings`). JSON is flexible and automatically accommodates new fields. |
| 4 | **Atomic two-phase extraction** | Phase 1 (product page) is required for Phase 2 (holdings). If Phase 1 fails, the entire extraction fails. If Phase 2 fails, the entire extraction fails. No partial data. |
| 5 | **Component ID extracted from page HTML** | The holdings JSON URL requires a page-specific component ID embedded in the HTML. Extracted via regex on the holdings download link. |
| 6 | **ETFs only** | Non-ETF funds (tracker funds) have different page structures and limited data (top 10 holdings only). Out of scope for this feature. |
| 7 | **Holdings JSON API preferred over CSV** | JSON API provides structured data with both display and raw values. CSV is a fallback. |
| 8 | **Per-section "as of" dates** | Holdings carry their own as-of date from the JSON response. Characteristics carry per-metric dates from the HTML. Sector/geography dates inherit from the holdings as-of date. |

## Risks

- **iShares page structure changes**: HTML patterns for Key Facts and characteristics are fragile. Mitigation: clear error messages, explicit logging of parse failures, symbol marked as failed.
- **Cloudflare/bot detection**: Product page has Cloudflare protection. Mitigation: CycleTLS for TLS fingerprinting, rate limiting (1-2s delay), proper User-Agent, graceful error handling.
- **Component ID changes**: The component ID embedded in page HTML may change if BlackRock redesigns the site. Mitigation: clear error message when component ID extraction fails.
- **Large holdings lists (350+)**: Memory and rendering concerns. Mitigation: stream parsing where possible; web template handles large tables (existing position tables handle similar sizes).
- **JSON API date parameter**: The `asOfDate` parameter behavior is unclear (required? optional? filters?). Mitigation: use the as-of date from the product page; if the API rejects it, fall back to no date parameter.
- **Derived sector/geography accuracy**: Computed allocations from holdings may not match iShares' published breakdown. Mitigation: document in UI that values are derived from holdings data.
