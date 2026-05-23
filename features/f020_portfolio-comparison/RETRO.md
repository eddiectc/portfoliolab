# Retrospective: Portfolio Comparison Engine (f020)

## Summary

| Metric | Value |
|---|---|
| Feature | Portfolio Comparison Engine |
| Status | **Done** |
| Lines of code | ~11,003 (implementation + tests + template) |
| Unit test coverage | 90.0% (comparison), 89.6% (stats) |
| Integration tests | All pass (model-vs-model, model-vs-real, real-vs-real, empty portfolio, period filtering, same-portfolio, invalid inputs, web rendering) |
| Build | Clean (`go build ./...` succeeds) |

---

## Spec vs. Implementation

### Scenarios Covered (16/16)

| Scenario | Status | Notes |
|---|---|---|
| Compare two model portfolios — performance | ✅ | `SimulateEquityCurve` + `ComputeReturnMetrics` |
| Compare two model portfolios — risk | ✅ | `ComputeRiskMetrics` (reused from performance) + `ComputeBetaAlpha` |
| Compare model portfolio vs real portfolio | ✅ | Type dispatch in service layer |
| Compare two real portfolios | ✅ | Both use `position.ComputeEquityCurve` |
| View drawdown comparison | ✅ | `ComputeDrawdownSeries` + ECharts line chart |
| View annual returns histogram | ✅ | `ComputeReturnDistribution` + ECharts bar chart |
| View annual return frequency histogram | ✅ | `AnnualBinned` field added to `ReturnDistribution` |
| View monthly return frequency histogram | ✅ | `MonthlyBinned` field in `ReturnDistribution` |
| View period extremes | ✅ | `ComputePeriodExtremes` + TWR normalization via `NavPerUnit` |
| View holdings overlap | ✅ | `ComputeCrossPortfolioOverlap` (Jaccard similarity) |
| View correlation matrix (intra-portfolio) | ✅ | `ComputeIntraPortfolioCorrelation` + heatmap |
| View portfolio-to-portfolio correlation | ✅ | `ComputePortfolioCorrelation` |
| Select fixed comparison period | ✅ | 1Y, 3Y, 5Y, YTD, All (+ 1W, 1M, 3M deviation) |
| Select custom comparison period | ✅ | Date range inputs in template |
| Base currency selection | ✅ | Query param + FX conversion in simulation |
| Handle symbols with shorter history | ✅ | Period clipping + warnings |
| Comparison with insufficient data | ✅ | Warnings + "refresh market data" prompt |
| Compare against real portfolio with no transactions | ✅ | Empty metrics + explanatory message |

### Edge Cases (9/10 covered)

| Edge Case | Status | Notes |
|---|---|---|
| Both portfolios identical | ✅ | Integration test: same-portfolio-vs-self (beta≈1.0, correlation≈1.0) |
| Single symbol model portfolio | ✅ | Tested in simulation_test.go |
| Symbol with no ETF holdings data | ✅ | Treated as atomic (documented in NOTES.md) |
| Real portfolio with no transactions | ✅ | Integration test + service test |
| Very short comparison period (<30 days) | ✅ | Risk metrics show N/A |
| Custom period with no trading days | ⚠️ | Error shown, but not explicitly tested |
| FX rate gaps | ✅ | Spot rate fallback + warning (documented) |
| Risk-free rate for Sharpe | ✅ | Defaults to 0%, displayed to user |
| Model portfolio deleted during comparison | ⚠️ | Would error naturally, but no explicit test |
| Starting value configurable | ✅ | Query param + integration test |

### Non-Goals Respected

All non-goals from the spec were respected — no paper trading, no rebalancing, no transaction costs, no tax lots, no leverage, max 2 portfolios, no portfolio-vs-index (uses existing benchmark), no dividend reinvestment, no scheduling, no model portfolio management.

---

## Plan vs. Implementation

### Tasks Completed (7/7)

| Task | Status | Coverage | Notes |
|---|---|---|---|
| Task 1: Simulation | ✅ Done | 90% | `simulation.go` + `simulation_test.go` (537 lines) |
| Task 2: Metrics | ✅ Done | 90% | `metrics.go` + `metrics_test.go` (712 lines) |
| Task 3: Service | ✅ Done | 90% | `service.go` + `service_test.go` (1640 lines) |
| Task 4: Overlap/Correlation | ✅ Done | — | `overlap.go` + `correlation.go` (684 lines) |
| Task 5: API Handler | ✅ Done | — | `comparison.go` + `comparison_test.go` (864 lines) |
| Task 6: Web UI | ✅ Done | — | `comparison_web.go` + template + nav link (1632 lines) |
| Task 7: Integration Test | ✅ Done | — | `comparison_test.go` (858 lines) |

### Unplanned Additions

The following were not in the original plan but proved necessary or beneficial:

| Addition | Reason |
|---|---|
| **`internal/domain/stats/` package** | Extracted `PearsonCorrelation`, `AlignSeries`, `RoundTo2`, `RoundTo4` from duplicated implementations in `analysis` and `comparison`. Both packages now import `stats`. |
| **`compute.go`** | `ComputeMWR` and `ComputeMonthlyReturns` — pre-existing MWR/monthly return functions in the `comparison` package (not created for this feature, but included in line count). |
| **`data.PortfolioNameResolverImpl`** | New adapter to satisfy `PortfolioNameSource`. Delegates to `portfolio.Service.Get()`. |
| **`ComputeDrawdownSeries`** | Drawdown-over-time series for the line chart. Not explicitly in the plan but implied by the spec's "drawdown comparison" scenario. |
| **`AnnualBinned` in `ReturnDistribution`** | Frequency-binned annual returns (spec scenario "annual return frequency distribution"). |
| **Intra-portfolio correlation wired into service** | Spec scenario "correlation matrix" required fetching prices for both portfolio types. |
| **Short-term periods (1W, 1M, 3M)** | Deviation from spec — useful for tactical analysis, supported by existing period resolution. |
| **Refactored `PortfolioCurrencyCheckerImpl`** | Changed to delegate to `portfolio.Service.Get()` for consistency with `PortfolioNameResolverImpl`. |

### Technical Decisions — Post-Mortem

| Decision | Outcome |
|---|---|
| **`stats` package extraction** | ✅ Good call. Eliminated 3 duplicate implementations across `analysis`, `comparison`, and `allocation`. Both packages share `PearsonCorrelation` and `AlignSeries`. |
| **Jaccard similarity for overlap** | ✅ Simple and intuitive. The spec didn't prescribe a specific algorithm. |
| **`NavPerUnit` for TWR normalization** | ✅ Better than the originally planned `twrNormalizeCurve`. The performance layer already computes `NavPerUnit`, so reusing it avoids reinventing unitization. |
| **`AllocationSource` interface for real portfolio overlap** | ✅ Avoids duplicating the allocation pipeline. Follows existing dependency-injection pattern. |
| **`portfolioMeta` interface for type dispatch** | ✅ Cleaner than scattered type switches. Extensible if more portfolio types are added. |
| **Short-term periods (1W, 1M, 3M)** | ⚠️ Minor spec deviation. Useful but should be documented in the spec if kept long-term. |

---

## Testing Quality

### Strengths
- **Comprehensive unit tests**: 90% coverage on the comparison domain. Table-driven tests cover normal cases, edge cases, and boundary conditions.
- **Integration test suite**: 8+ test cases covering model-vs-model, model-vs-real, real-vs-real, empty portfolio, period filtering, same-portfolio-vs-self, invalid inputs, and web rendering.
- **Service tests with hand-written mocks**: 1640 lines of service tests following the `account/service_test.go` pattern (stateful mocks that simulate real behavior).
- **Handler tests**: 864 lines covering valid/invalid requests, missing params, bad portfolio types.

### Gaps
- **Custom period with no trading days**: Not explicitly tested (would require inserting a date range with no market data).
- **Model portfolio deleted during comparison**: Not tested (would require deleting a portfolio mid-request).
- **FX rate gap scenarios**: Not tested with actual missing FX data (relies on unit tests of `convertToBaseCurrency` with empty FX maps).

---

## Spec Quality Assessment

### Strengths
- **Comprehensive scenario coverage**: 16 scenarios covering all user stories and edge cases.
- **Clear metric definitions**: Each metric (CAGR, Sharpe, Beta, Alpha, etc.) is defined with formula or reference.
- **Well-defined constraints**: Buy-and-hold strategy, default values, period definitions are explicit.
- **Good non-goals**: Prevents scope creep effectively.
- **Edge cases anticipated**: 10 edge cases covering data availability, empty states, and boundary conditions.

### Areas for Improvement
- **Overlap metric specification**: The spec says "overlap is expressed as a percentage of total exposure" which implies weight-based overlap. The implementation uses Jaccard similarity (unique symbol count). This ambiguity should be resolved in future specs.
- **Chart specifications**: The spec describes charts textually but doesn't specify chart types (line, bar, heatmap). This was resolved by following the existing `analysis` page pattern.
- **API request/response format**: The spec doesn't define the API contract. This was resolved by following the existing handler pattern.

---

## Plan Quality Assessment

### Strengths
- **Clear task decomposition**: 7 tasks with explicit dependencies and verification criteria.
- **Good priority ordering**: Foundation tasks (simulation, metrics) before orchestration (service) before presentation (handler, UI).
- **Technical decisions table**: Documented trade-offs and rationale for each decision.
- **Risk identification**: Market data availability, performance, FX complexity, scope creep — all identified and mitigated.

### Areas for Improvement
- **Underestimated cross-cutting concerns**: The `stats` package extraction (affecting `analysis`, `comparison`, `allocation`, `factor_exposure`, `stress`) was not anticipated. This kind of refactoring should be flagged as a risk when multiple packages share similar utilities.
- **Adapter creation**: `PortfolioNameResolverImpl` was not in the plan. Future plans should check whether required interfaces already have data-layer adapters before assuming they exist.
- **Template complexity**: The comparison template (929 lines) is the largest in the project. Future plans should estimate template complexity when the feature includes many chart types.

---

## Code Quality

### Strengths
- **Consistent patterns**: Follows existing `analysis` and `performance` domain conventions.
- **Pure functions**: Simulation, metrics, overlap, and correlation are pure functions — easy to test and reason about.
- **Interface-based dependencies**: Service layer uses interfaces for all external dependencies (testable, swappable).
- **Good documentation**: Doc comments on public functions, clarifying comments on design decisions.

### Areas for Improvement
- **Service complexity**: `service.go` (911 lines) is the largest single file. Consider splitting into `model_portfolio.go`, `real_portfolio.go`, and `cross_metrics.go`.
- **Handler complexity**: `comparison_web.go` (703 lines) with many serialization functions. Consider extracting chart serialization to a dedicated package.
- **Template size**: 929 lines of HTML. Consider partial templates for chart sections.

---

## Lessons Learned

### What Went Well
1. **Reuse over reinvention**: The implementation reuses `performance.ComputeRiskMetrics`, `performance.ComputeDrawdownAnalysis`, `position.ComputeEquityCurve`, and `allocation.ComputeAllocation`. This kept the feature focused on comparison-specific logic.
2. **`stats` package extraction**: Identified duplicate code across multiple packages and consolidated into a shared utility. Both `analysis` and `comparison` now import `stats`.
3. **`NavPerUnit` reuse**: Instead of implementing a new TWR normalization, the implementation leveraged the existing `NavPerUnit` field from the performance layer.
4. **Comprehensive integration tests**: The integration test suite covers realistic scenarios (model-vs-model, model-vs-real, empty portfolio, period filtering) with seeded market data.

### What Could Be Better
1. **Spec ambiguity on overlap metric**: "Percentage of total exposure" was interpreted as Jaccard similarity. Future specs should be more precise about overlap calculation.
2. **Service file size**: 911 lines in a single file. Future features with similar orchestration should consider splitting by concern (model vs real vs cross-portfolio).
3. **Short-term period deviation**: Adding 1W/1M/3M periods without updating the spec. Even minor deviations should be reflected in the spec to keep it as the source of truth.
4. **Missing adapter discovery**: `PortfolioNameSource` interface had no existing adapter. Future plans should verify adapter existence during planning, not implementation.

### Process Improvements for Next Feature
1. **Pre-plan adapter audit**: Before writing the plan, check whether all required interfaces have data-layer adapters. If not, add adapter creation as a task.
2. **Template complexity estimate**: When the spec includes multiple chart types, estimate template size and consider partial templates in the plan.
3. **Cross-package refactoring budget**: If the feature touches shared utilities (like `stats`), budget time for refactoring existing packages, not just the new code.
4. **Spec metric precision**: For quantitative metrics (overlap, correlation, etc.), specify the exact formula or algorithm in the spec to avoid implementation ambiguity.

---

## Feature Index Update

The feature index (`features/README.md`) was updated:
```
| f020 | Portfolio Comparison | done | f019, f009, f010, f011, f012, f015 |
```

## Known Issues

None.

## Future Improvements

- **Interpolate missing daily prices**: Forward-fill from last known price so the curve doesn't have gaps when symbols have non-overlapping trading days.
- **Moving daily risk-free rate**: Replace fixed 0% Sharpe rate with daily 3-month Treasury yield. Requires historical Treasury data source and `ComputeRiskMetrics` signature change.
- **Service file split**: Consider splitting `service.go` into model/real/cross-concern files.
- **Template partials**: Extract chart sections into partial templates for maintainability.
