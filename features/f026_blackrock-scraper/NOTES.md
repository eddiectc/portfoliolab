# Notes: BlackRock/iShares Data Extractor

## Decisions
- 2026-06-01: Changed `CharacteristicsFieldsMask` from `uint16` to `uint32` — the original 14-bit mask would overflow when adding 3 new BlackRock fields (17 total entries, `1 << 16` overflows `uint16`).
