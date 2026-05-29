# Implementation Plan: Dimensional Fund Advisors (DFA) Scraper

## Overview
Implement a specialized data extractor for Dimensional Fund Advisors (DFA) ETFs to fetch comprehensive fund data (holdings, profile, NAV history, allocations) directly from the provider, bypassing incomplete Yahoo Finance data.

## Task Dependencies
T1 (Setup) $\rightarrow$ T2 (Client) $\rightarrow$ T3-T9 (Parsers) $\rightarrow$ T10 (Integration) $\rightarrow$ T11 (Testing) $\rightarrow$ T12 (Registration)

## Tasks

### Task 1: Setup Project Structure [PRIORITY: HIGH]
**Description:** Create the package directory and basic file skeletons for the Dimensional extractor.
- [x] Create `internal/domain/extractor/dimensional/` directory.
- [x] Create `extractor.go`, `client.go`, `parsers.go`, `matcher.go`, `extractor_test.go`, `parsers_test.go`, and `matcher_test.go`.
- [x] Define the `Extractor` struct and `NewExtractor` function.

**Verification:** Files created and package compiles.

### Task 2: Implement Dimensional Client [PRIORITY: HIGH]
**Description:** Implement a browser-grade HTTP client using `CycleTLS` to bypass bot protection and enforce rate limiting.
- [x] Implement `Client` struct with `cycleTLS.CycleTLS`.
- [x] Configure JA3 fingerprint and User-Agent for Dimensional's site (start with Chrome 129 and verify).
- [x] Implement `Fetch` method with a minimum delay (e.g., 1s) between requests.
- [x] Add `SetFetchFunc` for testing purposes.

**Verification:** Client successfully fetches a Dimensional fund page without being blocked.

### Task 3: Implement Fund Info & Profile Parsers [PRIORITY: HIGH]
**Description:** Extract basic fund identity and metadata.
- [x] Implement `ParseFundInfo` (Symbol, Name).
- [x] Implement `ParseFundProfile` (AUM, TER, Inception Date, Family, Legal Type).
- [x] Write unit tests with HTML samples.

**Verification:** Parsers correctly extract identity and profile data from sample HTML.

### Task 4: Implement As-Of Date Parser [PRIORITY: HIGH]
**Description:** Extract the "As of" reference date for the fund's data.
- [x] Implement `ParseAsOfDate` to find the reference date in the NAV table header.
- [x] Ensure support for common date formats used by Dimensional.
- [x] Write unit tests.

**Verification:** Parser correctly extracts the date as a `time.Time` object.

### Task 5: Implement Holdings Parser [PRIORITY: HIGH]
**Description:** Extract the full list of fund holdings.
- [x] Implement `ParseHoldingsCSV` to extract all securities and weights.
- [x] Handle potential malformed CSV rows.
- [x] Filter out cash/currency positions.
- [x] Write unit tests.

**Verification:** Full holdings list is extracted and filtered correctly.

### Task 6: Implement Country Allocation Parser [PRIORITY: MEDIUM]
**Description:** Extract geographic distribution of the fund.
- [x] Implement `ParseCountryAllocation`.
- [x] Clean country names (remove numbering).
- [x] Write unit tests.

**Verification:** Country weights are correctly parsed.

### Task 7: Implement Sector Weightings Parser [PRIORITY: MEDIUM]
**Description:** Extract sector-level allocations.
- [x] Implement `ParseSectors`.
- [x] Handle aggregation if the source provides security-level sector data.
- [x] Write unit tests.

**Verification:** Sector weights are correctly parsed.

### Task 8: Implement NAV History Parser [PRIORITY: MEDIUM]
**Description:** Extract historical NAV data points.
- [x] Implement `ParseNavHistory`.
- [x] Parse dates and NAV values into `extractor.NavPoint` slice.
- [x] Use `govalues/decimal` for high-precision NAV storage.
- [x] Write unit tests.

**Verification:** NAV history is correctly extracted and parsed.

### Task 9: Implement URL Matcher [PRIORITY: LOW]
**Description:** Implement logic to determine if a URL belongs to Dimensional.
- [x] Implement `URLMatcher` and `Match` method.
- [x] Write unit tests.

**Verification:** Matcher correctly identifies Dimensional URLs.

### Task 10: Integrate Parsers into Extractor [PRIORITY: HIGH]
**Description:** Wire all parsers into the `Extract` method to provide atomic extraction.
- [x] Implement `Extract` coordinating CSV and HTML paths.
- [x] Implement `extractISIN` helper.
- [x] Map results to `extractor.ExtractResult`.

**Verification:** `Extractor.Extract` returns a complete result for a valid URL and fails for invalid/incomplete data.

### Task 11: Testing & Validation [PRIORITY: HIGH]
**Description:** Comprehensive testing of the extractor.
- [x] Add real HTML samples to `testdata/`.
- [x] Run all unit tests for parsers.
- [x] Run integration tests with a mock client.
- [x] Perform a "smoke test" against the live site to verify TLS fingerprint.

**Verification:** All tests pass; live extraction succeeds for at least one fund.

### Task 12: Registry Integration [PRIORITY: HIGH]
**Description:** Register the Dimensional extractor in the system.
- [x] Add `extractorReg.Register(dimensional.NewExtractor())` in `internal/api/router.go`.

**Verification:** Symbols assigned to the Dimensional provider are routed to the new extractor.

## Technical Decisions
| Decision | Choice | Reason |
|---|---|---|
| Extraction Strategy | API-First (`public/v2`) | Significantly more robust and faster than DOM scraping; avoids browser overhead. |
| Bot Bypass | `CycleTLS` | Used for API requests to maintain realistic JA3 fingerprints and User-Agents. |
| Parsing Strategy | JSON / `encoding/json` | Standard library JSON parsing is sufficient for the API responses. |
| Testability | `ClientAPI` Interface | Decouples the extractor from the network client, allowing for precise mocking of API responses. |
| Atomicity | All-or-nothing | Required by SPEC.md; prevents persisting partial or stale fund data. |
| Precision | `govalues/decimal` | Ensures high precision for NAV and weights, as per project conventions. |

## Risks
- **Site Layout Change**: Regex parsing is brittle. *Mitigation: Comprehensive unit tests with HTML snapshots.*
- **Bot Protection Update**: TLS fingerprints can become obsolete. *Mitigation: Keep `CycleTLS` configuration updated; use a dedicated `Client` for easy updates.*
- **Provider ID Mismatch**: Dimensional might use internal IDs instead of tickers. *Mitigation: Rely on the `sourceURL` provided by the dispatcher, which is based on the provider configuration.*
