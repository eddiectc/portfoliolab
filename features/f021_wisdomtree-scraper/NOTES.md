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

## 2026-08-31 — Task 4 done (parsers rewritten against new sources)

- **`parsers.go`** — v1 parsers (modal CSV/HTML parsing) deleted; all section parsers now read the
  React Flight tables / section objects or the JSON APIs, same `extractor.ExtractResult` contract.
- **Holdings** (`ParseHoldingsFromAPI`): `Symbol`/`Name`/`Sector`/`MarketValue`/`Shares` straight from the
  record; `Percent = wgt * 100` passed through `round10()` to strip float-multiplication artifacts
  (`0.1476209845990145 * 100 = 14.762098459901448` → `14.7620984599`, matching the old site's 10dp percent
  convention); `AsOfDate` = row `dt` truncated to `YYYY-MM-DD`; `AssetClass` from the `assetGroup` code
  (`EQ`/`BD`/`CF`/`DER` → labels, unknown codes passed through); identifier slot carries the FIGI — the API
  returns no per-holding ISINs (RESEARCH.md §6.2).
- **Cash-row filtering — deviation from plan wording**: the plan says "keep cash-row filtering by name
  keywords (existing convention)", but v2 filters on `securityTicker` nil/whitespace instead. The name-keyword
  approach was a v1 hack that mis-filtered real tickers (e.g. FirstCash Holdings — name contains "CASH");
  the new API provides the structural signal (verified on fixtures: non-ticker rows are exactly `CASH W-O`
  on QGRW (1/101 rows) and `DREYFUS TRSY OBLIG CASH MGMT CL INS` + `US DOLLAR` on EZM (2/508 rows)).
- **NAV history** (`ParseNavHistoryFromAPI`): one `NavPoint{Date, NAV, Currency}` per record, in the
  API's ascending order (since inception). **`FundInfoFromHistory`**: ticker + name from the first record,
  inception = first record's date. **`LatestAUM`**: last record's `aum`.
- **AUM unit — plan label wrong, plan example right**: the plan says "millions → USD, e.g. `47442.9648` →
  $47,442,965"; the example is ×1000, and the captures confirm thousands-of-USD: QGRW last `aum 47442.9648`
  → **$47.4M** and EZM `946609.717` → **$946.6M** are both plausible fund sizes, while ×10⁶ would give
  $47B/$946B (absurd). `LatestAUM` converts ×1000 with round-half-up. (Task 2's note "aum is in millions"
  was mislabelled — same value, correct unit is thousands.)
- **Overview** (`ParseFundProfileFromFlight`): Product Overview table (ISIN, asset class, base currency,
  use of income, inception) + Fees table TER on UCITS pages; US pages carry the expense ratio in the
  overview and have **no ISIN and no Fees table** (empty profile fields, not an error). The site displays
  TER as a percent; the contract wants a fraction (`/100`). Structure + Key Service Providers tables fill
  legal form/structure/methodology/domicile/issuer/custodian/manager. Missing both overview and fees →
  `nil, nil` (assembled as an optional-ish section by the orchestrator, which treats a *present but
  empty* overview as the "no overview" failure — see Task 5).
- **Country / Market cap / Characteristics** (flight tables): country rows → `CountryAllocation` (weight
  fraction × 100, `round10`); market-cap rows by label (`Large`/`Mid`/`Small`/`Micro`, total row separate);
  characteristics P/E, P/B, P/S, P/CF, dividend yield from the Fund characteristic table.
- **Sectors / Themes** (`ParseSectorsFromFlight` / `ParseThemesFromFlight`): section entries → name,
  `weight` fraction × 100 (`round10`), entry `date`; absent section → `nil, nil` (QGRW has sectors, no
  themes; EZM both; WMGT both).
- **Fixtures**: `flight_ezm_sector.html` added (EZM sector + theme sections).
- **Tests**: `parsers_test.go` rewritten for the v2 surface — every parser against real fixtures in both
  regions (QGRW/WMGT UCITS + EZM US), absent-table/section → `nil, nil` cases, float-artifact rounding,
  AUM ×1000 conversion, percent-fraction conventions, date parsing (both regions + ambiguous `05/04`).
  32 tests in the package, all pass.

## 2026-08-31 — Task 5 done (orchestrator `Extract()` + e2e tests)

- **`extractor.go` `Extract()`**: 1. fetch page → 2. `ExtractWtClassID` (explicit failure if absent) →
  3. `FundHoldings` + `FundHistory` (sequential; shared 1 s rate limit) → 4. `DecodeFlight` → 5. assemble.
  Context is checked before the fetches and after them.
- **As-of semantics** (RESEARCH.md §9.8): the extraction `AsOfDate` is the **Net Asset Value table's
  "As of" header** (second column: UCITS `28/08/2026`, US `8/28/2026`), not the page-level "As of" banner
  — the NAV table is the fund's reporting anchor and is present on both regions. Missing NAV table or
  non-date header fails the extraction (required). Holdings keep their own `dt` as `Holding.AsOfDate`.
- **Error semantics**: required (fund info, overview profile, NAV table/as-of, tradeable holdings) are
  atomic — any failure rejects the whole extraction. Optional sections (countries, market cap,
  characteristics, sectors, themes) are `nil` on absence and their parse errors are ignored. API
  transport failures are wrapped distinctly (`fetch page:`, `wtClassID:`, `fund-holdings API:`,
  `fund-history API:`) from parse failures (RESEARCH.md §9.1 undocumented-API risk).
- **`e2e_test.go`** (rewritten): injected fetch serves the concatenated page captures + both API bodies;
  full pipeline for **QGRW (UCITS)** and **EZM (US)** asserting: fetch order (page → fund-holdings →
  fund-history, URLs built from the *extracted* wtClassID, exactly 3 fetches, **no product-charts call**),
  source, as-of date (2026-08-28 both), inception (2024-04-16 / 2007-02-23), ISIN (BBG000BBJQV0, FIGI slot),
  NAV history length + last point, AUM, country top (United States 99.54 / 96.84), market cap,
  characteristics, sector top (Information Technology 58.4027 / Financials 19.322, with section dates),
  holdings (all Equity, per-row AsOfDate, first-row spot checks incl. Percent 14.7620984599).
- **Holdings counts — deviation from plan numbers**: the plan's e2e line says "holdings count (101 / 493)";
  the fixtures hold **100 tradeable / 101 rows** (QGRW, one cash row) and **506 / 508** (EZM, two cash
  rows) — the plan's numbers were pre-measurement guesses. Tests assert the actual fixture counts.
- **Tests**: `extractor_test.go` rewritten for the v2 orchestrator — error paths (page fetch failure,
  missing wtClassID, holdings API failure, no tradeable rows, missing NAV table, canceled context), each
  asserting the distinct wrap. Full package suite (32 tests) green.
- **Known follow-up (Task 6)**: `tests/integration/wisdomtree_extractor_e2e_test.go` is v1 (modal fixtures +
  a `SetClient` seam that no longer exists) and breaks `go test ./...` — it references exactly the v1
  fixtures the cleanup task deletes.
