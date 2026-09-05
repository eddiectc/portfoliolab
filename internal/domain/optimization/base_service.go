package optimization

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/eddiectc/portfoliolab/internal/util"
	"github.com/govalues/decimal"
)

// BaseService provides shared data-fetching logic for portfolio optimization.
// It resolves symbols, fetches historical prices, converts to a base currency,
// and computes data spans. Domain-specific computation delegates to the
// efficientfrontier or hierarchicalriskparity packages.
type BaseService struct {
	marketHistory    MarketDataHistorySource
	marketDataSymbol MarketDataSymbolResolver
	symbolLister     SymbolLister
	portfolioSymbols PortfolioSymbolSource
	modelPortfolio   ModelPortfolioSource
	fxRates          FxRateSource
	logger           *slog.Logger
}

// NewBaseService creates a new base optimization service.
func NewBaseService(
	marketHistory MarketDataHistorySource,
	marketDataSymbol MarketDataSymbolResolver,
	symbolLister SymbolLister,
	portfolioSymbols PortfolioSymbolSource,
	modelPortfolio ModelPortfolioSource,
	fxRates FxRateSource,
) *BaseService {
	return &BaseService{
		marketHistory:    marketHistory,
		marketDataSymbol: marketDataSymbol,
		symbolLister:     symbolLister,
		portfolioSymbols: portfolioSymbols,
		modelPortfolio:   modelPortfolio,
		fxRates:          fxRates,
	}
}

// WithLogger sets the logger for the service.
func (s *BaseService) WithLogger(logger *slog.Logger) {
	s.logger = logger
}

// FetchPrices resolves market data symbols, fetches historical prices,
// and returns prices keyed by internal symbol. Also returns warnings and excluded symbols.
func (s *BaseService) FetchPrices(ctx context.Context, symbols []string, period string) (map[string][]market.HistoricalPrice, []string, []string) {
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

// ConvertToBaseCurrency converts price series to the base currency using cached FX rates.
// It modifies the price slices in-place (mutating Close and Currency fields).
// The caller must not reuse the prices map after this call. Returns warnings for
// symbols where FX data was missing.
func (s *BaseService) ConvertToBaseCurrency(ctx context.Context, prices map[string][]market.HistoricalPrice, baseCurrency string) []string {
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

// ResolveBaseCurrency determines the base currency from the first symbol's
// price data, falling back to the requested base currency, then USD.
func ResolveBaseCurrency(baseCurrency string, prices map[string][]market.HistoricalPrice, symbols []string) string {
	if baseCurrency != "" {
		return baseCurrency
	}
	// Use the first symbol's currency as base.
	for _, sym := range symbols {
		if prices, ok := prices[sym]; ok && len(prices) > 0 {
			if prices[0].Currency != "" {
				return prices[0].Currency
			}
		}
	}
	return "USD"
}

// GetCandidateSymbols returns all known internal symbols for autocomplete.
func (s *BaseService) GetCandidateSymbols(ctx context.Context) ([]string, error) {
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
func (s *BaseService) GetSymbolsFromPortfolio(ctx context.Context, portfolioID int64) ([]string, error) {
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
func (s *BaseService) GetSymbolsFromModelPortfolio(ctx context.Context, modelPortfolioID int64) ([]string, error) {
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

// ComputeDataSpan returns the actual data coverage per symbol.
// If warnings is non-nil, it appends a warning for each symbol whose data
// span is shorter than ~80% of the expected trading days for the requested period.
func ComputeDataSpan(pricesBySymbol map[string][]market.HistoricalPrice, requestedPeriod string, warnings *[]string) map[string]DataSpan {
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
			if warnings != nil {
				*warnings = append(*warnings, fmt.Sprintf("%s: expected ~%d trading days for %s, got %d", sym, expectedDays, requestedPeriod, len(hp)))
			}
		}
	}
	return span
}

// expectedTradingDays returns the approximate number of trading days for a period label.
func expectedTradingDays(period string) int {
	switch period {
	case "3M":
		return 63
	case "6M":
		return 126
	case "1Y":
		return 252
	case "3Y":
		return 756
	case "5Y":
		return 1260
	case "10Y":
		return 2520
	default:
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
