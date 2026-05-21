package comparison

import (
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
	ID                 int64               `json:"id"`
	Name               string              `json:"name"`
	Type               PortfolioType       `json:"type"`
	ReturnMetrics      *ReturnMetrics      `json:"return_metrics,omitempty"`
	RiskMetrics        *RiskMetrics        `json:"risk_metrics,omitempty"`
	Drawdown           *DrawdownResult     `json:"drawdown,omitempty"`
	YearlyReturns      []YearlyReturn      `json:"yearly_returns,omitempty"`
	PeriodExtremes     *PeriodExtremes     `json:"period_extremes,omitempty"`
	ReturnDistribution *ReturnDistribution `json:"return_distribution,omitempty"`
	Warnings           []string            `json:"warnings,omitempty"`
	Message            string              `json:"message,omitempty"` // empty-state message
}

// ReturnMetrics holds summary return calculations for a portfolio.
type ReturnMetrics struct {
	TWRPct              *decimal.Decimal `json:"twr_pct,omitempty"`
	AnnualizedTWRPct    *decimal.Decimal `json:"annualized_twr_pct,omitempty"`
	SimpleReturnPct     *decimal.Decimal `json:"simple_return_pct,omitempty"`
	AnnualizedSimplePct *decimal.Decimal `json:"annualized_simple_pct,omitempty"`
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

// YearlyReturn holds a calendar year and its percentage return.
type YearlyReturn struct {
	Year      int              `json:"year"`
	ReturnPct *decimal.Decimal `json:"return_pct,omitempty"`
}

// CrossPortfolioMetrics holds the metrics computed between two portfolios.
type CrossPortfolioMetrics struct {
	BetaAlpha   *BetaAlphaResult            `json:"beta_alpha,omitempty"`
	Correlation *PortfolioCorrelationResult `json:"correlation,omitempty"`
	Overlap     *OverlapResult              `json:"overlap,omitempty"`
	Warnings    []string                    `json:"warnings,omitempty"`
}

// OverlapResult holds portfolio overlap information.
type OverlapResult struct {
	TopHoldingsA []HoldingWeight  `json:"top_holdings_a"`
	TopHoldingsB []HoldingWeight  `json:"top_holdings_b"`
	OverlapPct   *decimal.Decimal `json:"overlap_pct,omitempty"`
	Warnings     []string         `json:"warnings,omitempty"`
}

// HoldingWeight maps a symbol to its weight in a portfolio.
type HoldingWeight struct {
	Symbol string          `json:"symbol"`
	Weight decimal.Decimal `json:"weight"` // as fraction (0.0-1.0)
	Name   string          `json:"name,omitempty"`
}

// modelPortfolioData holds the resolved model portfolio with metadata needed
// for simulation.
type modelPortfolioData struct {
	ID       int64
	Name     string
	Weights  []ModelPortfolioWeight
	Currency string // base currency of the portfolio
}
