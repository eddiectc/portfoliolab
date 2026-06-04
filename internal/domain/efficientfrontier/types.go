package efficientfrontier

import (
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// FrontierRequest holds the input for an efficient frontier computation.
type FrontierRequest struct {
	// Symbols is the list of candidate symbol names (internal symbols).
	Symbols []string
	// Prices maps each symbol to its historical price series.
	// Prices must be sorted by date ascending.
	Prices map[string][]market.HistoricalPrice
	// Period is the lookback period used (e.g. "1Y", "3Y", "5Y").
	Period string
	// RiskFreeRate is the annualized risk-free rate as a decimal
	// (e.g. 0.045 = 4.5%). Used for Sharpe ratio computation.
	RiskFreeRate float64
}

// FrontierResult holds the complete output of an efficient frontier computation.
type FrontierResult struct {
	// FrontierPoints are the 20-30 points sampled from the efficient set,
	// sorted by ascending volatility.
	FrontierPoints []FrontierPoint `json:"frontier_points"`
	// MaxSharpe is the portfolio with the highest Sharpe ratio.
	MaxSharpe *OptimizedPortfolio `json:"max_sharpe,omitempty"`
	// MinVariance is the analytical global minimum variance portfolio.
	MinVariance *OptimizedPortfolio `json:"min_variance,omitempty"`
	// HighestReturn is the portfolio with the highest expected return
	// on the efficient frontier.
	HighestReturn *OptimizedPortfolio `json:"highest_return,omitempty"`
	// Symbols is the ordered list of symbols used in the computation.
	// Weights in each portfolio are indexed by this slice.
	Symbols []string `json:"symbols"`
	// TradingDays is the number of trading days used for annualization.
	TradingDays int `json:"trading_days"`
	// ComputedAt is the time the frontier was computed.
	ComputedAt time.Time `json:"computed_at"`
	// Warnings are non-fatal issues (e.g. data gaps, FX unavailable).
	Warnings []string `json:"warnings,omitempty"`
	// Message is an empty-state message when frontier could not be computed.
	Message string `json:"message,omitempty"`
}

// FrontierPoint is a single point on the efficient frontier curve.
type FrontierPoint struct {
	// ReturnPct is the annualized expected return as a percentage.
	ReturnPct float64 `json:"return_pct"`
	// VolatilityPct is the annualized volatility as a percentage.
	VolatilityPct float64 `json:"volatility_pct"`
	// SharpeRatio is the Sharpe ratio (return - riskFree) / volatility.
	SharpeRatio float64 `json:"sharpe_ratio"`
	// Weights are the portfolio weights as fractions (0.0-1.0), one per symbol.
	Weights []float64 `json:"weights"`
}

// OptimizedPortfolio is a named portfolio with weights and key metrics.
type OptimizedPortfolio struct {
	// Name is a display label (e.g. "Max Sharpe", "Min Variance").
	Name string `json:"name"`
	// ReturnPct is the annualized expected return as a percentage.
	ReturnPct float64 `json:"return_pct"`
	// VolatilityPct is the annualized volatility as a percentage.
	VolatilityPct float64 `json:"volatility_pct"`
	// SharpeRatio is the Sharpe ratio.
	SharpeRatio float64 `json:"sharpe_ratio"`
	// Weights are the portfolio weights as fractions (0.0-1.0), one per symbol.
	Weights []float64 `json:"weights"`
}

// FrontierError is a typed error for frontier computation failures.
type FrontierError struct {
	// Code is a machine-readable error identifier.
	Code string
	// Message is a human-readable description.
	Message string
}

func (e *FrontierError) Error() string {
	return fmt.Sprintf("efficient frontier: %s — %s", e.Code, e.Message)
}

// ErrInsufficientSymbols is returned when fewer than 2 symbols are provided.
var ErrInsufficientSymbols = &FrontierError{
	Code:    "insufficient_symbols",
	Message: "at least 2 symbols are required",
}

// ErrTooManySymbols is returned when more than 10 symbols are provided.
var ErrTooManySymbols = &FrontierError{
	Code:    "too_many_symbols",
	Message: "maximum 10 symbols supported",
}

// ErrInsufficientData is returned when price data is too short for computation.
var ErrInsufficientData = &FrontierError{
	Code:    "insufficient_data",
	Message: "insufficient price data for computation",
}

// ErrSingularMatrix is returned when the covariance matrix is singular
// and no fallback is possible.
var ErrSingularMatrix = &FrontierError{
	Code:    "singular_matrix",
	Message: "covariance matrix is singular and cannot be inverted",
}

// portfolioEval is an evaluated candidate portfolio used internally
// during frontier computation.
type portfolioEval struct {
	weights    []float64
	return_    float64
	volatility float64
	sharpe     float64
}

// ErrNumericalFailure is returned when the optimization fails numerically.
var ErrNumericalFailure = &FrontierError{
	Code:    "numerical_failure",
	Message: "optimization failed due to numerical error",
}
