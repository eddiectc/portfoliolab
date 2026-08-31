# Notes: Dimensional Fund Advisors (DFA) Scraper

## Implementation Pivot: API-First Approach
Initially, the plan was to use a headless browser to scrape rendered HTML pages. However, discovery revealed a set of public JSON APIs (`/public/v2/fundcenter` and `/public/v2/fundcenter/funddetail`) that provide all required data more reliably and faster.

### Key Findings:
- **Two-Step Mapping**: Funds are identified by ISIN, but the detail API requires an internal `portfolioNumber`. This must be mapped using the Fund Center registry first.
- **Wall Bypass**: The "Professional Investor" affirmation wall, which was a major concern in the initial research, is not required for the `public/v2` endpoints.
- **CSV Holdings**: Full holdings are still provided as static CSVs, but the URL to these CSVs is now retrieved dynamically from the Detail API.

## Testing Lessons
- **Closed-Loop Validation Error**: During initial implementation, tests were written based on assumed JSON structures. This created a "closed loop" where the code and tests shared the same incorrect assumption, causing tests to pass while the application failed.
- **Ground Truth**: Tests must be validated against actual sample data (e.g., `samples/funddetail.json`) to ensure the implementation matches the real-world API response.

## Implementation Review Fixes (2026-05-30)
- **Empty holdings validation**: Added `len(holdings) == 0` check in `Extract()` per spec atomicity requirement. Returns explicit error instead of silently returning empty holdings.
- **Test quality**: Replaced `testify/assert` with standard library `t.Errorf` per project conventions. Converted all tests to table-driven format.
- **matcher_test.go**: Was empty; populated with comprehensive URL matching tests (6 cases).
- **parsers_test.go**: Expanded from 2 test cases to 25+ covering edge cases (cash filtering, percent signs, commas, currency symbols, large lists, empty CSV, invalid JSON).
- **extractor_test.go**: Expanded from 3 test cases to 10 covering all error paths (registry failure, ISIN not found, detail API failure, CSV fetch failure, empty holdings atomic failure, missing CSV URL, context cancellation, invalid ISIN, int NAV values).
- **testdata/**: Created with real sample data (fundcenter_registry.json, funddetail.json, holdings.csv).
- **Integration tests**: Added `tests/integration/dimensional_extractor_test.go` with 7 tests covering routing, registration, symbol CRUD with data_source_url, NAV data type isolation, and full-stack round-trip.
- **Code cleanup**: Removed unused `bypassFunc` type from `client.go`.
- **EOF handling**: Replaced fragile `err.Error() == "EOF"` string comparison with `errors.Is(err, io.EOF)`.
- **Silent failure fix**: Added `slog.Warn` logging for skipped CSV rows (malformed, short, unparseable weight).
- **Explicit error logging**: Extractor returns errors (standard pattern); logging happens at the dispatcher/handler layer via existing `slog` middleware.

## Mutual Fund Detection (2026-08-31)
- **Root cause of the DDGT extraction failure**: The Dimensional Fund Center publishes full holdings CSVs only for UCITS ETFs. `IE00B2PC0609` (DFA Global Targeted Value, USD, Acc.) is a **mutual fund** — its `funddetail` response carries no `charsEtfTopHoldingsDaily` lens, so no `fullHoldingsCsvUrl` exists. The old generic "full holdings CSV URL not found in response" error gave no actionable direction. The Dimensional site's own "Download all holdings" link is likewise absent for mutual funds (verified against the relaunched frontend, ddxtools 2.69.1).
- **Detection**: `fundFacts` carries `isEtf` / `isDfaUcitsEtf` boolean flags. `ParseFundDetail` now maps them to `FundProfile.LegalType` ("ETF" / "Mutual Fund") and returns the legal type alongside the name.
- **Actionable error**: When the CSV URL is missing and `LegalType != "ETF"`, `Extract` returns "… is a Dimensional mutual fund, not a UCITS ETF — full holdings are only published for UCITS ETFs — use the UCITS ETF share class <ISIN> instead". The suggested ISIN is found by matching the mutual fund's `marketingName` against the registry (e.g. "Global Targeted Value Fund (USD, Acc.)" → "Global Targeted Value UCITS ETF (Acc.)", `isDfaUcitsEtf` required); if no unique match exists the suggestion is omitted.
- **Tests**: three new `TestExtractor_Extract` cases (suggestion, no-matching-ETF, ETF-without-CSV → generic error), `TestSuggestUcitsEtf` table test, and a mutual-fund legal-type case in `TestParseFundDetail`.
- **User-facing fix**: for DDGT, track the UCITS ETF `IE000S67ID55` (DFA Global Targeted Value UCITS ETF (Acc)) instead — it publishes all 100+ holdings.
