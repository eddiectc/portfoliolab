# Retrospective: Model Portfolio

## What Went Well

- **Clean domain isolation** — The `modelportfolio` domain is fully self-contained (models, validator, service, tests) with no circular dependencies on allocation or other domains. The only cross-domain dependency is the `SymbolChecker`/`SymbolCreator` interfaces for inline symbol creation, which follow the existing pattern from broker imports.

- **Plan fidelity was excellent** — All 8 tasks completed exactly as planned. The dependency chain (DB → domain → repo/service → API → web → inline symbols → allocation integration → nav/tests) was accurate and no reordering was needed.

- **Task sizing was appropriate** — Each task was completable in a single focused session. No task needed to be split further or caused scope overflow.

- **Single-table JSON design was the right call** — The `model_portfolios` table with `entries TEXT (JSON)` avoided a second table and JOINs. Entries are always CRUD'd as a complete set, so the relational pattern (`model_portfolio_entries` table) would have added complexity without benefit. The JSON marshal/unmarshal boundary at the repository layer keeps the domain types clean.

- **Inline symbol creation reuses existing infrastructure** — Leveraging `symbolmapping.Service.Create()` (which already triggers background market data fetch) meant Task 6 was lightweight — just interface wiring, no new fetch logic.

- **Allocation integration avoided service-layer changes** — The "load model portfolio" feature on the Allocation page uses only the existing GET `/api/model-portfolios/{id}` endpoint + client-side JS to pre-fill the form. No new backend endpoints or service methods were needed.

- **Test coverage is thorough** — 43 unit tests across validator and service, 16 API handler tests, 13 web handler tests, and 12 integration tests (covering full CRUD, inline symbol creation, apply-as-target, and web page rendering). All pass cleanly.

## What Could Be Improved

- **`GetAllForSelector` uses `List(1000, 0)` instead of `ListAll`** — The service's `GetAllForSelector` fetches portfolios via `List(ctx, 1000, 0)` rather than the `ListAll` method that was also implemented. Since this is only for a dropdown selector, 1000 is functionally sufficient, but using `ListAll` would be more semantically correct and avoid the arbitrary cap.

- **Weight sum error message not exposed via `errors.Is` for the dynamic case** — When weights don't sum to 100%, the validator returns a new `*ModelPortfolioError` with a dynamic message (including current total and delta) rather than the sentinel `ErrWeightSumNot100`. This means `errors.Is(err, ErrWeightSumNot100)` won't match the dynamic case. The handler's `handleServiceError` uses `errors.As` as a fallback, so it works in practice, but the sentinel pattern is inconsistent.

- **No server-side XSS sanitization on model portfolio names** — Names are rendered directly in templates (`{{.Name}}`) which Go templates auto-escape, so this is safe. However, names are also used in flash messages (`"Model portfolio \""+mp.Name+"\" created"`) using string concatenation — the flash is then rendered via template escaping, so it's safe, but the concatenation pattern is worth noting for future audits.

- **Form re-render on validation error doesn't preserve entry order deterministically** — When the form is re-rendered after a validation error, `parseEntriesForm` iterates `r.Form["symbol"]` and `r.Form["weight"]` in order. This works for the HTML form submission order, but if a future change affects form field ordering, entries could appear shuffled.

## Spec vs Reality

| Spec Item | Status | Notes |
|---|---|---|
| Create model portfolio (happy path) | ✅ Implemented | API + web form both work |
| Inline symbol creation | ✅ Implemented | Reuses `symbolmapping.Service.Create()` |
| Reject invalid weights (sum ≠ 100%) | ✅ Implemented | 0.01% tolerance as specified |
| Reject negative/zero weights | ✅ Implemented | `IsPos()` check |
| Reject duplicate name | ✅ Implemented | `GetByName` check in service |
| Reject rename to existing name | ✅ Implemented | Same-name self-exclusion works |
| Edit (change weights, add/remove symbols) | ✅ Implemented | Full entries replacement model |
| Delete model portfolio | ✅ Implemented | API 204 + web POST redirect |
| List model portfolios | ✅ Implemented | Paginated API + full list for web |
| Empty model portfolio list | ✅ Implemented | Template shows empty state + CTA |
| Apply as target allocation (with existing) | ✅ Implemented | JS fetches model, pre-fills form |
| Apply as target allocation (no existing) | ✅ Implemented | Same flow works for both cases |
| Apply with symbols not in real portfolio | ✅ Implemented | Target accepts any symbols |
| Weight rounding (0.01% tolerance) | ✅ Implemented | Uses `decimal.Decimal` exact arithmetic |
| Single symbol (100%) valid | ✅ Implemented | Validator allows it |
| Name ≤ 100 chars | ✅ Implemented | `maxNameLength` constant |
| Delisted symbol warning | ⚠️ Not implemented | Spec mentions showing a warning when a symbol in the model is no longer tradeable. Not implemented — model portfolios are allocation blueprints and symbol status isn't tracked. Low impact since f020 (comparison) is where this would matter most. |

**Summary:** 15 of 16 spec items fully implemented. One edge case (delisted symbol warning) deferred — it's more relevant to f020 (portfolio comparison) than to the core model portfolio CRUD.

## Plan vs Reality

| Plan Aspect | Assessment |
|---|---|
| **Task breakdown** | ✅ Accurate — 8 tasks, all independently testable, clean dependency chain |
| **Task sizing** | ✅ Appropriate — no task was too large or too small |
| **Dependencies** | ✅ Accurate — linear chain was correct, no parallelization missed |
| **Technical decisions** | ✅ All held — single table with JSON, inline symbol creation via existing service, web-only allocation integration |
| **Risk assessment** | ✅ No materialized risks — the "none" assessment was correct; JSON corruption didn't occur, template complexity was manageable |
| **Deviations** | One minor deviation: NOTES.md documents that market data fetch was already handled by `symbolmapping.Service.Create()` (no extra wiring needed). This was a plan over-estimation, not a problem. |

## Learnings

- **JSON column for "entries that are always CRUD'd together" is a good pattern** — When child records are never queried or modified independently of the parent, a JSON column avoids an extra table, migration, and JOIN. This pattern (used here for `model_portfolios.entries` and previously for `symbol_details`) is worth applying to similar cases.

- **Dropdown selector optimization** — The `GetAllForSelector` method demonstrates that not every consumer needs the full domain model. Creating lightweight summary types (`ModelPortfolioSummary`) avoids transferring full entry payloads when only id/name/count are needed.

- **Allocation integration via client-side fetch is cleaner than a backend "apply" endpoint** — By fetching the model via the existing GET endpoint and pre-filling the form, the user can review/adjust before saving. A direct "apply" endpoint would bypass this review step and require additional service-layer logic.

- **Weight validation with `decimal.Decimal` avoids floating-point issues entirely** — The 0.01% tolerance is handled with exact integer arithmetic (scale=2). No rounding or epsilon comparison needed.

## Action Items

- [x] ~~Consider using `ListAll` in `GetAllForSelector`~~ — fixed: `GetAllForSelector` now uses `repo.ListAll()` instead of `List(1000, 0)`
- [x] ~~Make `ErrWeightSumNot100` consistent~~ — fixed: added `Is(target error) bool` to `ModelPortfolioError` matching by code. Dynamic errors (with total/delta message) now match `errors.Is(err, ErrWeightSumNot100)`. Added test in `validator_test.go`.
- [ ] **For f020 (portfolio comparison):** implement delisted symbol detection and warning when comparing/applying model portfolios containing symbols with no recent market data
- [ ] **Consider adding a `GetAllForSelector` SQL query** that returns only `id, name, json_length(entries)` instead of fetching full rows and computing entry count in Go — minor optimization for large portfolio collections
