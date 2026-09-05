package comparison

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/allocation"
	"github.com/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/performance"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// --- Interfaces ---

// ModelPortfolioSource fetches model portfolios by ID.
type ModelPortfolioSource interface {
	Get(ctx context.Context, id int64) (modelportfolio.ModelPortfolio, error)
}

// EquityCurveSource computes the equity curve for a real portfolio.
type EquityCurveSource interface {
	ComputeEquityCurve(ctx context.Context, filters performance.PerformanceFilters) (*performance.PerformanceResult, error)
}

// MarketDataHistorySource fetches historical prices for a symbol.
type MarketDataHistorySource interface {
	GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
}

// MarketDataSymbolResolver maps an internal symbol to its market data provider symbol.
type MarketDataSymbolResolver interface {
	GetMarketDataSymbol(ctx context.Context, internalSymbol string) (string, error)
}

// SymbolDetailsSource retrieves cached symbol details.
type SymbolDetailsSource interface {
	GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbol.SymbolDetails, error)
}

// PortfolioNameSource returns the display name of a real portfolio by ID.
type PortfolioNameSource interface {
	GetPortfolioName(ctx context.Context, portfolioID int64) (string, error)
}

// PortfolioCurrencySource returns the base currency of a portfolio.
type PortfolioCurrencySource interface {
	GetPortfolioCurrency(ctx context.Context, portfolioID int64) (string, error)
}

// AllocationSource computes the allocation breakdown for a portfolio.
type AllocationSource interface {
	ComputeAllocation(ctx context.Context, filter allocation.AllocationFilter) (*allocation.AllocationResult, error)
}

// --- Service ---

// Service orchestrates portfolio comparisons. It resolves portfolio inputs
// (model or real), fetches required data, and computes the full comparison result.
type Service struct {
	modelPortfolios   ModelPortfolioSource
	equityCurve       EquityCurveSource
	marketHistory     MarketDataHistorySource
	marketDataSymbol  MarketDataSymbolResolver
	symbolDetails     SymbolDetailsSource
	portfolioName     PortfolioNameSource
	portfolioCurrency PortfolioCurrencySource
	allocation        AllocationSource
	logger            *slog.Logger
}

// NewService creates a new comparison service.
func NewService(
	modelPortfolios ModelPortfolioSource,
	equityCurve EquityCurveSource,
	marketHistory MarketDataHistorySource,
	marketDataSymbol MarketDataSymbolResolver,
	symbolDetails SymbolDetailsSource,
	portfolioName PortfolioNameSource,
	portfolioCurrency PortfolioCurrencySource,
	allocation AllocationSource,
) *Service {
	return &Service{
		modelPortfolios:   modelPortfolios,
		equityCurve:       equityCurve,
		marketHistory:     marketHistory,
		marketDataSymbol:  marketDataSymbol,
		symbolDetails:     symbolDetails,
		portfolioName:     portfolioName,
		portfolioCurrency: portfolioCurrency,
		allocation:        allocation,
	}
}

// WithLogger sets the logger for the service.
func (s *Service) WithLogger(logger *slog.Logger) {
	s.logger = logger
}

// ComputeComparison computes the full comparison between two portfolios.
//
// It resolves each portfolio (model → simulate equity curve; real → fetch
// existing equity curve), computes per-portfolio metrics (returns, risk,
// drawdown, yearly performance, period extremes, return distribution), then
// computes cross-portfolio metrics (beta/alpha, correlation).
//
// Returns an error only when data cannot be fetched. Data quality issues
// (missing prices, short history) are reported as warnings in the result.
func (s *Service) ComputeComparison(ctx context.Context, req ComparisonRequest) (*ComparisonResult, error) {
	result := &ComparisonResult{
		ComputedAt: time.Now().UTC(),
	}

	// Resolve date range.
	dateFrom, dateTo := s.resolveDateRange(req)

	// Resolve base currency.
	baseCurrency := req.BaseCurrency
	if baseCurrency == "" {
		// Try to infer from portfolio A, then B.
		var err error
		baseCurrency, err = s.resolveBaseCurrency(ctx, req.PortfolioAID, req.PortfolioAType)
		if err != nil || baseCurrency == "" {
			// Ignore the error: an empty result falls back to USD below.
			baseCurrency, _ = s.resolveBaseCurrency(ctx, req.PortfolioBID, req.PortfolioBType)
		}
		if baseCurrency == "" {
			baseCurrency = "USD" // fallback
		}
	}

	// Resolve Portfolio A equity curve.
	aCurve, aData, aWarnings, err := s.resolveEquityCurve(ctx, req.PortfolioAID, req.PortfolioAType, req.StartingValue, baseCurrency, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("resolve portfolio A: %w", err)
	}
	result.Warnings = append(result.Warnings, aWarnings...)

	// Resolve Portfolio B equity curve.
	bCurve, bData, bWarnings, err := s.resolveEquityCurve(ctx, req.PortfolioBID, req.PortfolioBType, req.StartingValue, baseCurrency, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("resolve portfolio B: %w", err)
	}
	result.Warnings = append(result.Warnings, bWarnings...)

	// Check if both portfolios have insufficient data.
	if len(aCurve) < 2 && len(bCurve) < 2 {
		return &ComparisonResult{
			ComputedAt: result.ComputedAt,
			PortfolioA: s.emptyPortfolioComparison(aData, "Insufficient data for comparison (fewer than 2 data points)"),
			PortfolioB: s.emptyPortfolioComparison(bData, "Insufficient data for comparison (fewer than 2 data points)"),
			Warnings:   result.Warnings,
			Message:    "Insufficient data for both portfolios. Ensure market data is cached for all symbols.",
		}, nil
	}

	// Align both curves to the intersection of their date ranges so metrics
	// are computed over the same period. This is essential for fair comparison.
	if len(aCurve) >= 2 && len(bCurve) >= 2 {
		commonFrom := aCurve[0].Date
		if bCurve[0].Date.After(commonFrom) {
			commonFrom = bCurve[0].Date
		}
		commonTo := aCurve[len(aCurve)-1].Date
		if bCurve[len(bCurve)-1].Date.Before(commonTo) {
			commonTo = bCurve[len(bCurve)-1].Date
		}
		if commonFrom.Before(commonTo) {
			aCurve = clipCurveToDateRange(aCurve, commonFrom, commonTo)
			bCurve = clipCurveToDateRange(bCurve, commonFrom, commonTo)
		} else {
			// No overlapping data.
			result.Warnings = append(result.Warnings, "No overlapping date range between portfolios")
		}
	}
	// If only one portfolio has data, its (unclipped) curve is used as-is;
	// cross-metric computation below requires both sides.

	// Compute per-portfolio metrics.
	result.PortfolioA = s.computePortfolioMetrics(aCurve, aData, baseCurrency, req.RiskFreeRatePct)
	result.PortfolioB = s.computePortfolioMetrics(bCurve, bData, baseCurrency, req.RiskFreeRatePct)

	// Compute cross-portfolio metrics (only if both have sufficient data).
	if len(aCurve) >= 2 && len(bCurve) >= 2 {
		result.CrossMetrics = s.computeCrossMetrics(ctx, aCurve, bCurve, aData, bData, req, dateFrom, dateTo, baseCurrency)
	}

	return result, nil
}

// resolveDateRange parses the period string into a date range.
func (s *Service) resolveDateRange(req ComparisonRequest) (time.Time, time.Time) {
	now := time.Now().UTC()

	if req.DateFrom != nil && req.DateTo != nil {
		return *req.DateFrom, *req.DateTo
	}

	dateTo := now
	if req.DateTo != nil {
		dateTo = *req.DateTo
	}

	if req.DateFrom != nil {
		return *req.DateFrom, dateTo
	}

	period := req.Period
	if period == "" || period == "All" {
		return time.Time{}, dateTo
	}

	switch period {
	case "1W":
		return now.AddDate(0, 0, -7), dateTo
	case "1M":
		return now.AddDate(0, -1, 0), dateTo
	case "3M":
		return now.AddDate(0, -3, 0), dateTo
	case "1Y":
		return now.AddDate(-1, 0, 0), dateTo
	case "3Y":
		return now.AddDate(-3, 0, 0), dateTo
	case "5Y":
		return now.AddDate(-5, 0, 0), dateTo
	case "YTD":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC), dateTo
	default:
		return time.Time{}, dateTo
	}
}

// resolveBaseCurrency tries to determine the base currency for a portfolio.
// Real portfolios have an explicit base currency. Model portfolios do not —
// they are currency-agnostic (the base currency is set by the comparison request).
// Returns empty string for model portfolios; the caller falls back to the
// request's BaseCurrency or USD.
func (s *Service) resolveBaseCurrency(ctx context.Context, portfolioID int64, portType PortfolioType) (string, error) {
	if portType == PortTypeReal && s.portfolioCurrency != nil {
		return s.portfolioCurrency.GetPortfolioCurrency(ctx, portfolioID)
	}
	if portType == PortTypeModel && s.modelPortfolios != nil {
		_, err := s.modelPortfolios.Get(ctx, portfolioID)
		if err != nil {
			return "", err
		}
		// Model portfolios are currency-agnostic — base currency comes from the request.
		return "", nil
	}
	return "", nil
}

// resolveEquityCurve resolves a portfolio to its equity curve points.
// For model portfolios, it simulates a buy-and-hold equity curve.
// For real portfolios, it fetches the existing equity curve.
// Returns (curve as comparison.EquityCurvePoint, metadata, warnings, error).
func (s *Service) resolveEquityCurve(
	ctx context.Context,
	portfolioID int64,
	portType PortfolioType,
	startingValue decimal.Decimal,
	baseCurrency string,
	dateFrom, dateTo time.Time,
) ([]EquityCurvePoint, portfolioMeta, []string, error) {
	switch portType {
	case PortTypeModel:
		return s.resolveModelPortfolio(ctx, portfolioID, startingValue, baseCurrency, dateFrom, dateTo)
	case PortTypeReal:
		return s.resolveRealPortfolio(ctx, portfolioID, baseCurrency, dateFrom, dateTo)
	default:
		return nil, nil, nil, fmt.Errorf("unknown portfolio type: %s", portType)
	}
}

// resolveModelPortfolio fetches the model portfolio and simulates its equity curve.
func (s *Service) resolveModelPortfolio(
	ctx context.Context,
	portfolioID int64,
	startingValue decimal.Decimal,
	baseCurrency string,
	dateFrom, dateTo time.Time,
) ([]EquityCurvePoint, portfolioMeta, []string, error) {
	mp, err := s.modelPortfolios.Get(ctx, portfolioID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get model portfolio %d: %w", portfolioID, err)
	}

	// Resolve market data symbols for entries.
	weights, symbolWarnings := s.resolveModelWeights(ctx, mp, baseCurrency)

	// Fetch historical prices for all symbols.
	pricesBySym, pricesWarnings := s.fetchHistoricalPricesForWeights(ctx, weights, dateFrom, dateTo)

	// Fetch FX rates if needed.
	fxRates, fxWarnings := s.fetchFxRates(ctx, weights, baseCurrency, dateFrom, dateTo)

	warnings := append(symbolWarnings, pricesWarnings...)
	warnings = append(warnings, fxWarnings...)

	// Simulate equity curve.
	simInput := SimulateEquityCurveInput{
		StartingValue: startingValue,
		Weights:       weights,
		PricesBySym:   pricesBySym,
		BaseCurrency:  baseCurrency,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		FxRates:       fxRates,
	}

	simOutput := SimulateEquityCurve(simInput)

	// Convert simulation output to comparison equity curve points.
	curve := convertToComparisonCurve(simOutput.EquityCurve)

	meta := &modelPortfolioMeta{
		ID:          portfolioID,
		Name:        mp.Name,
		Weights:     weights,
		Currency:    baseCurrency,
		PricesBySym: pricesBySym,
	}

	return curve, meta, warnings, nil
}

// resolveRealPortfolio fetches the existing equity curve for a real portfolio.
func (s *Service) resolveRealPortfolio(
	ctx context.Context,
	portfolioID int64,
	baseCurrency string,
	dateFrom, dateTo time.Time,
) ([]EquityCurvePoint, portfolioMeta, []string, error) {
	filters := performance.PerformanceFilters{
		PortfolioID: &portfolioID,
		DateFrom:    &dateFrom,
		DateTo:      &dateTo,
	}

	result, err := s.equityCurve.ComputeEquityCurve(ctx, filters)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("compute equity curve for portfolio %d: %w", portfolioID, err)
	}

	// Convert performance equity curve to comparison equity curve.
	curve := convertPerformanceToComparisonCurve(result.EquityCurve)

	name := fmt.Sprintf("Portfolio %d", portfolioID)
	if s.portfolioName != nil {
		resolved, err := s.portfolioName.GetPortfolioName(ctx, portfolioID)
		if err == nil && resolved != "" {
			name = resolved
		}
	}

	// Fetch historical prices for symbols in the portfolio (for intra-portfolio correlation).
	pricesBySym := s.fetchPricesForRealPortfolio(ctx, portfolioID, dateFrom, dateTo)

	meta := &realPortfolioMeta{
		ID:           portfolioID,
		Name:         name,
		BaseCurrency: result.BaseCurrency,
		Warnings:     result.Warnings,
		PricesBySym:  pricesBySym,
	}

	return curve, meta, result.Warnings, nil
}

// fetchPricesForRealPortfolio fetches historical prices for the symbols held
// in a real portfolio, using the allocation service to resolve current positions.
// Returns prices keyed by market symbol. Returns nil if any dependency is missing.
func (s *Service) fetchPricesForRealPortfolio(ctx context.Context, portfolioID int64, dateFrom, dateTo time.Time) map[string][]market.HistoricalPrice {
	if s.allocation == nil || s.marketHistory == nil {
		return nil
	}
	result, err := s.allocation.ComputeAllocation(ctx, allocation.AllocationFilter{
		PortfolioIDs: []int64{portfolioID},
	})
	if err != nil || !result.MarketDataAvailable || len(result.Rows) == 0 {
		return nil
	}

	pricesBySym := make(map[string][]market.HistoricalPrice)
	for _, row := range result.Rows {
		marketSym := row.Symbol
		if s.marketDataSymbol != nil {
			resolved, err := s.marketDataSymbol.GetMarketDataSymbol(ctx, row.Symbol)
			if err == nil {
				marketSym = resolved
			}
		}
		prices, err := s.marketHistory.GetHistoricalPrices(ctx, marketSym, dateFrom, dateTo)
		if err != nil || len(prices) == 0 {
			continue
		}
		pricesBySym[marketSym] = prices
	}
	return pricesBySym
}

// resolveModelWeights resolves market data symbols for model portfolio entries
// and builds the weight slice for simulation.
func (s *Service) resolveModelWeights(ctx context.Context, mp modelportfolio.ModelPortfolio, baseCurrency string) ([]ModelPortfolioWeight, []string) {
	weights := make([]ModelPortfolioWeight, 0, len(mp.Entries))
	var warnings []string

	for _, entry := range mp.Entries {
		marketSym := entry.Symbol // default: same as internal symbol
		if s.marketDataSymbol != nil {
			resolved, err := s.marketDataSymbol.GetMarketDataSymbol(ctx, entry.Symbol)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not resolve market data symbol for %s — using internal symbol", entry.Symbol))
			} else {
				marketSym = resolved
			}
		}

		// Determine symbol currency from symbol details.
		currency := baseCurrency // default: assume same as base
		if s.symbolDetails != nil {
			details, err := s.symbolDetails.GetByInternalSymbol(ctx, entry.Symbol)
			if err == nil && details != nil && details.Currency != "" {
				currency = details.Currency
			}
		}

		// Convert weight_pct (percentage like 50.00) to fraction (0.50).
		weightFrac, _ := entry.WeightPct.Quo(decimal.MustNew(100, 0))

		weights = append(weights, ModelPortfolioWeight{
			Symbol:    entry.Symbol,
			Weight:    weightFrac,
			Currency:  currency,
			MarketSym: marketSym,
		})
	}

	return weights, warnings
}

// fetchHistoricalPricesForWeights fetches historical prices for all symbols
// in the weight list. Returns prices keyed by market symbol.
func (s *Service) fetchHistoricalPricesForWeights(ctx context.Context, weights []ModelPortfolioWeight, dateFrom, dateTo time.Time) (map[string][]market.HistoricalPrice, []string) {
	pricesBySym := make(map[string][]market.HistoricalPrice)
	var warnings []string

	for _, w := range weights {
		// Skip cash symbols — they have no market data to fetch.
		if strings.HasPrefix(w.Symbol, "$CASH") {
			continue
		}
		prices, err := s.marketHistory.GetHistoricalPrices(ctx, w.MarketSym, dateFrom, dateTo)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to fetch prices for %s: %v", w.Symbol, err))
			continue
		}
		if len(prices) == 0 {
			warnings = append(warnings, fmt.Sprintf("no price data for %s", w.Symbol))
			continue
		}
		pricesBySym[w.MarketSym] = prices
	}

	return pricesBySym, warnings
}

// normalizeCurrency converts Yahoo "GBp" (pence) to "GBP".
// Yahoo returns some UK stock prices in GBp instead of GBP.
func normalizeCurrency(cur string) string {
	if cur == "GBp" {
		return "GBP"
	}
	return cur
}

// fetchFxRates fetches historical FX rates for symbols whose currency differs
// from the base currency. Returns prices keyed by FX pair and any warnings.
func (s *Service) fetchFxRates(ctx context.Context, weights []ModelPortfolioWeight, baseCurrency string, dateFrom, dateTo time.Time) (map[string][]market.HistoricalPrice, []string) {
	fxRates := make(map[string][]market.HistoricalPrice)

	// Collect unique FX pairs.
	pairSet := make(map[string]bool)
	for _, w := range weights {
		cur := normalizeCurrency(w.Currency)
		base := normalizeCurrency(baseCurrency)
		if cur != "" && cur != base {
			pair := market.FormatFxPair(cur, base)
			pairSet[pair] = true
		}
	}

	var fxWarnings []string
	for pair := range pairSet {
		prices, err := s.marketHistory.GetHistoricalPrices(ctx, pair, dateFrom, dateTo)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to fetch FX rates", "pair", pair, "error", err)
			}
			fxWarnings = append(fxWarnings, fmt.Sprintf("failed to fetch FX rates for %s: %v", pair, err))
			continue
		}
		if len(prices) == 0 {
			if s.logger != nil {
				s.logger.Info("no FX rate data, spot rate fallback will be used", "pair", pair)
			}
			fxWarnings = append(fxWarnings, fmt.Sprintf("no FX rate data for %s — spot rate fallback will be used", pair))
			continue
		}
		fxRates[pair] = prices
	}

	return fxRates, fxWarnings
}

// convertToComparisonCurve converts simulation output equity curve points
// to comparison equity curve points (same type, but explicit conversion).
func convertToComparisonCurve(points []EquityCurvePoint) []EquityCurvePoint {
	// Same type — just return as-is.
	return points
}

// convertPerformanceToComparisonCurve converts performance equity curve points
// to comparison equity curve points, preserving NavPerUnit for TWR-aware metrics.
func convertPerformanceToComparisonCurve(points []performance.EquityCurvePoint) []EquityCurvePoint {
	result := make([]EquityCurvePoint, len(points))
	for i, p := range points {
		result[i] = EquityCurvePoint{
			Date:           p.Date,
			PortfolioValue: p.PortfolioValue,
			NavPerUnit:     p.NavPerUnit,
		}
	}
	return result
}

// portfolioMeta is the interface for portfolio metadata returned by resolveEquityCurve.
type portfolioMeta interface {
	getID() int64
	getName() string
	getType() PortfolioType
}

// modelPortfolioMeta implements portfolioMeta for model portfolios.
type modelPortfolioMeta struct {
	ID          int64
	Name        string
	Weights     []ModelPortfolioWeight
	Currency    string
	PricesBySym map[string][]market.HistoricalPrice // marketSym -> price series
}

func (m *modelPortfolioMeta) getID() int64           { return m.ID }
func (m *modelPortfolioMeta) getName() string        { return m.Name }
func (m *modelPortfolioMeta) getType() PortfolioType { return PortTypeModel }

// realPortfolioMeta implements portfolioMeta for real portfolios.
type realPortfolioMeta struct {
	ID           int64
	Name         string
	BaseCurrency string
	Warnings     []string
	PricesBySym  map[string][]market.HistoricalPrice // marketSym -> price series
}

func (r *realPortfolioMeta) getID() int64           { return r.ID }
func (r *realPortfolioMeta) getName() string        { return r.Name }
func (r *realPortfolioMeta) getType() PortfolioType { return PortTypeReal }

// emptyPortfolioComparison returns a PortfolioComparison with an empty-state message.
func (s *Service) emptyPortfolioComparison(meta portfolioMeta, message string) *PortfolioComparison {
	return &PortfolioComparison{
		ID:   meta.getID(),
		Name: meta.getName(),
		Type: meta.getType(),
		ReturnMetrics: &ReturnMetrics{
			HasInsufficientData: true,
		},
		Message: message,
	}
}

// computePortfolioMetrics computes all per-portfolio metrics from an equity curve.
func (s *Service) computePortfolioMetrics(curve []EquityCurvePoint, meta portfolioMeta, baseCurrency string, riskFreeRatePct *decimal.Decimal) *PortfolioComparison {
	pc := &PortfolioComparison{
		ID:   meta.getID(),
		Name: meta.getName(),
		Type: meta.getType(),
	}

	if len(curve) < 2 {
		pc.ReturnMetrics = &ReturnMetrics{HasInsufficientData: true}
		pc.Message = "Insufficient data (fewer than 2 equity curve points)"
		return pc
	}

	// Set effective date range from the actual data.
	pc.EffectiveDateFrom = &curve[0].Date
	pc.EffectiveDateTo = &curve[len(curve)-1].Date

	// For real portfolios, build a NAV-based curve for TWR-aware metrics.
	// NavPerUnit is cash-flow-independent (unitized), so metrics derived from
	// it isolate investment performance from deposit/withdrawal timing.
	// For model portfolios, navCurve == curve (no cash flows).
	navCurve := s.prepareCurveForMetrics(curve, meta)

	// Store the TWR-equivalent curve. All comparison charts/metrics use
	// navCurve so they are cash-flow-independent and consistent with TWR.
	pc.ValueGrowthSeries = navCurve

	// --- Return metrics — TWR only (cash-flow-independent) ---
	pc.ReturnMetrics = s.computeReturnMetrics(navCurve)

	// --- Risk metrics ---
	pc.RiskMetrics = s.computeRiskMetrics(navCurve, riskFreeRatePct)

	// --- Drawdown ---
	pc.Drawdown = s.computeDrawdown(navCurve)
	pc.DrawdownSeries = s.computeDrawdownSeries(navCurve)

	// --- Yearly returns — from NAV curve (cash-flow-independent) ---
	pc.YearlyReturns = s.computeYearlyReturns(navCurve)

	// --- Period extremes ---
	extremes := ComputePeriodExtremes(navCurve)
	pc.PeriodExtremes = &extremes

	// --- Return distribution ---
	dist := ComputeReturnDistribution(navCurve)
	pc.ReturnDistribution = &dist

	// --- Intra-portfolio correlation ---
	pc.IntraCorrelation = s.computeIntraPortfolioCorrelation(meta)

	return pc
}

// computeReturnMetrics computes summary return metrics from the TWR-equivalent
// (cash-flow-independent) curve. All comparison metrics use this single curve
// so they are consistent with TWR.
func (s *Service) computeReturnMetrics(curve []EquityCurvePoint) *ReturnMetrics {
	metrics := &ReturnMetrics{}

	// CAGR — annualized TWR-equivalent.
	cagr := ComputeCAGR(curve)
	metrics.CAGRPct = cagr.CAGRPct
	metrics.DaysElapsed = cagr.DaysElapsed

	// TWR from NAV curve (cash-flow-independent).
	// For model portfolios, curve == raw curve so TWR == simple return.
	// For real portfolios, curve uses NavPerUnit which isolates investment
	// performance from deposit/withdrawal timing.
	if len(curve) >= 2 {
		firstF, _ := curve[0].PortfolioValue.Float64()
		lastF, _ := curve[len(curve)-1].PortfolioValue.Float64()
		if firstF > 0 {
			ret, _ := decimal.NewFromFloat64((lastF/firstF - 1.0) * 100.0)
			ret = ret.Round(2)
			metrics.TWRPct = &ret

			// Annualized TWR.
			days := curve[len(curve)-1].Date.Sub(curve[0].Date).Hours() / 24.0
			if days > 0 {
				retF, _ := ret.Float64()
				annualizedF := math.Pow(1.0+retF/100.0, 365.0/days) - 1.0
				annualized, _ := decimal.NewFromFloat64(annualizedF * 100.0)
				annualized = annualized.Round(2)
				metrics.AnnualizedTWRPct = &annualized
			}
		}
	}

	return metrics
}

// computeRiskMetrics computes volatility, Sharpe, Sortino from daily returns.
func (s *Service) computeRiskMetrics(curve []EquityCurvePoint, riskFreeRatePct *decimal.Decimal) *RiskMetrics {
	// Convert to performance.DailyReturn-compatible format.
	perfCurve := make([]performance.EquityCurvePoint, len(curve))
	for i, p := range curve {
		perfCurve[i] = performance.EquityCurvePoint{
			Date:           p.Date,
			PortfolioValue: p.PortfolioValue,
		}
	}

	dailyReturns := performance.ComputeDailyReturns(perfCurve)
	perfRisk := performance.ComputeRiskMetrics(dailyReturns, riskFreeRatePct)

	return &RiskMetrics{
		AnnualizedVolatilityPct: perfRisk.AnnualizedVolatilityPct,
		SharpeRatio:             perfRisk.SharpeRatio,
		SortinoRatio:            perfRisk.SortinoRatio,
	}
}

// computeDrawdown computes drawdown statistics from the equity curve.
func (s *Service) computeDrawdown(curve []EquityCurvePoint) *DrawdownResult {
	// Convert to performance.NavPoint for drawdown computation.
	navPoints := make([]performance.NavPoint, len(curve))
	for i, p := range curve {
		navPoints[i] = performance.NavPoint{
			Date:           p.Date,
			NavPerUnit:     p.PortfolioValue, // for model portfolios, PortfolioValue == NAV
			PortfolioValue: p.PortfolioValue,
		}
	}

	drawdown := performance.ComputeDrawdownAnalysis(navPoints)

	return &DrawdownResult{
		MaxDrawdownPct:       drawdown.MaxDrawdownPct,
		CurrentDrawdownPct:   drawdown.CurrentDrawdownPct,
		DrawdownDurationDays: drawdown.DrawdownDurationDays,
	}
}

// computeDrawdownSeries computes the drawdown-over-time series from the equity curve.
func (s *Service) computeDrawdownSeries(curve []EquityCurvePoint) []DrawdownSeriesPoint {
	return ComputeDrawdownSeries(curve)
}

// computeIntraPortfolioCorrelation computes the pairwise correlation matrix
// for symbols within a single portfolio.
func (s *Service) computeIntraPortfolioCorrelation(meta portfolioMeta) *IntraPortfolioCorrelationResult {
	var pricesBySym map[string][]market.HistoricalPrice
	switch m := meta.(type) {
	case *modelPortfolioMeta:
		pricesBySym = m.PricesBySym
	case *realPortfolioMeta:
		pricesBySym = m.PricesBySym
	default:
		return nil
	}
	if len(pricesBySym) < 2 {
		return nil
	}
	return ComputeIntraPortfolioCorrelation(IntraPortfolioCorrelationInput{
		Prices: pricesBySym,
		Period: "1Y", // default period for comparison context
	})
}

// computeYearlyReturns computes calendar-year returns from the equity curve.
func (s *Service) computeYearlyReturns(curve []EquityCurvePoint) []YearlyReturn {
	// Convert to performance.NavPoint.
	navPoints := make([]performance.NavPoint, len(curve))
	for i, p := range curve {
		navPoints[i] = performance.NavPoint{
			Date:           p.Date,
			NavPerUnit:     p.PortfolioValue,
			PortfolioValue: p.PortfolioValue,
		}
	}

	perfYearly := performance.ComputeYearlyPerformance(navPoints)

	result := make([]YearlyReturn, len(perfYearly))
	for i, yr := range perfYearly {
		result[i] = YearlyReturn{
			Year:      yr.Year,
			ReturnPct: yr.ReturnPct,
		}
	}
	return result
}

// prepareCurveForMetrics returns the appropriate curve for computing
// portfolio metrics. For real portfolios with NavPerUnit (cash-flow-aware NAV),
// it builds a curve from NAV values which are already cash-flow-independent.
// For model portfolios or portfolios without NavPerUnit, the raw curve is used.
func (s *Service) prepareCurveForMetrics(curve []EquityCurvePoint, meta portfolioMeta) []EquityCurvePoint {
	if _, isReal := meta.(*realPortfolioMeta); isReal {
		return buildNavCurve(curve)
	}
	return curve
}

// buildNavCurve builds a curve from NavPerUnit values.
// NavPerUnit is computed by the performance layer via unitization (real
// portfolios) or set to PortfolioValue in simulation (model portfolios).
// It is always non-nil and already cash-flow-independent (TWR-equivalent).
func buildNavCurve(curve []EquityCurvePoint) []EquityCurvePoint {
	if len(curve) == 0 {
		return curve
	}

	result := make([]EquityCurvePoint, len(curve))
	for i, p := range curve {
		result[i] = EquityCurvePoint{
			Date:           p.Date,
			PortfolioValue: *p.NavPerUnit,
		}
	}
	return result
}

// computeCrossMetrics computes cross-portfolio metrics (beta/alpha, correlation, overlap).
func (s *Service) computeCrossMetrics(
	ctx context.Context,
	aCurve, bCurve []EquityCurvePoint,
	aData, bData portfolioMeta,
	req ComparisonRequest,
	dateFrom, dateTo time.Time,
	baseCurrency string,
) *CrossPortfolioMetrics {
	cross := &CrossPortfolioMetrics{}

	// Beta/Alpha (A relative to B).
	betaAlpha := ComputeBetaAlpha(aCurve, bCurve)
	cross.BetaAlpha = &betaAlpha

	// Correlation.
	correlation := ComputePortfolioCorrelation(aCurve, bCurve)
	cross.Correlation = &correlation

	// Capture ratios (A relative to B).
	captureRatios := ComputeCaptureRatios(aCurve, bCurve)
	cross.CaptureRatios = &captureRatios

	// Overlap — only available when both portfolios have holdings data
	// (i.e. both are model portfolios with known weights).
	overlap := s.computeOverlap(ctx, aData, bData)
	if overlap != nil {
		cross.Overlap = overlap
	}

	return cross
}

// computeOverlap computes cross-portfolio holdings overlap.
// Returns nil if either portfolio lacks holdings data (e.g. real portfolios).
func (s *Service) computeOverlap(ctx context.Context, aData, bData portfolioMeta) *OverlapResult {
	holdingsA, okA := s.buildPortfolioHoldings(ctx, aData)
	holdingsB, okB := s.buildPortfolioHoldings(ctx, bData)

	if !okA || !okB {
		// Overlap requires holdings data from both portfolios.
		// Real portfolios (equity curve only) don't expose positions.
		return nil
	}

	return ComputeCrossPortfolioOverlap(CrossPortfolioOverlapInput{
		PortfolioA:     holdingsA,
		PortfolioAName: aData.getName(),
		PortfolioB:     holdingsB,
		PortfolioBName: bData.getName(),
	})
}

// buildPortfolioHoldings converts portfolio metadata to holdings suitable for
// overlap computation. Returns (holdings, ok). ok is false when the portfolio
// type doesn't expose holdings.
func (s *Service) buildPortfolioHoldings(ctx context.Context, meta portfolioMeta) ([]PortfolioHolding, bool) {
	switch m := meta.(type) {
	case *modelPortfolioMeta:
		return s.buildModelHoldings(ctx, m)
	case *realPortfolioMeta:
		return s.buildRealHoldings(ctx, m)
	default:
		return nil, false
	}
}

// buildModelHoldings builds holdings from a model portfolio's weights.
func (s *Service) buildModelHoldings(ctx context.Context, meta *modelPortfolioMeta) ([]PortfolioHolding, bool) {
	holdings := make([]PortfolioHolding, 0, len(meta.Weights))
	for _, w := range meta.Weights {
		holding := PortfolioHolding{
			Symbol:    w.Symbol,
			Weight:    w.Weight, // already a fraction (0.0-1.0)
			QuoteType: "EQUITY", // default
		}

		// Enrich with symbol details (quote type, name, top holdings for ETFs).
		if s.symbolDetails != nil {
			details, err := s.symbolDetails.GetByInternalSymbol(ctx, w.Symbol)
			if err == nil && details != nil {
				if details.QuoteType != "" {
					holding.QuoteType = details.QuoteType
				}
				if details.ShortName != "" {
					holding.Name = details.ShortName
				}
				holding.TopHoldings = details.TopHoldings
				holding.Sector = details.Sector
				holding.SectorWeightings = details.SectorWeightings
				holding.GeographicAllocations = details.GeographicAllocations
			}
		}

		holdings = append(holdings, holding)
	}

	return holdings, true
}

// buildRealHoldings builds holdings from a real portfolio's current allocation.
// Uses the allocation service to get the allocation breakdown, then converts
// each row to a PortfolioHolding.
func (s *Service) buildRealHoldings(ctx context.Context, meta *realPortfolioMeta) ([]PortfolioHolding, bool) {
	if s.allocation == nil {
		return nil, false
	}

	result, err := s.allocation.ComputeAllocation(ctx, allocation.AllocationFilter{
		PortfolioIDs: []int64{meta.ID},
	})
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to compute allocation for overlap", "portfolioID", meta.ID, "error", err)
		}
		return nil, false
	}

	if !result.MarketDataAvailable || len(result.Rows) == 0 {
		return nil, false
	}

	holdings := make([]PortfolioHolding, 0, len(result.Rows))
	for _, row := range result.Rows {
		holding := PortfolioHolding{
			Symbol: row.Symbol,
			Weight: func() decimal.Decimal {
				frac, _ := row.AllocationPct.Quo(decimal.MustNew(100, 0)) // percentage → fraction
				return frac
			}(),
			QuoteType: "EQUITY", // default
		}

		// Enrich with symbol details (quote type, name, top holdings for ETFs).
		if s.symbolDetails != nil {
			details, err := s.symbolDetails.GetByInternalSymbol(ctx, row.Symbol)
			if err == nil && details != nil {
				if details.QuoteType != "" {
					holding.QuoteType = details.QuoteType
				}
				if details.ShortName != "" {
					holding.Name = details.ShortName
				}
				holding.TopHoldings = details.TopHoldings
				holding.Sector = details.Sector
				holding.SectorWeightings = details.SectorWeightings
				holding.GeographicAllocations = details.GeographicAllocations
			}
		}

		holdings = append(holdings, holding)
	}

	return holdings, true
}

// clipCurveToDateRange returns only the points in the curve that fall within
// [dateFrom, dateTo] (inclusive). The curve must be sorted ascending by date.
func clipCurveToDateRange(curve []EquityCurvePoint, dateFrom, dateTo time.Time) []EquityCurvePoint {
	if len(curve) == 0 {
		return curve
	}
	// Find start index (first point >= dateFrom).
	start := 0
	for start < len(curve) && curve[start].Date.Before(dateFrom) {
		start++
	}
	// Find end index (last point <= dateTo).
	end := len(curve) - 1
	for end >= 0 && curve[end].Date.After(dateTo) {
		end--
	}
	if start > end {
		return nil
	}
	return curve[start : end+1]
}
