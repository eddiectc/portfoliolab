package hierarchicalriskparity

import (
	"context"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/optimization"
)

// --- Service ---

// Service orchestrates data fetching and delegates to the computation engine.
type Service struct {
	*optimization.BaseService
}

// NewService creates a new HRP service.
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

// ComputeHrpRequest holds the input for the service-level HRP computation.
type ComputeHrpRequest struct {
	// Symbols is the list of candidate internal symbol names.
	Symbols []string
	// Period is the lookback period (e.g. "1Y", "3Y", "5Y").
	Period string
	// BaseCurrency is the target currency for price conversion.
	// If empty, the first symbol's currency is used as base.
	BaseCurrency string
}

// ServiceResult wraps the engine HrpResult with service-level metadata.
type ServiceResult struct {
	// Result is the HRP computation output (may have Message for empty states).
	Result *HrpResult
	// Warnings are non-fatal issues collected during data fetching.
	Warnings []string
	// ExcludedSymbols are symbols that could not be resolved or had no data.
	ExcludedSymbols []string
	// SymbolDataSpan is the actual data coverage per symbol.
	SymbolDataSpan map[string]optimization.DataSpan
}

// ComputeHrp resolves symbols, fetches historical prices, and computes
// the Hierarchical Risk Parity allocations. It returns a ServiceResult with
// HRP data, warnings for partial data, and any excluded symbols.
func (s *Service) ComputeHrp(ctx context.Context, req ComputeHrpRequest) (*ServiceResult, error) {
	period := req.Period
	if period == "" {
		period = "3Y"
	}

	var warnings []string

	// Resolve market data symbols and fetch prices.
	pricesBySymbol, resolveWarnings, resolveExcluded := s.FetchPrices(ctx, req.Symbols, period)
	warnings = append(warnings, resolveWarnings...)

	// If all symbols were excluded, return empty state.
	if len(pricesBySymbol) == 0 {
		return &ServiceResult{
			Result: &HrpResult{
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

	// Build engine request — preserve original symbol order (filtered to symbols with data).
	engineReq := HrpRequest{
		Symbols: make([]string, 0, len(pricesBySymbol)),
		Prices:  pricesBySymbol,
		Period:  period,
	}
	for _, sym := range req.Symbols {
		if _, ok := pricesBySymbol[sym]; ok {
			engineReq.Symbols = append(engineReq.Symbols, sym)
		}
	}

	// Delegate to computation engine.
	// Engine errors (e.g. insufficient symbols after filtering, insufficient data)
	// are converted to empty-state results so the UI can display a message.
	result, engineErr := ComputeHrp(engineReq)
	if engineErr != nil {
		warnings = append(warnings, engineErr.Error())
		return &ServiceResult{
			Result: &HrpResult{
				Symbols:    engineReq.Symbols,
				ComputedAt: time.Now().UTC(),
				Message:    engineErr.Error(),
			},
			Warnings:        warnings,
			ExcludedSymbols: resolveExcluded,
			SymbolDataSpan:  optimization.ComputeDataSpan(pricesBySymbol, period, &warnings),
		}, nil
	}

	// Merge engine warnings with service warnings.
	if result != nil {
		warnings = append(warnings, result.Warnings...)
	}

	// Compute actual data span per symbol (also emits insufficient-data warnings).
	dataSpan := optimization.ComputeDataSpan(pricesBySymbol, period, &warnings)

	return &ServiceResult{
		Result:          result,
		Warnings:        warnings,
		ExcludedSymbols: resolveExcluded,
		SymbolDataSpan:  dataSpan,
	}, nil
}
