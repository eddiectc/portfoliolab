package efficientfrontier

import (
	"context"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/optimization"
)

// --- Service ---

// Service orchestrates data fetching and delegates to the computation engine.
type Service struct {
	*optimization.BaseService
}

// NewService creates a new efficient frontier service.
func NewService(
	marketHistory optimization.MarketDataHistorySource,
	marketDataSymbol optimization.MarketDataSymbolResolver,
	symbolLister optimization.SymbolLister,
	portfolioSymbols optimization.PortfolioSymbolSource,
	modelPortfolio optimization.ModelPortfolioSource,
	fxRates optimization.FxRateSource,
) *Service {
	return &Service{
		BaseService: optimization.NewBaseService(
			marketHistory,
			marketDataSymbol,
			symbolLister,
			portfolioSymbols,
			modelPortfolio,
			fxRates,
		),
	}
}

// ComputeFrontierRequest holds the input for the service-level frontier computation.
type ComputeFrontierRequest struct {
	// Symbols is the list of candidate internal symbol names.
	Symbols []string
	// Period is the lookback period (e.g. "1Y", "3Y", "5Y").
	Period string
	// RiskFreeRate is the annualized risk-free rate as a decimal (e.g. 0.045).
	RiskFreeRate float64
	// BaseCurrency is the target currency for price conversion.
	// If empty, the first symbol's currency is used as base.
	BaseCurrency string
}

// ServiceResult wraps the engine FrontierResult with service-level metadata.
type ServiceResult struct {
	// Result is the frontier computation output (may have Message for empty states).
	Result *FrontierResult
	// Warnings are non-fatal issues collected during data fetching.
	Warnings []string
	// ExcludedSymbols are symbols that could not be resolved or had no data.
	ExcludedSymbols []string
	// SymbolDataSpan is the actual data coverage per symbol.
	// Populated for symbols that made it into the computation.
	SymbolDataSpan map[string]optimization.DataSpan
}

// ComputeFrontier resolves symbols, fetches historical prices, and computes
// the efficient frontier. It returns a ServiceResult with frontier data,
// warnings for partial data, and any excluded symbols.
func (s *Service) ComputeFrontier(ctx context.Context, req ComputeFrontierRequest) (*ServiceResult, error) {
	period := req.Period
	if period == "" {
		period = "1Y"
	}
	riskFreeRate := req.RiskFreeRate
	if riskFreeRate <= 0 {
		riskFreeRate = defaultRiskFreeRate
	}

	var warnings []string

	// Resolve market data symbols and fetch prices.
	pricesBySymbol, resolveWarnings, resolveExcluded := s.FetchPrices(ctx, req.Symbols, period)
	warnings = append(warnings, resolveWarnings...)

	// If all symbols were excluded, return empty state.
	if len(pricesBySymbol) == 0 {
		return &ServiceResult{
			Result: &FrontierResult{
				Symbols:    req.Symbols,
				ComputedAt: time.Now().UTC(),
				Message:    "No price data available for any candidate symbol.",
			},
			Warnings:        warnings,
			ExcludedSymbols: resolveExcluded,
		}, nil
	}

	// Apply FX conversion if symbols have different currencies.
	baseCurrency := optimization.ResolveBaseCurrency(req.BaseCurrency, pricesBySymbol, req.Symbols)
	fxWarnings := s.ConvertToBaseCurrency(ctx, pricesBySymbol, baseCurrency)
	warnings = append(warnings, fxWarnings...)

	// Build engine request.
	engineReq := FrontierRequest{
		Symbols:      make([]string, 0, len(pricesBySymbol)),
		Prices:       pricesBySymbol,
		Period:       period,
		RiskFreeRate: riskFreeRate,
	}
	for sym := range pricesBySymbol {
		engineReq.Symbols = append(engineReq.Symbols, sym)
	}

	// Delegate to computation engine.
	result, err := ComputeFrontier(engineReq)
	if err != nil {
		return nil, err
	}

	// Merge engine warnings with service warnings.
	if result != nil {
		warnings = append(warnings, result.Warnings...)
	}

	// Compute actual data span per symbol.
	dataSpan := optimization.ComputeDataSpan(pricesBySymbol, period, nil)

	return &ServiceResult{
		Result:          result,
		Warnings:        warnings,
		ExcludedSymbols: resolveExcluded,
		SymbolDataSpan:  dataSpan,
	}, nil
}

// defaultRiskFreeRate is the default annualized risk-free rate (4.5%).
const defaultRiskFreeRate = 0.045
