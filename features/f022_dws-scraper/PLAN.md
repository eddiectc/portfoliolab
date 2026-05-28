# Implementation Plan: Data Extractor Framework with DWS Provider

## Overview
Implementation of a specialized data extractor for DWS Xtrackers ETFs. This extractor will utilize the DWS JSON API to fetch comprehensive fund details, including full holdings, accurate TER, and NAV history, integrating seamlessly with the existing extractor framework established in f021.

## Task Dependencies
Task 1 (Client) $\rightarrow$ Task 2 (Parsers) $\rightarrow$ Task 3 (Extractor) $\rightarrow$ Task 4 (Registration) $\rightarrow$ Task 5 (Integration)

## Tasks

### Task 1: API Client Implementation [PRIORITY: HIGH]
**Corresponds to:** Story 1 (Data extraction)
**Description:** Build a robust HTTP client capable of interacting with the DWS JSON API.

- [x] Create `internal/domain/extractor/dws/client.go`.
- [x] Implement `Client` using `CycleTLS` to ensure browser-grade fingerprinting (consistent with WisdomTree provider).
- [x] Implement a `Fetch(endpoint string, slug string)` method to construct and call DWS API URLs: `https://etf.dws.com/api/pdp/en-gb/etf/{slug}/{endpoint}`.
- [x] Implement rate limiting with a minimum delay between requests.
- [x] Write unit tests for the client with mock responses.

**Verification:** Client can successfully fetch JSON from various DWS endpoints for a known slug without being blocked.

### Task 2: JSON Parsers and Aggregators [PRIORITY: HIGH]
**Corresponds to:** Story 1 (Data extraction)
**Description:** Implement the logic to transform DWS JSON responses into the `extractor.ExtractResult` domain models.

- [x] Create `internal/domain/extractor/dws/parsers.go`.
- [x] Define Go structs matching the DWS API JSON schemas.
- [x] Implement `ParseFundInfo`, `ParseFundProfile` (including TER), and `ParseNavHistory`.
- [x] Implement `ParseHoldings` to extract all security holdings.
- [x] Implement aggregation logic to derive `CountryAllocation` and `SectorWeightings` from the raw holdings list.
- [x] Implement "As of" date parsing to provide the reference date for the entire extraction.
- [x] Create `testdata/` directory with real JSON samples from DWS API.
- [x] Write exhaustive unit tests for each parser.

**Verification:** All parsers correctly map JSON to domain models, and aggregations (Country/Sector) match expected sums.

### Task 3: Extractor Orchestration [PRIORITY: HIGH]
**Corresponds to:** Story 1, Story 2 (Integration)
**Description:** Implement the `Extractor` interface and manage the atomic extraction process.

- [x] Create `internal/domain/extractor/dws/extractor.go`.
- [x] Implement `Extractor` interface (`Name()`, `Extract()`).
- [x] Implement the `Extract` method to coordinate multiple API calls (Settings, Holdings, Performance Chart).
- [x] Enforce atomicity: Fail the entire extraction if "Holdings" or "Reference Date" are missing or empty.
- [x] Map the combined results into a single `extractor.ExtractResult`.
- [x] Write extractor tests using a mock client to verify orchestration and error handling.

**Verification:** `Extract` returns a complete `ExtractResult` on success and a descriptive error if required sections are missing.

### Task 4: Matcher and System Registration [PRIORITY: MEDIUM]
**Corresponds to:** Story 2 (Integration)
**Description:** Integrate the DWS provider into the system's dispatcher.

- [x] Create `internal/domain/extractor/dws/matcher.go` and implement `URLMatcher` to recognize DWS slugs/URLs.
- [x] Register the DWS extractor in `internal/api/router.go` using `extractorReg.Register(dws.NewExtractor())`.
- [x] Verify that symbols assigned to the DWS provider are correctly routed to the DWS extractor.

**Verification:** A request for a DWS-configured symbol triggers the DWS extractor instead of the default.

### Task 5: End-to-End Integration Testing [PRIORITY: MEDIUM]
**Corresponds to:** All Stories
**Description:** Verify the full data pipeline from the live DWS API to the system.

- [x] Create `internal/domain/extractor/dws/real_test.go`.
- [x] Implement tests that perform live extractions for a set of known DWS ETFs.
- [x] Verify that the resulting `ExtractResult` contains accurate and complete data.
- [x] Test edge cases: invalid slugs (404), rate limit triggers, and partial API failures.

**Verification:** Live tests pass, confirming that the extractor works against the current DWS API.

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| **HTTP Client** | `CycleTLS` | Maintains consistency with existing scrapers and provides protection against bot detection on financial sites. |
| **Data Mapping** | Parser-level aggregation | Aggregating Country/Sector weights within the parser layer keeps the `Extractor` orchestration clean and the logic testable via JSON samples. |
| **Atomicity** | Strict All-or-Nothing | Following the project pattern: partial data is dangerous for portfolio analytics; it's better to fail and log than to persist incomplete holdings. |
| **Identifier** | Slug-based lookup | DWS API requires specific slugs; the system will rely on these slugs being provided in the symbol configuration. |

## Risks
- **API Changes**: DWS may change their internal JSON API without notice. *Mitigation: Comprehensive unit tests with `testdata` to quickly identify breaking changes.*
- **Rate Limiting**: Aggressive rate limiting could slow down background refreshes. *Mitigation: Implement a configurable delay in the client.*
