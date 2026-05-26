package symbol

import "time"

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
	TopHoldings          []TopHolding
	SectorWeightings     []SectorWeighting
	AggregatePositions   *AggregatePositions
	FundProfile          *FundProfile
	EquityValuation      *EquityValuation
	GeographicAllocations []GeographicAllocation

	// Metadata
	ExtractorAsOfDate time.Time // provider's "as of" date; zero when from Yahoo
	FetchedAt         time.Time  // when the system fetched the data
}

// TopHolding represents a single holding in an ETF's portfolio.
type TopHolding struct {
	Symbol    string
	Name      string
	Percent   float64 // 0-100 percentage, e.g. 1.399 = 1.399% (not 0-1 fraction)
}

// SectorWeighting represents the allocation to a single sector.
type SectorWeighting struct {
	Sector  string // e.g. "technology", "financial_services"
	Percent float64 // 0-100 percentage, e.g. 25.5 = 25.5% (not 0-1 fraction)
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
}

// EquityValuation represents aggregate valuation ratios of an ETF's equity holdings.
type EquityValuation struct {
	PriceToEarnings float64
	PriceToBook     float64
	PriceToCashflow float64
	PriceToSales    float64
}

// GeographicAllocation represents a country/region exposure entry.
type GeographicAllocation struct {
	Country string
	Percent float64 // 0-100 percentage, e.g. 45.2 = 45.2% (not 0-1 fraction)
}

// StaleSymbol represents a symbol whose details need refreshing.
type StaleSymbol struct {
	InternalSymbol   string
	MarketDataSymbol string
	DataSourceURL    string // empty string = use default (Yahoo Finance)
	FetchedAt        time.Time
}
