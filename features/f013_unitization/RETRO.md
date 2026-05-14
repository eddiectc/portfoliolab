# Retrospective: Unitization (f013)

## What Went Well
- **Clean separation of concerns**: `ComputeNavHistory` is a pure function in `unitization.go` with no external dependencies, making it easy to test and reason about.
- **No database changes needed**: Computing units/NAV on-the-fly from existing transaction data avoided migrations and keeps the data always consistent.
- **Reused existing infrastructure**: Pre-cash-flow snapshots (already built for TWR) served double duty for NAV breakpoints, avoiding duplicate computation.
- **Comprehensive test coverage**: 19 test functions covering initial deposit, subsequent deposits, withdrawals, zero-value, withdrawal cap, fractional units, same-day dedup, deposits-only fallback, buy-before-deposit, and no-deposit scenarios.
- **Inception date refinement**: Changing from `firstDepositIdx` to `inceptionDate` (a time.Time) made the API clearer and enabled the "points before deposit show 0 NAV" behavior that matches the spec.

## What Could Be Improved
- **Signature change left tests broken**: The `ComputeNavHistory` signature was updated (adding `inceptionDate`), but unit tests weren't updated in the same session. This left the codebase in a broken state for ~1 day. **Lesson**: Update tests immediately when changing function signatures.
- **Typo in type name**: `navbreakpoint` (lowercase b) vs `navBreakpoint` in `equity_curve.go` was a compilation error that could have been caught with an earlier `go build`. **Lesson**: Run `go build ./...` after each file edit, not just `go test`.
- **Misleading test name**: `TestComputeNavHistory_NonDepositFirstTransaction` originally tested the wrong behavior (expected unitization to work without a deposit). It wasn't caught until the signature change forced a full review. **Lesson**: Test names should precisely describe the expected behavior; review test semantics, not just compilation.

## Spec vs Reality
- **All spec scenarios implemented and tested**: Initial deposit, subsequent deposit, withdrawal, NAV changes with market, buy/sell doesn't change units, non-deposit before unitization, zero-value portfolio, fractional units, multiple transactions same day.
- **Inception logic**: The spec says "first deposit triggers unitization" — implementation correctly finds the first deposit date and passes it as `inceptionDate`. Points before that date show 0 units/0 NAV.
- **Edge cases**: All spec edge cases (withdrawal cap, zero-value, same-day dedup, no deposits) are handled and tested.
- **One deviation**: The plan specified P&L-based simple return, but at inception `PortfolioValue == NetDeposit` making it always 0. Switched to value-based (`end/begin - 1`) which is more meaningful. Documented in NOTES.md.

## Plan vs Reality
- **Task breakdown was effective**: 13 tasks, each independently testable. Dependencies were accurate.
- **Tasks were appropriately sized**: No task required splitting during implementation.
- **One unplanned change**: The `ComputeNavHistory` signature evolved from `(snapshots, breakpoints, firstDepositIdx)` to `(equityCurve, breakpoints, inceptionDate)`. This was a valid improvement (cleaner API, enables pre-inception points) but should have been reflected in the plan.
- **Validation task missing**: The plan didn't include a final "Validation / Hardening" task (per the skill template). Adding one would have caught the broken tests and typo earlier.

## Learnings
- **Always update tests with signature changes**: Never leave the codebase in a broken state between sessions. If a signature changes, update all callers and tests in the same edit batch.
- **Run `go build` after structural changes**: Compilation errors from typos or missing imports should be caught immediately, not deferred to the next agent.
- **Add a Validation task**: Every feature plan should end with a "Validation / Hardening" task that includes running the full test suite and checking for compilation across all packages.
- **Cross-reference test names with spec behavior**: When renaming or refactoring tests, verify the assertion matches the spec, not just that it compiles.

## Action Items
- [ ] Add a "Validation / Hardening" task template to future PLAN.md drafts
- [ ] Include "run `go build ./...`" in the per-task self-check checklist
- [ ] When changing function signatures, update all callers + tests in the same session
