# Notes: Import IBKR Flex XML

## Implementation Decisions

### FX trades processed before instrument type check

The plan lists "classify instrument type" as the first step in `processTrade`. However, FX trades (`assetCategory=CASH`) must be handled **before** the `isSupportedTrade` check, since `isSupportedTrade` only accepts `STK` instruments and would reject FX trades as "unsupported". The ordering is:

1. Check if FX trade (`assetCategory=CASH`) → handle, return early
2. Check if supported instrument (`STK COMMON/ETF`) → skip if not
3. Resolve symbol → skip if unmapped
4. Check duplicate → skip if exists

### FX target currency extraction

FX symbol format is `{sourceCurrency}.{targetCurrency}` (e.g. `GBP.USD`). The target currency is extracted by splitting on `.` with `strings.SplitN(trade.Symbol, ".", 2)` and taking the second part. This is more robust than `strings.TrimSuffix` which would require knowing the exact source currency suffix.

### FX trades generate composite external references

Each FX trade splits into two transactions (withdrawal + deposit). Both share a composite external reference derived from the original transaction ID:

- `{transactionID}_fx_withdrawal` — source currency withdrawal
- `{transactionID}_fx_deposit` — target currency deposit

This ensures both halves are traceable to the original IBKR transaction and individually de-duplicated.

### FX duplicate detection is per-trade, not per-entry

When an FX trade is detected as a duplicate (by its original `transactionID`), the **entire trade** is skipped — it doesn't generate 2 skipped entries. This means the skipped count reflects the number of source records, not the number of generated preview entries. The plan's test expectation of "16 skipped" for all-duplicates was corrected to 15 (15 source records, not 16 generated entries).

### Cash transaction "Deposits/Withdrawals" classified by amount sign

Cash transactions with `type="Deposits/Withdrawals"` are classified by the sign of the `amount` field: positive → deposit, negative → withdrawal. Zero or empty amount defaults to deposit. This matches the Rust reference implementation.

### ConfirmImport builds `*transaction.Transaction` directly

The plan mentions building `transaction.CreateRequest` for each importable transaction. In practice, the service builds `*transaction.Transaction` structs directly, since the `TransactionCreator.BatchCreate` interface accepts `[]*transaction.Transaction`. The `CreateRequest` DTO is only used at the API handler layer for JSON deserialization.

### Transfer netCash uses original amount (not absolute)

For transfers, the `NetCash` field on the transaction uses the original `cashTransfer` value from the XML (positive for deposits, negative for withdrawals). The preview display shows absolute values for consistency, but the actual transaction preserves the sign. This aligns with the existing transaction model where `NetCash` is signed.

### Withdrawal cash transactions use original negative amount

For cash transactions classified as withdrawals (negative `amount`), the `Quantity` and `NetCash` fields preserve the negative sign from the XML. The preview display shows the absolute value for readability. This is consistent with how deposits work (positive amount throughout).

## Plan Deviations

| Plan Item | Actual | Reason |
|---|---|---|
| `CreateRequest` built in ConfirmImport | `*transaction.Transaction` built directly | `BatchCreate` interface accepts `[]*transaction.Transaction`; `CreateRequest` is API-layer only |
| 16 skipped for all-duplicates preview | 15 skipped | FX trade is 1 source record → 1 skip, not 2 entries |
| Instrument type check first | FX check first | `isSupportedTrade` rejects `CASH` instruments; FX must be handled before that check |

## Test Coverage

- 59 tests total (27 parser + 32 service)
- All tests pass: `go test ./internal/domain/ibkrimport/...`
- Full sample XML integration test verifies all 16 transactions created correctly
- Edge cases: all-duplicates, unmapped symbols, FX trades, transfers, cash transaction types, rollback, empty report
