package position

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/performance"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// RefreshResult summarizes the outcome of a market data refresh.
type RefreshResult struct {
	SymbolsRefreshed  []string `json:"symbols_refreshed"`
	FxPairsRefreshed  []string `json:"fx_pairs_refreshed"`
	FailedSymbols     []string `json:"failed_symbols,omitempty"`
}

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
func (s *Service) RefreshMarketData(ctx context.Context, filters performance.PerformanceFilters) (*RefreshResult, error) {
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

	// 7. Refresh current quotes for all symbols via the market data service.
	var symbolsRefreshed []string
	var failedSymbols []string

	if len(symbols) > 0 && s.marketService != nil {
		result := s.marketService.RefreshQuotes(ctx, symbols)
		symbolsRefreshed = result.Refreshed
		failedSymbols = result.Failed
	}

	// 8. Determine currencies that need FX conversion.
	currencies := collectUniqueCurrencies(allTxns)
	var fxPairsRefreshed []string

	if s.marketService != nil {
		// Collect unique FX pairs to refresh.
		pairs := make([]marketservice.FxPair, 0, len(currencies))
		for _, currency := range currencies {
			if currency == baseCurrency || currency == "" {
				continue
			}
			pairs = append(pairs, marketservice.FxPair{
				BaseCurrency:  currency,
				QuoteCurrency: baseCurrency,
			})
		}
		if len(pairs) > 0 {
			result := s.marketService.RefreshFxRates(ctx, pairs)
			for _, pair := range result.Refreshed {
				fxPairsRefreshed = append(fxPairsRefreshed, market.FormatFxPair(pair.BaseCurrency, pair.QuoteCurrency))
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

// filterByDateRange filters transactions to the given date range.
func filterByDateRange(txns []transaction.Transaction, dateFrom, dateTo time.Time) []transaction.Transaction {
	var result []transaction.Transaction
	for _, txn := range txns {
		if !dateFrom.IsZero() && txn.Date.Before(dateFrom) {
			continue
		}
		if !dateTo.IsZero() && txn.Date.After(dateTo) {
			continue
		}
		result = append(result, txn)
	}
	return result
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
