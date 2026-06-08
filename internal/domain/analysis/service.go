package analysis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"codeberg.org/eddiectc/portfoliolab/internal/util"
	"github.com/govalues/decimal"
)

// PositionSource fetches and enriches positions for analysis.
type PositionSource interface {
	GetOpenPositions(ctx context.Context, accountIDs []int64, limit, offset int) ([]position.Position, error)
	EnrichWithMarketData(ctx context.Context, positions []position.Position, baseCurrency string) []position.PositionWithMarket
}

// SymbolDetailsSource retrieves cached symbol details.
type SymbolDetailsSource interface {
	GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbol.SymbolDetails, error)
}

// MarketDataHistorySource fetches historical prices for correlation and factor computation.
type MarketDataHistorySource interface {
	GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
}

// AccountResolver resolves account IDs from portfolio or returns all accounts.
type AccountResolver interface {
	GetAccountsByPortfolio(ctx context.Context, portfolioID int64) ([]position.AccountRef, error)
	GetAllAccounts(ctx context.Context) ([]position.AccountRef, error)
}

// PortfolioCurrencySource returns the base currency of a portfolio.
type PortfolioCurrencySource interface {
	GetPortfolioCurrency(ctx context.Context, portfolioID int64) (string, error)
}

// MarketDataSymbolResolver maps an internal symbol to its market data provider symbol
// (e.g., Yahoo Finance ticker) for fetching historical prices.
type MarketDataSymbolResolver interface {
	GetMarketDataSymbol(ctx context.Context, internalSymbol string) (string, error)
}

// SymbolRefresher triggers a background refresh of symbol details for stale symbols.
type SymbolRefresher interface {
	RefreshSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error
}

// Service orchestrates data fetching and delegates to the computation functions.
// It is the single entry point for all portfolio analysis.
type Service struct {
	positions          PositionSource
	symbolDetails      SymbolDetailsSource
	marketHistory      MarketDataHistorySource
	accounts           AccountResolver
	portfolioCurrency  PortfolioCurrencySource
	marketDataSymbol   MarketDataSymbolResolver
	symbolRefresher    SymbolRefresher
	logger             *slog.Logger
}

// NewService creates a new analysis service.
func NewService(
	positions PositionSource,
	symbolDetails SymbolDetailsSource,
	marketHistory MarketDataHistorySource,
	accounts AccountResolver,
	portfolioCurrency PortfolioCurrencySource,
	marketDataSymbol MarketDataSymbolResolver,
) *Service {
	return &Service{
		positions:       positions,
		symbolDetails:   symbolDetails,
		marketHistory:   marketHistory,
		accounts:        accounts,
		portfolioCurrency: portfolioCurrency,
		marketDataSymbol:  marketDataSymbol,
	}
}

// WithSymbolRefresher sets the optional symbol refresher for triggering
// background refreshes of stale symbol details. If nil, refresh is skipped.
func (s *Service) WithSymbolRefresher(refresher SymbolRefresher) {
	s.symbolRefresher = refresher
}

// WithLogger sets the logger for the service.
func (s *Service) WithLogger(logger *slog.Logger) {
	s.logger = logger
}

// ComputeAnalysis computes the full portfolio analysis result.
//
// It resolves account IDs from filters, fetches open positions, enriches them
// with market data, fetches symbol details and historical prices, then computes
// each analytical section (overlap, correlation, allocation, stress test, factor
// exposure). Warnings from all sections are collected in the result.
//
// If filters.Section is set, only that section is computed. If filters.Period
// is empty, "1Y" is used for the correlation lookback.
func (s *Service) ComputeAnalysis(ctx context.Context, filters AnalysisFilters) (*AnalysisResult, error) {
	// Resolve account IDs.
	accountIDs, baseCurrency, resolveWarnings, err := s.resolveAccounts(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("resolve accounts: %w", err)
	}

	// Fetch open positions (unbounded — analysis needs all positions).
	const fetchLimit = 10000
	positions, err := s.positions.GetOpenPositions(ctx, accountIDs, fetchLimit, 0)
	if err != nil {
		return nil, fmt.Errorf("fetch open positions: %w", err)
	}

	// Empty state: no positions.
	if len(positions) == 0 {
		return &AnalysisResult{
			PortfolioID: getPortfolioID(filters),
			ComputedAt:  time.Now().UTC(),
			Message:     "No open positions found. Analysis requires at least one open position.",
		}, nil
	}

	// Filter out cash positions — they have no market data, symbol details,
	// or price history and would only generate spurious warnings.
	filtered := filterCashPositions(positions)
	if len(filtered) == 0 {
		return &AnalysisResult{
			PortfolioID: getPortfolioID(filters),
			ComputedAt:  time.Now().UTC(),
			Message:     "No investable positions found (only cash). Analysis requires at least one stock or ETF position.",
		}, nil
	}

	// Collect warnings from data fetching.
	dataWarnings := resolveWarnings

	// Enrich positions with market data.
	enriched := s.positions.EnrichWithMarketData(ctx, filtered, baseCurrency)

	// Check if any positions lack market data.
	missingMarketData := 0
	for _, p := range enriched {
		if !p.MarketDataAvailable {
			missingMarketData++
		}
	}
	if missingMarketData > 0 {
		dataWarnings = append(dataWarnings, fmt.Sprintf("market data unavailable for %d of %d positions", missingMarketData, len(enriched)))
	}

	// Compute total portfolio value for stress tests.
	totalPortfolioValue := s.computeTotalPortfolioValue(enriched)

	// Fetch symbol details for each unique symbol.
	symbolDetailsMap, detailsWarnings := s.fetchSymbolDetails(ctx, enriched)
	dataWarnings = append(dataWarnings, detailsWarnings...)

	// Build PositionWithDetails slice.
	positionDetails, buildWarnings := s.buildPositionWithDetails(enriched, symbolDetailsMap)
	dataWarnings = append(dataWarnings, buildWarnings...)

	// Fetch historical prices for correlation and time-series factors.
	period := filters.Period
	if period == "" {
		period = "1Y"
	}
	pricesBySymbol, pricesWarnings := s.fetchHistoricalPrices(ctx, enriched, period)
	dataWarnings = append(dataWarnings, pricesWarnings...)

	// Trigger background refresh for stale symbol details.
	s.refreshStaleSymbols(ctx, enriched, symbolDetailsMap)

	// Compute sections.
	result := &AnalysisResult{
		PortfolioID: getPortfolioID(filters),
		ComputedAt:  time.Now().UTC(),
		Warnings:    dataWarnings,
	}

	wantAll := filters.Section == ""

	if wantAll || filters.Section == string(SectionOverlap) {
		result.Overlap = ComputeOverlap(positionDetails)
		result.Warnings = append(result.Warnings, result.Overlap.Warnings...)
	}

	if wantAll || filters.Section == string(SectionCorrelation) {
		result.Correlation = ComputeCorrelation(pricesBySymbol, period)
		result.Warnings = append(result.Warnings, result.Correlation.Warnings...)
	}

	if wantAll || filters.Section == string(SectionSectorAllocation) {
		result.SectorAllocation = ComputeSectorAllocation(positionDetails)
		result.Warnings = append(result.Warnings, result.SectorAllocation.Warnings...)
	}

	if wantAll || filters.Section == string(SectionGeographicAllocation) {
		result.GeographicAllocation = ComputeGeographicAllocation(positionDetails)
		result.Warnings = append(result.Warnings, result.GeographicAllocation.Warnings...)
	}

	if wantAll || filters.Section == string(SectionStressTest) {
		sectorAlloc := result.SectorAllocation
		if sectorAlloc == nil {
			// Compute sector allocation even if not requested, needed for stress tests.
			sectorAlloc = ComputeSectorAllocation(positionDetails)
		}
		result.StressTest = ComputeStressTests(sectorAlloc, totalPortfolioValue)
		result.Warnings = append(result.Warnings, result.StressTest.Warnings...)
	}

	if wantAll || filters.Section == string(SectionFactorExposure) {
		result.FactorExposure = ComputeFactorExposure(positionDetails, pricesBySymbol)
		result.Warnings = append(result.Warnings, result.FactorExposure.Warnings...)
	}

	return result, nil
}

// resolveAccounts resolves account IDs and base currency from filters.
// Returns account IDs, base currency, any warnings, and an error if accounts
// could not be resolved.
func (s *Service) resolveAccounts(ctx context.Context, filters AnalysisFilters) ([]int64, string, []string, error) {
	if filters.PortfolioID != nil {
		accounts, err := s.accounts.GetAccountsByPortfolio(ctx, *filters.PortfolioID)
		if err != nil {
			return nil, "", nil, fmt.Errorf("get accounts for portfolio %d: %w", *filters.PortfolioID, err)
		}
		if len(accounts) == 0 {
			return []int64{}, "", nil, nil
		}
		ids := make([]int64, len(accounts))
		for i, a := range accounts {
			ids[i] = a.ID
		}
		// Base currency from the portfolio.
		baseCurrency, err := s.portfolioCurrency.GetPortfolioCurrency(ctx, *filters.PortfolioID)
		if err != nil {
			return ids, "", []string{"could not determine portfolio base currency — FX conversion skipped"}, nil
		}
		return ids, baseCurrency, nil, nil
	}

	// No portfolio filter → all accounts.
	accounts, err := s.accounts.GetAllAccounts(ctx)
	if err != nil {
		return nil, "", nil, fmt.Errorf("list all accounts: %w", err)
	}
	if len(accounts) == 0 {
		return []int64{}, "", nil, nil
	}
	ids := make([]int64, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	// When no portfolio filter, try to get base currency from first account's portfolio.
	var baseCurrency string
	var warnings []string
	if s.portfolioCurrency != nil && accounts[0].PortfolioID != 0 {
		baseCurrency, err = s.portfolioCurrency.GetPortfolioCurrency(ctx, accounts[0].PortfolioID)
		if err != nil {
			warnings = append(warnings, "could not determine portfolio base currency — FX conversion skipped")
		}
	}
	return ids, baseCurrency, warnings, nil
}

// computeTotalPortfolioValue sums market values across all enriched positions.
func (s *Service) computeTotalPortfolioValue(enriched []position.PositionWithMarket) decimal.Decimal {
	var total decimal.Decimal
	for _, p := range enriched {
		if !p.MarketDataAvailable {
			continue
		}
		// Use MarketValueBase if available (converted to base currency),
		// otherwise use MarketValue in position currency.
		if p.MarketValueBase != nil {
			total, _ = total.Add(*p.MarketValueBase)
		} else if !p.MarketValue.IsZero() {
			total, _ = total.Add(p.MarketValue)
		}
	}
	return total
}

// fetchSymbolDetails fetches cached symbol details for each unique symbol
// in the enriched positions. Symbols missing from cache produce warnings.
// Returns the details map and any warnings.
func (s *Service) fetchSymbolDetails(ctx context.Context, enriched []position.PositionWithMarket) (map[string]*symbol.SymbolDetails, []string) {
	if s.symbolDetails == nil {
		return map[string]*symbol.SymbolDetails{}, []string{"symbol details source not configured"}
	}

	// Collect unique symbols.
	symbolSet := make(map[string]struct{})
	for _, p := range enriched {
		symbolSet[p.Symbol] = struct{}{}
	}

	details := make(map[string]*symbol.SymbolDetails, len(symbolSet))
	var missing []string
	for sym := range symbolSet {
		sd, err := s.symbolDetails.GetByInternalSymbol(ctx, sym)
		if err != nil {
			missing = append(missing, sym)
			continue
		}
		details[sym] = sd
	}
	var warnings []string
	if len(missing) > 0 {
		warnings = append(warnings, fmt.Sprintf("symbol details not cached for %d symbol(s): %s", len(missing), joinStrings(missing, ", ")))
	}
	return details, warnings
}

// buildPositionWithDetails constructs the analysis input from enriched positions
// and fetched symbol details. Portfolio weight is position market value / total portfolio value.
// Returns the position details and any warnings.
func (s *Service) buildPositionWithDetails(enriched []position.PositionWithMarket, details map[string]*symbol.SymbolDetails) ([]PositionWithDetails, []string) {
	// Compute total portfolio value.
	var totalMV decimal.Decimal
	for _, p := range enriched {
		if !p.MarketDataAvailable {
			continue
		}
		if p.MarketValueBase != nil {
			totalMV, _ = totalMV.Add(*p.MarketValueBase)
		} else if !p.MarketValue.IsZero() {
			totalMV, _ = totalMV.Add(p.MarketValue)
		}
	}

	if totalMV.IsZero() {
		return []PositionWithDetails{}, []string{"no market data available for any position — analysis results will be empty"}
	}

	// Convert total to float64 for percentage computation.
	totalFloat, _ := totalMV.Float64()
	if totalFloat == 0 {
		return []PositionWithDetails{}, []string{"total portfolio value underflows float64 — analysis results will be empty"}
	}

	var warnings []string
	result := make([]PositionWithDetails, 0, len(enriched))
	for _, p := range enriched {
		if !p.MarketDataAvailable {
			continue
		}

		// Portfolio weight as percentage (0-100).
		var mvFloat float64
		var mvOK bool
		if p.MarketValueBase != nil {
			mvFloat, mvOK = p.MarketValueBase.Float64()
		} else {
			mvFloat, mvOK = p.MarketValue.Float64()
		}
		if !mvOK {
			warnings = append(warnings, p.Symbol+": market value conversion failed — excluded from weighted calculations")
			continue
		}

		weightPct := (mvFloat / totalFloat) * 100.0

		result = append(result, PositionWithDetails{
			Symbol:          p.Symbol,
			PortfolioWeight: weightPct,
			SymbolDetails:   details[p.Symbol],
		})
	}

	return result, warnings
}

// fetchHistoricalPrices fetches historical prices for each unique symbol,
// mapped to their market data provider symbol. Returns prices keyed by
// internal symbol for consumption by ComputeCorrelation and ComputeFactorExposure,
// plus any warnings about missing data.
func (s *Service) fetchHistoricalPrices(ctx context.Context, enriched []position.PositionWithMarket, period string) (map[string][]market.HistoricalPrice, []string) {
	if s.marketHistory == nil || s.marketDataSymbol == nil {
		return map[string][]market.HistoricalPrice{}, []string{"historical price data source not configured — correlation and time-series factors unavailable"}
	}

	// Collect unique symbols.
	symbolSet := make(map[string]struct{})
	for _, p := range enriched {
		symbolSet[p.Symbol] = struct{}{}
	}

	// Determine date range from period.
	cutoff, _ := util.PeriodCutoff(period)
	now := time.Now().UTC()

	// Factor exposure (momentum/volatility) needs at least 12M of price data
	// regardless of the selected period. Use the earlier of the two cutoffs.
	minCutoff := now.AddDate(0, 0, -365)
	if cutoff.Before(minCutoff) {
		minCutoff = cutoff
	}

	prices := make(map[string][]market.HistoricalPrice, len(symbolSet))
	var missingSymbols []string

	for sym := range symbolSet {
		// Resolve market data symbol.
		marketSymbol, err := s.marketDataSymbol.GetMarketDataSymbol(ctx, sym)
		if err != nil {
			missingSymbols = append(missingSymbols, sym)
			continue
		}

		// Fetch historical prices.
		hp, err := s.marketHistory.GetHistoricalPrices(ctx, marketSymbol, minCutoff, now)
		if err != nil {
			missingSymbols = append(missingSymbols, sym)
			continue
		}

		if len(hp) > 0 {
			prices[sym] = hp
		} else {
			missingSymbols = append(missingSymbols, sym)
		}
	}

	var warnings []string
	if len(missingSymbols) > 0 {
		warnings = append(warnings, fmt.Sprintf("no historical price data for %d symbol(s): %s", len(missingSymbols), joinStrings(missingSymbols, ", ")))
	}
	return prices, warnings
}

// refreshStaleSymbols triggers a background refresh for symbols whose cached
// details are older than the stale threshold (7 days).
func (s *Service) refreshStaleSymbols(ctx context.Context, enriched []position.PositionWithMarket, details map[string]*symbol.SymbolDetails) {
	if s.symbolRefresher == nil {
		return
	}

	staleThreshold := 7 * 24 * time.Hour
	now := time.Now().UTC()

	for _, p := range enriched {
		sd := details[p.Symbol]
		if sd == nil || sd.FetchedAt.IsZero() {
			continue
		}
		if now.Sub(sd.FetchedAt) > staleThreshold {
			// Resolve market data symbol for refresh.
			marketSymbol, err := s.marketDataSymbol.GetMarketDataSymbol(ctx, p.Symbol)
			if err != nil {
				continue
			}
			// Non-blocking: fire and forget in background.
			go func(internal, market string) {
				_ = s.symbolRefresher.RefreshSymbol(context.Background(), internal, market)
			}(p.Symbol, marketSymbol)
		}
	}
}

// filterCashPositions removes cash positions (symbols starting with '$')
// that have no market data, symbol details, or price history.
func filterCashPositions(positions []position.Position) []position.Position {
	filtered := make([]position.Position, 0, len(positions))
	for _, p := range positions {
		if p.Symbol != "" && p.Symbol[0] == '$' {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

func getPortfolioID(filters AnalysisFilters) int64 {
	if filters.PortfolioID != nil {
		return *filters.PortfolioID
	}
	return 0
}

// joinStrings joins strings with a separator, limiting output length.
func joinStrings(items []string, sep string) string {
	if len(items) <= 5 {
		result := ""
		for i, s := range items {
			if i > 0 {
				result += sep
			}
			result += s
		}
		return result
	}
	result := ""
	for i, s := range items[:5] {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return fmt.Sprintf("%s (%d more)", result, len(items)-5)
}
