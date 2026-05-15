package performance

import (
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// EquityCurvePoint is a single data point on the equity curve.
// Date is the trading day. PortfolioValue is the total market value of all
// open positions plus cash balances, converted to the base currency.
// NetDeposit is the cumulative sum of deposits minus withdrawals up to and
// including this date, converted to the base currency.
// NavPerUnit is the net asset value per portfolio unit on this date.
// Nil when unitization is not applicable (no deposits). Units is the total
// number of portfolio units outstanding. Nil when unitization is not applicable.
type EquityCurvePoint struct {
	Date           time.Time        `json:"date"`
	PortfolioValue decimal.Decimal  `json:"portfolio_value"`
	NetDeposit     decimal.Decimal  `json:"net_deposit"`
	NavPerUnit     *decimal.Decimal `json:"nav_per_unit,omitempty"`
	Units          *decimal.Decimal `json:"units,omitempty"`
}

// ReturnMetrics holds summary return calculations derived from the equity curve.
// ProfitLoss is the absolute profit or loss: current portfolio value minus
// total net deposit. Positive means the portfolio gained money, negative
// means it lost money. Always available when there are at least 2 data points.
// TWRPct is the Time-Weighted Return: geometrically links sub-period returns
// between cash flow events, isolating investment performance from deposit/
// withdrawal timing. Nil when insufficient data or all sub-period values are
// non-positive. Expressed as a percentage (e.g. 12.50 = 12.50%).
// AnnualizedTWRPct is the annualized TWR: (1 + TWR)^(365 / days) - 1.
// Nil when fewer than 2 data points or zero days elapsed.
// MWRPct is the Money-Weighted Return (Internal Rate of Return): finds the
// discount rate that makes the net present value of all cash flows plus the
// terminal portfolio value equal to zero. Unlike TWR, MWR is affected by the
// timing and magnitude of deposits/withdrawals. Nil when insufficient data
// or begin value is non-positive. Expressed as an annualized percentage.
// HoldingPeriodMWRPct is the MWR expressed as a holding-period return
// (not annualized): (1 + MWR)^(days/365) - 1. Nil when MWR is nil.
// SimpleReturnPct is total profit (PortfolioValue - NetDeposit at last point)
// as a percentage of total NetDeposit. Answers "for every unit deposited,
// how much profit was made?" Nil when net deposit is non-positive.
// AnnualizedSimpleReturnPct is the annualized simple return.
// Nil when the simple return is nil or zero days elapsed.
// HasInsufficientData is true when fewer than 2 data points are available.
type ReturnMetrics struct {
	ProfitLoss                *decimal.Decimal `json:"profit_loss,omitempty"`
	TWRPct                    *decimal.Decimal `json:"twr_pct,omitempty"`
	AnnualizedTWRPct        *decimal.Decimal `json:"annualized_twr_pct,omitempty"`
	MWRPct                  *decimal.Decimal `json:"mwr_pct,omitempty"`
	HoldingPeriodMWRPct     *decimal.Decimal `json:"holding_period_mwr_pct,omitempty"`
	SimpleReturnPct         *decimal.Decimal `json:"simple_return_pct,omitempty"`         // Total profit / total net deposit (%). Nil when net deposit ≤ 0.
	AnnualizedSimpleReturnPct *decimal.Decimal `json:"annualized_simple_return_pct,omitempty"` // Annualized simple return. Nil when simple return is nil or zero days elapsed.
	// Profit breakdown — sum of these equals profit_loss.
	// UnrealizedPnL and RealizedPnL come from position summaries (capital gains only).
	// Dividends, Interest, Fees, Taxes come from transaction aggregation.
	UnrealizedPnL    *decimal.Decimal `json:"unrealized_pnl,omitempty"`    // Open position price gains/losses
	RealizedPnL      *decimal.Decimal `json:"realized_pnl,omitempty"`      // Closed position locked-in gains/losses
	Dividends        *decimal.Decimal `json:"dividends,omitempty"`         // Dividend income
	Interest         *decimal.Decimal `json:"interest,omitempty"`          // Interest income
	Fees             *decimal.Decimal `json:"fees,omitempty"`              // Transaction and other fees (negative)
	Taxes            *decimal.Decimal `json:"taxes,omitempty"`             // Tax withholdings (negative)
	HasInsufficientData     bool             `json:"has_insufficient_data"`
}

// NavSummary holds the current unitization state of the portfolio.
// NavPerUnit is the current net asset value per unit.
// TotalUnits is the total number of portfolio units outstanding.
// TotalValue is the total market value (units × NAV).
// InceptionDate is the date of the first deposit (portfolio inception).
type NavSummary struct {
	NavPerUnit    decimal.Decimal `json:"nav_per_unit"`
	TotalUnits    decimal.Decimal `json:"total_units"`
	TotalValue    decimal.Decimal `json:"total_value"`
	InceptionDate time.Time       `json:"inception_date"`
}

// YearlyPerformance is a sequence of calendar-year returns.
type YearlyPerformance []YearlyReturn

// MonthlyReturn holds the monthly return data for a single month.
// ReturnPct is the time-weighted return for the month (%). Nil when no data.
// BenchmarkReturnPct is the benchmark return for the month (%). Nil when no benchmark.
// DiffPct is portfolio return minus benchmark return (%). Nil when no benchmark.
type MonthlyReturn struct {
	ReturnPct          *decimal.Decimal `json:"return_pct,omitempty"`
	BenchmarkReturnPct *decimal.Decimal `json:"benchmark_return_pct,omitempty"`
	DiffPct            *decimal.Decimal `json:"diff_pct,omitempty"`
}

// YearlyMonthlyReturns holds monthly returns grouped by year.
type YearlyMonthlyReturns struct {
	Year   int                    `json:"year"`
	Months map[int]MonthlyReturn  `json:"months"` // month number (1-12) -> return data
}

// PerformanceResult is the complete output of a performance computation.
// EquityCurve is the time-series of portfolio value and net deposit points.
// ReturnMetrics are the summary return calculations.
// BaseCurrency is the currency all values are expressed in.
// Warnings lists any non-fatal issues encountered during computation.
// BenchmarkTicker is the ticker of the selected benchmark (empty if none).
// BenchmarkPrices are the cached benchmark prices for the period.
// BenchmarkMWRPct is the benchmark money-weighted return for the period.
// BenchmarkCurrency is the currency of the benchmark price data.
// BenchmarkWarning is a data availability warning (empty if OK).
// NavSummary holds the current unitization state (nil when no deposits).
// RiskMetrics holds volatility, Sharpe, and Sortino ratios.
// DrawdownAnalysis holds max/current drawdown statistics.
// YearlyPerformance holds calendar-year return breakdown.
// MonthlyReturns holds monthly return heatmap data (portfolio vs benchmark).
type PerformanceResult struct {
	EquityCurve           []EquityCurvePoint     `json:"equity_curve"`
	ReturnMetrics         ReturnMetrics          `json:"return_metrics"`
	BaseCurrency          string                 `json:"base_currency"`
	Warnings              []string               `json:"warnings,omitempty"`
	BenchmarkTicker       string                 `json:"benchmark_ticker,omitempty"`
	BenchmarkPrices       []market.HistoricalPrice `json:"benchmark_prices,omitempty"`
	BenchmarkMWRPct       *decimal.Decimal       `json:"benchmark_mwr_pct,omitempty"`
	BenchmarkCurrency     string                 `json:"benchmark_currency,omitempty"`
	BenchmarkWarning      string                 `json:"benchmark_warning,omitempty"`
	NavSummary            *NavSummary            `json:"nav_summary,omitempty"`
	RiskMetrics           RiskMetrics            `json:"risk_metrics"`
	DrawdownAnalysis      DrawdownAnalysis       `json:"drawdown_analysis"`
	YearlyPerformance     YearlyPerformance      `json:"yearly_returns,omitempty"`
	MonthlyReturns        []YearlyMonthlyReturns `json:"monthly_returns,omitempty"`
}

// PerformanceFilters holds optional filter criteria for performance queries.
// Nil PortfolioID means "all portfolios". Empty Period defaults to "All".
// Nil DateFrom/DateTo means the full available range.
// Benchmark is an optional benchmark ticker for comparison (empty = none).
// Mode selects the performance view: "equity" (default, total return) or "nav" (NAV per unit).
type PerformanceFilters struct {
	PortfolioID *int64
	Period      string // "1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"
	DateFrom    *time.Time
	DateTo      *time.Time
	Benchmark   string
	Mode        string // "equity" (default) or "nav"
}
