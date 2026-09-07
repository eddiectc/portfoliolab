package performance

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// MarketDataProvider abstracts market data retrieval for historical prices
// and FX rates. Defined here so the performance package doesn't depend on
// the position package's MarketDataService.
type MarketDataProvider interface {
	GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
	GetLatestPriceDatePerSymbol(ctx context.Context, symbols []string) map[string]*time.Time
	GetHistoricalFxRate(ctx context.Context, baseCurrency, quoteCurrency string, date time.Time) (*market.FxRate, error)
}

// dateSnapshot captures the portfolio state at a specific date during
// the equity curve walk.
type dateSnapshot struct {
	date                 time.Time
	positions            map[string]decimal.Decimal
	positionCurrency     map[string]string
	cashBalance          map[string]decimal.Decimal
	netDeposit           map[string]decimal.Decimal
	preCashFlowSnapshots []preCashFlowSnapshot
}

// preCashFlowSnapshot captures the portfolio state just before a cash flow.
type preCashFlowSnapshot struct {
	date             time.Time
	positions        map[string]decimal.Decimal
	positionCurrency map[string]string
	cashBalance      map[string]decimal.Decimal
}

// ComputeEquityCurve computes the portfolio equity curve from pre-fetched data.
// It is a pure computation function with no repository dependencies.
//
// txns must be sorted by date ASC, then ID ASC.
// pricesBySymbol maps symbol -> []HistoricalPrice for the full date range.
// baseCurrency is the currency all values are expressed in.
// dateFrom/dateTo define the output period (state includes all history).
func ComputeEquityCurve(
	ctx context.Context,
	txns []transaction.Transaction,
	pricesBySymbol map[string][]market.HistoricalPrice,
	baseCurrency string,
	dateFrom, dateTo time.Time,
	marketProvider MarketDataProvider,
	logger *slog.Logger,
) (*PerformanceResult, error) {
	if len(txns) == 0 {
		return &PerformanceResult{
			EquityCurve:   []EquityCurvePoint{},
			ReturnMetrics: ReturnMetrics{HasInsufficientData: true},
			BaseCurrency:  baseCurrency,
			NavSummary:    nil,
		}, nil
	}

	if logger != nil {
		logger.Debug("performance: computing equity curve",
			"baseCurrency", baseCurrency,
			"dateFrom", dateFrom.Format("2006-01-02"),
			"dateTo", dateTo.Format("2006-01-02"),
			"txns", len(txns),
		)
	}

	// Walk transactions chronologically, capturing state at each date.
	snapshots, _ := walkTxns(txns)

	if logger != nil {
		logger.Debug("performance: walk complete", "snapshots", len(snapshots))
	}

	// Collect unique non-cash symbols.
	symbols := CollectSymbols(txns)
	var warnings []string
	// Check for symbols with no cached prices.
	if len(symbols) > 0 {
		for _, sym := range symbols {
			if _, ok := pricesBySymbol[sym]; !ok {
				warnings = append(warnings, "missing market data for "+sym)
			}
		}
	}

	// Check staleness.
	if marketProvider != nil && len(symbols) > 0 {
		latestDates := marketProvider.GetLatestPriceDatePerSymbol(ctx, symbols)
		now := time.Now().UTC()
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

	// Build equity curve points from snapshots.
	points := buildCurvePoints(ctx, snapshots, pricesBySymbol, baseCurrency, marketProvider, logger)

	// Compute pre-cash-flow breakpoints (needed for both NAV and TWR).
	preCashFlowValues := computePreCashFlowValues(
		ctx, snapshots, pricesBySymbol, baseCurrency, marketProvider, logger,
	)

	// Interpolate for non-transaction days, extending through dateTo.
	points = InterpolateDaily(ctx, points, snapshots, dateTo, pricesBySymbol, baseCurrency, marketProvider)

	if logger != nil {
		logger.Debug("performance: interpolation complete", "points", len(points))
	}

	// Compute NAV history (unitization) on the full (pre-slice) curve.
	var inceptionDate time.Time
	for _, txn := range txns {
		if txn.Type == "deposit" {
			inceptionDate = txn.Date
			break
		}
	}

	navBreakpoints := make([]NavBreakpoint, len(preCashFlowValues))
	for i, bp := range preCashFlowValues {
		navBreakpoints[i] = NavBreakpoint{Date: bp.date, Value: bp.value}
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

	// Slice to period range.
	if !dateFrom.IsZero() {
		points = sliceFrom(points, dateFrom)
	}

	// Filter breakpoints to visible period.
	visibleBreakpoints := filterBreakpoints(preCashFlowValues, points, dateTo)

	// Compute return metrics.
	returnMetrics := ComputePeriodReturn(points, visibleBreakpoints, baseCurrency)

	// Compute additional metrics from the sliced curve.
	navPoints := convertToNavPoints(points)
	dailyReturns := ComputeDailyReturns(points)
	riskMetrics := ComputeRiskMetrics(dailyReturns, nil)
	drawdownAnalysis := ComputeDrawdownAnalysis(navPoints)
	yearlyPerformance := ComputeYearlyPerformance(navPoints)

	// Build NavSummary.
	var navSummary *NavSummary
	if len(navHistory) > 0 {
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

// walkTxns walks transactions chronologically and captures portfolio state
// at each unique date. Also captures pre-cash-flow snapshots for TWR.
// Returns (snapshots, finalState).
func walkTxns(txns []transaction.Transaction) ([]dateSnapshot, dateSnapshot) {
	var snapshots []dateSnapshot
	positionCurrency := make(map[string]string)
	quantities := make(map[string]decimal.Decimal)
	cashBalance := make(map[string]decimal.Decimal)
	netDeposit := make(map[string]decimal.Decimal)
	preCashFlowByDate := make(map[string][]preCashFlowSnapshot)

	for i, txn := range txns {
		if txn.Type == "buy" || txn.Type == "sell" {
			positionCurrency[txn.Symbol] = txn.Currency
		}

		if txn.Type == "deposit" || txn.Type == "withdrawal" {
			dateKey := txn.Date.Format(time.RFC3339)
			preCashFlowByDate[dateKey] = append(preCashFlowByDate[dateKey], preCashFlowSnapshot{
				date:             txn.Date,
				positions:        copyDecMap(quantities),
				positionCurrency: copyStrMap(positionCurrency),
				cashBalance:      copyDecMap(cashBalance),
			})
		}

		if txn.Type == "buy" || txn.Type == "sell" {
			qty, _ := quantities[txn.Symbol].Add(txn.Quantity)
			quantities[txn.Symbol] = qty
		}
		bal, _ := cashBalance[txn.Currency].Add(txn.NetCash)
		cashBalance[txn.Currency] = bal
		if txn.Type == "deposit" || txn.Type == "withdrawal" {
			dep, _ := netDeposit[txn.Currency].Add(txn.NetCash)
			netDeposit[txn.Currency] = dep
		}

		isLastForDate := i == len(txns)-1 || txns[i+1].Date.After(txn.Date)
		if isLastForDate {
			dateKey := txn.Date.Format(time.RFC3339)
			snapshots = append(snapshots, dateSnapshot{
				date:                 txn.Date,
				positions:            copyDecMap(quantities),
				positionCurrency:     copyStrMap(positionCurrency),
				cashBalance:          copyDecMap(cashBalance),
				netDeposit:           copyDecMap(netDeposit),
				preCashFlowSnapshots: preCashFlowByDate[dateKey],
			})
		}
	}

	return snapshots, dateSnapshot{
		positions:        copyDecMap(quantities),
		positionCurrency: copyStrMap(positionCurrency),
		cashBalance:      copyDecMap(cashBalance),
		netDeposit:       copyDecMap(netDeposit),
	}
}

func CollectSymbols(txns []transaction.Transaction) []string {
	set := make(map[string]bool)
	for _, txn := range txns {
		if (txn.Type == "buy" || txn.Type == "sell") && !isCashSymbol(txn.Symbol) {
			set[txn.Symbol] = true
		}
	}
	symbols := make([]string, 0, len(set))
	for sym := range set {
		symbols = append(symbols, sym)
	}
	return symbols
}

func buildCurvePoints(
	ctx context.Context,
	snapshots []dateSnapshot,
	pricesBySymbol map[string][]market.HistoricalPrice,
	baseCurrency string,
	marketProvider MarketDataProvider,
	logger *slog.Logger,
) []EquityCurvePoint {
	priceLookup := buildPriceLookupFF(buildPriceLookup(pricesBySymbol))
	fxPairs := collectFxPairs(snapshots, baseCurrency)
	fxLookup := buildFxLookupFF(ctx, marketProvider, fxPairs, snapshots, logger)

	var points []EquityCurvePoint
	for _, snap := range snapshots {
		var portfolioValue, posValue decimal.Decimal

		for symbol, qty := range snap.positions {
			dateKey := snap.date.Format("2006-01-02")
			price, found := lookupPrice(priceLookup, symbol, dateKey)
			if !found {
				continue
			}
			value, _ := qty.Mul(price.Close)
			if price.Currency != baseCurrency {
				var ok bool
				value, ok = convertWithFxLookup(fxLookup, price.Currency, baseCurrency, value, snap.date)
				if !ok {
					continue
				}
			}
			posValue, _ = posValue.Add(value)
			portfolioValue, _ = portfolioValue.Add(value)
		}

		var cashValue decimal.Decimal
		for currency, balance := range snap.cashBalance {
			if currency != baseCurrency {
				var ok bool
				balance, ok = convertWithFxLookup(fxLookup, currency, baseCurrency, balance, snap.date)
				if !ok {
					continue
				}
			}
			cashValue, _ = cashValue.Add(balance)
			portfolioValue, _ = portfolioValue.Add(balance)
		}

		var netDepositBase decimal.Decimal
		for currency, deposit := range snap.netDeposit {
			if currency != baseCurrency {
				var ok bool
				deposit, ok = convertWithFxLookup(fxLookup, currency, baseCurrency, deposit, snap.date)
				if !ok {
					continue
				}
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

func sliceFrom(points []EquityCurvePoint, dateFrom time.Time) []EquityCurvePoint {
	if dateFrom.IsZero() {
		return points
	}
	idx := sort.Search(len(points), func(i int) bool {
		return !points[i].Date.Before(dateFrom)
	})
	if idx >= len(points) {
		return []EquityCurvePoint{}
	}
	return points[idx:]
}

func filterBreakpoints(bps []twrBreakpoint, points []EquityCurvePoint, dateTo time.Time) []twrBreakpoint {
	if len(points) == 0 {
		return nil
	}
	firstDate := points[0].Date
	var result []twrBreakpoint
	for _, bp := range bps {
		if !bp.date.After(firstDate) {
			continue
		}
		if !dateTo.IsZero() && bp.date.After(dateTo) {
			continue
		}
		result = append(result, bp)
	}
	return result
}

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

func copyDecMap(src map[string]decimal.Decimal) map[string]decimal.Decimal {
	dst := make(map[string]decimal.Decimal, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyStrMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func tradingDayBeforeOrOn(t time.Time) time.Time {
	d := t
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func isCashSymbol(symbol string) bool {
	return strings.HasPrefix(symbol, "$CASH-")
}
