# Retrospective: Allocation

## What Went Well

- **Clean domain separation** — The new `internal/domain/allocation/` package is self-contained with clear interfaces (`PositionSource`, `AccountLister`, `TargetRepository`). No circular dependencies; dependency inversion at the handler layer mirrors existing patterns.
- **Task dependency ordering was accurate** — Tasks executed in plan order (1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10) with no rework or backtracking. The dependency graph in the plan correctly identified that domain logic (tasks 1-6) could proceed before presentation layer (tasks 7-10).
- **Comprehensive test coverage** — 125 total tests across 4 files (94 domain, 14 handler/web, 17 integration) covering all 17 spec scenarios plus edge cases. Table-driven tests for tolerance boundaries and validation are well-structured.
- **Cash handling worked out cleanly** — Aggregating `$CASH-*` positions into a single "Cash" row, with multi-currency FX conversion, required no special cases beyond the plan. The `buildCashRow` function handles the aggregation naturally.
- **Error handling was thorough** — The post-review fix adding `DriftWarning`, `RebalanceWarning`, `TargetWarning` fields to the web page data struct was a good catch. It ensures the page gracefully degrades when individual computations fail (e.g., drift fails but allocation still renders).
- **Spec was implementation-ready** — The spec's constraints (5% tolerance, exact 100% sum, 1dp display, 2dp shares) translated directly into code without ambiguity.

## What Could Be Improved

- **Spec scenario for "mixed currencies" was missing** — The spec assumed all portfolios share a base currency. During implementation (Task 3 review), we discovered that selecting portfolios with different base currencies would produce meaningless allocation percentages. This was caught by the review process and added as `ErrMixedCurrencies` with tests, but it should have been in the spec's edge cases. **Lesson:** When a feature involves multi-portfolio aggregation, explicitly consider cross-portfolio constraints.
- **`ErrDuplicateSymbol` was added beyond the plan** — The spec says "enter a target percentage for each symbol" but doesn't explicitly forbid duplicate symbols in the save request. The implementation adds duplicate detection, which is correct, but the spec should have mentioned this edge case.
- **Web template is large** — The `allocation/list.html` template is ~250 lines handling 4 distinct sections (allocation table, drift table, target form, rebalancing suggestions). This is manageable but could benefit from template partials if the feature grows (e.g., adding allocation history in a future feature).
- **`GetMarketPrice` added to `position.Service`** — The plan didn't anticipate needing a `GetMarketPrice` method on the position service. This was needed for rebalancing suggestions on symbols not yet held (price lookup). It was a reasonable addition but should have been foreseen during planning.

## Spec vs Reality

| Area | Assessment |
|---|---|
| **Scenario coverage** | All 17 spec scenarios are implemented and tested. No scenario was dropped or significantly altered. |
| **Edge cases** | All spec edge cases handled. Two additional edge cases discovered during implementation: `ErrMixedCurrencies` (multi-portfolio filter with conflicting base currencies) and `ErrDuplicateSymbol` (duplicate symbols in target save). Both added with tests. |
| **Constraints** | All constraints met: 1dp display rounding, exact 100% sum validation, 5% drift tolerance, 2dp share rounding, cash as regular target entry, error response format `{"error": "...", "code": "..."}`. |
| **Non-Goals respected** | No automatic rebalancing, no tax awareness, no allocation history, no multiple profiles, no configurable thresholds. |
| **Testing requirements** | All met: web rendering regression tests (11 template tests), drift tolerance boundary tests (table-driven), percentage validation tests (table-driven), multi-currency cash aggregation tests, rebalancing calculation tests with known-correct values. |

## Plan vs Reality

| Area | Assessment |
|---|---|
| **Task breakdown** | 10 tasks as planned, all completed. No tasks were split or merged. |
| **Task sizing** | Tasks were appropriately sized. The largest was Task 3 (allocation computation) at ~250 lines of service code plus tests, which was manageable in a single session. |
| **Dependencies** | Dependency graph was accurate. No unexpected cross-task dependencies emerged. The `GetMarketPrice` addition to `PositionSource` (Task 6) required a corresponding change in `position.Service` but this was a small, contained change. |
| **Technical decisions** | All 10 technical decisions in the plan were implemented as described. No decision was reversed or significantly altered. |
| **Risks** | The identified risks (market data availability, FX gaps, performance, decimal precision) were all mitigated as planned. No new risks emerged. |

## Learnings

- **Cross-portfolio constraints need spec attention** — When a feature aggregates across multiple portfolios, the spec should explicitly address what happens when portfolios have incompatible configurations (e.g., different base currencies). Add this to the spec quality checklist.
- **Rebalancing needs price resolution for unheld symbols** — The plan assumed held symbols would have prices from market data enrichment. It didn't consider that rebalancing suggestions might reference symbols not yet in the portfolio. This required adding `GetMarketPrice` to the `PositionSource` interface. **For future features:** When suggesting trades, always consider the case where the target symbol isn't yet held.
- **Error isolation in web pages is important** — The post-review fix of adding warning fields (instead of failing the entire page when one computation fails) improved UX significantly. **For future features:** Plan for partial failure modes in multi-section pages from the start.
- **The 10-task plan was well-sized** — Breaking a feature with API + domain + web + DB + tests into 10 sequential tasks worked well. Each task was independently testable and verifiable. This granularity is a good template for similarly scoped features.

## Action Items

- [x] Spec quality checklist items (cross-entity constraints, error isolation) — captured in this retro; not added to skill file (project-agnostic)
- [x] Template partials — extracted `allocation/list.html` into 3 partials (`partials/allocation/table.html`, `partials/allocation/drift-target.html`, `partials/allocation/rebalance.html`). Main template reduced from ~250 to ~70 lines.
- [x] Price resolution note — added to `docs/CONVENTIONS.md` under Domain Logic
