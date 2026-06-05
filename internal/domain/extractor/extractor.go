package extractor

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/govalues/decimal"
)

// ExtractResult holds all data sections extracted from a provider page.
// The extractor is responsible for populating all fields; if any section
// fails to parse, the entire extraction is rejected (atomic — no partial data).
type ExtractResult struct {
	// Source is the identifier of the extractor that produced this result.
	Source string

	// AsOfDate is the provider's "as of" date for the extracted data,
	// distinct from when the system fetched it.
	AsOfDate time.Time

	// FundInfo contains symbol and name from the provider page.
	FundInfo *FundInfo

	// FundProfile contains AUM, TER, inception date, etc.
	FundProfile *FundProfile

	// Holdings is the full list of security holdings (not limited to top 10).
	Holdings []Holding

	// NavHistory is the time series of NAV data points.
	NavHistory []NavPoint

	// Themes is the thematic breakdown (e.g. "AI", "Clean Energy").
	Themes []Theme

	// Sectors is the sector weighting breakdown.
	Sectors []SectorWeighting

	// CountryAllocation is the geographic breakdown by country.
	CountryAllocation []CountryAllocation

	// MarketCap is the market capitalization breakdown.
	MarketCap *MarketCapBreakdown

	// Characteristics is the fund characteristics data (P/E, P/B, etc.).
	Characteristics *FundCharacteristics

	// RiskMeasures contains risk metrics (volatility, Sharpe, beta, etc.).
	// Optional — may be nil for funds that don't publish risk data.
	RiskMeasures *RiskMeasures

	// AssetClassAllocation is the exposure by asset class relative to AUM.
	// Values can be negative (short positions) and do not sum to 100%.
	// Optional — may be nil.
	AssetClassAllocation []AssetClassEntry

	// EquityDerivativesByRegion is the composition within the equity sleeve.
	// Values sum to ~100% but individual values can be negative.
	// Optional — may be nil.
	EquityDerivativesByRegion []RegionDerivativeEntry

	// CurrencyDerivativesAllocation is the composition within the currency sleeve.
	// Values sum to ~100% but individual values can be negative.
	// Optional — may be nil.
	CurrencyDerivativesAllocation []CurrencyDerivativeEntry
}

// FundInfo contains basic fund identity from the provider.
type FundInfo struct {
	Symbol string
	Name   string
}

// FundProfile contains fund-level metadata (AUM, TER, inception date, family, legal type).
type FundProfile struct {
	Family                 string
	LegalType              string
	TotalNetAssets         float64
	AnnualExpenseRatio     float64
	AnnualHoldingsTurnover float64
	InceptionDate          time.Time
	Isin                   string  // ISIN code (e.g. "LU2951555585")
	ShareClassName         string  // share class name (e.g. "R USD UCITS ETF")
	OngoingCharges         float64 // ongoing charges ratio percentage (e.g. 0.75)
	Benchmark              string  // benchmark index name (e.g. "FTSE All-World Index")
	AssetClassification    string  // asset class (e.g. "Equity", "Fixed Income")
	DistributionStrategy   string  // distribution strategy (e.g. "INCM", "ACUM")
	MarketRegionFocus      string  // market region focus (e.g. "Global", "Europe")
	BaseCurrency           string  // base currency of the fund (e.g. "USD", "EUR", "GBP")
	// BlackRock/iShares-specific fields
	SFDRClassification string // e.g. "Other", "Article 6", "Article 8", "Article 9"
	Domicile           string // e.g. "Ireland", "Luxembourg"
	RebalanceFrequency string // e.g. "Quarterly", "Semi-Annually"
	ProductStructure   string // e.g. "Physical", "Synthetic"
	Methodology        string // e.g. "Optimised", "Representative"
	FundManager        string // e.g. "BlackRock Asset Management Ireland Limited"
	Custodian          string // e.g. "State Street Custodial Services (Ireland) Limited"
	IssuingCompany     string // e.g. "iShares IV plc"
	BenchmarkTicker    string // e.g. Bloomberg ticker of the benchmark
}

// Holding is a single security holding with weight percentage.
type Holding struct {
	Symbol        string
	Name          string
	Percent       float64
	SecurityType  string   // e.g. "Common Stock", "Corporate Bond" (Vanguard)
	CouponRate    *float64 // bond holdings only (Vanguard)
	FinalMaturity *string  // bond holdings only (Vanguard)
	AsOfDate      string   // effective date of the holdings data (Vanguard)
	// BlackRock/iShares-specific fields
	Sector         string  // e.g. "Information Technology"
	AssetClass     string  // e.g. "Equity", "Cash"
	MarketValue    float64 // market value in base currency
	NotionalValue  float64 // notional value
	Shares         float64 // number of shares/units
	Price          float64 // price per share
	ISIN           string  // ISIN, or "-" for cash
	Location       string  // country, e.g. "United States"
	Exchange       string  // e.g. "NASDAQ"
	MarketCurrency string  // e.g. "USD"
}

// NavPoint is a single NAV data point.
type NavPoint struct {
	Date     time.Time
	NAV      decimal.Decimal
	Currency string // currency of the NAV value; empty means use the symbol's currency
}

// Theme is a thematic allocation entry.
type Theme struct {
	Name    string
	Percent float64
}

// SectorWeighting is a sector allocation entry.
type SectorWeighting struct {
	Sector  string
	Percent float64
	Date    string // per-section "as of" date from the provider (Vanguard)
}

// CountryAllocation is a geographic allocation entry.
type CountryAllocation struct {
	Country    string
	Percent    float64
	RegionName string // region grouping (e.g. "Developed Markets") (Vanguard)
	RegionCode string // region code (Vanguard)
	Date       string // per-section "as of" date from the provider (Vanguard)
}

// MarketCapBreakdown contains market capitalization distribution.
type MarketCapBreakdown struct {
	Total float64
	Large float64
	Mid   float64
	Small float64
}

// FundCharacteristics contains valuation and other fund characteristics.
type FundCharacteristics struct {
	PriceToEarnings          float64
	EstimatedPriceToEarnings float64
	PriceToBook              float64
	PriceToCashflow          float64
	PriceToSales             float64
	DividendYield            float64
	// Equity-specific fields (Vanguard)
	MedianMarketCap  float64 // median market cap of holdings
	ForwardROE       float64 // forward 5-year return on equity
	ForwardEPSGrowth float64 // forward 5-year EPS growth
	RevenueRatio     float64 // revenue / revenue prior year
	// Bond-specific fields (Vanguard)
	AverageCoupon   float64 // average coupon rate
	AverageMaturity float64 // average maturity in years
	AverageQuality  float64 // average quality rating
	AverageDuration float64 // average duration
	// BlackRock/iShares-specific fields
	Beta3Y             float64 // 3-year beta
	StandardDeviation3Y float64 // 3-year standard deviation
	NumberOfHoldings   int     // number of holdings
	// FieldsPresent tracks which fields were actually populated by the parser.
	// A zero value means the field was not present in the source data (distinct
	// from the field being genuinely zero).
	FieldsPresent CharacteristicsFieldsMask
}

// CharacteristicsFieldsMask tracks which FundCharacteristics fields were
// populated by the parser. A zero value means the field was not present
// in the source data (distinct from the field being genuinely zero).
type CharacteristicsFieldsMask uint32

const (
	CharacteristicPriceToEarnings CharacteristicsFieldsMask = 1 << iota
	CharacteristicEstimatedPriceToEarnings
	CharacteristicPriceToBook
	CharacteristicPriceToCashflow
	CharacteristicPriceToSales
	CharacteristicDividendYield
	CharacteristicMedianMarketCap
	CharacteristicForwardROE
	CharacteristicForwardEPSGrowth
	CharacteristicRevenueRatio
	CharacteristicAverageCoupon
	CharacteristicAverageMaturity
	CharacteristicAverageQuality
	CharacteristicAverageDuration
	// BlackRock/iShares-specific fields
	CharacteristicBeta3Y
	CharacteristicStandardDeviation3Y
	CharacteristicNumberOfHoldings
)

// HasCharacteristic reports whether the given characteristic field was
// present in the source data.
func (fc *FundCharacteristics) HasCharacteristic(field CharacteristicsFieldsMask) bool {
	return fc.FieldsPresent&field != 0
}

// RiskMeasures contains risk metrics from fund factsheets.
type RiskMeasures struct {
	Volatility    float64        // annualized volatility percentage
	SharpeRatio   float64        // Sharpe ratio
	InfoRatio     float64        // information ratio
	Beta          float64        // beta relative to benchmark
	Correlation   float64        // correlation with benchmark
	TrackingError float64        // tracking error percentage
	FieldsPresent RiskFieldsMask // bitmask of which fields were actually parsed
}

// RiskFieldsMask tracks which RiskMeasures fields were populated by the parser.
// A zero value means the field was not present in the source data (distinct from
// the field being genuinely zero).
type RiskFieldsMask uint8

const (
	RiskFieldVolatility RiskFieldsMask = 1 << iota
	RiskFieldSharpeRatio
	RiskFieldInfoRatio
	RiskFieldBeta
	RiskFieldCorrelation
	RiskFieldTrackingError
)

// HasField reports whether the given risk field was present in the source data.
func (rm *RiskMeasures) HasField(field RiskFieldsMask) bool {
	return rm.FieldsPresent&field != 0
}

// AllFieldsPresent reports whether all six risk fields were parsed.
func (rm *RiskMeasures) AllFieldsPresent() bool {
	return rm.FieldsPresent == (RiskFieldVolatility | RiskFieldSharpeRatio |
		RiskFieldInfoRatio | RiskFieldBeta | RiskFieldCorrelation | RiskFieldTrackingError)
}

// AssetClassEntry is a single asset class allocation entry.
// Percent can be negative (short positions) and entries do not sum to 100%.
type AssetClassEntry struct {
	AssetClass string  // e.g. "Equities", "Bonds", "Gold", "Oil", "Cash"
	Percent    float64 // exposure relative to AUM, can be negative
}

// RegionDerivativeEntry is a single regional derivative exposure entry.
// Values sum to ~100% but individual values can be negative.
type RegionDerivativeEntry struct {
	Region  string  // e.g. "North America", "Europe", "Asia", "Emerging Countries"
	Percent float64 // composition within equity sleeve, can be negative
}

// CurrencyDerivativeEntry is a single currency derivative exposure entry.
// Values sum to ~100% but individual values can be negative.
type CurrencyDerivativeEntry struct {
	Currency string  // e.g. "USD", "EUR", "JPY"
	Percent  float64 // composition within currency sleeve, can be negative
}

// Extractor extracts data from a specific provider's web pages.
// Each provider (e.g. WisdomTree) implements this interface.
type Extractor interface {
	// Name returns the unique identifier for this extractor (e.g. "wisdomtree").
	Name() string

	// Extract fetches and parses data from the given source URL.
	// Returns an ExtractResult with all populated sections, or an error
	// if any section fails (atomic — no partial data).
	Extract(ctx context.Context, sourceURL string) (*ExtractResult, error)
}

// URLMatcher checks if a URL belongs to this extractor's domain.
type URLMatcher interface {
	Match(rawURL string) bool
}

// registryEntry pairs an Extractor with its URLMatcher for URL-based lookup.
type registryEntry struct {
	extractor Extractor
	matcher   URLMatcher
}

// Registry manages a collection of extractors indexed by name and URL pattern.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]Extractor
	byURL  []registryEntry // ordered by registration; first match wins
}

// NewRegistry creates a new empty extractor registry.
func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]Extractor),
	}
}

// Register adds an extractor to the registry. It is indexed by name and
// by URL pattern (the extractor must implement URLMatcher).
// Returns an error if an extractor with the same name is already registered.
func (r *Registry) Register(e Extractor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := e.Name()
	if _, exists := r.byName[name]; exists {
		return fmt.Errorf("extractor %q already registered", name)
	}

	r.byName[name] = e
	if matcher, ok := e.(URLMatcher); ok {
		r.byURL = append(r.byURL, registryEntry{extractor: e, matcher: matcher})
	}
	return nil
}

// FindByURL returns the first registered extractor whose URLMatcher matches
// the given URL. Returns an error if no extractor matches.
func (r *Registry) FindByURL(rawURL string) (Extractor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, entry := range r.byURL {
		if entry.matcher.Match(rawURL) {
			return entry.extractor, nil
		}
	}
	return nil, fmt.Errorf("no extractor registered for URL %q", rawURL)
}

// Get returns the extractor with the given name.
// Returns an error if no extractor with that name is registered.
func (r *Registry) Get(name string) (Extractor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("extractor %q not registered", name)
	}
	return e, nil
}

// Dispatcher routes extraction requests to the correct extractor based on
// the source URL. It holds a reference to the registry.
type Dispatcher struct {
	registry *Registry
}

// NewDispatcher creates a new dispatcher backed by the given registry.
func NewDispatcher(registry *Registry) *Dispatcher {
	return &Dispatcher{registry: registry}
}

// Dispatch finds the matching extractor for the source URL and calls its
// Extract method. Returns the ExtractResult or an error if no extractor
// matches or extraction fails.
func (d *Dispatcher) Dispatch(ctx context.Context, sourceURL string) (*ExtractResult, error) {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL %q: %w", sourceURL, err)
	}

	extractor, err := d.registry.FindByURL(parsed.String())
	if err != nil {
		return nil, fmt.Errorf("dispatch to %q: %w", sourceURL, err)
	}

	result, err := extractor.Extract(ctx, parsed.String())
	if err != nil {
		return nil, fmt.Errorf("extract from %q (%s): %w", sourceURL, extractor.Name(), err)
	}

	result.Source = extractor.Name()

	return result, nil
}
