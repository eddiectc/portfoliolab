package symbols

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// ErrNotFound indicates no cached symbol details exist for the requested symbol.
var ErrNotFound = errors.New("symbol details not found")

// StaleThreshold is the duration after which cached symbol details are
// considered stale and eligible for background refresh.
const StaleThreshold = 7 * 24 * time.Hour

// SymbolDetailsRepository persists and retrieves cached symbol details.
type SymbolDetailsRepository interface {
	Upsert(ctx context.Context, details *symbol.SymbolDetails) error
	GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbol.SymbolDetails, error)
	ListStale(ctx context.Context, olderThan time.Time) ([]symbol.StaleSymbol, error)
}

// DataSourceURLSource looks up and updates the data_source_url for a symbol.
type DataSourceURLSource interface {
	// GetDataSourceURLByInternalSymbol returns the data_source_url for a symbol.
	// Returns empty string and nil error if no URL is configured.
	// Returns (nil, error) if the symbol mapping doesn't exist.
	GetDataSourceURLByInternalSymbol(ctx context.Context, internalSymbol string) (string, int64, error)
	// UpdateDataSourceURL sets the data_source_url for a symbol mapping by ID.
	UpdateDataSourceURL(ctx context.Context, id int64, url string) error
}

// MarketDataRepository stores NAV history data points.
type MarketDataRepository interface {
	Upsert(ctx context.Context, m *market.MarketData) error
}

// Service orchestrates fetching, caching, and retrieval of symbol details.
type Service struct {
	repo              SymbolDetailsRepository
	fetcher           market.SymbolDetailsFetcher
	dispatcher        *extractor.Dispatcher
	dataSourceURLRepo DataSourceURLSource
	marketDataRepo    MarketDataRepository
}

// NewService creates a new symbol details service.
func NewService(repo SymbolDetailsRepository, fetcher market.SymbolDetailsFetcher) *Service {
	return &Service{repo: repo, fetcher: fetcher}
}

// WithExtractorDispatcher sets the extractor dispatcher for URL-based
// symbol details routing. When a symbol has a data_source_url configured,
// the dispatcher routes the fetch to the appropriate extractor instead of
// Yahoo Finance.
func (s *Service) WithExtractorDispatcher(dispatcher *extractor.Dispatcher) *Service {
	s.dispatcher = dispatcher
	return s
}

// WithDataSourceURLRepo sets the data source URL source for looking up
// and updating data_source_url on symbols.
func (s *Service) WithDataSourceURLRepo(repo DataSourceURLSource) *Service {
	s.dataSourceURLRepo = repo
	return s
}

// WithMarketDataRepo sets the market data repository for storing NAV history.
func (s *Service) WithMarketDataRepo(repo MarketDataRepository) *Service {
	s.marketDataRepo = repo
	return s
}

// FetchAndStore fetches symbol details from the market data provider and
// stores them in the cache. Returns an error if the fetch or storage fails.
// If the symbol has a data_source_url configured and an extractor dispatcher
// is available, it routes through the extractor instead of Yahoo Finance.
func (s *Service) FetchAndStore(ctx context.Context, internalSymbol, marketDataSymbol string) error {
	details, navHistory, source, err := s.fetchDetails(ctx, internalSymbol, marketDataSymbol)
	if err != nil {
		return fmt.Errorf("fetch symbol details for %s: %w", internalSymbol, err)
	}

	// Store symbol details.
	if err := s.repo.Upsert(ctx, details); err != nil {
		return fmt.Errorf("store symbol details for %s: %w", internalSymbol, err)
	}

	// Store NAV history (if any and market data repo available).
	if len(navHistory) > 0 && s.marketDataRepo != nil {
		if err := s.storeNavHistory(ctx, internalSymbol, details.Currency, navHistory, source); err != nil {
			return fmt.Errorf("store NAV history for %s: %w", internalSymbol, err)
		}
	}

	return nil
}

// fetchDetails fetches symbol details either through the extractor dispatcher
// (when a data_source_url is configured) or through Yahoo Finance (default).
// Returns the details, any NAV history points, and an error.
func (s *Service) fetchDetails(ctx context.Context, internalSymbol, marketDataSymbol string) (*symbol.SymbolDetails, []extractor.NavPoint, string, error) {
	// Check if there's a data_source_url configured for this symbol.
	sourceURL := ""
	if s.dataSourceURLRepo != nil {
		url, _, err := s.dataSourceURLRepo.GetDataSourceURLByInternalSymbol(ctx, internalSymbol)
		if err == nil {
			sourceURL = url
		}
	}

	// Route through extractor if URL is configured and dispatcher available.
	if sourceURL != "" && s.dispatcher != nil {
		result, err := s.dispatcher.Dispatch(ctx, sourceURL)
		if err != nil {
			return nil, nil, "", fmt.Errorf("extract from %s: %w", sourceURL, err)
		}
		details := extractResultToSymbolDetails(result, internalSymbol)
		return details, result.NavHistory, result.Source, nil
	}

	// Default: Yahoo Finance.
	details, err := s.fetcher.FetchSymbolDetails(ctx, marketDataSymbol)
	if err != nil {
		return nil, nil, "", err
	}
	details.InternalSymbol = internalSymbol
	return details, nil, "yahoo", nil
}

// storeNavHistory stores NAV data points in the market_data table.
func (s *Service) storeNavHistory(ctx context.Context, internalSymbol, currency string, navPoints []extractor.NavPoint, source string) error {
	now := time.Now()
	for _, np := range navPoints {
		md := &market.MarketData{
			Symbol:    internalSymbol,
			Price:     np.NAV,
			Currency:  currency,
			DataType:  "nav",
			Source:    source,
			Date:      np.Date,
			FetchedAt: now,
		}
		if err := s.marketDataRepo.Upsert(ctx, md); err != nil {
			return fmt.Errorf("upsert NAV for %s on %s: %w", internalSymbol, np.Date, err)
		}
	}
	return nil
}

// GetByInternalSymbol retrieves cached symbol details for a symbol.
// Returns an error (wrapping ErrNotFound) if no details are cached.
func (s *Service) GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbol.SymbolDetails, error) {
	details, err := s.repo.GetByInternalSymbol(ctx, internalSymbol)
	if err != nil {
		return nil, fmt.Errorf("get symbol details for %s: %w", internalSymbol, err)
	}
	return details, nil
}

// GetStaleSymbols returns symbols whose cached details are older than the
// stale threshold (7 days). Returns internal_symbol + market_data_symbol pairs
// suitable for refresh. Cash symbols ($CASH*) are excluded since they have
// no market data to fetch.
func (s *Service) GetStaleSymbols(ctx context.Context) ([]symbol.StaleSymbol, error) {
	olderThan := time.Now().Add(-StaleThreshold)
	stale, err := s.repo.ListStale(ctx, olderThan)
	if err != nil {
		return nil, fmt.Errorf("list stale symbol details: %w", err)
	}
	var filtered []symbol.StaleSymbol
	for _, s := range stale {
		if strings.HasPrefix(s.InternalSymbol, "$CASH") {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered, nil
}

// RefreshSymbol re-fetches and updates the cached details for a single symbol.
// Routes through the extractor dispatcher if a data_source_url is configured.
// Returns an error if the fetch or storage fails.
func (s *Service) RefreshSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error {
	return s.FetchAndStore(ctx, internalSymbol, marketDataSymbol)
}

// GetDataSourceURL retrieves the configured data source URL for a symbol.
// Returns empty string if no URL is configured (uses default Yahoo Finance).
func (s *Service) GetDataSourceURL(ctx context.Context, internalSymbol string) (string, error) {
	if s.dataSourceURLRepo == nil {
		return "", nil
	}
	url, _, err := s.dataSourceURLRepo.GetDataSourceURLByInternalSymbol(ctx, internalSymbol)
	if err != nil {
		return "", fmt.Errorf("get data source URL for %s: %w", internalSymbol, err)
	}
	return url, nil
}

// SetDataSourceURL sets the data source URL for a symbol.
// Pass empty string to clear (revert to default Yahoo Finance).
func (s *Service) SetDataSourceURL(ctx context.Context, internalSymbol string, url string) error {
	if s.dataSourceURLRepo == nil {
		return fmt.Errorf("symbol mapping repo not configured")
	}
	_, id, err := s.dataSourceURLRepo.GetDataSourceURLByInternalSymbol(ctx, internalSymbol)
	if err != nil {
		return fmt.Errorf("get symbol mapping for %s: %w", internalSymbol, err)
	}
	if err := s.dataSourceURLRepo.UpdateDataSourceURL(ctx, id, url); err != nil {
		return fmt.Errorf("set data source URL for %s: %w", internalSymbol, err)
	}
	return nil
}

// extractResultToSymbolDetails converts an extractor ExtractResult to the
// canonical SymbolDetails type used throughout the application.
func extractResultToSymbolDetails(result *extractor.ExtractResult, internalSymbol string) *symbol.SymbolDetails {
	details := &symbol.SymbolDetails{
		InternalSymbol:    internalSymbol,
		ExtractorAsOfDate: result.AsOfDate,
		FetchedAt:         time.Now(),
	}

	// Fund info — symbol and name.
	if result.FundInfo != nil {
		details.ShortName = result.FundInfo.Name
		details.LongName = result.FundInfo.Name
	}

	// Holdings — all holdings, not limited to top 10.
	if len(result.Holdings) > 0 {
		details.TopHoldings = make([]symbol.TopHolding, len(result.Holdings))
		for i, h := range result.Holdings {
			details.TopHoldings[i] = symbol.TopHolding{
				Symbol:        h.Symbol,
				Name:          h.Name,
				Percent:       h.Percent,
				SecurityType:  h.SecurityType,
				CouponRate:    h.CouponRate,
				FinalMaturity: h.FinalMaturity,
				AsOfDate:      h.AsOfDate,
			}
		}
	}

	// Sectors.
	if len(result.Sectors) > 0 {
		details.SectorWeightings = make([]symbol.SectorWeighting, len(result.Sectors))
		for i, sw := range result.Sectors {
			details.SectorWeightings[i] = symbol.SectorWeighting{
				Sector:  sw.Sector,
				Percent: sw.Percent,
				Date:    sw.Date,
			}
		}
	}

	// Country allocation — maps to GeographicAllocations.
	if len(result.CountryAllocation) > 0 {
		details.GeographicAllocations = make([]symbol.GeographicAllocation, len(result.CountryAllocation))
		for i, ca := range result.CountryAllocation {
			details.GeographicAllocations[i] = symbol.GeographicAllocation{
				Country:    ca.Country,
				Percent:    ca.Percent,
				RegionName: ca.RegionName,
				RegionCode: ca.RegionCode,
				Date:       ca.Date,
			}
		}
	}

	// Fund profile.
	if result.FundProfile != nil {
		details.FundProfile = &symbol.FundProfile{
			Family:                 result.FundProfile.Family,
			LegalType:              result.FundProfile.LegalType,
			TotalNetAssets:         result.FundProfile.TotalNetAssets,
			AnnualExpenseRatio:     result.FundProfile.AnnualExpenseRatio,
			AnnualHoldingsTurnover: result.FundProfile.AnnualHoldingsTurnover,
			InceptionDate:          result.FundProfile.InceptionDate,
			Isin:                   result.FundProfile.Isin,
			ShareClassName:         result.FundProfile.ShareClassName,
			OngoingCharges:         result.FundProfile.OngoingCharges,
			Benchmark:              result.FundProfile.Benchmark,
			AssetClassification:    result.FundProfile.AssetClassification,
			DistributionStrategy:   result.FundProfile.DistributionStrategy,
			MarketRegionFocus:      result.FundProfile.MarketRegionFocus,
		}
	}

	// Equity valuation from characteristics.
	if result.Characteristics != nil {
		details.EquityValuation = &symbol.EquityValuation{
			PriceToEarnings:          result.Characteristics.PriceToEarnings,
			EstimatedPriceToEarnings: result.Characteristics.EstimatedPriceToEarnings,
			PriceToBook:              result.Characteristics.PriceToBook,
			PriceToCashflow:          result.Characteristics.PriceToCashflow,
			PriceToSales:             result.Characteristics.PriceToSales,
			DividendYield:            result.Characteristics.DividendYield,
			MedianMarketCap:          result.Characteristics.MedianMarketCap,
			ForwardROE:               result.Characteristics.ForwardROE,
			ForwardEPSGrowth:         result.Characteristics.ForwardEPSGrowth,
			RevenueRatio:             result.Characteristics.RevenueRatio,
		}

		// Bond characteristics from characteristics (conditional — only if bond fields present).
		if result.Characteristics.HasCharacteristic(extractor.CharacteristicAverageCoupon) {
			details.BondCharacteristics = &symbol.BondCharacteristics{
				AverageCoupon:   result.Characteristics.AverageCoupon,
				AverageMaturity: result.Characteristics.AverageMaturity,
				AverageQuality:  result.Characteristics.AverageQuality,
				AverageDuration: result.Characteristics.AverageDuration,
			}
		}
	}

	// Market cap breakdown.
	if result.MarketCap != nil {
		details.MarketCapBreakdown = &symbol.MarketCapBreakdown{
			Total: result.MarketCap.Total,
			Large: result.MarketCap.Large,
			Mid:   result.MarketCap.Mid,
			Small: result.MarketCap.Small,
		}
	}

	// Themes.
	if len(result.Themes) > 0 {
		details.Themes = make([]symbol.ThemeBreakdown, len(result.Themes))
		for i, th := range result.Themes {
			details.Themes[i] = symbol.ThemeBreakdown{
				Name:    th.Name,
				Percent: th.Percent,
			}
		}
	}

	// Risk measures.
	if result.RiskMeasures != nil {
		details.RiskMeasures = &symbol.RiskMeasures{
			Volatility:    result.RiskMeasures.Volatility,
			SharpeRatio:   result.RiskMeasures.SharpeRatio,
			InfoRatio:     result.RiskMeasures.InfoRatio,
			Beta:          result.RiskMeasures.Beta,
			Correlation:   result.RiskMeasures.Correlation,
			TrackingError: result.RiskMeasures.TrackingError,
			FieldsPresent: symbol.SymbolRiskFieldsMask(result.RiskMeasures.FieldsPresent),
		}
	}

	// Asset class allocation.
	if len(result.AssetClassAllocation) > 0 {
		details.AssetClassAllocation = make([]symbol.AssetClassEntry, len(result.AssetClassAllocation))
		for i, ac := range result.AssetClassAllocation {
			details.AssetClassAllocation[i] = symbol.AssetClassEntry{
				AssetClass: ac.AssetClass,
				Percent:    ac.Percent,
			}
		}
	}

	// Equity derivatives by region.
	if len(result.EquityDerivativesByRegion) > 0 {
		details.EquityDerivativesByRegion = make([]symbol.RegionDerivativeEntry, len(result.EquityDerivativesByRegion))
		for i, rd := range result.EquityDerivativesByRegion {
			details.EquityDerivativesByRegion[i] = symbol.RegionDerivativeEntry{
				Region:  rd.Region,
				Percent: rd.Percent,
			}
		}
	}

	// Currency derivatives allocation.
	if len(result.CurrencyDerivativesAllocation) > 0 {
		details.CurrencyDerivativesAllocation = make([]symbol.CurrencyDerivativeEntry, len(result.CurrencyDerivativesAllocation))
		for i, cd := range result.CurrencyDerivativesAllocation {
			details.CurrencyDerivativesAllocation[i] = symbol.CurrencyDerivativeEntry{
				Currency: cd.Currency,
				Percent:  cd.Percent,
			}
		}
	}

	return details
}
