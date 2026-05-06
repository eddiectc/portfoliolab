# Notes: Import Trading 212 CSV

## Decisions
- 2026-05-06: Task 0 extracted shared types/interfaces into `internal/domain/brokerimport/`. The `ImportService` handler interface was kept in the handlers package (not moved to brokerimport) since it's a handler-layer concern that references `brokerimport.PreviewResponse`/`ImportResult` as return types. The domain-level interfaces (SymbolResolver, DuplicateChecker, etc.) were moved to brokerimport as planned.
- IBKR-specific error variables (`ErrAccountNotFound`, `ErrInvalidXML`, `ErrNoImportableTransactions`) stayed in `ibkrimport/service.go` — they're broker-specific and not shared.
- 2026-05-06: Task 2 service follows IBKR pattern exactly: type aliases in `models.go`, `Service` struct with dependency injection, `Preview`/`ConfirmImport`/`CreateSymbol`/`AddBrokerSymbolMapping` methods. Net cash for buys is negative total, sells positive. Cash transactions use `$CASH-{currency}` symbol with quantity=total, price=1. Withdrawal net cash is negated (cash outflow).
- Type aliases in `ibkrimport/models.go` and `ibkrimport/service.go` preserve the existing package API so no callers outside the package need changes.

## Deviations from Plan
- None. Task 0 followed the plan exactly.

## Future Improvements
- None noted.

## Known Issues
- None.
