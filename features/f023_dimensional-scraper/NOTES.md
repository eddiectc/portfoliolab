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
