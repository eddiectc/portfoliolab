package comparison

import (
	"fmt"
	"time"

	"github.com/govalues/decimal"
)

// PortfolioType identifies the kind of portfolio being compared.
type PortfolioType string

const (
	PortTypeModel PortfolioType = "model"
	PortTypeReal  PortfolioType = "real"
)

// ComparisonRequest holds the parameters for a portfolio comparison.
type ComparisonRequest struct {
	// Portfolio A
	PortfolioAID   int64         // model portfolio ID or real portfolio ID
	PortfolioAType PortfolioType // "model" or "real"
	// Portfolio B
	PortfolioBID   int64         // model portfolio ID or real portfolio ID
	PortfolioBType PortfolioType // "model" or "real"
	// Period
	Period   string     // "1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"
	DateFrom *time.Time // optional custom start
	DateTo   *time.Time // optional custom end
	// Base currency for all values.
	BaseCurrency string
	// StartingValue is the initial investment amount for model portfolio
	// simulation. Ignored for real portfolios (which use actual cash flows).
	StartingValue decimal.Decimal
}

// ComparisonResult is the top-level envelope returned by ComputeComparison.
type ComparisonResult struct {
	ComputedAt   time.Time              `json:"computed_at"`
	PortfolioA   *PortfolioComparison   `json:"portfolio_a"`
	PortfolioB   *PortfolioComparison   `json:"portfolio_b"`
	CrossMetrics *CrossPortfolioMetrics `json:"cross_metrics,omitempty"`
	Warnings     []string               `json:"warnings,omitempty"`
	Message      string                 `json:"message,omitempty"` // empty-state message when data unavailable
}

// PortfolioComparison holds the per-portfolio metrics for one side of the comparison.
type PortfolioComparison struct {
	ID   int64         `json:"id"`
	Name string        `json:"name"`
	Type PortfolioType `json:"type"`
	// EffectiveDateFrom and EffectiveDateTo are the actual date range used for
	// computation, which may be narrower than the requested period due to data
	// availability (e.g. a symbol with shorter history clips the period).
	EffectiveDateFrom  *time.Time                       `json:"effective_date_from,omitempty"`
	EffectiveDateTo    *time.Time                       `json:"effective_date_to,omitempty"`
	ValueGrowthSeries  []EquityCurvePoint               `json:"value_growth_series,omitempty"`
	ReturnMetrics      *ReturnMetrics                   `json:"return_metrics,omitempty"`
	RiskMetrics        *RiskMetrics                     `json:"risk_metrics,omitempty"`
	Drawdown           *DrawdownResult                  `json:"drawdown,omitempty"`
	DrawdownSeries     []DrawdownSeriesPoint            `json:"drawdown_series,omitempty"`
	YearlyReturns      []YearlyReturn                   `json:"yearly_returns,omitempty"`
	PeriodExtremes     *PeriodExtremes                  `json:"period_extremes,omitempty"`
	ReturnDistribution *ReturnDistribution              `json:"return_distribution,omitempty"`
	IntraCorrelation   *IntraPortfolioCorrelationResult `json:"intra_correlation,omitempty"`
	Warnings           []string                         `json:"warnings,omitempty"`
	Message            string                           `json:"message,omitempty"` // empty-state message
}

// ReturnMetrics holds summary return calculations for a portfolio.
// All metrics are TWR-equivalent (cash-flow-independent).
type ReturnMetrics struct {
	TWRPct              *decimal.Decimal `json:"twr_pct,omitempty"`
	AnnualizedTWRPct    *decimal.Decimal `json:"annualized_twr_pct,omitempty"`
	CAGRPct             *decimal.Decimal `json:"cagr_pct,omitempty"`
	DaysElapsed         int              `json:"days_elapsed"`
	HasInsufficientData bool             `json:"has_insufficient_data"`
}

// RiskMetrics holds risk and risk-adjusted return statistics.
type RiskMetrics struct {
	AnnualizedVolatilityPct *decimal.Decimal `json:"annualized_volatility_pct,omitempty"`
	SharpeRatio             *decimal.Decimal `json:"sharpe_ratio,omitempty"`
	SortinoRatio            *decimal.Decimal `json:"sortino_ratio,omitempty"`
}

// DrawdownResult holds drawdown statistics.
type DrawdownResult struct {
	MaxDrawdownPct       *decimal.Decimal `json:"max_drawdown_pct,omitempty"`
	CurrentDrawdownPct   *decimal.Decimal `json:"current_drawdown_pct,omitempty"`
	DrawdownDurationDays *int             `json:"drawdown_duration_days,omitempty"`
}

// DrawdownSeriesPoint is a single point on the drawdown-over-time series.
// Pct is the drawdown from the running peak at that date, expressed as a
// positive percentage (e.g. 15.50 = 15.50% below peak).
type DrawdownSeriesPoint struct {
	Date time.Time       `json:"date"`
	Pct  decimal.Decimal `json:"pct"`
}

// YearlyReturn holds a calendar year and its percentage return.
type YearlyReturn struct {
	Year      int              `json:"year"`
	ReturnPct *decimal.Decimal `json:"return_pct,omitempty"`
}

// CrossPortfolioMetrics holds the metrics computed between two portfolios.
type CrossPortfolioMetrics struct {
	BetaAlpha     *BetaAlphaResult            `json:"beta_alpha,omitempty"`
	Correlation   *PortfolioCorrelationResult `json:"correlation,omitempty"`
	CaptureRatios *CaptureRatiosResult        `json:"capture_ratios,omitempty"`
	Overlap       *OverlapResult              `json:"overlap,omitempty"`
	Warnings      []string                    `json:"warnings,omitempty"`
}

// CaptureRatiosResult holds the upside and downside capture ratios of
// portfolio A relative to portfolio B (the benchmark).
type CaptureRatiosResult struct {
	// UpsideCapturePct is the percentage of the benchmark's upside that the
	// portfolio captures. Computed as sum(portfolio up-day returns) /
	// sum(benchmark up-day returns) × 100. A value of 100 means the portfolio
	// captures the benchmark's upside exactly. Nil when insufficient data.
	UpsideCapturePct *decimal.Decimal `json:"upside_capture_pct,omitempty"`
	// DownsideCapturePct is the percentage of the benchmark's downside that the
	// portfolio captures. Computed as sum(portfolio down-day returns) /
	// sum(benchmark down-day returns) × 100. A value of 100 means the portfolio
	// captures the benchmark's downside exactly. Lower is better. Nil when
	// insufficient data.
	DownsideCapturePct *decimal.Decimal `json:"downside_capture_pct,omitempty"`
	// OverlapDays is the number of aligned daily return observations used.
	OverlapDays int `json:"overlap_days"`
}

// OverlapResult holds portfolio overlap information.
type OverlapResult struct {
	// TopHoldings are the expanded underlying holdings (ETFs broken into constituents).
	TopHoldingsA []HoldingWeight  `json:"top_holdings_a"`
	TopHoldingsB []HoldingWeight  `json:"top_holdings_b"`
	OverlapPct   *decimal.Decimal `json:"overlap_pct,omitempty"`
	Warnings     []string         `json:"warnings,omitempty"`
}

// MergedHolding represents a single holding in the merged holdings table,
// showing its weight in both portfolios and the overlap percentage.
type MergedHolding struct {
	Symbol     string          `json:"symbol"`
	Name       string          `json:"name,omitempty"`
	WeightA    decimal.Decimal `json:"weight_a"` // as fraction (0.0-1.0)
	WeightB    decimal.Decimal `json:"weight_b"` // as fraction (0.0-1.0)
	OverlapPct float64         `json:"overlap_pct"` // min(weightA, weightB) * 100, or 0 if unique
}

// HoldingWeight maps a symbol to its weight in a portfolio.
type HoldingWeight struct {
	ISIN   string          `json:"isin,omitempty"`
	Symbol string          `json:"symbol"`
	Weight decimal.Decimal `json:"weight"` // as fraction (0.0-1.0)
	Name   string          `json:"name,omitempty"`
}

// WeightPct returns the weight formatted as a percentage string with 1dp.
func (h HoldingWeight) WeightPct() string {
	f, _ := h.Weight.Float64()
	return fmt.Sprintf("%.1f%%", f*100)
}

// modelPortfolioData holds the resolved model portfolio with metadata needed
// for simulation.
type modelPortfolioData struct {
	ID       int64
	Name     string
	Weights  []ModelPortfolioWeight
	Currency string // base currency of the portfolio
}
