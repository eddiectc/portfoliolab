# Feature f025 — vanguard-scraper: Notes

## Overview

Vanguard UK (`vanguardinvestor.co.uk`) exposes a public REST + GraphQL API. The extractor uses a two-phase approach: REST lookup for identifier resolution + fund profile, then GraphQL for deep data (holdings, allocations, characteristics, prices).

See **RESEARCH.md** for full API endpoint details, query specifications, and technical findings.

## Key Design Decisions

1. **Two-phase extraction**: REST for portId + profile overview, GraphQL for deep data.
2. **GraphQL as primary data source**: Holdings (paginated), sector/country allocation (with benchmark), and fund characteristics.
3. **Holdings pagination**: 1500 items per page, `lastItemKey` is a JSON string. Tested with 4070 holdings (3 pages).
4. **All-or-nothing**: If any query fails, the entire extraction fails. Partial data is not persisted.
5. **Benchmark comparison**: Sector and country allocations include benchmark percentages alongside fund percentages.
6. **Fund characteristics**: Both equity-specific (P/E, P/B, market cap) and bond-specific (coupon, maturity, duration) codes available. Null for non-applicable types.

## Test Fund

- **VWRL** (Vanguard FTSE All-World UCITS ETF USD Distributing)
- URL: `vanguard-ftse-all-world-ucits-etf-usd-distributing`
- portId: 9505
- Holdings: 4070 items (3 pages)
- Sectors: 12 (ICB standard)
- Countries: 107 (FTSE Country of Risk)
