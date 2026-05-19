# Definition of Done

Checklist applied to every feature before it is declared complete.

## Code Quality
- [ ] Code follows project conventions (docs/CONVENTIONS.md)
- [ ] Code formatted with `gofmt` / `goimports`
- [ ] No ignored errors (`errcheck` clean)
- [ ] No unresolved TODOs or temporary workarounds
- [ ] No scope creep beyond the approved spec

## Testing
- [ ] All spec scenarios have passing tests
- [ ] Tests cover happy paths, error paths, and edge cases
- [ ] Edge cases listed in SPEC.md are handled
- [ ] Unit tests are co-located (`*_test.go` next to source), no DB, no network
- [ ] Hand-written mocks simulate real repository behavior (not permissive)
- [ ] Table-driven tests used for comprehensive coverage
- [ ] Integration tests use in-memory SQLite with real schema (when applicable)
- [ ] `go test ./...` passes

## Domain Logic
- [ ] Monetary values use `decimal.Decimal` — no `float64`
- [ ] Rounding rules and edge cases documented in code comments
- [ ] Position/P&L calculations verified against known-correct values

## Data Layer
- [ ] Database migrations are idempotent and SQLite-compatible
- [ ] Parameterized queries only (no string concatenation)
- [ ] Cross-layer data audit complete (new filterable fields visible on all read paths)
- [ ] Repos handle `decimal.Decimal` ↔ string conversion correctly

## API & Web
- [ ] API returns consistent error responses (`{"error": "...", "code": "..."}`)
- [ ] Explicit errors, no silent fallbacks
- [ ] Web handlers delegate to API/service layer (no duplicated computation)
- [ ] Templates compile without errors; URLs pre-built in handlers

## Documentation
- [ ] NOTES.md documents all deviations and decisions
- [ ] PLAN.md checkboxes reflect actual completion status
- [ ] API.md updated if new endpoints were added
- [ ] Feature index (features/README.md) status updated

## Process
- [ ] Implementation matches the plan; deviations documented in NOTES.md
- [ ] Implementation reviewed via /review-impl
- [ ] Retrospective completed via /retro
