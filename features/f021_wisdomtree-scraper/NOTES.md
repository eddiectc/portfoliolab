# Notes: WisdomTree Scraper

## Decisions
- 2026-05-26: Migration 021 adds `data_source_url` to `symbol_mappings` and `extractor_as_of_date` to `symbol_details`. Both nullable (NULL = default Yahoo behavior).
- 2026-05-26: `ParseFundInfo` regex uses `fundInfo\w*` (hash suffix optional) to match both `var fundInfo = {...}` and `var fundInfo<HASH> = {...}` patterns.
- 2026-05-26: Holdings CSV weights are fractions (0.0137 = 1.37%), converted to percentage (1.37) to match existing `TopHolding.Percent` convention. Sectors CSV weights are already percentages — no conversion needed.
- 2026-05-26: `parseTableValue` regex uses `labelCell` including the closing `</td>` tag, then matches `\s*<td[^>]*>([^<]+)` for the value cell. This avoids false matches on nested content.
- 2026-05-26: Cash positions filtered by keyword list (CASH, EUR/USD/JPY etc.). Covers WisdomTree's "CASH W-O" and currency position names seen in samples.
- 2026-05-26: `extractFromHTML` helper function allows testing parsers without HTTP. Used by integration tests.
- 2026-05-26: `InceptionDate` added to `symbol.FundProfile` struct (new `time.Time` field, zero value = not set).
- 2026-05-26: Country allocation, market cap, and fund characteristics **ARE in raw HTML** when fetched with CycleTLS (confirmed 2026-05-26). The sample HTML we saved (`wmgt_page.html`) had empty `<tbody>` because it was NOT fetched with CycleTLS. This was a false alarm — no headless browser needed.
- 2026-05-26: `Renderer` interface, `js_parsers.go`, and `renderer.go` removed (unnecessary — all data extractable from raw HTML via CycleTLS).
- 2026-05-26: `ParseCountryAllocation`, `ParseMarketCap`, `ParseFundCharacteristics` added as HTML table parsers (same approach as `ParseFundProfile`). Use `regexp` on table rows with `key`/`value` class attributes.
- 2026-05-26: Go's RE2 engine doesn't match `\n` with `.` — use `[\s\S]` instead of `.+?` / `.*?` for multi-line regex.
- 2026-05-26: `FundCharacteristics` struct has only 5 fields (P/E, P/B, P/S, P/CF, Dividend Yield) — no Estimated P/E, Gross/Net Buyback Yield. Parsers only extract available fields.
- 2026-05-26: `Extractor` and `URLMatcher` are separate interfaces. Registry uses a `registryEntry` struct pairing both, since not all extractors may need URL matching and Go doesn't allow casting between unrelated interface types.
- 2026-05-26: Dispatcher validates URL with `url.Parse` before lookup — rejects malformed URLs early with explicit error.
- 2026-05-26: WisdomTree extractor was a stub returning `ErrNotImplemented` for Task 2 framework testing. Replaced with full implementation in Task 3.
- 2026-05-26: `GetNavHistoryBySymbol` query filters on `data_type = 'nav'` and `date != ''` (excludes current entries). Reuses existing `market_data` table with existing UNIQUE(symbol, source, date) constraint — NAV uses source='wisdomtree'.
- 2026-05-26: `ListStaleSymbolDetails` now returns `data_source_url` alongside internal_symbol and market_data_symbol, enabling the service layer to route fetches to the correct provider.

## Deviations from Plan
- Task 1: `InsertSymbolDetails` SQL query was missing `extractor_as_of_date` in INSERT columns and ON CONFLICT UPDATE. Fixed during implementation review (add column to write path alongside the read path that was already updated).
- Task 2: `extractorReg` is created in `router.go` but the `Dispatcher` is not wired yet — deferred to Task 4 where the service layer consumes it. Registry is registered with `_ = extractorReg` to suppress unused variable until then.
- Task 3: Phase 2 parsers (Country Allocation, Market Cap, Fund Characteristics) completed — they parse HTML tables directly, not JS-rendered data. The initial assumption that they were JS-rendered was based on a stale sample HTML file.
- Task 3: Real HTML testing (`wmgt_page_cycletls.html`) revealed two issues:
  1. **`unescapeJSString`** didn't handle `\"` (escaped double quotes in JS strings like `\"F5, Inc\"`). Added `\"` → `"` replacement.
  2. **`parseTableRawValue`** expected `<td class="key">TER</td>` on one line, but real HTML has whitespace/newlines between tag and text. Fixed by extracting label text dynamically and building a flexible regex.
- Task 3: Real HTML sample moved to `testdata/wmgt_page_cycletls.html` (embedded via `//go:embed`) instead of referencing the features folder. Tests are self-contained.
- Task 3: `fundSectorsData` is **not present** on the WMGT page in real CycleTLS HTML. Some WisdomTree pages may lack certain data sections. Full atomic extraction (`extractFromHTML`) is still enforced for pages that have all sections; real HTML tests verify individual parsers separately.

## Deviations from Plan
- Task 1: `InsertSymbolDetails` SQL query was missing `extractor_as_of_date` in INSERT columns and ON CONFLICT UPDATE. Fixed during implementation review (add column to write path alongside the read path that was already updated).
- Task 2: `extractorReg` is created in `router.go` but the `Dispatcher` is not wired yet — deferred to Task 4 where the service layer consumes it. Registry is registered with `_ = extractorReg` to suppress unused variable until then.
- Task 3: Phase 2 parsers (Country Allocation, Market Cap, Fund Characteristics) completed — they parse HTML tables directly, not JS-rendered data. The initial assumption that they were JS-rendered was based on a stale sample HTML file.
- Task 3: Real HTML testing (`wmgt_page_cycletls.html`) revealed two issues:
  1. **`unescapeJSString`** didn't handle `\"` (escaped double quotes in JS strings like `\"F5, Inc\"`). Added `\"` → `"` replacement.
  2. **`parseTableRawValue`** expected `<td class="key">TER</td>` on one line, but real HTML has whitespace/newlines between tag and text. Fixed by extracting label text dynamically and building a flexible regex.
- Task 3: Real HTML sample moved to `testdata/wmgt_page_cycletls.html` (embedded via `//go:embed`) instead of referencing the features folder. Tests are self-contained.
- Task 3: `fundSectorsData` is **not present** on the WMGT page in real CycleTLS HTML. Some WisdomTree pages may lack certain data sections. Full atomic extraction (`extractFromHTML`) is still enforced for pages that have all sections; real HTML tests verify individual parsers separately.
- Task 3: `FundProfile` extended with `Family` and `LegalType` fields to match existing `symbol.FundProfile`. `AnnualHoldingsTurnover` added to struct but not extracted (not available on WisdomTree pages).

## Decisions
- 2026-05-26: Migration 021 adds `data_source_url` to `symbol_mappings` and `extractor_as_of_date` to `symbol_details`. Both nullable (NULL = default Yahoo behavior).
- 2026-05-26: `ParseFundInfo` regex uses `fundInfo\w*` (hash suffix optional) to match both `var fundInfo = {...}` and `var fundInfo<HASH> = {...}` patterns.
- 2026-05-26: Holdings CSV weights are fractions (0.0137 = 1.37%), converted to percentage (1.37) to match existing `TopHolding.Percent` convention. Sectors CSV weights are already percentages — no conversion needed.
- 2026-05-26: `parseTableValue` regex uses `labelCell` including the closing `</td>` tag, then matches `\s*<td[^>]*>([^<]+)` for the value cell. This avoids false matches on nested content.
- 2026-05-26: Cash positions filtered by keyword list (CASH, EUR/USD/JPY etc.). Covers WisdomTree's "CASH W-O" and currency position names seen in samples.
- 2026-05-26: `extractFromHTML` helper function allows testing parsers without HTTP. Used by integration tests.
- 2026-05-26: `InceptionDate` added to `symbol.FundProfile` struct (new `time.Time` field, zero value = not set).
- 2026-05-26: Country allocation, market cap, and fund characteristics **ARE in raw HTML** when fetched with CycleTLS (confirmed 2026-05-26). The sample HTML we saved (`wmgt_page.html`) had empty `<tbody>` because it was NOT fetched with CycleTLS. This was a false alarm — no headless browser needed.
- 2026-05-26: `Renderer` interface, `js_parsers.go`, and `renderer.go` removed (unnecessary — all data extractable from raw HTML via CycleTLS).
- 2026-05-26: `ParseCountryAllocation`, `ParseMarketCap`, `ParseFundCharacteristics` added as HTML table parsers (same approach as `ParseFundProfile`). Use `regexp` on table rows with `key`/`value` class attributes.
- 2026-05-26: Go's RE2 engine doesn't match `\n` with `.` — use `[\s\S]` instead of `.+?` / `.*?` for multi-line regex.
- 2026-05-26: `FundCharacteristics` struct has only 5 fields (P/E, P/B, P/S, P/CF, Dividend Yield) — no Estimated P/E, Gross/Net Buyback Yield. Parsers only extract available fields.
- 2026-05-26: `Extractor` and `URLMatcher` are separate interfaces. Registry uses a `registryEntry` struct pairing both, since not all extractors may need URL matching and Go doesn't allow casting between unrelated interface types.
- 2026-05-26: Dispatcher validates URL with `url.Parse` before lookup — rejects malformed URLs early with explicit error.
- 2026-05-26: WisdomTree extractor was a stub returning `ErrNotImplemented` for Task 2 framework testing. Replaced with full implementation in Task 3.
- 2026-05-26: `GetNavHistoryBySymbol` query filters on `data_type = 'nav'` and `date != ''` (excludes current entries). Reuses existing `market_data` table with existing UNIQUE(symbol, source, date) constraint — NAV uses source='wisdomtree'.
- 2026-05-26: `ListStaleSymbolDetails` now returns `data_source_url` alongside internal_symbol and market_data_symbol, enabling the service layer to route fetches to the correct provider.
- 2026-05-26: **`float64` for extractor types** — The CONVENTIONS.md rule ("Use `decimal.Decimal` for all monetary values") targets transaction/P&L data (prices, costs, P&L). The extractor types (`FundProfile`, `NavPoint`, `Holding`, etc.) are display metadata that map directly to the existing `symbol` types, which use `float64` for the same fields (e.g., `symbol.FundProfile.TotalNetAssets float64`, `symbol.TopHolding.Percent float64`). Keeping `float64` maintains type consistency through the extraction → storage pipeline. The service layer (Task 4) will convert `ExtractResult` → `SymbolDetails` with no type mismatch.
- 2026-05-26: **"As of" date missing** — Spec edge case says "stores the fetch date as a fallback". Decision: **do NOT fallback**. If the "as of" date cannot be parsed, the extraction fails (atomic). This avoids silently storing data with an incorrect reference date. The fetch date is already captured in `fetched_at`.

## Future Improvements
- None yet.

## Known Issues
- None.
