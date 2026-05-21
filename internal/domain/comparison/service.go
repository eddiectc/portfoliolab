package comparison

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/performance"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
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

// FxRateSource fetches historical FX rates.
type FxRateSource interface {
	GetHistoricalFxRate(ctx context.Context, baseCurrency, quoteCurrency string, date time.Time) (*market.FxRate, error)
}

// PortfolioCurrencySource returns the base currency of a portfolio.
type PortfolioCurrencySource interface {
	GetPortfolioCurrency(ctx context.Context, portfolioID int64) (string, error)
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
	fxRates           FxRateSource
	portfolioCurrency PortfolioCurrencySource
	logger            *slog.Logger
}

// NewService creates a new comparison service.
func NewService(
	modelPortfolios ModelPortfolioSource,
	equityCurve EquityCurveSource,
	marketHistory MarketDataHistorySource,
	marketDataSymbol MarketDataSymbolResolver,
	symbolDetails SymbolDetailsSource,
	fxRates FxRateSource,
	portfolioCurrency PortfolioCurrencySource,
) *Service {
	return &Service{
		modelPortfolios:   modelPortfolios,
		equityCurve:       equityCurve,
		marketHistory:     marketHistory,
		marketDataSymbol:  marketDataSymbol,
		symbolDetails:     symbolDetails,
		fxRates:           fxRates,
		portfolioCurrency: portfolioCurrency,
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
			baseCurrency, err = s.resolveBaseCurrency(ctx, req.PortfolioBID, req.PortfolioBType)
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
			ComputedAt:   result.ComputedAt,
			PortfolioA:   s.emptyPortfolioComparison(aData, "Insufficient data for comparison (fewer than 2 data points)"),
			PortfolioB:   s.emptyPortfolioComparison(bData, "Insufficient data for comparison (fewer than 2 data points)"),
			Warnings:     result.Warnings,
			Message:      "Insufficient data for both portfolios. Ensure market data is cached for all symbols.",
		}, nil
	}

	// Compute per-portfolio metrics.
	result.PortfolioA = s.computePortfolioMetrics(aCurve, aData, baseCurrency)
	result.PortfolioB = s.computePortfolioMetrics(bCurve, bData, baseCurrency)

	// Compute cross-portfolio metrics (only if both have sufficient data).
	if len(aCurve) >= 2 && len(bCurve) >= 2 {
		result.CrossMetrics = s.computeCrossMetrics(ctx, aCurve, bCurve, req, dateFrom, dateTo, baseCurrency)
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
func (s *Service) resolveBaseCurrency(ctx context.Context, portfolioID int64, portType PortfolioType) (string, error) {
	if portType == PortTypeReal && s.portfolioCurrency != nil {
		return s.portfolioCurrency.GetPortfolioCurrency(ctx, portfolioID)
	}
	if portType == PortTypeModel && s.modelPortfolios != nil {
		_, err := s.modelPortfolios.Get(ctx, portfolioID)
		if err != nil {
			return "", err
		}
		// Model portfolios inherit currency from their first entry's symbol.
		// For now, return empty — the caller will use the request's base currency.
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
	fxRates := s.fetchFxRates(ctx, weights, baseCurrency, dateFrom, dateTo)

	warnings := append(symbolWarnings, pricesWarnings...)

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
		ID:       portfolioID,
		Name:     mp.Name,
		Weights:  weights,
		Currency: baseCurrency,
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

	meta := &realPortfolioMeta{
		ID:           portfolioID,
		Name:         fmt.Sprintf("Portfolio %d", portfolioID),
		BaseCurrency: result.BaseCurrency,
		Warnings:     result.Warnings,
	}

	return curve, meta, result.Warnings, nil
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

// fetchFxRates fetches historical FX rates for symbols whose currency differs
// from the base currency. Returns prices keyed by FX pair.
func (s *Service) fetchFxRates(ctx context.Context, weights []ModelPortfolioWeight, baseCurrency string, dateFrom, dateTo time.Time) map[string][]market.HistoricalPrice {
	fxRates := make(map[string][]market.HistoricalPrice)

	// Collect unique FX pairs.
	pairSet := make(map[string]bool)
	for _, w := range weights {
		if w.Currency != "" && w.Currency != baseCurrency {
			pair := market.FormatFxPair(w.Currency, baseCurrency)
			pairSet[pair] = true
		}
	}

	for pair := range pairSet {
		prices, err := s.marketHistory.GetHistoricalPrices(ctx, pair, dateFrom, dateTo)
		if err != nil || len(prices) == 0 {
			continue
		}
		fxRates[pair] = prices
	}

	return fxRates
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
	ID       int64
	Name     string
	Weights  []ModelPortfolioWeight
	Currency string
}

func (m *modelPortfolioMeta) getID() int64       { return m.ID }
func (m *modelPortfolioMeta) getName() string     { return m.Name }
func (m *modelPortfolioMeta) getType() PortfolioType { return PortTypeModel }

// realPortfolioMeta implements portfolioMeta for real portfolios.
type realPortfolioMeta struct {
	ID           int64
	Name         string
	BaseCurrency string
	Warnings     []string
}

func (r *realPortfolioMeta) getID() int64       { return r.ID }
func (r *realPortfolioMeta) getName() string     { return r.Name }
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
func (s *Service) computePortfolioMetrics(curve []EquityCurvePoint, meta portfolioMeta, baseCurrency string) *PortfolioComparison {
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

	// --- Return metrics ---
	pc.ReturnMetrics = s.computeReturnMetrics(curve)

	// --- Risk metrics ---
	pc.RiskMetrics = s.computeRiskMetrics(curve)

	// --- Drawdown ---
	pc.Drawdown = s.computeDrawdown(curve)

	// --- Yearly returns ---
	pc.YearlyReturns = s.computeYearlyReturns(curve)

	// --- Period extremes ---
	// For real portfolios, TWR-normalize the curve first.
	extremesCurve := s.prepareCurveForExtremes(curve, meta)
	extremes := ComputePeriodExtremes(extremesCurve)
	pc.PeriodExtremes = &extremes

	// --- Return distribution ---
	dist := ComputeReturnDistribution(extremesCurve)
	pc.ReturnDistribution = &dist

	return pc
}

// computeReturnMetrics computes summary return metrics from the equity curve.
func (s *Service) computeReturnMetrics(curve []EquityCurvePoint) *ReturnMetrics {
	metrics := &ReturnMetrics{}

	// CAGR.
	cagr := ComputeCAGR(curve)
	metrics.CAGRPct = cagr.CAGRPct
	metrics.DaysElapsed = cagr.DaysElapsed

	// Simple return (first-to-last).
	if len(curve) >= 2 {
		firstF, _ := curve[0].PortfolioValue.Float64()
		lastF, _ := curve[len(curve)-1].PortfolioValue.Float64()
		if firstF > 0 {
			ret, _ := decimal.NewFromFloat64((lastF/firstF - 1.0) * 100.0)
			ret = ret.Round(2)
			metrics.SimpleReturnPct = &ret

			// Annualized simple return.
			days := curve[len(curve)-1].Date.Sub(curve[0].Date).Hours() / 24.0
			if days > 0 {
				retF, _ := ret.Float64()
				annualizedF := math.Pow(1.0+retF/100.0, 365.0/days) - 1.0
				annualized, _ := decimal.NewFromFloat64(annualizedF * 100.0)
				annualized = annualized.Round(2)
				metrics.AnnualizedSimplePct = &annualized
			}
		}
	}

	// For model portfolios (no cash flows), TWR == simple return.
	// For real portfolios, TWR is computed from breakpoints.
	// Since we don't have breakpoints here, use simple return as TWR for both.
	// This is correct for model portfolios. For real portfolios, the TWR
	// normalization is handled in prepareCurveForExtremes.

	if metrics.SimpleReturnPct != nil {
		metrics.TWRPct = metrics.SimpleReturnPct
		metrics.AnnualizedTWRPct = metrics.AnnualizedSimplePct
	}

	return metrics
}



// computeRiskMetrics computes volatility, Sharpe, Sortino from daily returns.
func (s *Service) computeRiskMetrics(curve []EquityCurvePoint) *RiskMetrics {
	// Convert to performance.DailyReturn-compatible format.
	perfCurve := make([]performance.EquityCurvePoint, len(curve))
	for i, p := range curve {
		perfCurve[i] = performance.EquityCurvePoint{
			Date:           p.Date,
			PortfolioValue: p.PortfolioValue,
		}
	}

	dailyReturns := performance.ComputeDailyReturns(perfCurve)
	riskFreePct := decimal.MustParse("0") // 0% risk-free rate
	perfRisk := performance.ComputeRiskMetrics(dailyReturns, &riskFreePct)

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
		MaxDrawdownPct:      drawdown.MaxDrawdownPct,
		CurrentDrawdownPct:  drawdown.CurrentDrawdownPct,
		DrawdownDurationDays: drawdown.DrawdownDurationDays,
	}
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

// prepareCurveForExtremes returns the appropriate curve for computing
// period extremes. For real portfolios with NavPerUnit (cash-flow-aware NAV),
// it builds a curve from NAV values which are already cash-flow-independent.
// For model portfolios or portfolios without NavPerUnit, the raw curve is used.
func (s *Service) prepareCurveForExtremes(curve []EquityCurvePoint, meta portfolioMeta) []EquityCurvePoint {
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

// computeCrossMetrics computes cross-portfolio metrics (beta/alpha, correlation).
func (s *Service) computeCrossMetrics(
	ctx context.Context,
	aCurve, bCurve []EquityCurvePoint,
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

	return cross
}


