package symbol

import (
	"time"
)

// SymbolDetails holds cached metadata about a symbol fetched from a market
// data provider. Generic fields (name, exchange) are populated for all symbols.
// ETF-specific fields (holdings, sectors, etc.) are only populated for ETFs.
type SymbolDetails struct {
	// Generic fields
	InternalSymbol string
	ShortName      string
	LongName       string
	Exchange       string
	Currency       string
	QuoteType      string // e.g. "ETF", "EQUITY"
	Sector         string // primary sector for individual stocks (e.g. "Technology")

	// ETF-specific fields (JSON in DB, deserialized here)
	TopHoldings                   []TopHolding
	SectorWeightings              []SectorWeighting
	AggregatePositions            *AggregatePositions
	FundProfile                   *FundProfile
	EquityValuation               *EquityValuation
	BondCharacteristics           *BondCharacteristics
	GeographicAllocations         []GeographicAllocation
	MarketCapBreakdown            *MarketCapBreakdown
	Themes                        []ThemeBreakdown
	RiskMeasures                  *RiskMeasures
	AssetClassAllocation          []AssetClassEntry
	EquityDerivativesByRegion     []RegionDerivativeEntry
	CurrencyDerivativesAllocation []CurrencyDerivativeEntry

	// Metadata
	ExtractorAsOfDate time.Time // provider's "as of" date; zero when from Yahoo
	FetchedAt         time.Time // when the system fetched the data
}

// TopHolding represents a single holding in an ETF's portfolio.
type TopHolding struct {
	Symbol        string
	Name          string
	Percent       float64  // 0-100 percentage, e.g. 1.399 = 1.399% (not 0-1 fraction)
	SecurityType  string   // e.g. "Common Stock", "Corporate Bond" (Vanguard)
	CouponRate    *float64 // bond holdings only (Vanguard)
	FinalMaturity *string  // bond holdings only (Vanguard)
	AsOfDate      string   // effective date of the holdings data (Vanguard)
}

// SectorWeighting represents the allocation to a single sector.
type SectorWeighting struct {
	Sector  string  // e.g. "technology", "financial_services"
	Percent float64 // 0-100 percentage, e.g. 25.5 = 25.5% (not 0-1 fraction)
	Date    string  // per-section "as of" date from the provider (Vanguard)
}

// AggregatePositions represents the broad asset class breakdown of an ETF.
type AggregatePositions struct {
	Stock       float64
	Bond        float64
	Cash        float64
	Convertible float64
	Preferred   float64
	Other       float64
}

// FundProfile represents fund-level metadata from a market data provider.
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
	Benchmark              string  // benchmark index name
	AssetClassification    string  // asset class (e.g. "Equity", "Fixed Income")
	DistributionStrategy   string  // distribution strategy (e.g. "INCM", "ACUM")
	MarketRegionFocus      string  // market region focus (e.g. "Global", "Europe")
}

// EquityValuation represents aggregate valuation ratios of an ETF's equity holdings.
type EquityValuation struct {
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
}

// BondCharacteristics represents bond-specific fund metrics.
// Nil for equity funds; populated for bond funds (Vanguard).
type BondCharacteristics struct {
	AverageCoupon   float64 // average coupon rate
	AverageMaturity float64 // average maturity in years
	AverageQuality  float64 // average quality rating
	AverageDuration float64 // average duration
}

// GeographicAllocation represents a country/region exposure entry.
type GeographicAllocation struct {
	Country    string
	Percent    float64 // 0-100 percentage, e.g. 45.2 = 45.2% (not 0-1 fraction)
	RegionName string  // region grouping (e.g. "Developed Markets") (Vanguard)
	RegionCode string  // region code (Vanguard)
	Date       string  // per-section "as of" date from the provider (Vanguard)
}

// MarketCapBreakdown contains market capitalization distribution.
type MarketCapBreakdown struct {
	Total float64 // 0-100 percentage
	Large float64 // 0-100 percentage
	Mid   float64 // 0-100 percentage
	Small float64 // 0-100 percentage
}

// ThemeBreakdown represents a thematic allocation entry.
type ThemeBreakdown struct {
	Name    string
	Percent float64 // 0-100 percentage
}

// RiskMeasures contains risk metrics from fund factsheets.
type RiskMeasures struct {
	Volatility    float64              // annualized volatility percentage
	SharpeRatio   float64              // Sharpe ratio
	InfoRatio     float64              // information ratio
	Beta          float64              // beta relative to benchmark
	Correlation   float64              // correlation with benchmark
	TrackingError float64              // tracking error percentage
	FieldsPresent SymbolRiskFieldsMask // bitmask of which fields were actually parsed
}

// SymbolRiskFieldsMask tracks which RiskMeasures fields were populated.
type SymbolRiskFieldsMask uint8

const (
	SymbolRiskFieldVolatility SymbolRiskFieldsMask = 1 << iota
	SymbolRiskFieldSharpeRatio
	SymbolRiskFieldInfoRatio
	SymbolRiskFieldBeta
	SymbolRiskFieldCorrelation
	SymbolRiskFieldTrackingError
)

// HasField reports whether the given risk field was present in the source data.
func (rm *RiskMeasures) HasField(field SymbolRiskFieldsMask) bool {
	return rm.FieldsPresent&field != 0
}

// AllFieldsPresent reports whether all six risk fields were parsed.
func (rm *RiskMeasures) AllFieldsPresent() bool {
	return rm.FieldsPresent == (SymbolRiskFieldVolatility | SymbolRiskFieldSharpeRatio |
		SymbolRiskFieldInfoRatio | SymbolRiskFieldBeta | SymbolRiskFieldCorrelation |
		SymbolRiskFieldTrackingError)
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

// StaleSymbol represents a symbol whose details need refreshing.
type StaleSymbol struct {
	InternalSymbol   string
	MarketDataSymbol string
	DataSourceURL    string // empty string = use default (Yahoo Finance)
	FetchedAt        time.Time
}
