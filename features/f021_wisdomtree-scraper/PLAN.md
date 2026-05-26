# Implementation Plan: Data Extractor Framework with WisdomTree Provider

## Overview

Build an extensible data extractor framework that routes symbol details fetching to alternative providers (WisdomTree) instead of Yahoo Finance. The user sets a source URL on the symbol mapping (e.g. `https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt---...`), and the dispatcher determines which extractor to use by matching the URL domain/pattern. The WisdomTree extractor parses inline CSV data embedded in JavaScript variables on fund pages for comprehensive data: full holdings (800+), NAV history, market cap breakdown, fund characteristics, theme/sector/country allocations. NAV history is stored in the existing `market_data` table with a new `nav` data type.

## Task Dependencies

```
Task 1 (DB Schema) ──┬──► Task 2 (URL Registry + Dispatcher)
                     │
                     ├──► Task 3 (WisdomTree Extractor)
                     │        │
                     │        └──► Task 4 (Service Layer) ──► Task 5 (Background Refresh)
                     │                                        │
                     │                                         └──► Task 7 (Web UI — URL input)
                     │
                     └──► Task 6 (API) ──────────────────────────┘

Task 9 (Cross-Layer Audit) runs alongside Tasks 3-6 and verifies all data_type queries.
```

## Tasks

### Task 1: Database schema changes [PRIORITY: HIGH]

**Corresponds to:** All scenarios (foundation)

**Description:** Add columns for source URL assignment, extractor metadata, and NAV support.

- [x] Migration 021: `ALTER TABLE symbol_mappings ADD COLUMN data_source_url TEXT DEFAULT NULL`
- [x] Migration 021: `ALTER TABLE symbol_details ADD COLUMN extractor_as_of_date TEXT` (provider's "as of" date, distinct from `fetched_at`)
- [x] Update `schema.sql` for sqlc
- [x] Add new sqlc queries for NAV: `GetNavHistoryBySymbol` (SELECT from market_data WHERE data_type='nav')
- [x] Run `sqlc generate`
- [x] Update `ListStaleSymbolDetails` query to include `data_source_url` column (add to SELECT)
- [x] Write tests (migration smoke test verifies schema applies cleanly)

**Verification:** `goose up` runs cleanly, `sqlc generate` succeeds, new queries compile.

---

### Task 2: URL registry and dispatcher [PRIORITY: HIGH]

**Corresponds to:** Story 5 (Extensible provider architecture)

**Description:** Create the extensible framework for registering extractors by URL pattern and dispatching to the correct extractor based on the source URL.

- [x] Create `internal/domain/extractor/` package
- [x] Define `Extractor` interface: `Name() string`, `Extract(ctx, url string) (*ExtractResult, error)` — receives the full source URL
- [x] Define `ExtractResult` struct: holds all extracted data sections (overview, holdings, nav, etc.) plus `AsOfDate time.Time`
- [x] Define `URLMatcher` type: `Match(url string) bool` — checks if URL belongs to this extractor
- [x] Define `Registry` type: `Register(Extractor)`, `FindByURL(url string) (Extractor, error)`, `Get(name string) (Extractor, error)`
- [x] Define `Dispatcher` type: `Dispatch(ctx, sourceURL string) (*ExtractResult, error)` — finds matching extractor by URL, returns explicit error for no match
- [x] Register WisdomTree at startup in `router.go` (new provider = new code)
- [x] Write unit tests for registry (register, find by URL, not found, get by name)
- [x] Write unit tests for dispatcher (dispatch to registered, error for no match, error propagated from extractor)
- [x] Write unit tests for URL matching (wisdomtree.eu matches, other domains don't)

**Verification:** Registry finds extractors by URL; dispatcher routes correctly; no match returns explicit error.

---

### Task 3: WisdomTree extractor [PRIORITY: HIGH]

**Corresponds to:** Story 2 (WisdomTree data extraction)

**Description:** Implement the WisdomTree-specific HTTP client and HTML parser. Data is embedded inline in the HTML as CSV strings inside JavaScript variables (NOT HTML tables). Uses regex to extract each `var fundXxxData = '...'` block, then parses the CSV. Extraction is atomic — if any section fails, entire extraction is rejected.

**Data sources** (from RESEARCH.md):

| Data | Source | Extraction Method |
|------|--------|-------------------|
| Fund Info | `var fundInfo<HASH> = {...}` | Regex + JSON parse |
| AUM, TER, Inception Date | Raw HTML tables (Product Overview + NAV) | Regex on table rows |
| Holdings | `var fundHoldingsData = '...'` | Regex + CSV parse |
| NAV History | `var fundMarketData<HASH> = '...'` | Regex + CSV parse |
| Themes | `var fundThemeData = '...'` | Regex + CSV parse |
| Sectors | `var fundSectorsData = '...'` | Regex + CSV parse |
| Country Allocation | HTML table in raw HTML (`id="country-allocation-section"`) | Regex on HTML table rows |
| Market Cap | HTML table in raw HTML (`id="fund-facts-section"`) | Regex on HTML table rows |
| Fund Characteristics | HTML table in raw HTML (`id="fund-facts-section"`) | Regex on HTML table rows |
| As Of Date | Raw HTML `<th>Net Asset Value</th><th>22 May 2026</th>` | Regex on raw HTML |

- [x] Create `internal/domain/extractor/wisdomtree/` package
- [x] Implement HTTP client with 1-2s rate limiting between requests and error handling for non-200 responses / Cloudflare blocks
- [x] Implement `ParseFundInfo` — regex `var fundInfo\w+ = \{...\}`, extract symbol + name
- [x] Implement `ParseFundProfile` — regex on raw HTML tables: AUM (`<td>Total AUM of fund</td>`), TER (`<td class="key">TER</td>`), Inception Date (`<td class="key">Inception Date</td>`)
- [x] Add `InceptionDate time.Time` to `symbol.FundProfile` struct (AUM → existing `TotalNetAssets`, TER → existing `AnnualExpenseRatio`)
- [x] Implement `ParseHoldings` — regex `var fundHoldingsData = '...'`, CSV parse (date, Weight, Security Description), filter out cash/currency positions
- [x] Implement `ParseNavHistory` — regex `var fundMarketData\w+ = '...'`, CSV parse (date, fund_ticker, nav, uv10KMP, uv10KNAV)
- [x] Implement `ParseThemes` — regex `var fundThemeData = '...'`, CSV parse (date, Weight, Security Description)
- [x] Implement `ParseSectors` — regex `var fundSectorsData = '...'`, CSV parse (date, securityName, weight, Sector, wgtSector), aggregate to sector totals
- [x] Implement `ParseAsOfDate` — regex `<th>\s*Net Asset Value\s*</th>\s*<th>\s*([^<]+)\s*</th>` from raw HTML
- [x] Implement `wisdomtree.URLMatcher` — matches `*.wisdomtree.eu/*` pattern
- [x] Implement `wisdomtree.Extractor` struct satisfying `extractor.Extractor` interface
- [x] Implement orchestrator: calls all parsers, returns `ExtractResult` or error (atomic — no partial data)
- [x] Write unit tests for each parser (table-driven with sample CSV snippets from `samples/`)
- [x] Write unit test for atomic extraction (one parser fails → entire extraction fails)
- [x] Write unit test for empty/missing data sections → error
- [x] Write unit test for URL matching (wisdomtree.eu URLs match, others don't)

**Phase 2 (HTML table parsing, lower priority):**

- [x] Decide on approach: **CycleTLS + HTML parsing** — confirmed working. Country allocation, market cap, and fund characteristics ARE in raw HTML as standard tables. No headless browser needed.
- [x] Implement `ParseCountryAllocation` — regex on HTML table in `id="country-allocation-section"` (strip "N. " prefix, parse %)
- [x] Implement `ParseMarketCap` — regex on HTML table in `id="fund-facts-section"` (total + breakdown)
- [x] Implement `ParseFundCharacteristics` — regex on HTML table in `id="fund-facts-section"` (P/E, P/B, P/S, P/CF, div yield)
- [x] Write unit tests for HTML table parsers (inline sample HTML in test file)
- [x] Remove `Renderer` interface (unnecessary — all data in raw HTML)

**Verification:** All parsers return correct structs from sample CSV/HTML; atomic extraction rejects partial results; implements `extractor.Extractor` interface; URL matcher works correctly.

---

### Task 4: Service layer — URL-aware symbol details [PRIORITY: HIGH]

**Corresponds to:** Story 1 (Provider-based symbol details routing), Story 2 (partial — persisting extracted data)

**Description:** Update the symbols service to route FetchAndStore/RefreshSymbol through the extractor dispatcher when a source URL is configured. Also persist extracted data (symbol details + NAV history).

- [ ] Update `symbol.StaleSymbol` type to include `DataSourceURL string`
- [ ] Update `SymbolDetailsRepository` interface: add `GetDataSourceURL(ctx, internalSymbol) (string, error)` and `SetDataSourceURL(ctx, internalSymbol, url string) error`
- [ ] Update `SymbolDetailsRepository` implementation: SQL queries for reading/writing `data_source_url`
- [ ] Update `symbols.Service` to accept `extractor.Dispatcher` and `MarketDataRepository` (for NAV storage)
- [ ] Update `FetchAndStore`: check source URL → dispatch to extractor → convert `ExtractResult` to `SymbolDetails` → upsert details + NAV history (atomic)
- [ ] Update `RefreshSymbol`: same routing logic
- [ ] Add `SetDataSourceURL` and `GetDataSourceURL` methods on service
- [ ] Convert `ExtractResult` fields to `symbol.SymbolDetails` fields:
  - Holdings → TopHoldings (all, not limited to 10)
  - Sectors → SectorWeightings
  - Themes → stored in new JSON column `themes`
  - Market Cap → stored in new JSON column `market_cap_breakdown`
  - Countries → GeographicAllocations (when available)
  - Fund info → update ShortName/LongName if present
  - Fund profile (AUM, TER, Inception Date) → FundProfile (TotalNetAssets, AnnualExpenseRatio, InceptionDate)
  - Characteristics → EquityValuation (P/E, P/B, P/CF, P/S)
- [ ] Write unit tests: FetchAndStore routes to extractor when URL set, routes to Yahoo when no URL
- [ ] Write unit tests: extraction error → not persisted (atomic)
- [ ] Write unit tests: NAV history stored with data_type='nav' and source='wisdomtree'
- [ ] Write unit tests: SetDataSourceURL / GetDataSourceURL

**Verification:** URL-configured symbols route to extractor; non-configured route to Yahoo; extracted data persisted correctly; NAV stored with correct data_type and source.

---

### Task 5: Background refresh integration [PRIORITY: MEDIUM]

**Corresponds to:** Story 4 (Background refresh integration)

**Description:** Update the MarketCache background fetcher to refresh both symbol details (via extractor or Yahoo) and NAV history for URL-configured symbols.

- [ ] Update `ListStaleSymbolDetails` SQL query to return `data_source_url` column
- [ ] Update `StaleSymbol` type in repo layer to include `DataSourceURL`
- [ ] Update `refreshStaleSymbolDetails` in MarketCache: pass source URL through to `RefreshSymbol` (already handled by service layer routing via internal symbol lookup)
- [ ] Add NAV history refresh alongside symbol details refresh in `RefreshSymbol` (already handled in Task 4)
- [ ] Update `RefreshAll` to also trigger NAV history refresh for URL-configured symbols (full re-fetch, not incremental)
- [ ] Write unit tests: stale refresh routes by URL
- [ ] Write unit tests: RefreshAll includes NAV for URL symbols

**Verification:** Background refresh routes to correct source; NAV refreshed for URL symbols; existing Yahoo-only symbols unaffected.

---

### Task 6: API — source URL assignment and extractor data [PRIORITY: MEDIUM]

**Corresponds to:** Story 1 (partial — API surface)

**Description:** Expose source URL assignment in the symbol CRUD API and include extractor-specific fields in responses.

- [ ] Update `symbolmapping.UpdateRequest` to include `DataSourceURL *string`
- [ ] Update `symbolmapping.SymbolMapping` type to include `DataSourceURL string`
- [ ] Update `symbolmapping.Service` to handle source URL assignment on Update
- [ ] Update `SymbolMappingRepository` SQL queries to read/write `data_source_url`
- [ ] Update `SymbolGetResponse` to include `data_source_url` field
- [ ] Update `SymbolDetailsResponse` to include extractor-specific fields: `ExtractorAsOfDate`, `MarketCapBreakdown`, `ThemeBreakdown`, `FullHoldings` (renamed from TopHoldings when from extractor)
- [ ] Update `HandleUpdate` to persist source URL assignment
- [ ] Update `toSymbolGetResponse` / `toSymbolDetailsResponse` to include new fields
- [ ] Write unit tests: PATCH sets data_source_url, GET returns it
- [ ] Write unit tests: SymbolDetailsResponse includes extractor fields when present

**Verification:** Source URL can be set via PATCH; returned in GET; extractor fields included in response.

---

### Task 7: Web UI — source URL input on symbol create/edit [PRIORITY: MEDIUM]

**Corresponds to:** Story 2 (user sets provider URL for symbol)

**Description:** Add a "Source URL" input field to the symbol create and edit forms so the user can specify the provider URL (e.g. WisdomTree ETF page). The dispatcher will match the URL domain to determine which extractor to use.

- [ ] Add `data_source_url` field to `symbolMappingForm` struct (or equivalent form struct)
- [ ] Update `symbol_mappings/create.html` template: add "Source URL" text input field
- [ ] Update `symbol_mappings/edit.html` template: add "Source URL" text input field, pre-populate with existing value
- [ ] Update create handler (`symbol_mappings/create.go`): accept and persist `data_source_url`
- [ ] Update edit handler (`symbol_mappings/edit.go`): accept and persist `data_source_url`
- [ ] Write unit tests: create with source URL persists correctly
- [ ] Write unit tests: edit with source URL updates correctly
- [ ] Write unit tests: empty source URL leaves field NULL (default Yahoo)

**Verification:** Source URL can be set on create and edit; displayed correctly on edit form; NULL when empty.

---

### Task 8: Web UI — extractor data display [PRIORITY: MEDIUM]

**Corresponds to:** Story 3 (Symbol details page — extractor data display)

**Description:** Update the symbol details web page to show all extracted data sections with "as of" dates, and render the NAV vs Price chart.

- [ ] Update `symbolDetailsDisplay` struct to include new sections: `MarketCapBreakdown`, `FundCharacteristics` (extended), `FullHoldings`, `ThemeBreakdown`, `CountryAllocation`, `ExtractorAsOfDate`
- [ ] Update `toDisplayDetails` to populate new sections from `SymbolDetails`
- [ ] Update `symbolDetailsPageData` to include chart data for NAV vs Price
- [ ] Add new template sections in `templates/symbol_details/view.html`:
  - Market Capitalization table (total + large/mid/small)
  - Fund Characteristics table (extended P/E, Est P/E, P/B, P/S, P/CF, div yield)
  - Full Holdings table (all securities, not limited to 10)
  - Theme Breakdown table
  - Sector Breakdown table (replaces Yahoo sector when extractor data present)
  - Country Allocation table (replaces Yahoo geographic when extractor data present)
  - NAV vs Price chart (ECharts, two series, no interpolation)
- [ ] Add conditional logic: show extractor sections when extractor data present, fall back to Yahoo sections otherwise
- [ ] Show "As of {date}" label on each extractor-sourced section
- [ ] Fetch NAV history in handler and serialize as JSON for ECharts
- [ ] Write unit tests: page renders extractor sections when data present
- [ ] Write unit tests: page falls back to Yahoo sections when no extractor data
- [ ] Write unit tests: "As of" date displayed correctly

**Verification:** All new sections render with correct data; "As of" date shown; NAV chart displays both series; Yahoo sections shown when no extractor data.

---

### Task 9: Cross-layer data audit — NAV data_type [PRIORITY: MEDIUM]

**Corresponds to:** All scenarios (data consistency)

**Description:** Audit all SQL queries and repository methods that filter on `data_type` to ensure NAV data is handled correctly. Create NAV-specific queries where needed.

- [ ] Audit `GetHistoricalPricesBySymbolAndRange` — filters `data_type IN ('stock', 'fx')`. **Decision:** Leave unchanged (price charting doesn't include NAV). Create separate `GetNavHistoryBySymbol` query instead.
- [ ] Audit `GetLatestQuote` — filters `data_type = 'stock'`. **Decision:** Leave unchanged (NAV is not a quote).
- [ ] Audit `GetLatestPriceDatePerSymbol` — filters `data_type = 'stock'`. **Decision:** Leave unchanged (gap-fill is for stock prices only).
- [ ] Audit `UpsertHistoricalPrices` — accepts `dataType` parameter. **Decision:** Already supports arbitrary data_type; pass 'nav' from service layer.
- [ ] Audit `GetDistinctCachedSymbols` — no data_type filter. **Decision:** Leave unchanged (returns all types).
- [ ] Verify NAV data is never returned by price-related queries (stock/fx only)
- [ ] Verify NAV data can be fetched by symbol using `GetNavHistoryBySymbol`
- [ ] Write integration test: NAV data stored and retrieved correctly, not mixed with stock prices

**Verification:** All existing queries unchanged (correctly exclude NAV); new NAV query works; no data leakage between types.

---

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Source URL storage | `data_source_url TEXT` column on `symbol_mappings` | User sets a URL when creating/updating a symbol; dispatcher matches URL domain/pattern to determine extractor. NULL = Yahoo Finance (default). |
| URL-based routing | Dispatcher matches URL → extractor by domain | User-friendly (paste URL from browser), no need to know provider names. Extensible: new provider = new URL pattern + new extractor code. |
| NAV history storage | New `data_type = 'nav'` in existing `market_data` table | Reuses existing upsert/query infrastructure. Source = 'wisdomtree' distinguishes from Yahoo. |
| Extractor architecture | Interface + Registry + Dispatcher in `internal/domain/extractor/` | Follows existing pattern of interfaces (e.g., `MarketDataFetcher`, `SymbolDetailsFetcher`). Registry maps URL patterns → extractors; Dispatcher routes by URL. |
| New symbol_details fields (market cap, themes) | New JSON columns on `symbol_details` table | Follows existing pattern (top_holdings, sector_weightings are JSON columns). Market cap and themes are structured data that varies by provider — JSON is flexible. |
| Extractor "as of" date | `extractor_as_of_date TEXT` column on `symbol_details` | Distinct from `fetched_at` (when system fetched). NULL when data is from Yahoo (Yahoo has no "as of" concept). |
| WisdomTree registration | Register at startup in `router.go` | Simpler than API endpoint; WisdomTree is the only provider in scope. Future providers registered the same way. |
| HTML parsing approach | Regex to extract CSV from JS variables + `encoding/csv` | Data is embedded inline in `<script>` blocks as CSV strings. No external library needed. |
| ~~JS-rendered data~~ (country, market cap, characteristics) | Phase 2 (lower priority) | ARE in raw HTML as standard tables. Previously misidentified as JS-rendered. Extractable with regex on HTML tables. |
| Atomic extraction | Buffer all sections in memory, return error if any fails; persist only on full success | Spec requirement: "partial data is not persisted". Simpler than DB transactions for this use case. |
| Full holdings display | No artificial limit (spec: "not limited to top 10") | Existing code limits to 10 for Yahoo data. Extractor data shows all holdings. Template handles large lists. |
| NAV vs Price chart | Two ECharts line series on same axes, no interpolation | Spec: "displayed as-is without interpolation". Non-overlapping date ranges shown naturally. |
| Extractor data replaces Yahoo data | When extractor data present, replace existing fields (holdings, sectors, geographic, fund profile, equity valuation) | Spec: "replaces or augments the existing symbol details fields". Template conditionally shows extractor sections over Yahoo sections. |

## Risks

- **WisdomTree page structure changes**: HTML/JS variable patterns are fragile. Mitigation: clear error messages, explicit logging of parse failures, symbol marked as failed (no silent degradation).
- **Cloudflare/bot detection**: WisdomTree may block automated requests. Mitigation: rate limiting (1-2s delay), proper User-Agent, graceful error handling (symbol marked failed, existing cached data preserved).
- **Large holdings lists (800+)**: Memory and rendering concerns. Mitigation: stream parsing where possible; web template handles large tables (existing position tables handle similar sizes).
- ~~**JS-rendered data (Phase 2)**~~: Country allocation, market cap, and fund characteristics **ARE in raw HTML** as standard tables. Previously misidentified as JS-rendered. No headless browser needed.
- **CSV parsing edge cases**: WisdomTree CSV contains `\u0026` for `&`, quoted strings, and mixed date formats. Mitigation: unit tests against actual sample files.
