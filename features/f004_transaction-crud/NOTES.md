# Notes: transaction-crud

## Decisions
- 2026-05-04: Migration numbered `004_create_transactions.sql` (not `003` as in the plan) because `003` was already used by f003_symbol-map. Plan text referenced `003` but actual file is `004`.
- 2026-05-04: Handler uses `parseTransactionListParams` to extract filters + pagination from query params, reusing existing `parsePagination` for limit/offset. Filter params (account_id, symbol, type, date_from, date_to) parsed separately with graceful fallback on parse errors.
- 2026-05-04: Created `AccountCheckerImpl` in data layer following `PortfolioCheckerImpl` pattern. Created `SymbolCheckerImpl` (wraps repo for `SymbolExists`) and `SymbolCreatorImpl` (wraps service for `CreateSymbol`) as adapters between transaction domain interfaces and symbol-map types.
- 2026-05-04: Integration tests use `setupTx` helper that creates portfolio + account + symbol mapping in one call, following the existing `setupTestDB` pattern. All 14 tests (including 8 validation sub-tests) pass against real SQLite in-memory DB with full router.
- 2026-05-04: Cascade delete is handled entirely by SQLite FK constraints, not application logic. Chain: `portfolios` → `accounts` (migration 002, `ON DELETE CASCADE`) → `transactions` (migration 004, `ON DELETE CASCADE`). FK support enabled via `PRAGMA foreign_keys=ON` in `data.Open()`. Verified by `TestTransaction_CascadeDeleteAccount` and `TestTransaction_CascadeDeletePortfolio` integration tests.
- 2026-05-04: Added symbol_mappings + broker_symbol_mappings tables to integration test setup (migration 003 was missing from test DB schema).
- 2026-05-04: `sql.NullString` used by sqlc for nullable columns (net_cash, external_system, external_reference) — repo layer will convert to `*decimal.Decimal` / `*string` in domain model.
- 2026-05-04: `decimal.MustNew(value, scale)` API: value is the integer shifted by 10^scale (e.g., `MustNew(15000, 2)` = 150.00). Used a helper `d(value, scale)` in tests for readability. `MustNew(0, 75)` was wrong — scale is decimal places count, not the fractional part.
- 2026-05-04: `decimal.Decimal.IsPos()` (not `IsPositive()`) for positive check.
- 2026-05-04: `decimal.Decimal.String()` preserves scale (e.g., `MustNew(15000, 2).String()` = "150.00" not "150"). Use `.Equal()` for comparisons, not string equality.
- 2026-05-04: `decimal.Parse(s string)` (not `NewFromString`) for deserialization. `MustParse` available for non-error-returning context.
- 2026-05-04: Repository uses `sql.NullString` ↔ `*decimal.Decimal` / `*string` conversion helpers (`toNullDecimal`, `toNullString`).
- 2026-05-04: Date range filter requires both `DateFrom` AND `DateTo` to be non-nil at the repo layer; single date bound falls through to no-filter path. This matches the sqlc query signatures which always take a pair.
- 2026-05-04: Service layer handles single-bound date filters by synthesizing the missing bound (DateFrom only → far-future DateTo; DateTo only → far-past DateFrom). This bridges the gap between repo query signatures and user-facing API.
- 2026-05-04: `limit=0` or `limit<0` defaults to 50; `offset<0` → 0. Follows existing account service convention (`limit <= 0 → defaultLimit`). Spec originally said "limit=0 means no limit" but corrected to match existing convention.
- 2026-05-04: Defined `SymbolChecker` and `SymbolCreator` interfaces in the transaction domain rather than depending directly on f003 types. This follows the same pattern as `AccountChecker` and enables clean mocking in tests.
- 2026-05-04: Helper functions `dec()` and `decp()` in mock_repository.go (renamed from `d`/`dp` to avoid conflict with validator_test.go helpers).
- 2026-05-04: `mapValidationError` uses string matching to map validator errors to service errors. This is simple and sufficient since both live in the same package.

## Deviations from Plan
- Migration filename: plan said `003_create_transactions.sql`, actual is `004_create_transactions.sql` (sequential after symbol_mappings)

## Future Improvements
- None yet

## Known Issues
- None
