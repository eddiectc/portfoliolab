package position

import (
	"context"
	"fmt"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// RefreshMarketData refreshes current market data (prices and FX rates) for
// the symbols and currencies relevant to the given performance filters.
// It fetches current quotes for all symbols held during the period and
// current FX rates for all currency pairs needed.
//
// Symbols are collected from:
//   - Transactions (buy/sell types) within the filtered date range
//   - Open positions across the resolved accounts
//
// FX rates are refreshed for every unique transaction/position currency
// that differs from the portfolio base currency.
func (s *Service) RefreshMarketData(ctx context.Context, filters PerformanceFilters) (*RefreshResult, error) {
	// 1. Resolve accounts from filters.
	accounts, err := s.resolveAccountsForPerformance(ctx, filters)
	if err != nil {
		return nil, err
	}

	// 2. Determine base currency.
	baseCurrency, err := determineBaseCurrency(accounts)
	if err != nil {
		return nil, err
	}

	// 3. Determine date range from period filter.
	dateFrom, dateTo := determineDateRange(filters)

	// 4. Get account IDs.
	accountIDs := make([]int64, len(accounts))
	for i, a := range accounts {
		accountIDs[i] = a.ID
	}

	// 5. Fetch all transactions and collect unique symbols.
	var allTxns []transaction.Transaction
	for _, id := range accountIDs {
		txns, fetchErr := s.transactions.ListAllTransactionsByAccount(ctx, id)
		if fetchErr != nil {
			return nil, fmt.Errorf("list transactions for account %d: %w", id, fetchErr)
		}
		allTxns = append(allTxns, txns...)
	}
	allTxns = filterByDateRange(allTxns, dateFrom, dateTo)

	symbols := collectUniqueSymbols(allTxns)

	// 6. Also collect symbols from open positions.
	for _, id := range accountIDs {
		positions, fetchErr := s.positions.GetOpenPositions(ctx, id, 10000, 0)
		if fetchErr != nil {
			return nil, fmt.Errorf("get open positions for account %d: %w", id, fetchErr)
		}
		for _, p := range positions {
			if !isCashPosition(p.Symbol) {
				symbols = append(symbols, p.Symbol)
			}
		}
	}

	// Deduplicate symbols.
	symbols = deduplicateStrings(symbols)

	// 7. Fetch current quotes for all symbols.
	var symbolsRefreshed []string
	var failedSymbols []string

	if len(symbols) > 0 && s.marketFetcher != nil {
		quotes := s.marketFetcher.FetchQuotesBatch(ctx, symbols)
		for _, sym := range symbols {
			if quote, found := quotes[sym]; found {
				if s.marketDataRepo != nil {
					if cacheErr := s.marketDataRepo.Upsert(ctx, quote); cacheErr != nil {
						if s.logger != nil {
							s.logger.Debug("failed to cache quote", "symbol", sym, "error", cacheErr)
						}
					}
				}
				symbolsRefreshed = append(symbolsRefreshed, sym)
			} else {
				failedSymbols = append(failedSymbols, sym)
			}
		}
	}

	// 8. Determine currencies that need FX conversion.
	currencies := collectUniqueCurrencies(allTxns)
	var fxPairsRefreshed []string

	for _, currency := range currencies {
		if currency == baseCurrency || currency == "" {
			continue
		}
		if s.fxProvider != nil {
			if _, found := s.fxProvider.GetCurrentRate(ctx, currency, baseCurrency); found {
				fxPairsRefreshed = append(fxPairsRefreshed, market.FormatFxPair(currency, baseCurrency))
			}
		}
	}

	return &RefreshResult{
		SymbolsRefreshed: symbolsRefreshed,
		FxPairsRefreshed: fxPairsRefreshed,
		FailedSymbols:    failedSymbols,
	}, nil
}

// collectUniqueCurrencies collects all unique non-empty currencies from
// transactions.
func collectUniqueCurrencies(txns []transaction.Transaction) []string {
	set := make(map[string]bool)
	for _, txn := range txns {
		if txn.Currency != "" {
			set[txn.Currency] = true
		}
	}
	currencies := make([]string, 0, len(set))
	for currency := range set {
		currencies = append(currencies, currency)
	}
	return currencies
}

// deduplicateStrings removes duplicate strings from a slice, preserving
// first-seen order.
func deduplicateStrings(items []string) []string {
	set := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !set[item] {
			set[item] = true
			result = append(result, item)
		}
	}
	return result
}
