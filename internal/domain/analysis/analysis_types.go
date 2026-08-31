package analysis

import (
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// AnalysisSection identifies a single analytical lens within the analysis result.
type AnalysisSection string

const (
	SectionOverlap              AnalysisSection = "overlap"
	SectionCorrelation          AnalysisSection = "correlation"
	SectionSectorAllocation     AnalysisSection = "sector_allocation"
	SectionGeographicAllocation AnalysisSection = "geographic_allocation"
	SectionStressTest           AnalysisSection = "stress_test"
	SectionFactorExposure       AnalysisSection = "factor_exposure"
)

// AnalysisFilters holds optional filter criteria for analysis queries.
// Nil PortfolioID means "all portfolios". Empty Section means "all sections".
// Empty Period defaults to "1Y" for the correlation lookback.
type AnalysisFilters struct {
	PortfolioID *int64
	Section     string // AnalysisSection value or empty for all
	Period      string // "1Y", "3Y", "5Y", "10Y" — correlation lookback
}

// AnalysisResult is the top-level envelope returned by the analysis endpoint.
// Sections that could not be computed (e.g., no ETFs for overlap) are nil
// and carry a Message explaining the empty state.
type AnalysisResult struct {
	PortfolioID          int64                 `json:"portfolio_id"`
	ComputedAt           time.Time             `json:"computed_at"`
	Overlap              *OverlapResult        `json:"overlap,omitempty"`
	Correlation          *CorrelationResult    `json:"correlation,omitempty"`
	SectorAllocation     *AllocationResult     `json:"sector_allocation,omitempty"`
	GeographicAllocation *AllocationResult     `json:"geographic_allocation,omitempty"`
	StressTest           *StressTestResult     `json:"stress_test,omitempty"`
	FactorExposure       *FactorExposureResult `json:"factor_exposure,omitempty"`
	Warnings             []string              `json:"warnings,omitempty"`
	Message              string                `json:"message,omitempty"` // empty-state message when no positions
}

// --- Overlap ---

// OverlapResult holds the ETF overlap analysis: pairwise overlap matrix and
// top concentrated stocks across all ETFs.
type OverlapResult struct {
	PairwiseMatrix        []OverlapPair       `json:"pairwise_matrix"`
	TopConcentratedStocks []ConcentratedStock `json:"top_concentrated_stocks"`
	Warnings              []string            `json:"warnings,omitempty"`
	Message               string              `json:"message,omitempty"`
}

// OverlapPair describes the overlap between two ETFs in the portfolio.
type OverlapPair struct {
	ETFA              string  `json:"etf_a"`
	ETFB              string  `json:"etf_b"`
	OverlappingCount  int     `json:"overlapping_count"`
	CombinedWeightPct float64 `json:"combined_weight_pct"`
}

// ConcentratedStock describes a single underlying stock held by multiple ETFs,
// aggregated across all ETFs holding it.
type ConcentratedStock struct {
	Symbol         string   `json:"symbol"`
	Name           string   `json:"name"`
	TotalWeightPct float64  `json:"total_weight_pct"`
	HeldByETFs     []string `json:"held_by_etfs"`
}

// --- Correlation ---

// CorrelationResult holds the pairwise Pearson correlation matrix of holdings.
// Matrix is N×N where N = len(Symbols). Matrix[i][j] is a pointer to the
// correlation between Symbols[i] and Symbols[j], or nil when there is
// insufficient overlapping data (the UI renders this as "-" or "N/A").
type CorrelationResult struct {
	Matrix   [][]*float64 `json:"matrix,omitempty"`
	Symbols  []string     `json:"symbols"`
	Period   string       `json:"period"`
	Warnings []string     `json:"warnings,omitempty"`
	Message  string       `json:"message,omitempty"`
}

// --- Allocation ---

// AllocationResult holds the weighted allocation breakdown by category
// (sector or geographic region). Breakdown maps category name → weighted %.
// UnknownWeightPct is the portion of the portfolio that could not be classified.
type AllocationResult struct {
	Breakdown        map[string]float64 `json:"breakdown"`
	UnknownWeightPct float64            `json:"unknown_weight_pct"`
	Warnings         []string           `json:"warnings,omitempty"`
	Message          string             `json:"message,omitempty"`
}

// --- Stress Test ---

// StressTestResult holds the estimated portfolio impact for each historical
// crisis scenario.
type StressTestResult struct {
	Scenarios []StressScenarioResult `json:"scenarios"`
	Warnings  []string               `json:"warnings,omitempty"`
	Message   string                 `json:"message,omitempty"`
}

// StressScenarioResult describes the estimated impact of a single historical
// crisis scenario on the portfolio.
type StressScenarioResult struct {
	Name                  string             `json:"name"`
	DateRange             string             `json:"date_range"`
	EstimatedReturnPct    float64            `json:"estimated_return_pct"`
	EstimatedDollarImpact decimal.Decimal    `json:"estimated_dollar_impact"`
	SectorContributions   map[string]float64 `json:"sector_contributions"`
}

// --- Factor Exposure ---

// FactorExposureResult holds proxy-based factor exposure metrics derived from
// cached valuation data (P/E, P/B, market cap, concentration) and, when
// price history is provided, time-series factors (momentum, volatility).
type FactorExposureResult struct {
	ValueGrowthTilt     FactorValueGrowth   `json:"value_growth_tilt"`
	SizeTilt            FactorSizeTilt      `json:"size_tilt"`
	Concentration       FactorConcentration `json:"concentration"`
	TopHoldingWeightPct float64             `json:"top_holding_weight_pct"`
	Quality             FactorQuality       `json:"quality"`
	Cost                FactorCost          `json:"cost"`
	Momentum            FactorMomentum      `json:"momentum"`
	Volatility          FactorVolatility    `json:"volatility"`
	Warnings            []string            `json:"warnings,omitempty"`
	Message             string              `json:"message,omitempty"`
}

// FactorValueGrowth describes the portfolio's value vs growth tilt based on
// weighted P/E and P/B relative to a benchmark reference.
type FactorValueGrowth struct {
	WeightedPE float64 `json:"weighted_pe"`
	WeightedPB float64 `json:"weighted_pb"`
	Tilt       string  `json:"tilt"` // "value", "growth", "neutral", or "unavailable"
}

// FactorSizeTilt describes the portfolio's size bias (large-cap vs mid-cap).
type FactorSizeTilt struct {
	LargeCapPct float64 `json:"large_cap_pct"`
	MidCapPct   float64 `json:"mid_cap_pct"`
	SmallCapPct float64 `json:"small_cap_pct"`
	Tilt        string  `json:"tilt"` // "large", "mid", "small", or "mixed"
}

// FactorConcentration describes portfolio concentration via the Herfindahl-Hirschman Index.
type FactorConcentration struct {
	HHI            float64 `json:"hhi"`
	Interpretation string  `json:"interpretation"` // "well-diversified", "moderately-concentrated", "highly-concentrated"
}

// FactorQuality describes portfolio quality based on valuation ratios.
// Lower P/CF and P/Sales indicate higher quality (more cash flow/sales per
// dollar invested). Compared to S&P 500 reference values.
type FactorQuality struct {
	WeightedPCF float64 `json:"weighted_p_cashflow"`
	WeightedPS  float64 `json:"weighted_p_sales"`
	Tilt        string  `json:"tilt"` // "high-quality", "low-quality", "neutral", or "unavailable"
}

// FactorCost describes portfolio cost exposure from fund-level metrics.
// Expense ratio and holdings turnover as portfolio-weighted averages.
type FactorCost struct {
	WeightedExpenseRatio float64 `json:"weighted_expense_ratio"` // e.g. 0.03 = 0.03%
	WeightedTurnover     float64 `json:"weighted_turnover"`      // e.g. 25.0 = 25%
}

// FactorMomentum describes portfolio momentum based on historical price returns.
// Returns are portfolio-weighted averages over 3M, 6M, and 12M windows.
type FactorMomentum struct {
	Return3M  float64 `json:"return_3m"`  // 3-month return %
	Return6M  float64 `json:"return_6m"`  // 6-month return %
	Return12M float64 `json:"return_12m"` // 12-month return %
	Tilt      string  `json:"tilt"`       // "positive", "negative", "neutral", or "unavailable"
}

// FactorVolatility describes portfolio volatility from daily returns.
// Annualized volatility as portfolio-weighted average.
type FactorVolatility struct {
	AnnualizedVol float64 `json:"annualized_vol"` // annualized volatility %
	Tilt          string  `json:"tilt"`           // "low", "medium", "high", or "unavailable"
}

// --- Input types ---

// PositionWithDetails combines a position's portfolio weight with its symbol
// details (holdings, sectors, valuation) for analysis computations.
type PositionWithDetails struct {
	Symbol          string
	PortfolioWeight float64 // position market value / total portfolio value (0-100 %)
	SymbolDetails   *symbol.SymbolDetails
}
