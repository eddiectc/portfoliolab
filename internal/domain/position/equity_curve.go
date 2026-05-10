package position

import (
	"context"
	"fmt"
	"sort"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// dateSnapshot captures the portfolio state at a specific date during
// the equity curve walk.
type dateSnapshot struct {
	date             time.Time
	positions        map[string]decimal.Decimal  // symbol -> quantity
	positionCurrency map[string]string           // symbol -> currency
	cashBalance      map[string]decimal.Decimal  // currency -> balance
	netDeposit       map[string]decimal.Decimal  // currency -> cumulative net deposit
}

// ComputeEquityCurve computes the portfolio equity curve for the given filters.
// It walks transactions chronologically, tracks positions and cash balances,
// fetches historical prices, and produces equity curve data points with
// daily interpolation.
//
// Returns a PerformanceResult containing the equity curve, return metrics
// placeholder, base currency, and any warnings (e.g., missing market data).
func (s *Service) ComputeEquityCurve(ctx context.Context, filters PerformanceFilters) (*PerformanceResult, error) {
	// 1. Resolve accounts from filters.
	accounts, err := s.resolveAccountsForPerformance(ctx, filters)
	if err != nil {
		return nil, err
	}

	// 2. Determine base currency (check consistency for all-portfolios mode).
	baseCurrency, err := determineBaseCurrency(accounts)
	if err != nil {
		return nil, err
	}

	// 3. Determine date range from period filter.
	dateFrom, dateTo := determineDateRange(filters)

	// 4. Fetch all transactions for resolved accounts.
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

	// 5. Filter to date range and sort by date ASC, then ID ASC.
	allTxns = filterByDateRange(allTxns, dateFrom, dateTo)
	sort.SliceStable(allTxns, func(i, j int) bool {
		if !allTxns[i].Date.Equal(allTxns[j].Date) {
			return allTxns[i].Date.Before(allTxns[j].Date)
		}
		return allTxns[i].ID < allTxns[j].ID
	})

	// 6. Handle empty state.
	if len(allTxns) == 0 {
		return &PerformanceResult{
			EquityCurve:   []EquityCurvePoint{},
			ReturnMetrics: ReturnMetrics{HasInsufficientData: true},
			BaseCurrency:  baseCurrency,
		}, nil
	}

	// 7. Walk transactions chronologically, capturing state at each date.
	snapshots := walkTransactions(allTxns)

	// 8. Collect unique non-cash symbols and read historical prices from cache.
	symbols := collectUniqueSymbols(allTxns)
	var warnings []string
	var pricesBySymbol map[string][]market.HistoricalPrice
	if len(symbols) > 0 && s.marketDataRepo != nil {
		// Read cached historical prices for each symbol.
		pricesBySymbol = make(map[string][]market.HistoricalPrice)
		var missingSymbols []string
		for _, sym := range symbols {
			prices, err := s.marketDataRepo.GetHistoricalPricesBySymbol(ctx, sym, dateFrom, dateTo)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to read cached prices", "symbol", sym, "error", err)
				}
				missingSymbols = append(missingSymbols, sym)
				continue
			}
			if len(prices) == 0 {
				missingSymbols = append(missingSymbols, sym)
				continue
			}
			pricesBySymbol[sym] = prices
		}
		for _, sym := range missingSymbols {
			warnings = append(warnings, fmt.Sprintf("missing market data for %s", sym))
		}

		// Check staleness for symbols that have cached data.
		if len(pricesBySymbol) > 0 {
			latestDates := s.marketDataRepo.GetLatestPriceDatePerSymbol(ctx, symbols)
			now := time.Now().UTC()
			for sym := range pricesBySymbol {
				latestDate, ok := latestDates[sym]
				if !ok || latestDate == nil {
					continue
				}
				// If the latest cached date is more than 1 calendar day old, warn.
				if latestDate.Before(now.AddDate(0, 0, -1)) {
					daysAgo := now.Sub(*latestDate).Hours() / 24
					warnings = append(warnings, fmt.Sprintf("stale market data for %s (last updated %.0f days ago)", sym, daysAgo))
				}
			}
		}
	}

	// 9. Build equity curve points from snapshots.
	points := buildEquityCurvePoints(snapshots, pricesBySymbol, baseCurrency, s.fxProvider, ctx)

	// 10. Interpolate for non-transaction days.
	points = interpolateDaily(points)

	returnMetrics := ComputeReturnMetrics(points, baseCurrency)

	return &PerformanceResult{
		EquityCurve:   points,
		ReturnMetrics: returnMetrics,
		BaseCurrency:  baseCurrency,
		Warnings:      warnings,
	}, nil
}

// resolveAccountsForPerformance resolves account IDs from performance filters.
// If PortfolioID is set, returns accounts for that portfolio.
// Otherwise returns all accounts.
func (s *Service) resolveAccountsForPerformance(ctx context.Context, filters PerformanceFilters) ([]AccountRef, error) {
	if filters.PortfolioID != nil {
		return s.accountLister.GetAccountsByPortfolio(ctx, *filters.PortfolioID)
	}
	return s.accountLister.GetAllAccounts(ctx)
}

// determineBaseCurrency determines the base currency from the resolved accounts.
// Returns an error if accounts span multiple portfolios with different currencies.
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
// "All" or empty period returns zero time for dateFrom (no lower bound).
func determineDateRange(filters PerformanceFilters) (time.Time, time.Time) {
	now := time.Now().UTC()

	// Explicit date overrides take precedence.
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

	// Parse period string.
	period := filters.Period
	if period == "" || period == "All" {
		return time.Time{}, dateTo // zero time = no lower bound
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
		dateFrom = time.Time{} // unknown period = no lower bound
	}

	return dateFrom, dateTo
}

// filterByDateRange filters transactions to the given date range.
// Zero dateFrom means no lower bound; zero dateTo means no upper bound.
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

// walkTransactions walks transactions chronologically and captures the
// portfolio state (positions, cash, net deposit) at each unique date.
// Transactions are assumed to be sorted by date ASC, then ID ASC.
func walkTransactions(txns []transaction.Transaction) []dateSnapshot {
	var snapshots []dateSnapshot

	positions := make(map[string]decimal.Decimal)
	positionCurrency := make(map[string]string)
	cashBalance := make(map[string]decimal.Decimal)
	netDeposit := make(map[string]decimal.Decimal)

	for i, txn := range txns {
		// Update position quantities for buy/sell.
		if txn.Type == "buy" {
			qty, _ := positions[txn.Symbol].Add(txn.Quantity)
			positions[txn.Symbol] = qty
			positionCurrency[txn.Symbol] = txn.Currency
		} else if txn.Type == "sell" {
			negQty := txn.Quantity.Neg()
			qty, _ := positions[txn.Symbol].Add(negQty)
			positions[txn.Symbol] = qty
			positionCurrency[txn.Symbol] = txn.Currency
		}

		// Update cash balance for all transaction types (net_cash captures cash flow).
		bal, _ := cashBalance[txn.Currency].Add(txn.NetCash)
		cashBalance[txn.Currency] = bal

		// Update cumulative net deposit for deposit/withdrawal types only.
		if txn.Type == "deposit" || txn.Type == "withdrawal" {
			dep, _ := netDeposit[txn.Currency].Add(txn.NetCash)
			netDeposit[txn.Currency] = dep
		}

		// Take snapshot at end of each date.
		isLastForDate := i == len(txns)-1 || txns[i+1].Date.After(txn.Date)
		if isLastForDate {
			snapshots = append(snapshots, dateSnapshot{
				date:             txn.Date,
				positions:        copyDecimalMap(positions),
				positionCurrency: copyStringMap(positionCurrency),
				cashBalance:      copyDecimalMap(cashBalance),
				netDeposit:       copyDecimalMap(netDeposit),
			})
		}
	}

	return snapshots
}

// collectUniqueSymbols collects all unique non-cash symbols from transactions
// that represent position trades (buy/sell).
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

// buildPriceLookup builds a nested map: symbol -> date string -> HistoricalPrice
// for O(1) price lookups during equity curve computation.
func buildPriceLookup(pricesBySymbol map[string][]market.HistoricalPrice) map[string]map[string]market.HistoricalPrice {
	lookup := make(map[string]map[string]market.HistoricalPrice)
	for symbol, prices := range pricesBySymbol {
		lookup[symbol] = make(map[string]market.HistoricalPrice, len(prices))
		for _, p := range prices {
			key := p.Date.Format("2006-01-02")
			lookup[symbol][key] = p
		}
	}
	return lookup
}

// buildEquityCurvePoints computes portfolio value and net deposit for each
// snapshot date, converting all values to the base currency.
func buildEquityCurvePoints(
	snapshots []dateSnapshot,
	pricesBySymbol map[string][]market.HistoricalPrice,
	baseCurrency string,
	fxProvider FxRateProvider,
	ctx context.Context,
) []EquityCurvePoint {
	priceLookup := buildPriceLookup(pricesBySymbol)

	var points []EquityCurvePoint
	for _, snap := range snapshots {
		// Compute portfolio value: positions + cash.
		var portfolioValue decimal.Decimal

		// Position values.
		for symbol, qty := range snap.positions {
			dateKey := snap.date.Format("2006-01-02")
			price, found := lookupPrice(priceLookup, symbol, dateKey)
			if !found {
				continue // no price available, skip this position
			}

			value, _ := qty.Mul(price.Close)
			// Convert from price currency to base currency.
			if price.Currency != baseCurrency {
				value, _ = convertToBase(ctx, fxProvider, price.Currency, baseCurrency, value, snap.date)
			}
			portfolioValue, _ = portfolioValue.Add(value)
		}

		// Cash balances.
		for currency, balance := range snap.cashBalance {
			if currency != baseCurrency {
				balance, _ = convertToBase(ctx, fxProvider, currency, baseCurrency, balance, snap.date)
			}
			portfolioValue, _ = portfolioValue.Add(balance)
		}

		// Net deposit in base currency.
		var netDepositBase decimal.Decimal
		for currency, deposit := range snap.netDeposit {
			if currency != baseCurrency {
				deposit, _ = convertToBase(ctx, fxProvider, currency, baseCurrency, deposit, snap.date)
			}
			netDepositBase, _ = netDepositBase.Add(deposit)
		}

		points = append(points, EquityCurvePoint{
			Date:           snap.date,
			PortfolioValue: portfolioValue,
			NetDeposit:     netDepositBase,
		})
	}

	return points
}

// lookupPrice retrieves a historical price for a symbol on a specific date.
func lookupPrice(lookup map[string]map[string]market.HistoricalPrice, symbol, dateKey string) (market.HistoricalPrice, bool) {
	if dates, ok := lookup[symbol]; ok {
		if price, ok := dates[dateKey]; ok {
			return price, true
		}
	}
	return market.HistoricalPrice{}, false
}

// convertToBase converts a value from one currency to another using the
// FX rate provider. If currencies match, returns the value unchanged.
// If no FX rate is available, returns the original value with found=false.
func convertToBase(
	ctx context.Context,
	fxProvider FxRateProvider,
	fromCurrency, toCurrency string,
	value decimal.Decimal,
	date time.Time,
) (decimal.Decimal, bool) {
	if fromCurrency == toCurrency {
		return value, true
	}

	if fxProvider == nil {
		return value, false
	}

	rate, found := fxProvider.GetRateForDate(ctx, fromCurrency, toCurrency, date)
	if !found || rate == nil {
		return value, false
	}

	converted, _ := value.Mul(rate.Rate)
	return converted, true
}

// interpolateDaily fills in non-transaction days by carrying forward the
// last known portfolio value and net deposit. Generates one point per
// calendar day between the first and last snapshot date.
func interpolateDaily(points []EquityCurvePoint) []EquityCurvePoint {
	if len(points) == 0 {
		return points
	}

	// Build a map of date -> point for quick lookup.
	pointMap := make(map[string]EquityCurvePoint, len(points))
	for _, p := range points {
		key := p.Date.Format("2006-01-02")
		pointMap[key] = p
	}

	dateFrom := points[0].Date
	dateTo := points[len(points)-1].Date

	var result []EquityCurvePoint
	var lastPoint *EquityCurvePoint

	for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if p, ok := pointMap[key]; ok {
			lastPoint = &p
			result = append(result, p)
		} else if lastPoint != nil {
			// Carry forward last known value.
			result = append(result, EquityCurvePoint{
				Date:           d,
				PortfolioValue: lastPoint.PortfolioValue,
				NetDeposit:     lastPoint.NetDeposit,
			})
		}
	}

	return result
}

// copyDecimalMap creates a deep copy of a decimal map.
func copyDecimalMap(src map[string]decimal.Decimal) map[string]decimal.Decimal {
	dst := make(map[string]decimal.Decimal, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// copyStringMap creates a copy of a string map.
func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
