package position

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
type EquityCurvePoint struct {
	Date           time.Time       `json:"date"`
	PortfolioValue decimal.Decimal `json:"portfolio_value"`
	NetDeposit     decimal.Decimal `json:"net_deposit"`
}

// ReturnMetrics holds summary return calculations derived from the equity curve.
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
// HasInsufficientData is true when fewer than 2 data points are available.
type ReturnMetrics struct {
	TWRPct               *decimal.Decimal `json:"twr_pct,omitempty"`
	AnnualizedTWRPct     *decimal.Decimal `json:"annualized_twr_pct,omitempty"`
	MWRPct               *decimal.Decimal `json:"mwr_pct,omitempty"`
	HoldingPeriodMWRPct  *decimal.Decimal `json:"holding_period_mwr_pct,omitempty"`
	HasInsufficientData  bool             `json:"has_insufficient_data"`
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
type PerformanceResult struct {
	EquityCurve        []EquityCurvePoint `json:"equity_curve"`
	ReturnMetrics      ReturnMetrics      `json:"return_metrics"`
	BaseCurrency       string             `json:"base_currency"`
	Warnings           []string           `json:"warnings,omitempty"`
	BenchmarkTicker    string             `json:"benchmark_ticker,omitempty"`
	BenchmarkPrices    []market.HistoricalPrice `json:"benchmark_prices,omitempty"`
	BenchmarkMWRPct    *decimal.Decimal   `json:"benchmark_mwr_pct,omitempty"`
	BenchmarkCurrency  string             `json:"benchmark_currency,omitempty"`
	BenchmarkWarning   string             `json:"benchmark_warning,omitempty"`
}

// RefreshResult summarizes the outcome of a market data refresh.
// SymbolsRefreshed lists symbols whose current price was successfully fetched.
// FxPairsRefreshed lists FX pairs whose current rate was successfully fetched.
// FailedSymbols lists symbols that could not be fetched.
type RefreshResult struct {
	SymbolsRefreshed  []string `json:"symbols_refreshed"`
	FxPairsRefreshed  []string `json:"fx_pairs_refreshed"`
	FailedSymbols     []string `json:"failed_symbols,omitempty"`
}

// PerformanceFilters holds optional filter criteria for performance queries.
// Nil PortfolioID means "all portfolios". Empty Period defaults to "All".
// Nil DateFrom/DateTo means the full available range.
// Benchmark is an optional benchmark ticker for comparison (empty = none).
type PerformanceFilters struct {
	PortfolioID *int64
	Period      string // "1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"
	DateFrom    *time.Time
	DateTo      *time.Time
	Benchmark   string
}
