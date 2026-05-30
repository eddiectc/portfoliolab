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
