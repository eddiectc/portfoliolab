package position

import (
	"time"

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
// TotalReturnPct is (current_value - net_deposit) / net_deposit × 100.
// AnnualizedReturnPct is the CAGR: (end_value / begin_value)^(365 / days) - 1.
// HasInsufficientData is true when fewer than 2 data points are available.
type ReturnMetrics struct {
	TotalReturnPct      *decimal.Decimal `json:"total_return_pct,omitempty"`
	AnnualizedReturnPct *decimal.Decimal `json:"annualized_return_pct,omitempty"`
	HasInsufficientData bool             `json:"has_insufficient_data"`
}

// PerformanceResult is the complete output of a performance computation.
// EquityCurve is the time-series of portfolio value and net deposit points.
// ReturnMetrics are the summary return calculations.
// BaseCurrency is the currency all values are expressed in.
// Warnings lists any non-fatal issues encountered during computation.
type PerformanceResult struct {
	EquityCurve   []EquityCurvePoint `json:"equity_curve"`
	ReturnMetrics ReturnMetrics      `json:"return_metrics"`
	BaseCurrency  string             `json:"base_currency"`
	Warnings      []string           `json:"warnings,omitempty"`
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
type PerformanceFilters struct {
	PortfolioID *int64
	Period      string // "1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"
	DateFrom    *time.Time
	DateTo      *time.Time
}
