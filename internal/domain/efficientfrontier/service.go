package efficientfrontier

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- Interfaces ---

// MarketDataHistorySource fetches historical prices for a symbol.
type MarketDataHistorySource interface {
	GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
}

// MarketDataSymbolResolver maps an internal symbol to its market data provider symbol.
type MarketDataSymbolResolver interface {
	GetMarketDataSymbol(ctx context.Context, internalSymbol string) (string, error)
}

// SymbolLister returns the list of all known internal symbols.
type SymbolLister interface {
	ListAllSymbols(ctx context.Context) ([]string, error)
}

// PortfolioSymbolSource returns the distinct symbols held in a real portfolio.
type PortfolioSymbolSource interface {
	GetSymbolsByPortfolio(ctx context.Context, portfolioID int64) ([]string, error)
}

// ModelPortfolioSource retrieves a model portfolio by ID.
type ModelPortfolioSource interface {
	Get(ctx context.Context, id int64) (ModelPortfolioRef, error)
}

// ModelPortfolioRef is a lightweight reference to a model portfolio's entries.
type ModelPortfolioRef struct {
	Symbols []string
}

// FxRateSource returns the current FX rate between two currencies.
type FxRateSource interface {
	GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*FxRate, error)
}

// FxRate holds a currency exchange rate.
type FxRate struct {
	BaseCurrency  string
	QuoteCurrency string
	Rate          float64
}

// --- Service Result ---

// ServiceResult wraps the engine FrontierResult with service-level metadata.
type ServiceResult struct {
	// Result is the frontier computation output (may have Message for empty states).
	Result *FrontierResult
	// Warnings are non-fatal issues collected during data fetching.
	Warnings []string
	// ExcludedSymbols are symbols that could not be resolved or had no data.
	ExcludedSymbols []string
}

// --- Service ---

// Service orchestrates data fetching and delegates to the computation engine.
type Service struct {
	marketHistory    MarketDataHistorySource
	marketDataSymbol MarketDataSymbolResolver
	symbolLister     SymbolLister
	portfolioSymbols PortfolioSymbolSource
	modelPortfolio   ModelPortfolioSource
	fxRates          FxRateSource
	logger           *slog.Logger
}

// NewService creates a new efficient frontier service.
func NewService(
	marketHistory MarketDataHistorySource,
	marketDataSymbol MarketDataSymbolResolver,
	symbolLister SymbolLister,
	portfolioSymbols PortfolioSymbolSource,
	modelPortfolio ModelPortfolioSource,
	fxRates FxRateSource,
) *Service {
	return &Service{
		marketHistory:    marketHistory,
		marketDataSymbol: marketDataSymbol,
		symbolLister:     symbolLister,
		portfolioSymbols: portfolioSymbols,
		modelPortfolio:   modelPortfolio,
		fxRates:          fxRates,
	}
}

// WithLogger sets the logger for the service.
func (s *Service) WithLogger(logger *slog.Logger) {
	s.logger = logger
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
	var excluded []string

	// Resolve market data symbols and fetch prices.
	pricesBySymbol, resolveWarnings, resolveExcluded := s.fetchPricesForSymbols(ctx, req.Symbols, period)
	warnings = append(warnings, resolveWarnings...)
	excluded = append(excluded, resolveExcluded...)

	// If all symbols were excluded, return empty state.
	if len(pricesBySymbol) == 0 {
		return &ServiceResult{
			Result: &FrontierResult{
				Symbols:    req.Symbols,
				ComputedAt: time.Now().UTC(),
				Message:    "No price data available for any candidate symbol.",
			},
			Warnings:        warnings,
			ExcludedSymbols: excluded,
		}, nil
	}

	// Apply FX conversion if symbols have different currencies.
	baseCurrency := req.BaseCurrency
	if baseCurrency == "" {
		// Use the first symbol's currency as base.
		for _, sym := range req.Symbols {
			if prices, ok := pricesBySymbol[sym]; ok && len(prices) > 0 {
				baseCurrency = prices[0].Currency
				break
			}
		}
	}
	if baseCurrency == "" {
		baseCurrency = "USD"
	}

	fxWarnings := s.convertToBaseCurrency(ctx, pricesBySymbol, baseCurrency)
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

	return &ServiceResult{
		Result:          result,
		Warnings:        warnings,
		ExcludedSymbols: excluded,
	}, nil
}

// GetCandidateSymbols returns all known internal symbols for autocomplete.
func (s *Service) GetCandidateSymbols(ctx context.Context) ([]string, error) {
	if s.symbolLister == nil {
		return []string{}, nil
	}
	symbols, err := s.symbolLister.ListAllSymbols(ctx)
	if err != nil {
		return nil, fmt.Errorf("list symbols: %w", err)
	}
	if symbols == nil {
		symbols = []string{}
	}
	return symbols, nil
}

// GetSymbolsFromPortfolio returns the distinct symbols held in a real portfolio.
func (s *Service) GetSymbolsFromPortfolio(ctx context.Context, portfolioID int64) ([]string, error) {
	if s.portfolioSymbols == nil {
		return []string{}, nil
	}
	symbols, err := s.portfolioSymbols.GetSymbolsByPortfolio(ctx, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("get portfolio symbols: %w", err)
	}
	if symbols == nil {
		symbols = []string{}
	}
	return symbols, nil
}

// GetSymbolsFromModelPortfolio returns the symbols in a model portfolio.
func (s *Service) GetSymbolsFromModelPortfolio(ctx context.Context, modelPortfolioID int64) ([]string, error) {
	if s.modelPortfolio == nil {
		return nil, fmt.Errorf("model portfolio source not configured")
	}
	ref, err := s.modelPortfolio.Get(ctx, modelPortfolioID)
	if err != nil {
		return nil, fmt.Errorf("get model portfolio: %w", err)
	}
	symbols := ref.Symbols
	if symbols == nil {
		symbols = []string{}
	}
	return symbols, nil
}

// fetchPricesForSymbols resolves market data symbols, fetches historical prices,
// and returns prices keyed by internal symbol. Also returns warnings and excluded symbols.
func (s *Service) fetchPricesForSymbols(ctx context.Context, symbols []string, period string) (map[string][]market.HistoricalPrice, []string, []string) {
	prices := make(map[string][]market.HistoricalPrice, len(symbols))
	var warnings, excluded []string

	start, periodWarning := periodCutoff(period)
	end := time.Now().UTC()
	if periodWarning != "" {
		warnings = append(warnings, periodWarning)
	}

	for _, sym := range symbols {
		// Resolve market data symbol.
		marketSymbol := sym
		if s.marketDataSymbol != nil {
			resolved, err := s.marketDataSymbol.GetMarketDataSymbol(ctx, sym)
			if err != nil {
				excluded = append(excluded, sym)
				warnings = append(warnings, fmt.Sprintf("%s: could not resolve market data symbol", sym))
				continue
			}
			marketSymbol = resolved
		}

		// Fetch historical prices.
		if s.marketHistory == nil {
			warnings = append(warnings, "historical price data source not configured")
			excluded = append(excluded, sym)
			continue
		}
		hp, err := s.marketHistory.GetHistoricalPrices(ctx, marketSymbol, start, end)
		if err != nil {
			excluded = append(excluded, sym)
			warnings = append(warnings, fmt.Sprintf("%s: failed to fetch price data", sym))
			continue
		}
		if len(hp) == 0 {
			excluded = append(excluded, sym)
			warnings = append(warnings, fmt.Sprintf("%s: no price data in %s period", sym, period))
			continue
		}

		prices[sym] = hp
	}

	return prices, warnings, excluded
}

// convertToBaseCurrency converts price series to the base currency using cached FX rates.
// Modifies prices in-place. Returns warnings for symbols where FX data was missing.
func (s *Service) convertToBaseCurrency(ctx context.Context, prices map[string][]market.HistoricalPrice, baseCurrency string) []string {
	if s.fxRates == nil || baseCurrency == "" {
		return nil
	}

	var warnings []string
	// Collect unique currencies.
	currencies := make(map[string]struct{})
	for _, hp := range prices {
		for _, p := range hp {
			if p.Currency != "" && p.Currency != baseCurrency {
				currencies[p.Currency] = struct{}{}
			}
		}
	}

	if len(currencies) == 0 {
		return nil
	}

	// Fetch FX rates for each currency pair.
	rates := make(map[string]float64)
	for curr := range currencies {
		fx, err := s.fxRates.GetCurrentFxRate(ctx, curr, baseCurrency)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("FX rate %s/%s: %v", curr, baseCurrency, err))
			continue
		}
		if fx == nil || fx.Rate == 0 {
			warnings = append(warnings, fmt.Sprintf("FX rate %s/%s: no rate available, prices left in %s", curr, baseCurrency, curr))
			continue
		}
		rates[curr] = fx.Rate
	}

	// Convert prices.
	for sym, hp := range prices {
		targetCurrency := ""
		for _, p := range hp {
			if p.Currency != "" {
				targetCurrency = p.Currency
				break
			}
		}
		if targetCurrency == "" || targetCurrency == baseCurrency {
			continue
		}
		rate, ok := rates[targetCurrency]
		if !ok {
			continue // already warned above
		}
		// Convert close prices.
		for i := range hp {
			if hp[i].Currency == targetCurrency {
				closeFloat, ok := hp[i].Close.Float64()
				if !ok {
					warnings = append(warnings, fmt.Sprintf("%s: could not convert close price to float at %s, price left in %s", sym, hp[i].Date.Format("2006-01-02"), targetCurrency))
					continue
				}
				converted := closeFloat * rate
				// Set the converted price and mark currency as base.
				closeDec, err := decimal.NewFromFloat64(converted)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("%s: FX conversion produced invalid value at %s, price left in %s", sym, hp[i].Date.Format("2006-01-02"), targetCurrency))
					continue
				}
				hp[i].Close = closeDec
				hp[i].Currency = baseCurrency
			}
		}
		// Track conversion as a note.
		warnings = append(warnings, fmt.Sprintf("%s: prices converted from %s to %s", sym, targetCurrency, baseCurrency))
	}

	return warnings
}

// periodCutoff returns the start date for the given lookback period string.
func periodCutoff(period string) (time.Time, string) {
	now := time.Now()
	switch period {
	case "3M":
		return now.AddDate(0, -3, 0), ""
	case "6M":
		return now.AddDate(0, -6, 0), ""
	case "1Y":
		return now.AddDate(-1, 0, 0), ""
	case "3Y":
		return now.AddDate(-3, 0, 0), ""
	case "5Y":
		return now.AddDate(-5, 0, 0), ""
	case "10Y":
		return now.AddDate(-10, 0, 0), ""
	default:
		return now.AddDate(-1, 0, 0),
			"unrecognized period " + period + " — defaulting to 1Y"
	}
}

// defaultRiskFreeRate is the default annualized risk-free rate (4.5%).
const defaultRiskFreeRate = 0.045
