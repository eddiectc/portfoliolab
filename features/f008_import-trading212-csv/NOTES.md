# Notes: Import Trading 212 CSV

## Decisions
- 2026-05-06: Task 0 extracted shared types/interfaces into `internal/domain/brokerimport/`. The `ImportService` handler interface was kept in the handlers package (not moved to brokerimport) since it's a handler-layer concern that references `brokerimport.PreviewResponse`/`ImportResult` as return types. The domain-level interfaces (SymbolResolver, DuplicateChecker, etc.) were moved to brokerimport as planned.
- IBKR-specific error variables (`ErrAccountNotFound`, `ErrInvalidXML`, `ErrNoImportableTransactions`) stayed in `ibkrimport/service.go` — they're broker-specific and not shared.
- 2026-05-06: Task 2 service follows IBKR pattern exactly: type aliases in `models.go`, `Service` struct with dependency injection, `Preview`/`ConfirmImport`/`CreateSymbol`/`AddBrokerSymbolMapping` methods. Net cash for buys is negative total, sells positive. Cash transactions use `$CASH-{currency}` symbol with quantity=total, price=1. Withdrawal net cash is negated (cash outflow).
- Type aliases in `ibkrimport/models.go` and `ibkrimport/service.go` preserve the existing package API so no callers outside the package need changes.
- 2026-05-06: Task 3 handler follows IBKR pattern exactly. `parseAccountID` is shared (already in ibkr_import.go, same package). `readCSVFile` is the CSV equivalent of IBKR's `readXMLFile` — reads `csv_file` form field instead of `xml_file`.

## Deviations from Plan
- None. Task 0 followed the plan exactly.

## Future Improvements
- None noted.

## Known Issues
- None.

## Session Log
- 2026-05-06: Task 4 (Web UI) completed. Created `trading212_import_web.go` (web handler), `t212_import.html` (upload page), `t212_import_preview.html` (preview page with resolve modal), and `trading212_import_web_test.go` (11 tests). Follows IBKR pattern exactly, targeting Trading 212 API endpoints and using "Trading212" as broker name.
- 2026-05-06: Task 5 (Router Wiring + Navigation) completed. Wired `trading212import.Service`, `Trading212ImportHandler`, and `Trading212ImportWebHandler` into `router.go` reusing the same dependency instances (symbolResolver, transactionRepo, accountChecker, symbolCreator, brokerSymbolAdder) as IBKR. Added "Trading 212 CSV" link to the Import dropdown in `templates/transaction/list.html`. Updated features/README.md status to "in-progress". `go build ./...` and `go test ./...` both pass.
- 2026-05-06: End-to-end verification passed. Feature f008 complete.
