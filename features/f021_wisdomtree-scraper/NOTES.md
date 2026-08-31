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
  `weight` fraction × 100 (`round10`), entry `date`; absent section → `nil, nil` (QGRW: sectors only;
  EZM: sectors only — its page has no theme section; WMGT: both).
- **Fixtures**: `flight_ezm_sector.html` added (EZM sector section only — the EZM page has no theme
  section; the e2e test asserts `Themes == nil` for EZM).
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

## 2026-08-31 — Task 4+5 implementation review (fixes applied)

Review of parsers + orchestrator against SPEC/PLAN/NOTES/DoD. Findings and dispositions:

- **Bug — characteristics silently dropped (fixed).** `ParseFundCharacteristicsFromFlight` never set
  `FieldsPresent`, but `symbols/service.go` gates `EquityValuation` persistence on
  `HasCharacteristic(CharacteristicPriceToEarnings)` (gate introduced by f025). Net effect: every WisdomTree
  equity valuation (P/E, P/B, P/S, P/CF, dividend yield) was silently discarded. The parser now sets a mask
  bit per field that is present and parseable (absent label or parse error → no bit; a parseable `0.00`
  **does** set the bit — the field exists, zero is a legitimate value). Regression assertions added to the
  package unit tests and both integration tests (PE 31.43 / DY 0.35 asserted through repo and API).
- **Build — `go test ./...` broken (fixed; Task 6 pulled forward).** The v1 integration test was rewritten
  in full. QGRW fixtures copied into `tests/integration/testdata/` (`page_qgrw_wtclassid.txt`,
  `flight_qgrw_tables.html`, `flight_qgrw_sector.html`, `holdings_qgrw.json`, `fund_history_qgrw.json`); v1
  `wmgt_*` fixtures deleted from `tests/integration/testdata/` and the dead package copies
  (`internal/domain/extractor/wisdomtree/testdata/wmgt_{page_cycletls.html,modal_all_holdings.html}`). The
  tests now drive the real service + API path with `Extractor.SetClient` (seam re-added — every other
  provider has one) plus a `fakeYahooFetcher` implementing `market.SymbolDetailsFetcher` with canned
  LSE/USD values — **no network calls** (the Yahoo fetcher only supplies Exchange/Currency in the extractor
  path). Assertions: 9 sectors (top IT 58.4027), 100 holdings (top NVDA), 4 countries (top US 99.54),
  nil themes, equity_valuation present, fund_profile (AUM 47442965, TER 0.0033, inception 2024-04-16),
  601 NAV rows in `market_data` (latest 2026-08-28). Remaining Task 6 scope: `goimports` + live portal
  verification (full suite already run — green except the pre-existing date-dependent failure below).
  PLAN.md's "re-point `real_test.go` embeds" is stale wording — no such file exists after the Task 5 rewrite.
- **Undocumented data loss — fund family (documented).** The new site's Product Overview table has no
  "Fund Umbrella" row (verified by dumping both region fixtures), so `FundProfile.Family` is no longer
  populated. This contradicts the 2026-08-30 entry's "all previously extracted data is obtainable" —
  corrected: everything except fund family. The field is intentionally left empty; the source no longer
  exists on the new site.
- **Spec drift — fund-history API failure is fatal (documented).** SPEC lists "NAV history" as an optional
  section, but `Extract()` treats a `fund-history` API failure as fatal. Justification: the required
  inception date (first record) and AUM (last record — spec overview item) both derive from the same API,
  so a failure would reject a required item; only the NAV row list itself is the genuinely optional part.
- **Spec item — annual holdings turnover (documented).** Not available anywhere on the new site (nor the
  old — v1 left it at zero). Intentionally omitted; field stays at zero.
- **Lint (golangci-lint installed by user during review).** One new issue in this diff: ST1005 (capitalized
  error string in the NAV as-of failure) — fixed to lowercase. The mask rework also left `tableValueAny`
  unused — deleted (`tableValueAnyRaw` is still used by market-cap/other parsers). `golangci-lint run` on
  the wisdomtree package: **0 issues**. `tests/integration` reports findings only in pre-existing files
  this feature never touched (account/blackrock/ibkr/migration_smoke/portfolio/vanguard/comparison/
  efficient_frontier tests).
- **Pre-existing failure (not this feature):** `TestBenchmarkChartAllPeriods/1M` is date-dependent — on
  2026-08-31 the 1M window (2026-08-01→31) contains no monthly benchmark points (latest is 2026-07-15),
  so the chart page has no data. Fails on the parent commit as well; left as-is. Since fixed in
  `8e59226` — the test now seeds benchmark prices for every calendar day from 2000 through today,
  so all period windows contain data regardless of run date.

## 2026-08-31 — Task 6 (Cleanup + full verification) complete

Remaining Task 6 tail executed (Tasks 1–5 and most of Task 6 were done by the 2026-08-31 review):

- **Stale references — none.** Repo-wide grep for v1 parser/function names
  (`ParseFundInfo`, `ParseHoldingsFromModal`, `ParseNavHistoryFromModal`, `ExtractModalURL`,
  `ParseAsOfDate`, `isCashPosition`, `tableValueAny`, `real_test`) finds hits only in other providers'
  own packages (expected). One stale comment in `flight_test.go:338` ("removed in Task 6" for
  `ParseAsOfDate`, actually removed in Task 4) — corrected.
- **`goimports -w .` — repo-wide sweep reverted (pre-existing drift, not f021).** Running it modified 114
  files (~1,144 lines, whitespace/alignment only). Verification: `gofmt -l` on HEAD blobs flags the same
  unrelated files (e.g. `ibkrimport/parser.go`, `analysis/analysis_types.go`, `position/position.go`), so
  the repo has pre-existing gofmt drift from earlier features. The wisdomtree package itself was already
  goimports-clean at HEAD; the only f021 change from the run was the comment fix above. The sweep was
  reverted to keep Task 6's commit focused (one concern per commit). **Open question for the user:**
  commit a separate format-only repo-wide `goimports` pass, or leave the drift for a dedicated task.
  (Resolved: the user committed the separate format-only pass as `fda1c42`.)
- **Full suite re-run:** `go test ./...` green except the documented pre-existing
  `TestBenchmarkChartAllPeriods/1M` (same date-dependent failure as at review time).
- **PLAN.md:** Tasks 4–6 checked off (Task 6's user-verification box and Task 7 remain — user's scope).
- **Completed by the user:** live portal verification passed (WisdomTree symbol refresh works, details
  page renders) and all stored `data_source_url` values updated to the new region/asset-class/slug
  format before deploying. **All f021 tasks complete** — feature done, retrospective pending.

## 2026-08-31 — Full-feature implementation review (`/review-impl`)

- **Result: pass** — no code issues. `go test ./...` green, `golangci-lint` 0 issues on the
  package, all SPEC scenarios covered (Stories 1–5 + edge cases; the two unavailable items — fund
  family, annual holdings turnover — documented as no longer existing on the new site).
- **Doc fixes applied** (all minor, no code):
  - `features/README.md` — f021 status `in-progress` → `done`.
  - `v1/` archive — added `SUPERSEDED` headers to `NOTES.md`, `PLAN.md`, `RETRO.md`
    (`RESEARCH.md` already had one) per the feature-revision convention in `features/README.md`.
  - This file — resolved the goimports "open question" (separate format-only pass committed as
    `fda1c42`) and the "pre-existing failure" note (`8e59226` made `TestBenchmarkChartAllPeriods`
    date-independent; full suite now green); corrected the Task 4 entry's EZM theme wording —
    the EZM page has **no** theme section (sectors only), matching the e2e's `Themes == nil`
    assertion.
  - `PLAN.md` — stale wording corrected: status → complete; AUM unit label "millions" →
    thousands of USD (two places); e2e holdings count 101/493 → 100/506 tradeable.
- **Remaining process step:** retrospective (`/retro f021`).
