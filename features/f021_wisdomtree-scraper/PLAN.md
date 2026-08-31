# Implementation Plan: f021 WisdomTree Scraper — Phase 2 (New-Site Rebuild)

**Date**: 2026-08-30
**Status**: Approved 2026-08-30 (user confirmed); implementation in progress
**Spec**: `SPEC.md` is **unchanged** (user decision 2026-08-30). The spec is implementation-agnostic and every
data section it requires is still available on the new site. All rebuild decisions live here and in `RESEARCH.md`.
**Context**: The 2026-08-30 relaunch (Sitecore → Next.js) broke every v1 parser. v1 code is replaced, not patched.
**Phase 1 plan**: archived at `v1/PLAN.md`.

## Overview

Rebuild the WisdomTree extractor for the new site:

1. **URL matcher** — new format only (`wisdomtree.com/{region}/products/{asset-class}/{slug}`); old `.eu` and
   legacy `.com` paths no longer matched (user updates stored URLs from the portal first).
2. **Data sources** — API-first (user preference), applied to the maximum the site allows:
   - `GET /api/fund-holdings/{wtClassID}` — full holdings (UCITS + US)
   - `GET /api/fund-history/{wtClassID}` — NAV + AUM + shares history since inception (UCITS + US)
   - React Flight payload embedded in the page — everything else (section tables, sector/theme breakdowns),
     for which **no API exists** (RESEARCH.md §5.4).
   - `wtClassID` (numeric, per share class) is extracted from the page body before any API call.
3. **`product-charts` API** — explicitly out of scope (user decision: ignore; US funds only anyway).
4. **Contract preserved** — `extractor.ExtractResult` and the optional-section `nil, nil` semantics are
   unchanged, so the service layer (original Task 4) needs no changes.

Extraction flow per symbol: 1 page fetch → wtClassID → 2 API calls (rate-limited) → flight decode → assemble.

## Task Dependencies

```
Task 1 (matcher) ─────┐
Task 2 (client+ID) ───┼──→ Task 4 (parsers) ──→ Task 5 (orchestrator) ──→ Task 6 (cleanup + verify)
Task 3 (flight) ──────┘                                                    Task 7 (user: portal URLs) → before deploy
```

Tasks 1–3 are independent and can be done in any order.

## Tasks

### Task 1: New URL matcher [PRIORITY: HIGH]

**Corresponds to:** Story 1 (provider routing by URL), RESEARCH.md §1a
**Description:** Replace the domain-suffix matcher with the new-format-only matcher.

- [x] Rewrite `wisdomtree/matcher.go`: match `^https?://(?:www\.)?wisdomtree\.com/[a-z]{2}/products/[a-z-]+/[a-z0-9-]+/?$`
      (any 2-letter region; both slug types — US ticker / EU name-slug; `www` optional; trailing slash optional)
- [x] Drop all `.eu` / legacy-domain matching (no backward compatibility — user decision)
- [x] Rewrite `matcher_test.go`:
      - matches: `us/products/equity/ezm`, `gb/products/equities/wisdomtree-us-quality-growth-ucits-etf---usd-acc`,
        with/without `www`, with/without trailing slash, other regions (`de`, `fr`, …)
      - rejected: old `.eu` URLs, `.com/etfs/...` legacy paths, non-WisdomTree domains, malformed paths
- [x] Write tests

**Verification:** `go test ./internal/domain/extractor/wisdomtree/ -run TestMatch`

### Task 2: Client API methods + wtClassID extraction [PRIORITY: HIGH]

**Corresponds to:** Story 2 (extraction — data sources), RESEARCH.md §3, §5.1, §5.2
**Description:** Add the two JSON API endpoints to the existing CycleTLS client and extract `wtClassID` from the page.

- [x] `client.go`: add `FundHoldings(ctx, wtClassID)` and `FundHistory(ctx, wtClassID)` (default view) using the
      existing `fetchFunc` (same CycleTLS transport, same 1 s rate limit — the single `SetFetchFunc` injection
      point already covers API bodies for tests); typed response structs per RESEARCH.md §5.1/§5.2
- [x] `wtClassID` helper (`wtclassid.go`): regex `wtClassID\\?":(\d{6,10})` on the raw page body (first match)
- [x] Move full API response fixtures into `testdata/`: `holdings_{qgrw,wmgt,ezm}.json`,
      `fund_history_{qgrw,wmgt,ezm}.json` (copied from `features/.../samples/`); plus 2 trimmed page
      snippets for wtClassID extraction tests
- [x] Write tests: unmarshal all 6 fixtures (QGRW 101 / WMGT 920 / EZM 508 holdings; 601 / 691 / 4914 history
      records), wtClassID extraction from page snippets, shared rate limit preserved

**Verification:** `go test ./internal/domain/extractor/wisdomtree/ -run "TestClient|TestExtractWtClassID"`

### Task 3: React Flight payload decoder [PRIORITY: HIGH]

**Corresponds to:** Story 2 (sections without API), Edge case "different provider pages have different data
sections", RESEARCH.md §4
**Description:** Decode the embedded flight payload and provide name-based access to tables and breakdown sections.

- [x] New `flight.go`:
      - extract all `self.__next_f.push([1,"..."])` chunks; JS-unescape (`\uXXXX`, `\"`, `\\`, `\/`); decode
      - **tables**: locate by `ariaLabel` / first-column name (never by position — US pages carry a different
        superset of tables); return label→value map
        (Product Overview, Net Asset Value, Fees, Market Capitalisation, Fund characteristic, Country, Structure)
      - **sector/theme sections**: locate by `sectionId` / `rankClassification`
        (`"Sector"` / `"EU Thematic Bucket"`); return `pctWeight` rankings (fractions)
      - **date parsing**: both `28/08/2026` (UCITS) and `8/27/2026` (US) formats
      - tolerant: scan all chunks, ignore unknown shapes, missing section → `nil, nil`
- [x] Create trimmed flight fixtures in `testdata/` (relevant chunks only, UCITS + US variants) plus sector/theme
      section fixtures (trimmed from `samples/sector_section_*.json`, `theme_section_wmgt.json`)
- [x] Write tests: table lookup by name (both regions), both date formats, absent section → `nil, nil`,
      unknown chunks ignored

**Verification:** `go test ./internal/domain/extractor/wisdomtree/ -run "TestDecodeFlight|TestParseFlightAsOfDate"`

### Task 4: Rewrite parsers against new sources [PRIORITY: HIGH]

**Corresponds to:** Story 2 (all data sections), Edge cases (800+ holdings, optional sections), RESEARCH.md §6
**Description:** Replace the v1 parsers; same `extractor.ExtractResult` contract, new input sources.

- [x] Holdings ← API: ticker (`securityTicker`), name, weight (`wgt` fraction), sector (`sectorName`), FIGI;
      keep cash-row filtering by name keywords (existing convention)
- [x] NAV history ← `fund-history` default view (ascending, since inception): each record → NAV point;
      latest `aum` (millions → USD, e.g. `47442.9648` → $47,442,965) → AUM field
- [x] Overview ← Product Overview table (ISIN, inception date, base currency, asset class) + Fees table (TER)
- [x] Country allocation, Market Capitalisation, Fund characteristic (P/E, P/B, P/S, P/CF, dividend yield)
      ← flight tables
- [x] Sector breakdown ← Sector Breakdown section; Theme breakdown ← Theme section (absent on some funds →
      `nil, nil`, e.g. QGRW has no themes)
- [x] Remove all v1 parsers that have no new-site equivalent (old CSV/HTML parsing)
- [x] Write tests: every parser against fixtures (UCITS QGRW/WMGT + US EZM variants); weight fraction→percent
      convention consistent with the old site; AUM unit conversion asserted

**Verification:** `go test ./internal/domain/extractor/wisdomtree/ -run "TestParse"`

### Task 5: Rebuild orchestrator `Extract()` [PRIORITY: HIGH]

**Corresponds to:** Story 2 (extraction run), Story 3 ("as of" dates), Edge cases (atomic required sections,
missing as-of, rate limiting), RESEARCH.md §10
**Description:** New flow: page → wtClassID → API calls → flight decode → assemble, with spec error semantics.

- [x] `extractor.go` `Extract()`:
      1. fetch page (CycleTLS) → 2. extract `wtClassID` (fail explicitly if absent) → 3. `fund-holdings` +
         `fund-history` (sequential, rate-limited) → 4. flight decode → 5. assemble `ExtractResult`
- [x] As-of semantics (RESEARCH.md §9.8): page "As of" table header → `extractor_as_of_date` (required —
      missing as-of fails the extraction, per spec); holdings section keeps its own `dt`
- [x] Error semantics per spec: required (fund info/overview, holdings, as-of) atomic — any failure rejects the
      whole extraction; optional sections → `nil, nil`; API failures surfaced distinctly from parse failures
      (undocumented-API risk, RESEARCH.md §9.1)
- [x] Rewrite `e2e_test.go`: injected fetch serving page + both API bodies — full flow for QGRW (UCITS) and
      EZM (US); assert every section, as-of date, holdings count (101 / 493), no product-charts calls
- [x] Write tests

**Verification:** `go test ./internal/domain/extractor/wisdomtree/` (full package)

### Task 6: Cleanup + full verification [PRIORITY: MEDIUM]

**Corresponds to:** DoD (no dead code, tests green, docs current)
**Description:** Remove v1 artifacts and verify the whole feature end-to-end.

- [ ] Delete v1 testdata fixtures (`wmgt_page_cycletls.html`, `wmgt_modal_all_holdings.html`) and any code paths
      left after Tasks 4–5; re-point `real_test.go` embeds to new fixtures
- [ ] Copy remaining fixtures into `testdata/` (full pages `qgrw_new.html`, `ezm_page.html` for e2e)
- [ ] Run full suite: `go test ./...`; `goimports -w .`; confirm no references to old parsers/variables
- [ ] Update `NOTES.md` (deviations, if any); check off tasks in this plan
- [ ] (User) Live verification from the portal: refresh a WisdomTree symbol and check the symbol details page

**Verification:** `go test ./...` green; `git status` clean

### Task 7 (user): Update stored source URLs in the portal [PRIORITY: HIGH]

**Corresponds to:** RESEARCH.md §1a, §9.5
**Description:** The matcher (Task 1) no longer matches old URLs — stored `data_source_url` values must be
migrated before the new code is deployed.

- [ ] For each WisdomTree symbol, set the new-format URL (region/asset-class/slug; e.g. WMGT →
      `https://www.wisdomtree.com/gb/products/equities/wisdomtree-megatrends-ucits-etf---usd-acc`)
- [ ] Ordering: **update URLs first, deploy second**. In the short window between the two, dispatch fails
      explicitly (symbols marked failed, cached data preserved) — acceptable.

**Verification:** All WisdomTree symbols match the new regex; a manual refresh succeeds

## Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Data sources | `fund-holdings` + `fund-history` APIs; flight payload for the rest | User decision (API-first) + RESEARCH §5.4: these are the only data APIs; section tables/breakdowns exist only in flight |
| `product-charts` API | Not called (deferred) | User decision: "ignore it"; US funds only, strictly additive — recorded as future work in NOTES.md |
| URL matching | New format only, no legacy fallback | User decision; stored URLs migrated from the portal (Task 7) |
| Fund identity | `wtClassID` extracted from page body each run | Per share class (exactly what the stored URL pins); self-heals on re-listing (RESEARCH §3, §9.3) |
| As-of date | Page "As of" table header (required); holdings keep their own `dt` | RESEARCH §9.8 recommendation; satisfies the spec's required-as-of rule |
| Table/section selection | By name (`ariaLabel`/`sectionId`), never position | US pages carry a different/superset table set (RESEARCH §4a) |
| Date formats | Both `DD/MM/YYYY` (UCITS) and `M/D/YYYY` (US) | RESEARCH §4a |
| Holdings fallback from flight | Not in Phase 2 (API only; flight-embedded holdings = future work) | Keeps scope tight; API verified on UCITS + US; API failures surfaced distinctly for diagnosis |
| Client / transport | Existing CycleTLS client, 1 s rate limit unchanged | Verified to pass Cloudflare on pages and APIs (RESEARCH §2); no client infra changes |
| Result contract | `extractor.ExtractResult` + `nil, nil` optional semantics unchanged | Service layer untouched (spec + original Task 4 remain valid) |
| Test fixtures | Package `testdata/`: full API JSONs + 2 full pages (e2e) + trimmed flight chunks (units); full captures stay in `features/.../samples/` | Follows existing provider pattern (dws/imgp/dimensional `testdata/`); hermetic unit tests stay small |

## Risks

- **Undocumented APIs change** (`/api/fund-holdings`, `/api/fund-history` are reverse-engineered) — errors
  surfaced distinctly (API vs parse); mitigation path documented: flight-embedded holdings as fallback (future work).
- **Flight payload is Next.js internal** — shapes are stable within the app; decoder is tolerant (scan all chunks,
  name-based matching, ignore unknowns); both UCITS and US fixtures pin the current shapes in tests.
- **Fund-to-fund section variance** (QGRW lacks themes; US pages differ) — `nil, nil` convention + fixtures for
  both variants (RESEARCH §9.2).
- **URL migration window** — between portal URL updates and deploy, extraction fails explicitly (no data loss);
  Task 7 ordering minimizes the window.
- **AUM unit trap** — API `aum` is in millions; conversion asserted in tests against the NAV-table cross-check
  (RESEARCH §7: QGRW `47442.9648` ↔ `US$47,442,965`).
- **Large responses** (WMGT holdings 350 KB, pages ~5 MB) — same CycleTLS client already handles 480 KB pages;
  no change, monitored in e2e timings.
