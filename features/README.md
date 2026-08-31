# Features

Each feature has its own directory: `features/<id>_<name>/`

- `SPEC.md` — BDD feature spec (what, implementation-agnostic)
- `PLAN.md` — Implementation plan with tasks (how, with checkboxes for progress)
- `NOTES.md` — Deviations, decisions, future improvements

Feature IDs are sequential: f001, f002, etc.

## Feature Revisions

A feature folder always describes the **current** state of its capability — the docs must let someone re-implement the feature as it exists today. When an external dependency changes in a way that requires reworking the feature (e.g., a provider's website relaunch), the feature is **revised in place** — it is not tracked as a new feature, and no new feature ID is taken.

- Keep the same feature ID. User stories describe the capability, not the implementation, so they typically stay unchanged.
- Archive superseded docs in a versioned subfolder (`v1/`, `v2/`, ...) — old `RESEARCH.md`, `PLAN.md`, `NOTES.md`, `RETRO.md`, `samples/` — each with a `SUPERSEDED` header pointing to the current version.
- Root docs become the current version (a `RESEARCH-NEWSITE.md` is renamed to `RESEARCH.md`); `PLAN.md` gains a new phase section; the revision event and decisions are logged in `NOTES.md`.
- Follow the normal workflow for the revision: spec revision → user review → plan → implement.
- During RESEARCH, verify every spec data item against the new source before claiming parity — an item the new source no longer publishes must be logged in `NOTES.md` as a documented empty, not discovered mid-implementation (f021: "fund family" row absent from the new site).

Example: `f021_wisdomtree-scraper` (WisdomTree site relaunch 2026-08-30, v1 implementation archived in `f021_wisdomtree-scraper/v1/`).

## Feature Index

| ID | Name | Status | Depends On |
|---|---|---|---|
| f001 | Portfolio CRUD | done | — |
| f002 | Account CRUD | done | f001 |
| f003 | Symbol Map | done | — |
| f004 | Transaction CRUD | done | f002, f003 |
| f005 | Transactions CRUD UI | done | f004, f002, f003 |
| f006 | Auto-create Symbol | done | f003, f004, f005 |
| f007 | Import IBKR Flex XML | done | f002, f003, f004 |
| f008 | Import Trading 212 CSV | done | f002, f003, f004 |
| f009 | Positions | done | f002, f003, f004, f005, f007, f008 |
| f010 | Portfolio Performance | done | f009 |
| f011 | Historical Market Data Caching | done | f010, f009, f004 |
| f012 | Performance Benchmark | done | f010, f011 |
| f013 | Unitization | done | f010, f012 |
| f014 | Benchmark Selection | done | f003, f011, f012 |
| f015 | Symbol Details | done | f003, f011 |
| f016 | Symbol Details — Geographic Data | done | f015 |
| f017 | Portfolio Analysis | done | f009, f010, f011, f015, f016 |
| f018 | Allocation | done | f009, f010, f011, f001 |
| f019 | Model Portfolio | done | f003, f009, f011, f018 |
| f020 | Portfolio Comparison | done | f019, f009, f010, f011, f012, f015 |
| f021 | WisdomTree Scraper | done | f015, f011 |
| f022 | DWS Scraper | done | f021 |
| f023 | Dimensional Scraper | done | f015, f011 |
| f024 | iMGP Scraper | done | f021 |
| f025 | Vanguard Scraper | done | f021 |
| f026 | BlackRock/iShares Scraper | done | f021 |
| f027 | Overlap Enhancement | done | f020, f015, f017 |
| f028 | Efficient Frontier | done | f003, f011, f019, f001 |
| f029 | Hierarchical Risk Parity | done | f003, f011, f019, f001 |