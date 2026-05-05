# Notes: Transactions CRUD UI

## Deviations from Spec/Plan

### HandleListPage initially deferred ListWithAccount
The web handler was initially wired to use `transactionSvc.List` with manual account name mapping (N+1 pattern) because the service layer didn't expose `ListWithAccount` yet. A TODO was left in the code. This was resolved during the implementation review: `ListWithAccount` was added to the service layer and the handler was updated to use it directly, eliminating the N+1 query.

### queryPreserve template function — reflection-based rewrite
The original `queryPreserve` function used a hard-coded local `TransactionFilter` struct type for type assertion. This never matched the actual `TransactionFilter` from `transaction_web.go` because Go type assertions require exact type identity, not structural equality. Rewrote to use reflection, reading fields by name generically. Returns `template.HTMLAttr` to prevent double-escaping of `&` in href attributes.

### External field validation error — added dedicated error code
The `mapValidationError` function in the handler didn't map external field validation errors to a specific error code, causing them to fall through to generic error display. Added `ErrInvalidExternalField` to the transaction service and mapped it in `transactionUserFriendlyError` and `mapFieldErrors`.

## Design Decisions

### OptionalDecimal type
Created `OptionalDecimal` wrapper (`IsSet bool`, `Dec decimal.Decimal`) for `UpdateRequest.NetCash` to distinguish "field omitted" from "field explicitly null". The `Value` field was renamed to `Dec` and the getter to `Get()` to avoid collision with `decimal.Decimal.Value()`. Custom `json.Unmarshaler`/`json.Marshaler` track presence via `IsSet` flag.

### Decimal zero check
Used `decimal.Decimal.IsZero()` instead of `Cmp(decimal.Zero) == 0` for cleaner API usage in the validator.

### Mock repository sorting
The mock `ListWithAccount` sorts results by date descending, then ID ascending before applying pagination. This matches the real repository's SQL `ORDER BY` and ensures deterministic test behavior. Without sorting, map iteration order is non-deterministic and pagination can return inconsistent results.

### Test assertions — price values over symbol names
Initial tests used `strings.Contains(body, "AAPL")` to verify transaction presence/absence. The template has `placeholder="e.g. AAPL"` in the symbol filter input, causing false positives. Changed to check unique price values (e.g., `150.00` for AAPL, `140.00` for GOOG) which only appear in transaction data rows.

### Pagination filter persistence — URL encoding
Filter params in pagination links are URL-encoded by Go's template engine (`&type=buy` becomes `&type=buy` in the HTML source). This is correct behavior — browsers decode it properly. Tests check for the URL-encoded form (`type%3dbuy`).

### Cash symbol auto-creation
The handler delegates cash symbol creation to the service layer, which already handles it. No additional logic needed in the web handler.

### Currency auto-fill
Client-side JavaScript with 500ms debounce fetches `/api/symbol-mappings/preview?symbol=X`. Only fills currency when the field is empty. Silently fails on error (user fills manually). User override takes precedence.

### Sell transactions with negative quantity
The spec calls out "quantity -5" for sell transactions. The domain validator accepts negative quantities (for short selling). The web form doesn't restrict the quantity input to positive values.

### Form layout
Single-page form with all fields (no multi-step wizard), consistent with existing account and symbol mapping forms. Uses `<fieldset>`/`<legend>` grouping for related fields.

### Delete confirmation
Uses JavaScript `confirm()` dialog, consistent with the symbol mapping delete pattern.

### List sorting
Default order: date descending, then symbol ascending, then type ascending, then ID ascending. Implemented in SQL via `ORDER BY`.

## Known Limitations

### No symbol autocomplete
The symbol field is a text input. Users must type the symbol manually. An autocomplete dropdown could be added as a follow-up (similar to the account dropdown pattern).

### No client-side validation
All validation is server-side. The form doesn't prevent submission of obviously invalid data (e.g., empty required fields). HTML5 `required` attributes are used on some fields but not all.

### Pagination is prev/next only
No page number navigation (e.g., "1 2 3 ... 10"). Users must click "Previous"/"Next" sequentially.

### No bulk operations
No select-all, bulk delete, or bulk edit. Operations are per-transaction only.

## Files

| File | Purpose |
|------|---------|
| `internal/domain/transaction/transaction.go` | Domain model, `OptionalDecimal` type, `TransactionWithAccount` |
| `internal/domain/transaction/validator.go` | Validation logic, `ErrInvalidNetCash`, `ErrInvalidExternalField` |
| `internal/domain/transaction/service.go` | Service layer, `ListWithAccount` method |
| `internal/data/queries/transaction.sql` | 16 `ListTransactionsWithAccount` query variants |
| `internal/data/transaction_repo.go` | Repository, `ListWithAccount` with filter routing |
| `internal/api/handlers/transaction_web.go` | 7 web handlers for CRUD + list |
| `internal/api/handlers/transaction_web_test.go` | 28 unit tests for web handlers |
| `templates/transaction/list.html` | List page with filters, pagination, delete |
| `templates/transaction/form.html` | Create/edit form with currency auto-fill JS |
| `templates/transaction/detail.html` | Detail page with edit/delete actions |
| `internal/web/renderer.go` | `queryPreserve` template function (reflection-based) |
| `internal/web/static/css/style.css` | Styles for filter bar, type badges, pagination |
| `migrations/005_backfill_net_cash.sql` | Backfill null `net_cash` values |
