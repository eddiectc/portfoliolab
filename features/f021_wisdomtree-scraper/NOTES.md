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
  region). Scheme/host matched case-insensitively (keeps the old matcher's host case behavior; paths per
  sitemaps are lowercase). `matcher_test.go` rewritten as `TestMatch` (26 cases) per the plan's verification
  command; all pass. No other code affected — the matcher contract (`Match(string) bool`) is unchanged.
