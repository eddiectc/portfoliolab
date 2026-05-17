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

	// ETF-specific fields (JSON in DB, deserialized here)
	TopHoldings          []TopHolding
	SectorWeightings     []SectorWeighting
	AggregatePositions   *AggregatePositions
	FundProfile          *FundProfile
	EquityValuation      *EquityValuation
	GeographicAllocations []GeographicAllocation

	// Metadata
	FetchedAt time.Time
}

// TopHolding represents a single holding in an ETF's portfolio.
type TopHolding struct {
	Symbol    string
	Name      string
	Percent   float64 // e.g. 0.01399 = 1.399%
}

// SectorWeighting represents the allocation to a single sector.
type SectorWeighting struct {
	Sector  string // e.g. "technology", "financial_services"
	Percent float64
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
	Percent float64 // e.g. 0.452 = 45.2%
}

// StaleSymbol represents a symbol whose details need refreshing.
type StaleSymbol struct {
	InternalSymbol   string
	MarketDataSymbol string
	FetchedAt        time.Time
}
