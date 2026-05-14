package position

import (
	"context"
	"fmt"
	"log/slog"
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
	positions        map[string]decimal.Decimal // symbol -> quantity
	positionCurrency map[string]string          // symbol -> currency
	cashBalance      map[string]decimal.Decimal // currency -> balance
	netDeposit       map[string]decimal.Decimal // currency -> cumulative net deposit
	// preCashFlowSnapshots captures the portfolio state just before each
	// deposit/withdrawal on this date. Used for TWR computation.
	preCashFlowSnapshots []preCashFlowSnapshot
}

// preCashFlowSnapshot captures the portfolio state (positions + cash) just
// before a cash flow (deposit/withdrawal) is applied. Used to compute
// sub-period returns for the Time-Weighted Return (TWR).
type preCashFlowSnapshot struct {
	date             time.Time
	positions        map[string]decimal.Decimal
	positionCurrency map[string]string
	cashBalance      map[string]decimal.Decimal
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

	// 3. Fetch ALL transactions for resolved accounts (not filtered by period).
	// The period filter only slices the output curve and return metrics —
	// the portfolio state must include all historical buys/sells/deposits.
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

	// 4. Handle empty state.
	if len(allTxns) == 0 {
		return &PerformanceResult{
			EquityCurve:   []EquityCurvePoint{},
			ReturnMetrics: ReturnMetrics{HasInsufficientData: true},
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

	// 5. Determine date range from period filter (for slicing output only).
	dateFrom, dateTo := determineDateRange(filters)

	if s.logger != nil {
		s.logger.Debug("performance: computing equity curve", "accounts", len(accountIDs), "baseCurrency", baseCurrency, "dateFrom", dateFrom.Format("2006-01-02"), "dateTo", dateTo.Format("2006-01-02"), "txns", len(allTxns))
	}

	// 6. Walk ALL transactions chronologically, capturing state at each date.
	snapshots, finalState := walkTransactions(allTxns)

	if s.logger != nil {
		s.logger.Debug("performance: walk complete", "snapshots", len(snapshots))
	}

	// 7. Collect unique non-cash symbols and read historical prices from cache.
	// Fetch prices from the earliest transaction to dateTo so the full
	// portfolio state can be valued, even though the output curve will
	// be sliced to the period range.
	priceFrom := allTxns[0].Date // earliest transaction
	symbols := collectUniqueSymbols(allTxns)
	var warnings []string
	var pricesBySymbol map[string][]market.HistoricalPrice
	if len(symbols) > 0 && s.marketService != nil {
		if s.logger != nil {
			s.logger.Debug("reading cached historical prices", "symbols", len(symbols), "dateFrom", priceFrom.Format("2006-01-02"), "dateTo", dateTo.Format("2006-01-02"))
		}
		// Read cached historical prices for each symbol.
		pricesBySymbol = make(map[string][]market.HistoricalPrice)
		var missingSymbols []string
		for _, sym := range symbols {
			prices, err := s.marketService.GetHistoricalPrices(ctx, sym, priceFrom, dateTo)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to read cached prices", "symbol", sym, "error", err)
				}
				missingSymbols = append(missingSymbols, sym)
				continue
			}
			if len(prices) == 0 {
				if s.logger != nil {
					s.logger.Debug("no cached prices found", "symbol", sym, "dateFrom", priceFrom.Format("2006-01-02"), "dateTo", dateTo.Format("2006-01-02"))
				}
				missingSymbols = append(missingSymbols, sym)
				continue
			}
			if s.logger != nil {
				s.logger.Debug("cached prices loaded", "symbol", sym, "count", len(prices))
			}
			pricesBySymbol[sym] = prices
		}
		for _, sym := range missingSymbols {
			warnings = append(warnings, fmt.Sprintf("missing market data for %s", sym))
		}

		// Check staleness for symbols that have cached data.
		if len(pricesBySymbol) > 0 {
			latestDates := s.marketService.GetLatestPriceDatePerSymbol(ctx, symbols)
			now := time.Now().UTC()
			// Truncate to date-only (midnight) for fair comparison with DB dates
			// which are also stored as YYYY-MM-DD (midnight UTC).
			nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			expectedLatest := tradingDayBeforeOrOn(nowDate)
			for sym := range pricesBySymbol {
				latestDate, ok := latestDates[sym]
				if !ok || latestDate == nil {
					continue
				}
				if latestDate.Before(expectedLatest) {
					daysAgo := nowDate.Sub(*latestDate).Hours() / 24
					warnings = append(warnings, fmt.Sprintf("stale market data for %s (last updated %.0f days ago)", sym, daysAgo))
				}
			}
		}
	}

	// 9. Build equity curve points from snapshots.
	points := buildEquityCurvePoints(snapshots, pricesBySymbol, baseCurrency, s.marketService, s.logger, ctx)

	// 10. Compute pre-cash-flow breakpoints (needed for both NAV and TWR).
	// Computed here before interpolation so breakpoints are available for
	// NAV history computation, which operates on the full (pre-slice) curve.
	preCashFlowValues := computePreCashFlowValues(
		snapshots, pricesBySymbol, baseCurrency, s.marketService, s.logger, ctx,
	)

	// 11. Interpolate for non-transaction days, extending through dateTo.
	// Uses cached historical prices so the curve reflects actual price changes
	// after the last transaction, not just a flat carry-forward.
	lastSnapshot := finalState
	points = interpolateDaily(points, dateTo, lastSnapshot.positions, lastSnapshot.positionCurrency, lastSnapshot.cashBalance, pricesBySymbol, baseCurrency, s.marketService, ctx)

	if s.logger != nil {
		s.logger.Debug("performance: interpolation complete", "points", len(points))
	}

	// 12. Compute NAV history (unitization) on the full (pre-slice) curve.
	// This populates NavPerUnit and Units on each equity curve point.

	// Find the first deposit date to trigger unitization.
	var inceptionDate time.Time
	for _, txn := range allTxns {
		if txn.Type == "deposit" {
			inceptionDate = txn.Date
			break
		}
	}

	navBreakpoints := make([]navBreakpoint, len(preCashFlowValues))
	for i, bp := range preCashFlowValues {
		navBreakpoints[i] = navBreakpoint{date: bp.date, value: bp.value}
	}
	navHistory := ComputeNavHistory(points, navBreakpoints, inceptionDate)
	if navHistory != nil {
		for i := range points {
			nav := navHistory[i].NavPerUnit
			units := navHistory[i].Units
			points[i].NavPerUnit = &nav
			points[i].Units = &units
		}
	}

	// 13. Slice to period range. The portfolio state includes all history,
	// but the output curve only shows the selected period.
	if !dateFrom.IsZero() {
		points = sliceFrom(points, dateFrom)
	}

	// 14. Filter breakpoints to the visible period so that TWR reflects
	// only the selected window. Exclude breakpoints on or before the first
	// curve point — that point is the starting value, and any cash flow
	// on that date already happened before (or at) the window start.
	visibleBreakpoints := filterBreakpointsForPeriod(preCashFlowValues, points, dateTo)

	// 15. Compute return metrics (TWR + annualized) from the sliced curve
	// and filtered breakpoints.
	if s.logger != nil {
		s.logger.Debug("performance: TWR breakpoints",
			"count", len(visibleBreakpoints),
			"baseCurrency", baseCurrency,
		)
		for i, bp := range visibleBreakpoints {
			s.logger.Debug("performance: TWR breakpoint",
				"idx", i,
				"date", bp.date.Format("2006-01-02"),
				"value", bp.value.String(),
			)
		}
	}
	returnMetrics := ComputePeriodReturn(points, visibleBreakpoints, baseCurrency)

	if s.logger != nil {
		if len(points) > 0 {
			s.logger.Debug("performance: equity curve built",
				"points", len(points),
				"firstDate", points[0].Date.Format("2006-01-02"),
				"firstValue", points[0].PortfolioValue.String(),
				"firstNetDeposit", points[0].NetDeposit.String(),
				"lastDate", points[len(points)-1].Date.Format("2006-01-02"),
				"lastValue", points[len(points)-1].PortfolioValue.String(),
				"lastNetDeposit", points[len(points)-1].NetDeposit.String(),
			)
		}
	}

	if s.logger != nil {
		twrStr := "nil"
		if returnMetrics.TWRPct != nil {
			twrStr = returnMetrics.TWRPct.String()
		}
		annStr := "nil"
		if returnMetrics.AnnualizedTWRPct != nil {
			annStr = returnMetrics.AnnualizedTWRPct.String()
		}
		s.logger.Debug("performance: return metrics computed",
			"twr", twrStr,
			"annualizedTWR", annStr,
			"hasInsufficientData", returnMetrics.HasInsufficientData,
		)
	}

	// 16. Compute additional metrics from the sliced (period-filtered) curve.
	// Daily returns, risk metrics, drawdown, and yearly performance are
	// computed on the visible period so they align with the selected window.
	navPoints := convertToNavPoints(points)
	dailyReturns := ComputeDailyReturns(points)
	riskMetrics := ComputeRiskMetrics(dailyReturns, nil)
	drawdownAnalysis := ComputeDrawdownAnalysis(navPoints)
	yearlyPerformance := ComputeYearlyPerformance(navPoints)

	// 17. Build NavSummary from the final NAV state.
	var navSummary *NavSummary
	if navHistory != nil && len(navHistory) > 0 {
		lastNav := navHistory[len(navHistory)-1]
		navSummary = &NavSummary{
			NavPerUnit:    lastNav.NavPerUnit,
			TotalUnits:    lastNav.Units,
			TotalValue:    lastNav.PortfolioValue,
			InceptionDate: navHistory[0].Date,
		}
	}

	return &PerformanceResult{
		EquityCurve:       points,
		ReturnMetrics:     returnMetrics,
		BaseCurrency:      baseCurrency,
		Warnings:          warnings,
		NavSummary:        navSummary,
		RiskMetrics:       riskMetrics,
		DrawdownAnalysis:  drawdownAnalysis,
		YearlyPerformance: yearlyPerformance,
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

// sliceFrom returns only the equity curve points on or after the given date.
// Zero dateFrom returns the full curve unchanged.
func sliceFrom(points []EquityCurvePoint, dateFrom time.Time) []EquityCurvePoint {
	if dateFrom.IsZero() {
		return points
	}
	// Binary search for the first point on or after dateFrom.
	idx := sort.Search(len(points), func(i int) bool {
		return !points[i].Date.Before(dateFrom)
	})
	if idx >= len(points) {
		return []EquityCurvePoint{}
	}
	return points[idx:]
}

// filterBreakpointsForPeriod keeps only breakpoints that fall strictly after
// the first curve point and within the date range. Breakpoints on or before
// the first point are excluded because that point is the starting value for
// the period, and any cash flow on that date is already reflected in it.
func filterBreakpointsForPeriod(bps []twrBreakpoint, points []EquityCurvePoint, dateTo time.Time) []twrBreakpoint {
	if len(points) == 0 {
		return nil
	}
	firstDate := points[0].Date
	var result []twrBreakpoint
	for _, bp := range bps {
		// Exclude breakpoints on or before the first curve point.
		if !bp.date.After(firstDate) {
			continue
		}
		// Exclude breakpoints after the window end.
		if !dateTo.IsZero() && bp.date.After(dateTo) {
			continue
		}
		result = append(result, bp)
	}
	return result
}

// convertToNavPoints converts EquityCurvePoint slices to NavPoint slices
// for use with drawdown and yearly performance computation.
func convertToNavPoints(points []EquityCurvePoint) []NavPoint {
	result := make([]NavPoint, len(points))
	for i, p := range points {
		navPerUnit := decimal.Zero
		units := decimal.Zero
		if p.NavPerUnit != nil {
			navPerUnit = *p.NavPerUnit
		}
		if p.Units != nil {
			units = *p.Units
		}
		result[i] = NavPoint{
			Date:           p.Date,
			NavPerUnit:     navPerUnit,
			Units:          units,
			PortfolioValue: p.PortfolioValue,
		}
	}
	return result
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
// Also captures pre-cash-flow snapshots for TWR computation.
// Returns the final state (after all transactions) for extending
// the equity curve beyond the last transaction date.
//
// The core tracking (quantities, cash, net deposit) is delegated to
// WalkPortfolioState, the shared single source of truth. walkTransactions
// adds position currency tracking and pre-cash-flow snapshot capture on top.
// Transactions are assumed to be sorted by date ASC, then ID ASC.
func walkTransactions(txns []transaction.Transaction) ([]dateSnapshot, dateSnapshot) {
	// Shared portfolio state tracking — single source of truth.
	portfolioSnaps := WalkPortfolioState(txns)
	finalState := FinalPortfolioState(txns)

	// Walk transactions for position currency tracking and
	// pre-cash-flow snapshot capture (for TWR).
	var snapshots []dateSnapshot
	positionCurrency := make(map[string]string)
	// Running state for pre-cash-flow capture.
	quantities := make(map[string]decimal.Decimal)
	cashBalance := make(map[string]decimal.Decimal)
	// Collect pre-cash-flow snapshots inline, keyed by date.
	preCashFlowByDate := make(map[string][]preCashFlowSnapshot)

	snapIdx := 0
	for i, txn := range txns {
		if txn.Type == "buy" || txn.Type == "sell" {
			positionCurrency[txn.Symbol] = txn.Currency
		}

		// Capture pre-cash-flow snapshot BEFORE processing deposit/withdrawal.
		// Quantities and cash balance here reflect all prior transactions
		// (including buys/sells on the same date).
		if txn.Type == "deposit" || txn.Type == "withdrawal" {
			dateKey := txn.Date.Format(time.RFC3339)
			preCashFlowByDate[dateKey] = append(preCashFlowByDate[dateKey], preCashFlowSnapshot{
				date:             txn.Date,
				positions:        copyDecimalMap(quantities),
				positionCurrency: copyStringMap(positionCurrency),
				cashBalance:      copyDecimalMap(cashBalance),
			})
		}

		// Update running state.
		if txn.Type == "buy" || txn.Type == "sell" {
			qty, _ := quantities[txn.Symbol].Add(txn.Quantity)
			quantities[txn.Symbol] = qty
		}
		bal, _ := cashBalance[txn.Currency].Add(txn.NetCash)
		cashBalance[txn.Currency] = bal

		// Check if this is the last transaction for this date.
		// Must check the next transaction's date, NOT the next portfolio snapshot,
		// because pre-cash-flow snapshots for deposits/withdrawals on this date
		// may not have been captured yet (e.g., buy before deposit on same date).
		isLastForDate := i == len(txns)-1 || txns[i+1].Date.After(txn.Date)

		// Align with portfolio snapshots at end of each date.
		if isLastForDate && snapIdx < len(portfolioSnaps) && portfolioSnaps[snapIdx].Date.Equal(txn.Date) {
			dateKey := portfolioSnaps[snapIdx].Date.Format(time.RFC3339)
			snapshots = append(snapshots, dateSnapshot{
				date:                 portfolioSnaps[snapIdx].Date,
				positions:            portfolioSnaps[snapIdx].Quantities,
				positionCurrency:     copyStringMap(positionCurrency),
				cashBalance:          portfolioSnaps[snapIdx].CashBalance,
				netDeposit:           portfolioSnaps[snapIdx].NetDeposit,
				preCashFlowSnapshots: preCashFlowByDate[dateKey],
			})
			snapIdx++
		}
	}

	return snapshots, dateSnapshot{
		positions:        finalState.Quantities,
		positionCurrency: copyStringMap(positionCurrency),
		cashBalance:      finalState.CashBalance,
		netDeposit:       finalState.NetDeposit,
	}
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
	marketService MarketDataService,
	logger *slog.Logger,
	ctx context.Context,
) []EquityCurvePoint {
	priceLookup := buildPriceLookupFF(buildPriceLookup(pricesBySymbol))

	// Collect unique FX pairs needed: position currencies and cash currencies
	// that differ from base currency.
	fxPairs := collectFxPairs(snapshots, baseCurrency)

	// Fetch historical FX rates for the full date range and build forward-fill
	// lookup so weekends/holidays use the last known rate instead of dropping
	// to unconverted values.
	fxLookup := buildFxLookupFF(ctx, marketService, fxPairs, snapshots, logger)

	var points []EquityCurvePoint
	for _, snap := range snapshots {
		// Compute portfolio value: positions + cash.
		var portfolioValue decimal.Decimal
		var posValue decimal.Decimal

		// Position values.
		for symbol, qty := range snap.positions {
			dateKey := snap.date.Format("2006-01-02")
			price, found := lookupPrice(priceLookup, symbol, dateKey)
			if !found {
				if logger != nil {
					logger.Debug("performance: no price found for position", "symbol", symbol, "qty", qty.String(), "date", dateKey)
				}
				continue // no price available, skip this position
			}

			value, _ := qty.Mul(price.Close)
			// Convert from price currency to base currency.
			if price.Currency != baseCurrency {
				var ok bool
				value, ok = convertWithFxLookup(fxLookup, price.Currency, baseCurrency, value, snap.date)
				if !ok {
					if logger != nil {
						logger.Warn("missing FX rate, skipping position value",
							"pair", market.FormatFxPair(price.Currency, baseCurrency),
							"symbol", symbol, "date", dateKey,
						)
					}
					continue // skip — can't include unconverted value
				}
			}
			posValue, _ = posValue.Add(value)
			portfolioValue, _ = portfolioValue.Add(value)
		}

		// Cash balances.
		var cashValue decimal.Decimal
		for currency, balance := range snap.cashBalance {
			if currency != baseCurrency {
				var ok bool
				balance, ok = convertWithFxLookup(fxLookup, currency, baseCurrency, balance, snap.date)
				if !ok {
					if logger != nil {
						logger.Warn("missing FX rate, skipping cash balance",
							"pair", market.FormatFxPair(currency, baseCurrency),
							"date", snap.date.Format("2006-01-02"),
						)
					}
					continue // skip — can't include unconverted value
				}
			}
			cashValue, _ = cashValue.Add(balance)
			portfolioValue, _ = portfolioValue.Add(balance)
		}

		// Net deposit in base currency.
		var netDepositBase decimal.Decimal
		for currency, deposit := range snap.netDeposit {
			if currency != baseCurrency {
				var ok bool
				deposit, ok = convertWithFxLookup(fxLookup, currency, baseCurrency, deposit, snap.date)
				if !ok {
					if logger != nil {
						logger.Warn("missing FX rate, skipping net deposit",
							"pair", market.FormatFxPair(currency, baseCurrency),
							"date", snap.date.Format("2006-01-02"),
						)
					}
					continue // skip — can't include unconverted value
				}
			}
			netDepositBase, _ = netDepositBase.Add(deposit)
		}

		if logger != nil {
			logger.Debug("performance: snapshot",
				"date", snap.date.Format("2006-01-02"),
				"posValue", posValue.String(),
				"cashValue", cashValue.String(),
				"portfolioValue", portfolioValue.String(),
				"netDeposit", netDepositBase.String(),
			)
		}

		points = append(points, EquityCurvePoint{
			Date:           snap.date,
			PortfolioValue: portfolioValue,
			NetDeposit:     netDepositBase,
		})
	}

	return points
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

// tradingDayBeforeOrOn returns the most recent trading day on or before the
// given date, skipping Saturday (6) and Sunday (0). Used for staleness checks
// so weekend gaps don't trigger false "stale data" warnings.
// Does not account for market holidays — those are rare enough to be a minor
// false positive (one extra day of "staleness").
func tradingDayBeforeOrOn(t time.Time) time.Time {
	d := t
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}
