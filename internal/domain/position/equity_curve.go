package position

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/performance"
	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// ComputeEquityCurve computes the portfolio equity curve for the given filters.
// It walks transactions chronologically, tracks positions and cash balances,
// fetches historical prices, and produces equity curve data points with
// daily interpolation.
//
// Returns a PerformanceResult containing the equity curve, return metrics,
// base currency, and any warnings (e.g., missing market data).
func (s *Service) ComputeEquityCurve(ctx context.Context, filters performance.PerformanceFilters) (*performance.PerformanceResult, error) {
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

	// 3. Fetch ALL transactions for resolved accounts.
	accountIDs := make([]int64, len(accounts))
	for i, a := range accounts {
		accountIDs[i] = a.ID
	}

	var allTxns []transaction.Transaction
	for _, id := range accountIDs {
		txns, err := s.transactions.ListAllTransactionsByAccount(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("list transactions for account %d: %w", id, err)
		}
		allTxns = append(allTxns, txns...)
	}

	// Handle empty state.
	if len(allTxns) == 0 {
		return &performance.PerformanceResult{
			EquityCurve:   []performance.EquityCurvePoint{},
			ReturnMetrics: performance.ReturnMetrics{HasInsufficientData: true},
			BaseCurrency:  baseCurrency,
			NavSummary:    nil,
		}, nil
	}

	// Sort by date ASC, then ID ASC.
	sort.SliceStable(allTxns, func(i, j int) bool {
		if !allTxns[i].Date.Equal(allTxns[j].Date) {
			return allTxns[i].Date.Before(allTxns[j].Date)
		}
		return allTxns[i].ID < allTxns[j].ID
	})

	// 4. Determine date range from period filter.
	dateFrom, dateTo := determineDateRange(filters)

	if s.logger != nil {
		s.logger.Debug("performance: computing equity curve",
			"accounts", len(accountIDs),
			"baseCurrency", baseCurrency,
			"dateFrom", dateFrom.Format("2006-01-02"),
			"dateTo", dateTo.Format("2006-01-02"),
			"txns", len(allTxns),
		)
	}

	// 5. Collect unique symbols and fetch historical prices.
	priceFrom := allTxns[0].Date
	symbols := collectUniqueSymbols(allTxns)
	var pricesBySymbol map[string][]market.HistoricalPrice
	if len(symbols) > 0 && s.marketService != nil {
		pricesBySymbol = make(map[string][]market.HistoricalPrice)
		for _, sym := range symbols {
			prices, err := s.marketService.GetHistoricalPrices(ctx, sym, priceFrom, dateTo)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to read cached prices", "symbol", sym, "error", err)
				}
				continue
			}
			if len(prices) == 0 {
				if s.logger != nil {
					s.logger.Debug("no cached prices found", "symbol", sym)
				}
				continue
			}
			pricesBySymbol[sym] = prices
		}
	}

	// 6. Delegate to performance package for computation.
	result, err := performance.ComputeEquityCurve(
		ctx, allTxns, pricesBySymbol, baseCurrency,
		dateFrom, dateTo, s, s.logger,
	)
	if err != nil {
		return nil, err
	}

	// 7. Compute profit breakdown so the numbers reconcile:
	//    profit_loss = unrealized + realized + dividends + interest - fees - taxes
	listFilters := ListFilters{AccountIDs: &accountIDs}

	// Position P&L from summaries.
	openSummary, errOpen := s.GetOpenPositionsSummary(ctx, listFilters, baseCurrency)
	closedSummary, errClosed := s.GetClosedPositionsSummary(ctx, listFilters, baseCurrency)
	if errOpen == nil {
		result.ReturnMetrics.UnrealizedPnL = &openSummary.TotalUnrealizedPnLB
	}
	if errClosed == nil {
		result.ReturnMetrics.RealizedPnL = &closedSummary.TotalRealizedPnLB
	}

	// Aggregate dividends, interest, fees, taxes from transactions.
	result.ReturnMetrics.Dividends, result.ReturnMetrics.Interest,
		result.ReturnMetrics.Fees, result.ReturnMetrics.Taxes = aggregateCashFlowTransactions(allTxns, baseCurrency, s)

	return result, nil
}

// aggregateCashFlowTransactions sums dividends, interest, fees, and taxes from
// all transactions, converting foreign currency amounts to base currency.
// Returns (dividends, interest, fees, taxes) — fees and taxes are negative values.
func aggregateCashFlowTransactions(txns []transaction.Transaction, baseCurrency string, svc *Service) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal, *decimal.Decimal) {
	var dividends, interest, fees, taxes decimal.Decimal

	// Collect unique foreign currencies for FX lookup.
	fxCurrencies := make(map[string]bool)
	for _, t := range txns {
		if t.Currency != baseCurrency && t.Currency != "" {
			fxCurrencies[t.Currency] = true
		}
	}

	// Fetch spot FX rates for foreign currencies.
	fxRates := make(map[string]decimal.Decimal)
	if svc.marketService != nil {
		ctx := context.Background()
		for cur := range fxCurrencies {
			rate, err := svc.marketService.GetCurrentFxRate(ctx, cur, baseCurrency)
			if err == nil && rate != nil {
				fxRates[cur] = rate.Rate
			}
		}
	}

	for _, t := range txns {
		var amount decimal.Decimal
		switch t.Type {
		case "dividend":
			amount = t.NetCash
		case "interest":
			amount = t.NetCash
		case "fee":
			amount = t.NetCash // already negative
		case "tax":
			amount = t.NetCash // already negative
		default:
			continue
		}

		// Convert to base currency if needed.
		if t.Currency != baseCurrency {
			if rate, ok := fxRates[t.Currency]; ok {
				amount, _ = amount.Mul(rate)
			}
			// If no FX rate available, skip (don't silently drop — but don't fail either)
		}

		switch t.Type {
		case "dividend":
			dividends, _ = dividends.Add(amount)
		case "interest":
			interest, _ = interest.Add(amount)
		case "fee":
			fees, _ = fees.Add(amount)
		case "tax":
			taxes, _ = taxes.Add(amount)
		}
	}

	return &dividends, &interest, &fees, &taxes
}

// resolveAccountsForPerformance resolves account IDs from performance filters.
func (s *Service) resolveAccountsForPerformance(ctx context.Context, filters performance.PerformanceFilters) ([]AccountRef, error) {
	if filters.PortfolioID != nil {
		return s.accountLister.GetAccountsByPortfolio(ctx, *filters.PortfolioID)
	}
	return s.accountLister.GetAllAccounts(ctx)
}

// determineBaseCurrency determines the base currency from the resolved accounts.
func determineBaseCurrency(accounts []AccountRef) (string, error) {
	if len(accounts) == 0 {
		return "", nil
	}

	currencies := make(map[string]bool)
	for _, a := range accounts {
		currencies[a.PortfolioCurrency] = true
	}

	if len(currencies) > 1 {
		return "", &PositionError{
			Code:    "mismatched_currencies",
			Message: "cannot aggregate portfolios with different base currencies",
		}
	}

	for currency := range currencies {
		return currency, nil
	}
	return "", nil
}

// determineDateRange parses the period string into a date range.
func determineDateRange(filters performance.PerformanceFilters) (time.Time, time.Time) {
	now := time.Now().UTC()

	if filters.DateFrom != nil && filters.DateTo != nil {
		return *filters.DateFrom, *filters.DateTo
	}

	dateTo := now
	if filters.DateTo != nil {
		dateTo = *filters.DateTo
	}

	if filters.DateFrom != nil {
		return *filters.DateFrom, dateTo
	}

	period := filters.Period
	if period == "" || period == "All" {
		return time.Time{}, dateTo
	}

	var dateFrom time.Time
	switch period {
	case "1W":
		dateFrom = now.AddDate(0, 0, -7)
	case "1M":
		dateFrom = now.AddDate(0, -1, 0)
	case "3M":
		dateFrom = now.AddDate(0, -3, 0)
	case "1Y":
		dateFrom = now.AddDate(-1, 0, 0)
	case "3Y":
		dateFrom = now.AddDate(-3, 0, 0)
	case "5Y":
		dateFrom = now.AddDate(-5, 0, 0)
	case "YTD":
		dateFrom = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default:
		dateFrom = time.Time{}
	}

	return dateFrom, dateTo
}

// collectUniqueSymbols collects all unique non-cash symbols from transactions.
func collectUniqueSymbols(txns []transaction.Transaction) []string {
	set := make(map[string]bool)
	for _, txn := range txns {
		if (txn.Type == "buy" || txn.Type == "sell") && !isCashPosition(txn.Symbol) {
			set[txn.Symbol] = true
		}
	}
	symbols := make([]string, 0, len(set))
	for sym := range set {
		symbols = append(symbols, sym)
	}
	return symbols
}

// Ensure Service implements performance.MarketDataProvider.
var _ performance.MarketDataProvider = (*Service)(nil)

// GetHistoricalPrices implements performance.MarketDataProvider.
func (s *Service) GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error) {
	if s.marketService == nil {
		return nil, nil
	}
	return s.marketService.GetHistoricalPrices(ctx, symbol, start, end)
}

// GetLatestPriceDatePerSymbol implements performance.MarketDataProvider.
func (s *Service) GetLatestPriceDatePerSymbol(ctx context.Context, symbols []string) map[string]*time.Time {
	if s.marketService == nil {
		return nil
	}
	return s.marketService.GetLatestPriceDatePerSymbol(ctx, symbols)
}

// GetHistoricalFxRate implements performance.MarketDataProvider.
func (s *Service) GetHistoricalFxRate(ctx context.Context, baseCurrency, quoteCurrency string, date time.Time) (*market.FxRate, error) {
	if s.marketService == nil {
		return nil, nil
	}
	return s.marketService.GetHistoricalFxRate(ctx, baseCurrency, quoteCurrency, date)
}
