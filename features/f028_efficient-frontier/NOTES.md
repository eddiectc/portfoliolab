# Notes: Efficient Frontier

## Decisions
- 2026-06-04: `portfolioEval` struct placed in `types.go` at package level so it can be referenced by both `frontier.go` computation and `frontier_test.go` tests.
- 2026-06-04: Daily returns use simple returns `(close[t]/close[t-1]) - 1` rather than log returns, matching the existing `analysis` package convention (`correlation.go`).
- 2026-06-04: Annualized return uses CAGR formula (compound) rather than arithmetic mean × 252, for mathematical correctness.
- 2026-06-04: Annualized volatility uses sample standard deviation (n-1 denominator) × sqrt(252/n), standard financial convention.
- 2026-06-04: Random seed fixed at 42 for deterministic frontier computation (reproducible results).
- 2026-06-04: `testhelpers.go` is a non-`_test.go` file because `makeTestPrices` returns `map[string][]market.HistoricalPrice` which is needed by multiple test files. The `testDec` helper is defined there to avoid the decimal import being unused in non-test builds.

## Deviations from Plan
- None so far.

## Future Improvements
- Consider adding a `ComputeSharpeRatio` helper function instead of inline computation.
- The `testhelpers.go` pattern (non-test file with test-only functions) could be refactored to a `_test.go` file if Go's test package scoping allows it.
- `testDec` in `testhelpers.go` duplicates `dec` in `returns_test.go`. This is unavoidable: `makeTestPrices` (in non-test `testhelpers.go`) needs a `float64→decimal.Decimal` helper, but `dec` lives in a `_test.go` file that is not compiled during regular builds. Both functions are identical; see `testhelpers.go` comment.

## Known Issues
- None.
