# Notes: Efficient Frontier

## Decisions
- 2026-06-04: Handler `efficientFrontierService` interface defined locally in the handlers package, following the `modelPortfolioService` pattern (not imported from efficientfrontier domain package).
- 2026-06-04: Data adapters (`SymbolListerImpl`, `PortfolioSymbolSourceImpl`, `ModelPortfolioSourceImpl`, `FxRateSourceImpl`) placed in `internal/data/efficient_frontier_adapters.go` to bridge existing services to the efficient frontier service interfaces. Uses concrete types (`*account.Service`, `*position.Service`, `*modelportfolio.Service`, `*marketservice.Service`) rather than additional abstraction layers.
- 2026-06-04: `handleComputeError` uses a `switch` on sentinel error values (`ErrInsufficientSymbols`, etc.) rather than `errors.As`, since the service returns the exact same pointer values.
- 2026-06-04: Period validation restricted to `1Y`, `3Y`, `5Y` (the spec's predefined periods) rather than the full set used by analysis (`3M`, `6M`, `10Y`).
- 2026-06-04:
 `portfolioEval` struct placed in `types.go` at package level so it can be referenced by both `frontier.go` computation and `frontier_test.go` tests.
- 2026-06-04: Daily returns use simple returns `(close[t]/close[t-1]) - 1` rather than log returns, matching the existing `analysis` package convention (`correlation.go`).
- 2026-06-04: Annualized return uses CAGR formula (compound) rather than arithmetic mean × 252, for mathematical correctness.
- 2026-06-04: Annualized volatility uses sample standard deviation (n-1 denominator) × sqrt(252/n), standard financial convention.
- 2026-06-04: Random seed fixed at 42 for deterministic frontier computation (reproducible results).
- 2026-06-04: `testhelpers.go` is a non-`_test.go` file because `makeTestPrices` returns `map[string][]market.HistoricalPrice` which is needed by multiple test files. The `testDec` helper is defined there to avoid the decimal import being unused in non-test builds.
- 2026-06-04: Service interfaces defined locally in `efficientfrontier` package (not imported from `marketservice`), following the `analysis.Service` pattern. The `marketservice.Service` is a concrete struct, not an interface.
- 2026-06-04: `FxRateSource.GetCurrentFxRate` returns `(*FxRate, error)` where nil means "no rate available" (not an error), allowing the service to warn but continue with prices in original currency.
- 2026-06-04: `GetCandidateSymbols` and `GetSymbolsFromPortfolio` return non-nil empty slices (`[]string{}`) when the source is nil or empty, matching the convention used in `analysis.Service`.

## Deviations from Plan
- **Service interfaces defined locally**: The plan said "reuse from `marketservice`" for `MarketDataHistorySource`, but the existing `marketservice.Service` is a concrete struct, not an interface. The `analysis.Service` pattern defines its own interfaces locally, so I followed that same approach. This keeps the efficientfrontier package self-contained and testable.
- **`periodCutoff` duplicated from analysis**: The `periodCutoff` function in `analysis/correlation.go` is unexported (lowercase), so it cannot be reused. The implementation is identical. If this becomes a maintenance concern, consider exporting it or placing it in a shared utility package.

## Future Improvements
- Consider adding a `ComputeSharpeRatio` helper function instead of inline computation.
- The `testhelpers.go` pattern (non-test file with test-only functions) could be refactored to a `_test.go` file if Go's test package scoping allows it.
- `testDec` in `testhelpers.go` duplicates `dec` in `returns_test.go`. This is unavoidable: `makeTestPrices` (in non-test `testhelpers.go`) needs a `float64→decimal.Decimal` helper, but `dec` lives in a `_test.go` file that is not compiled during regular builds. Both functions are identical; see `testhelpers.go` comment.
- **FX conversion precision**: `convertToBaseCurrency` uses `decimal.NewFromFloat64` which goes through a float64 intermediate. For production use with large amounts, consider a string-based conversion path to avoid floating-point rounding.

## Known Issues
- None.
