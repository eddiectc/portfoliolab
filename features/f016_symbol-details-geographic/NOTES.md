# Notes: Symbol Details — Geographic Data

## Decisions
- 2026-05-17: Tasks 2 and 3 combined into a single commit — the domain type (`GeographicAllocations` field) was a hard compilation dependency for the repo layer, so Task 3 was completed alongside Task 2 rather than waiting for its own turn.

## Deviations from Plan
- None significant. Task ordering adjusted slightly (2+3 together) due to compilation dependency.

## Future Improvements
- None yet.

## Known Issues
- Pre-existing: `TestSymbolDetails_CreateAndEnrich` in `tests/integration` is flaky (async cache timing race). Not related to this feature.

## Post-Review Fixes
- 2026-05-17: Implementation review caught `Percent` value convention inconsistency. Field name "Percent" implies 0–100, but `TopHolding.Percent` and `SectorWeighting.Percent` stored raw Yahoo fractions (0–1) with `* 100` at display. `GeographicAllocation.Percent` in the fetcher stored `100`, clashing with that fraction convention. Resolved by normalizing all three `Percent` fields to 0–100 percentage scale: fetcher converts on parse, web handler displays directly. Commit 78ff8cf (supersedes 37d1d80).

## Validation (Task 7, 2026-05-17)
- `go test ./...`: All pass except pre-existing flaky `TestSymbolDetails_CreateAndEnrich`
- `go vet ./...`: Clean
- Cross-layer consistency: `geographic_allocations` flows correctly through DB (TEXT/JSON) → sqlc (`sql.NullString`) → repo (JSON deserialization) → domain (`[]GeographicAllocation`) → API (`[]symbol.GeographicAllocation`, sorted desc) → web (`[]displayGeographicAllocation`, pre-formatted)
- Spec scenarios verified: stock country extraction, API response format, web UI rendering (single/multi/null), background refresh reuse, unavailable data handling
- No TODOs/FIXMEs introduced by this feature
