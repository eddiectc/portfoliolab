-- name: InsertSymbolDetails :one
INSERT INTO symbol_details (
    internal_symbol, short_name, long_name, exchange, currency, quote_type,
    top_holdings, sector_weightings, aggregate_positions, fund_profile, equity_valuation,
    geographic_allocations, fetched_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(internal_symbol) DO UPDATE SET
    short_name = excluded.short_name,
    long_name = excluded.long_name,
    exchange = excluded.exchange,
    currency = excluded.currency,
    quote_type = excluded.quote_type,
    top_holdings = excluded.top_holdings,
    sector_weightings = excluded.sector_weightings,
    aggregate_positions = excluded.aggregate_positions,
    fund_profile = excluded.fund_profile,
    equity_valuation = excluded.equity_valuation,
    geographic_allocations = excluded.geographic_allocations,
    fetched_at = excluded.fetched_at,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetSymbolDetailsByInternalSymbol :one
SELECT * FROM symbol_details WHERE internal_symbol = ?;

-- name: ListStaleSymbolDetails :many
SELECT sm.internal_symbol, sm.market_data_symbol, sd.fetched_at
FROM symbol_mappings sm
LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
ORDER BY sd.fetched_at ASC;
