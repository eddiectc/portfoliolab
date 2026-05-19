package allocation

import (
	"time"

	"github.com/govalues/decimal"
)

// AllocationRow represents a single symbol's allocation in the portfolio.
// MarketValue is in the symbol's native currency; MarketValueBase (when set)
// is converted to the portfolio base currency via FX rates.
type AllocationRow struct {
	Symbol           string             `json:"symbol"`
	MarketValue      decimal.Decimal    `json:"market_value"`
	MarketValueBase  *decimal.Decimal   `json:"market_value_base,omitempty"`
	AllocationPct    decimal.Decimal    `json:"allocation_pct"`
	Currency         string             `json:"currency"`
	HasMarketData    bool               `json:"has_market_data"`
	AccountBreakdown []AccountBreakdown `json:"account_breakdown,omitempty"`
}

// AccountBreakdown shows how a symbol's position is distributed across accounts.
type AccountBreakdown struct {
	AccountID       int64            `json:"account_id"`
	AccountName     string           `json:"account_name"`
	Quantity        decimal.Decimal  `json:"quantity"`
	MarketValue     decimal.Decimal  `json:"market_value"`
	MarketValueBase *decimal.Decimal `json:"market_value_base,omitempty"`
	PctOfSymbol     decimal.Decimal  `json:"pct_of_symbol"`
}

// AllocationResult holds the complete allocation breakdown for a portfolio
// (or set of portfolios). Rows are sorted by allocation percentage descending.
type AllocationResult struct {
	Rows                []AllocationRow `json:"rows"`
	TotalValueBase      decimal.Decimal `json:"total_value_base"`
	BaseCurrency        string          `json:"base_currency"`
	CashRow             *AllocationRow  `json:"cash_row,omitempty"`
	LastUpdated         time.Time       `json:"last_updated"`
	MarketDataAvailable bool            `json:"market_data_available"`
	Warnings            []string        `json:"warnings,omitempty"`
	Message             string          `json:"message,omitempty"` // empty-state message
}

// AllocationFilter holds optional filter criteria for allocation queries.
// Empty PortfolioIDs means "all portfolios".
type AllocationFilter struct {
	PortfolioIDs []int64
}

// TargetAllocation represents a user-defined target weight for a symbol
// within a portfolio.
type TargetAllocation struct {
	PortfolioID int64           `json:"portfolio_id"`
	Symbol      string          `json:"symbol"`
	TargetPct   decimal.Decimal `json:"target_pct"`
}

// TargetEntry is the input type for saving target allocations.
type TargetEntry struct {
	Symbol    string          `json:"symbol"`
	TargetPct decimal.Decimal `json:"target_pct"`
}

// DriftRow compares actual vs target allocation for a single symbol.
type DriftRow struct {
	Symbol     string          `json:"symbol"`
	ActualPct  decimal.Decimal `json:"actual_pct"`
	TargetPct  decimal.Decimal `json:"target_pct"`
	DriftPct   decimal.Decimal `json:"drift_pct"`   // actual - target
	IsBalanced bool            `json:"is_balanced"` // |drift| <= 5%
}

// DriftResult holds the drift comparison between actual and target allocations.
type DriftResult struct {
	Rows         []DriftRow `json:"rows"`
	BaseCurrency string     `json:"base_currency"`
	HasTarget    bool       `json:"has_target"`
	Warnings     []string   `json:"warnings,omitempty"`
	Message      string     `json:"message,omitempty"`
}

// RebalanceSuggestion recommends a trade to close the gap between actual and
// target allocation for a symbol.
type RebalanceSuggestion struct {
	Symbol         string          `json:"symbol"`
	Direction      string          `json:"direction"` // "buy" or "sell"
	Shares         decimal.Decimal `json:"shares"`
	DollarValue    decimal.Decimal `json:"dollar_value"`
	DriftReduction decimal.Decimal `json:"drift_reduction"`
}

// RebalanceResult holds rebalancing suggestions for the portfolio.
type RebalanceResult struct {
	Suggestions      []RebalanceSuggestion `json:"suggestions"`
	BaseCurrency     string                `json:"base_currency"`
	TotalDollarValue decimal.Decimal       `json:"total_dollar_value"`
	Warnings         []string              `json:"warnings,omitempty"`
	IsBalanced       bool                  `json:"is_balanced"`
	Message          string                `json:"message,omitempty"`
}

// --- Errors ---

// ErrNoPortfolios indicates no portfolios exist for the allocation query.
var ErrNoPortfolios = &AllocationError{Code: "no_portfolios", Message: "no portfolios found"}

// ErrInvalidTargetPct indicates a target percentage is outside the valid range [0, 100].
var ErrInvalidTargetPct = &AllocationError{Code: "invalid_target_pct", Message: "target percentage must be between 0 and 100"}

// ErrTargetSumNot100 indicates the sum of target percentages does not equal 100.
var ErrTargetSumNot100 = &AllocationError{Code: "target_sum_not_100", Message: "target percentages must sum to exactly 100"}

// ErrZeroTotalValue indicates the portfolio has zero or negative total value.
var ErrZeroTotalValue = &AllocationError{Code: "zero_total_value", Message: "portfolio has zero or negative total value"}

// ErrMixedCurrencies indicates the selected portfolios have conflicting base
// currencies, making allocation computation ambiguous.
var ErrMixedCurrencies = &AllocationError{Code: "mixed_currencies", Message: "selected portfolios have mixed base currencies"}

// ErrDuplicateSymbol indicates a symbol appears more than once in the target
// allocation entries.
var ErrDuplicateSymbol = &AllocationError{Code: "duplicate_symbol", Message: "target allocation contains duplicate symbols"}

// AllocationError is a typed error for allocation domain operations.
type AllocationError struct {
	Code    string
	Message string
}

func (e *AllocationError) Error() string {
	return e.Message
}
