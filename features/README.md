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
| f005 | Transactions CRUD UI | spec | f004, f002, f003 |
