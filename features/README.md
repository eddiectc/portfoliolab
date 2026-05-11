# Features

Each feature has its own directory: `features/<id>_<name>/`

- `SPEC.md` — BDD feature spec (what, implementation-agnostic)
- `PLAN.md` — Implementation plan with tasks (how, with checkboxes for progress)
- `NOTES.md` — Deviations, decisions, future improvements

Feature IDs are sequential: f001, f002, etc.

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
| f012 | Performance Benchmark | spec | f010, f011 |
