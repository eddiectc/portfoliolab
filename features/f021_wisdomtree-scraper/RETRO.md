# Retrospective: Data Extractor Framework with WisdomTree Provider — Phase 2 (New-Site Rebuild)

**Feature ID**: f021
**Date**: 2026-08-31
**Scope**: This retro covers the Phase 2 rebuild (2026-08-30 → 2026-08-31, 2 days: research → plan →
7 tasks → 2 implementation reviews → user live verification). The v1 build (2026-05-26 → 05-28) has its
own retrospective at `v1/RETRO.md` and is not re-covered here except where Phase 2 validates or contradicts
it.

**Inputs reviewed**: `SPEC.md` (unchanged since 2026-05-26), `PLAN.md` (Phase 2, approved 2026-08-30),
`RESEARCH.md` (2026-08-30), `NOTES.md`, git history (`fad2bb7` → `531db0a`, 12 commits), implementation in
`internal/domain/extractor/wisdomtree/` (13 files, ~3,100 LOC incl. tests), integration test
`tests/integration/wisdomtree_extractor_e2e_test.go`. Final state: full suite green, `golangci-lint` clean
on the package, `/review-impl` passed 2026-08-31, live portal verification passed, stored URLs migrated.

---

## What Went Well

- **The feature-revision convention was exercised for the first time and held up.** Same feature ID, v1
  docs archived in `v1/` with `SUPERSEDED` headers, spec left untouched (it was implementation-agnostic —
  every story still describes the capability, not the old site), rebuild done at the plan level. The
  feature folder is re-implementable against the current site, and f021 now serves as the worked example
  in `features/README.md`.

- **RESEARCH.md-first again paid off (second consecutive provider feature).** Before planning: sitemap-
  verified URL formats, Cloudflare check against the existing CycleTLS client, exhaustive API surface map
  (§5.4 — proving which data has *no* API), per-fund section variance, cross-checks of the same data point
  from two sources (§7), and full raw captures in `samples/`. The "old parser → new source" data map (§6)
  became the skeleton of the plan. No mid-implementation surprises about *where* data lives; the only
  corrections were value-level (units, sample rows), see "Could Be Improved".

- **API-first found strictly better data.** The reverse-engineered `/api/fund-holdings` and
  `/api/fund-history` endpoints replaced v1's HTML/CSV parsing and added fields v1 never had
  (`securityTicker`, `figi`, `shares`, `marketValueBase`, per-holding sector). The existing CycleTLS
  client + 1 s rate limit covered pages *and* APIs with zero client changes — the single `SetFetchFunc`
  injection point already made API bodies testable.

- **Contract preservation kept the blast radius to one package.** `extractor.ExtractResult` and the
  optional-section `nil, nil` semantics were unchanged, so the service layer, dispatcher, DB schema,
  migration set, and web UI were all untouched. Five downstream provider features (f022–f026) depend on
  f021's framework — zero churn. The rebuild diff is ~3.2k insertions against ~26k deletions of v1
  fixtures/parsers: a replacement, not a patch, exactly as planned.

- **Error semantics match the spec's "no silent fallback" constraint.** Per-step distinct error wrapping
  (`fetch page:`, `wtClassID:`, `fund-holdings API:`, `fund-history API:`) separates transport from parse
  failures — critical for reverse-engineered, undocumented APIs. Atomicity for required items and
  `nil, nil` for optional sections are both pinned by unit and e2e tests.

- **Hermetic, fixture-driven tests.** Real captures (6 full API JSONs, trimmed flight chunks, 2 full
  pages for e2e) covering both UCITS (QGRW/WMGT) and US (EZM) variants; the rewritten integration test
  drives the real service + API path with an injected fetcher and a fake Yahoo stub — no network. The
  e2e asserts fetch order, URL construction from the *extracted* wtClassID, exactly 3 fetches, and no
  `product-charts` call.

- **Reviews caught a real bug before "done".** The `/review-impl` of Tasks 4+5 found that
  `ParseFundCharacteristicsFromFlight` never set `FieldsPresent`, so the f025 service-layer gate
  (`HasCharacteristic(PE)`) silently discarded every WisdomTree equity valuation. All unit tests and the
  package e2e passed despite this — only a cross-layer review caught it. This is the highest-value
  process moment of the rebuild.

- **Discipline in commits and docs.** One concern per commit (12 commits, each mappable to a plan task),
  PLAN checkboxes tracked continuously, NOTES entries per task, deviations logged the same day they
  happened, and the user task (URL migration) sequenced correctly (URLs first, deploy second) — the
  documented failure window never dropped data.

## What Could Be Improved

- **Plan and research carried unmeasured numbers and one wrong unit.** PLAN's e2e line said "holdings
  count (101 / 493)" — pre-measurement guesses; fixtures hold 100 / 506 tradeable. The AUM unit was
  mislabelled "millions" in two places (RESEARCH §5.2 and the Task 2 note) while the worked example next
  to it was ×1000 (thousands) — the example was right, the label wrong; Task 4 caught it by plausibility
  check. RESEARCH §5.1's example field values (assetGroup label, NVDA row) were also stale vs the actual
  captures. *Fix pattern proven during the work: treat fixtures as authoritative and correct the doc when
  they disagree — do it at research time instead.*

- **The cross-layer bug class (field populated, gate unmet) was caught by review, not by tests.** The
  `FieldsPresent` failure mode is silent, cross-feature, and invisible to package-level tests. The
  integration test only asserted persistence *after* the fix. A provider-feature test that asserts an
  extracted section is present in the DB *before* implementation review would have caught it on its own.
  (See Action Items.)

- **Parity was over-claimed in research.** "All previously extracted data is obtainable" was wrong for
  two spec items: fund family ("Fund Umbrella" row no longer exists on the new site) and annual holdings
  turnover (never exposed by the old site either). NOTES corrected it after Task 4 verified by dumping
  both-region fixtures. *A per-spec-item availability check belongs in RESEARCH, not discovered during
  parsing.*

- **One genuine spec drift, documented rather than fixed.** SPEC lists NAV history among *optional*
  sections, but `Extract()` treats a `fund-history` API failure as fatal — because required items
  (inception date, AUM) derive from the same API. The justification is sound and is in NOTES, but the
  spec's required/optional split is by *section* while the real constraint is by *data item*.

- **Plan wording drift during the build.** "Re-point `real_test.go` embeds" (no such file after the Task 5
  rewrite), the stale holdings counts, and the formatter item: `goimports -w .` swept 114 unrelated files
  of pre-existing drift. The sweep was correctly reverted to keep the commit focused, but the plan item
  should have said "run goimports on changed files". (The drift itself was later committed separately as
  `fda1c42` — the right pattern, but it cost a planning miss to discover.)

- **NOTES.md is missing the "Future Improvements" section its own text points to.** Two entries claim
  `product-charts` deferral and the flight-holdings fallback are "recorded as future work in NOTES.md",
  but NOTES has no such section. The information currently lives only in PLAN.md's Technical Decisions.

- **Pre-existing flaky test noise.** `TestBenchmarkChartAllPeriods/1M` (date-dependent) was noted in three
  separate NOTES entries across the rebuild before being fixed in `8e59226`. Harmless but recurring
  "is the suite green?" friction; date-dependent tests should seed their own windows.

## Spec vs Reality

- **Spec survived a total provider rebuild unchanged** — the strongest possible validation of keeping it
  implementation-agnostic. All five stories and all listed edge cases are implemented and tested (matcher
  new-format-only; provider routing; atomic required / `nil, nil` optional sections; distinct API-vs-
  parse errors; unregistered-provider failure; 800+ holdings uncapped; non-overlapping NAV/price chart).

- **Two spec data items are unavailable from the source, not from us.** "Fund family" and "annual
  holdings turnover" are named in Story 2's overview list. Neither exists on the new site (turnover never
  existed on the old one — v1 stored zero). Fields are intentionally empty/zero and documented. The spec
  has no concept of "item the provider no longer publishes"; today this is NOTES-only knowledge.

- **The fund-history optional/required drift** (above) is the only behavioral deviation from the spec's
  letter. It makes the system *stricter* than specified (fail instead of persisting without NAV rows),
  which is the spec's own stated philosophy — but it is unamended spec text.

- **"As of" semantics met exactly**: the NAV table's "As of" header (present on both regions) is the
  extraction reference date; missing/non-date header fails the extraction; holdings keep their own
  `dt`. The spec's "provider's as-of, not the fetch date" rule is honored and pinned by e2e.

## Plan vs Reality

- **Task breakdown effective; sizing good.** 7 tasks in dependency order (1–3 independent, 4 → 5 → 6,
  7 user), each independently testable with a working verification command, each fitting one commit batch.
  Nothing needed splitting; nothing was too small to matter.

- **One dependency the plan did not show.** Task 6 (cleanup) implicitly owned deleting the v1 integration
  test fixtures — but the *test file itself* (`wisdomtree_extractor_e2e_test.go`) broke `go test ./...`
  the moment Tasks 4–5 landed, so its rewrite was pulled forward into the review-fix commit. The plan
  listed the fixture deletion as Task 6 without naming the dependent test file.

- **User task 7 sequenced correctly and closed the loop.** Portal URLs migrated before deploy; the
  documented explicit-failure window between the two caused no data loss; live verification passed.

- **Deviations were small, all documented in NOTES, most improving the plan**: cash-row filter switched
  from v1 name-keywords (which mis-filtered "FirstCash") to the API's structural `securityTicker` signal;
  wtClassID regex tightened (RE2 has no lookbehind — quote anchor instead); fixture names shortened;
  `DecodeFlight` returns a struct rather than separate accessors. None changed scope or the contract.

- **Risks section was accurate.** The reverse-engineered-API risk was the live one; its mitigations
  (distinct error wrapping, flight-holdings fallback documented as future work) are exactly what shipped.
  The AUM-unit trap called out in Risks materialized as a doc error, not a code bug — tests caught it.

## Learnings

1. **RESEARCH must include a per-spec-item availability matrix** (each spec data item → new source →
   verified against a captured sample) before the plan claims parity. Would have surfaced "fund family
   gone" on day 0 instead of during Task 4.
2. **Plan numbers must be measured from captured fixtures** (record counts, unit conversions with a
   worked example *and* its unit label side by side). Guesses produce plan corrections and doc drift.
3. **For provider features, integration tests must assert persistence, not just extraction.** A new
   provider filling a shared struct must be tested through every service-layer gate (e.g.
   `HasCharacteristic` masks) — the `FieldsPresent` bug passed every package test. The AGENTS.md
   cross-layer field-mapping audit rule should gain this explicit gate check.
4. **The spec's required/optional enumeration should be per data item, not per section** — a section
   that sources a required item (fund-history → inception/AUM) cannot be optional even if the section's
   rows can be. Amend SPEC wording in a later pass or accept the drift permanently.
5. **Formatter hygiene in plans**: scope format runs to changed files; repo-wide formatting is a
   separate chore commit (`fda1c42` is the template for that).
6. **Feature revisions: keep doing exactly what was done here.** The archive → research → unchanged-spec
   → plan-phase → in-place-NOTES flow worked end-to-end and is now a documented precedent.
7. **Undocumented-API strategy validated**: tolerant decode + name-based (never positional) selection +
   distinct error wrapping + fixture-pinned shapes means a silent site change becomes a loud,
   diagnosable failure — never corrupt data.

## Action Items

User review 2026-08-31: items 1–2 approved and executed; items 3–5 closed as
non-issues (no action) — the spec/NOTES drift stays as documented.

- [x] **AGENTS.md (cross-layer audit)**: extended the field-mapping audit with a
      persistence-gate check (new item 7 + gate rule of thumb). *(Done 2026-08-31.
      Prevents the FieldsPresent bug class.)*
- [x] **features/README.md (feature-revision convention)**: added "verify every
      spec data item against the new source before claiming parity" (fund-family
      lesson). *(Done 2026-08-31.)*
- Closed as non-issue (user, 2026-08-31): f021 NOTES.md "Future Improvements"
      section — the two forward references stay as-is; the deferral is clear in
      context.
- Closed as non-issue (user, 2026-08-31): SPEC.md wording pass — required/optional
      drift and provider-side data loss (fund family, turnover) accepted as
      documented in NOTES.md.
- Closed as non-issue (user, 2026-08-31): formatter-scope plan hygiene.
- Close: repo-wide goimports drift — resolved via `fda1c42`; pre-existing
      `TestBenchmarkChartAllPeriods/1M` — resolved via `8e59226`. *(No action;
      both fixed before sign-off.)*
