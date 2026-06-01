# Notes: BlackRock/iShares Data Extractor

## Decisions
- 2026-06-01: Changed `CharacteristicsFieldsMask` from `uint16` to `uint32` — the original 14-bit mask would overflow when adding 3 new BlackRock fields (17 total entries, `1 << 16` overflows `uint16`).
- 2026-06-01: `jsonNode` uses a custom `UnmarshalJSON` to handle mixed string/object cells in the holdings JSON API (some cells are plain strings like ticker/name, others are `{"display":..., "raw":...}` objects).
- 2026-06-01: `parseFloatValue` strips parenthetical date suffixes BEFORE stripping `%` — the `%` can appear mid-string (e.g. `16.62% (as of 30/Apr/2026)`), so `TrimSuffix("%")` alone would miss it.

## Deviations from Plan
- Task 3: Test `TestExtractor_Extract_MissingAsOfDate` renamed to `TestExtractor_Extract_Cancellation` (context cancellation) — the as-of date test is covered by `TestParseAsOfDate` in parsers_test.go. The extractor-level test focuses on context cancellation instead.

## Future Improvements
- None yet.

## Known Issues
- None.
