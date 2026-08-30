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

## 2026-08-30 — Task 2 done (client API methods + wtClassID)

- **`client.go`**: added `FundHoldings(ctx, wtClassID)` and `FundHistory(ctx, wtClassID)` (default view) on the
  existing client. Shared unexported `fetchURL` (the CycleTLS `fetchFunc` + 1 s rate limit now apply to pages
  and APIs through one code path); typed unexported record structs (`holdingRecord`, `fundHistoryRecord`) with
  `float64` JSON fields — `wgt`/`pctWeight` are **fractions** (0.0367 = 3.67%), `aum` is in **millions**.
- **`wtclassid.go`**: `ExtractWtClassID(pageBody)` — regex `\\?"wtClassID\\?":(\d{6,10})` (opening quote
  anchored, see deviation below), first match. Verified against all 3 captured pages (QGRW=49567173,
  WMGT=46987205, EZM=1000518; every occurrence per page identical, first-match-safe). Exported + unexported
  field both match, so a single pattern covers both sites.
- **Fixtures**: 6 full API JSONs in `testdata/` (short names: `holdings_{qgrw,wmgt,ezm}.json`,
  `fund_history_{qgrw,wmgt,ezm}.json`) + 2 trimmed page snippets for extraction tests. Full captures stay in
  `features/.../samples/`.
- **Tests**: unmarshal all 6 fixtures with per-fund record counts and spot-checks; URL construction for both
  endpoints (with/without `www`); wtClassID extraction (valid page, embedded-quote form, absent, first-match
  wins); not-found + non-200 + invalid-JSON error paths; shared rate limit (two sequential API calls ≥1 s apart).
  All pass; full suite green except the pre-existing `TestBenchmarkChartAllPeriods/1M`.
- **Deviations from plan wording** (minor): fixture names shortened vs the plan's `*_full.json` suffix; the
  wtClassID test is named `TestExtractWtClassID` (plan's verification command updated accordingly); the
  wtClassID regex is stricter than the plan's `wtClassID\\?":(\d{6,10})` — it anchors the field's opening
  quote (`\\?"wtClassID\\?":…`). Go's RE2 has no lookbehind, so a quote anchor is the available guard
  against substring matches in longer field names (e.g. `parentwtClassID`); all occurrences in the 3
  captured pages are quote-prefixed (verified: 324/324, 2027/2027, 112/112). The plan's regex is
  preserved in this note for traceability.
- **RESEARCH.md §5.1 example values are stale vs the actual captures** (fixture is authoritative — affects Task 4):
  - `assetGroup` is a code, not a label: `EQ`/`BD`/`CF` (not "Equity").
  - QGRW NVDA row: `wgt 0.0367`, `pctWeight 0.0347`, `sectorName "Semiconductors"` (not "Technology"),
    `figi "BBG00LLQ3ZK"` (not "BBG000XN470").
  - `sectorName` granularity varies by fund: QGRW/WMGT use GICS sectors ("Semiconductors", "Software"),
    EZM uses coarse buckets ("Information Technology"). Task 4 must not assume one granularity.

## 2026-08-30 — Task 3 done (React Flight payload decoder)

- **`flight.go`**: `DecodeFlight(pageBody) *FlightPayload` — one-pass assembly of all
  `self.__next_f.push([1,"..."])` chunks (regex over the raw body), JS-unescape, then per assembled row:
  split on the first `:` into row-ID + data; ignore rows without a prefix (continuation fragments of
  string rows — the payload splits long strings mid-content, e.g. `59:"<!DOCTYPE html>…` / `459:…</html>"`);
  decode the data as JSON and walk the tree for embedded tables and section objects. Unknown shapes
  (module preload rows `xx:I[…]`, plain strings, malformed chunks) are skipped silently.
- **Tables** (`Flight.Table(names…)`): a table is any object with `columns` + `rows` arrays; lookup
  matches `ariaLabel` **or** first-column name, first match in document order (QGRW has two tables
  sharing a first-column name — verified on the fixture); `Value(label)` returns the first matching
  label→value pair. `AsOf` is parsed from the second column header (UCITS `28/08/2026`, US
  `8/27/2026`; non-dates → no AsOf, no error).
- **Sections** (`Flight.SectionByClassification(c)`): each object with a `ranking` array becomes a
  `Section`; the sector fragment alone nests five ranking arrays (`Sector`, `Aggregate Country`,
  `Incorporated Country`, `Index Constituent`, `Fund` — all verified in the captures), so lookup goes
  by the entries' `rankClassification` (constants `RankClassificationSector` / `…Theme` / `…Fund`).
  Entries expose `name`, `rank`, `weight` (fraction), `date` (`dt`). `sectionId` is stored for
  reference ("sector-breakdown" / "theme-breakdown-chart") but is not the lookup axis.
- **Unescape strategy**: chunks are JS strings, not JSON — but the real captures only ever contain the
  JSON-safe escapes (`\"`, `\\`, `\uXXXX`, `\/`, `\b\f\n\r\t`), so unescaping is: replace `\uXXXX` with
  the codepoint (two-byte for surrogate pairs), then `json.Unmarshal` of the whole quoted string. A
  chunk containing any other escape (e.g. `\x`) fails to unmarshal and is dropped — pinned by
  `TestDecodeFlight_Tolerant` together with orphan continuation, module-preload, and plain-string rows.
- **Fixtures** (trimmed from the full `samples/` captures, real chunk bytes verbatim):
  `flight_qgrw_tables.html` (all QGRW tables incl. country + structure), `flight_ezm_tables.html` (all
  EZM tables), `flight_qgrw_sector.html`, `flight_wmgt_sector.html`, `flight_wmgt_theme.html`.
- **Tests** (19 subtests, all pass): both regions by ariaLabel and by first column, shared-first-column
  disambiguation, both as-of date formats + ambiguous `05/04` case + non-date header, NAV/fees value
  spot-checks, US-specific tables present / UCITS-specific absent on the US page, embedded-holdings
  `pctWeight` fractions, sector (both regions) + theme sections, missing table/section → nil, garbage-chunk
  tolerance, empty input.
- **Deviations**: none of substance. `DecodeFlight` returns a struct (not separate functions) so one
  decode serves every accessor; `AsOf` is a `*time.Time` field rather than a second return value, so
  "missing section" stays `nil`-clean as the plan requires.
