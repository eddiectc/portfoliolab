package hierarchicalriskparity

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/util"
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

// ServiceResult wraps the engine HrpResult with service-level metadata.
type ServiceResult struct {
	// Result is the HRP computation output (may have Message for empty states).
	Result *HrpResult
	// Warnings are non-fatal issues collected during data fetching.
	Warnings []string
	// ExcludedSymbols are symbols that could not be resolved or had no data.
	ExcludedSymbols []string
	// SymbolDataSpan is the actual data coverage per symbol.
	// Populated for symbols that made it into the computation.
	SymbolDataSpan map[string]DataSpan
}

// DataSpan describes the actual date range and trading day count for a symbol.
type DataSpan struct {
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	TradingDays     int    `json:"trading_days"`
	RequestedPeriod string `json:"requested_period"`
	ActualPeriod    string `json:"actual_period"`
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

// NewService creates a new HRP service.
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

// ComputeHrp resolves symbols, fetches historical prices, and computes
// the Hierarchical Risk Parity allocations. It returns a ServiceResult with
// HRP data, warnings for partial data, and any excluded symbols.
func (s *Service) ComputeHrp(ctx context.Context, req ComputeHrpRequest) (*ServiceResult, error) {
	period := req.Period
	if period == "" {
		period = "3Y"
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
			Result: &HrpResult{
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
			ExcludedSymbols: excluded,
			SymbolDataSpan:  s.computeDataSpan(pricesBySymbol, period, &warnings),
		}, nil
	}

	// Merge engine warnings with service warnings.
	if result != nil {
		warnings = append(warnings, result.Warnings...)
	}

	// Compute actual data span per symbol (also emits insufficient-data warnings).
	dataSpan := s.computeDataSpan(pricesBySymbol, period, &warnings)

	return &ServiceResult{
		Result:          result,
		Warnings:        warnings,
		ExcludedSymbols: excluded,
		SymbolDataSpan:  dataSpan,
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
// Returns an empty slice if the model portfolio source is not configured
// (consistent with GetCandidateSymbols and GetSymbolsFromPortfolio).
func (s *Service) GetSymbolsFromModelPortfolio(ctx context.Context, modelPortfolioID int64) ([]string, error) {
	if s.modelPortfolio == nil {
		return []string{}, nil
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

	start, periodWarning := util.PeriodCutoff(period)
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
// It modifies the price slices in-place (mutating Close and Currency fields).
// The caller must not reuse the prices map after this call. Returns warnings for
// symbols where FX data was missing.
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

// computeDataSpan returns the actual data coverage per symbol.
// If warnings is non-nil, it appends a warning for each symbol whose data
// span is shorter than ~80% of the expected trading days for the requested period.
func (s *Service) computeDataSpan(pricesBySymbol map[string][]market.HistoricalPrice, requestedPeriod string, warnings *[]string) map[string]DataSpan {
	span := make(map[string]DataSpan, len(pricesBySymbol))
	expectedDays := expectedTradingDays(requestedPeriod)
	for sym, hp := range pricesBySymbol {
		if len(hp) == 0 {
			continue
		}
		span[sym] = DataSpan{
			StartDate:       hp[0].Date.Format("2006-01-02"),
			EndDate:         hp[len(hp)-1].Date.Format("2006-01-02"),
			TradingDays:     len(hp),
			RequestedPeriod: requestedPeriod,
			ActualPeriod:    approximatePeriodLabel(hp[0].Date, hp[len(hp)-1].Date),
		}
		// Warn if data is significantly shorter than requested (~80% threshold).
		if expectedDays > 0 && len(hp) < expectedDays*8/10 {
			*warnings = append(*warnings, fmt.Sprintf("%s: expected ~%d trading days for %s, got %d", sym, expectedDays, requestedPeriod, len(hp)))
		}
	}
	return span
}

// expectedTradingDays returns the approximate number of trading days for a period label.
func expectedTradingDays(period string) int {
	switch period {
	case "1Y":
		return 252
	case "3Y":
		return 756
	case "5Y":
		return 1260
	default:
		// For unrecognized periods, estimate ~252 trading days per year.
		// util.PeriodCutoff already warned; use its fallback (1Y = 365 calendar days ≈ 252 trading).
		return 0
	}
}

// approximatePeriodLabel returns a human-readable label for the date range
// (e.g. "3M", "6M", "1Y", "2Y").
func approximatePeriodLabel(start, end time.Time) string {
	days := int(end.Sub(start).Hours() / 24)
	if days < 30 {
		return fmt.Sprintf("%dD", days)
	}
	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%dM", months)
	}
	years := months / 12
	remainingMonths := months % 12
	if remainingMonths == 0 {
		return fmt.Sprintf("%dY", years)
	}
	return fmt.Sprintf("%dY%dM", years, remainingMonths)
}
