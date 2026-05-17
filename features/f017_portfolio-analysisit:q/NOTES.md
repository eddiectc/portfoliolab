# Notes: Portfolio Analysis

## Decisions
- 2025-05-17: Task 1 complete — domain types defined in `internal/domain/analysis/analysis_types.go`. All types use `float64` for percentage/weight fields (consistent with `symbol.SymbolDetails` which stores percentages as float64). `EstimatedDollarImpact` uses `decimal.Decimal` since it represents an actual monetary amount.
- 2025-05-17: `PositionWithDetails` was already defined in Task 1 (`analysis_types.go`), so Task 2 reused it without modification.
- 2025-05-17: `roundTo2` helper uses `int(v*100+0.5)/100` for float64 rounding — simple, no math library dependency, consistent with display-only precision.
- 2026-05-17: Task 3 complete — `roundTo2` fixed to use `math.Round(v*100)/100` instead of `int(v*100+0.5)/100`. The original formula truncated toward zero for negative values (e.g. -0.999 → -0.99 instead of -1.0), which broke negative correlation display. `math.Round` handles both positive and negative correctly.

## Deviations from Plan
- Task 2: Plan said single ETF returns empty for both pairwise and concentrated stocks. Changed to still compute concentrated stocks for 1 ETF — useful to see what a single ETF is most concentrated in, even without pairwise comparison. Pairwise still requires 2+ ETFs.

## Future Improvements
- N/A

## Known Issues
- None.

## Session 2026-05-17 (Task 4)

### Prerequisite: Add `sector` field for individual stocks
The plan referenced `assetProfile.Sector` for individual stocks, but this field was fetched from Yahoo Finance and not persisted. Added:
- Migration 018: `sector TEXT` column on `symbol_details` table
- `Sector` field on `SymbolDetails` struct (in `internal/types/symbol/symbol_details.go`)
- Fetcher populates `Sector` from `assetProfile.Sector` for non-ETF symbols
- Updated sqlc schema + queries (`symbol_details.sql`, `schema.sql`)
- Updated repo to serialize/deserialize the field
- Updated test schema in `symbol_details_repo_test.go`
- Ran `sqlc generate` to regenerate types

### Implementation
- `internal/domain/analysis/allocation.go`: `ComputeSectorAllocation` and `ComputeGeographicAllocation`
- `internal/domain/analysis/allocation_test.go`: 17 unit tests covering happy paths, edge cases, partial data, empty states, and sorting
- Helper functions `getETFWithSectorWeightings`, `getStockWithSector`, `getETFWithGeographicAllocations`, `getStockWithCountry` for test fixtures
- `AllocationBreakdownSorted` convenience function for ordered output
- `normalizeSector` placeholder for consistent sector naming (currently passthrough — Yahoo sectors are already consistent)
