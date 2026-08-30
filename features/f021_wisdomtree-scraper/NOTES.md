# Implementation Notes — f021 WisdomTree Scraper (Phase 2)

> Phase 1 notes are archived at `v1/NOTES.md`.

## 2026-08-30 — Site relaunch detected

- WisdomTree relaunched its website (Sitecore → React/Next.js on Vercel). Old URLs 404; new URL format is
  `{region}/products/{asset-class}/{ticker}/` (e.g. `gb/products/equities/qgrw`).
- Full research done in `RESEARCH.md`: all previously extracted data is obtainable. API-first where possible:
  holdings (`/api/fund-holdings/{wtClassID}`) and NAV history (`/api/fund-history/{wtClassID}`) are JSON APIs;
  section tables (overview, fees, country, market cap, characteristics, sectors, themes) come from the React
  Flight payload (no APIs exist for them).
- **Decisions** (user):
  - API-first: prefer even undocumented JSON APIs over HTML/payload parsing — applied; only 3 endpoints exist and
    the section tables have no API.
  - URL matching: new format only, no backward compatibility — user updates stored URLs from the portal
    **before** deploying the new matcher (ordering matters: update URLs first, ship second).
  - `product-charts` data (growth-of-$10k, premium/discount — US funds only): out of scope, ignored.
- Old docs (v1 `RESEARCH.md`, `PLAN.md`, `NOTES.md`, `RETRO.md`, `samples/`) moved to `v1/` archive;
  `RESEARCH-NEWSITE.md` renamed to `RESEARCH.md` (current).

## 2026-08-30 — Rebuild scoped

- **Spec unchanged** (user decision): `SPEC.md` is implementation-agnostic and all sections it requires still
  exist on the new site — the rebuild is a **plan-level** change, not a spec change.
- Rebuild plan written in `PLAN.md` (7 tasks: matcher, client+wtClassID, flight decoder, parsers, orchestrator,
  cleanup/verify, + user task: portal URL migration).
- Phase-2 defaults flagged for review: as-of = page header (holdings keep own `dt`), no flight-holdings fallback
  (future work), `product-charts` deferred.

## 2026-08-30 — Phase 2 approved, Task 1 done

- **Decision** (user): Phase 2 plan approved as written (API-first sources, no flight-holdings fallback,
  `product-charts` deferred). Implementation started.
- **Task 1 (URL matcher) complete**: `matcher.go` rewritten to the new-format-only regex
  (`wisdomtree.com/{region}/products/{asset-class}/{slug}/`, optional `www` + trailing slash, any 2-letter
  region). Whole URL lowercased before matching, so scheme, host, and path are all case-insensitive
  (superset of the old matcher's host case behavior). `matcher_test.go` rewritten as `TestMatch` per the
  plan's verification command; all pass. No other code affected — the matcher contract (`Match(string) bool`)
  is unchanged.

## Task 1 Review (2026-08-30)

- Review found four stale v1 tests asserting the removed matcher behavior; all updated:
  - `TestExtractor_Match` (wisdomtree package) — `.eu` URL now expected to **not** match.
  - `TestVanguard_Dispatcher_UnregisteredProvider` (tests/integration) — the "registered provider still
    routes" check now uses a new-format URL instead of legacy `.com/uk/en/ics/etfs/WMGG/`.
  - `TestWisdomTree_ServiceLayerRoundTrip` + `TestWisdomTree_FullAPIRoundTrip` (v1 e2e) — `data_source_url`
    updated to new format (dispatch only; fetch stays mocked with the embedded v1 page). These tests are
    superseded wholesale in Tasks 5–6 with new-site samples.
- Case handling documented precisely (whole URL lowercased, path included); added an uppercase-path test
  case pinning that behavior (26 → 27 cases).
- `features/README.md` — f021 status set to `in-progress` for the Phase 2 rebuild.
- Pre-existing, unrelated failure observed: `TestBenchmarkChartAllPeriods/1M` (date-dependent; fails on the
  parent commit too) — left as-is.
