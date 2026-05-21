package modelportfolio

import (
	"github.com/govalues/decimal"
)

// ModelPortfolio is a named allocation blueprint — a set of symbol + weight
// pairs that sum to 100%. It can be applied as a target allocation on any
// real portfolio.
type ModelPortfolio struct {
	ID        int64                 `json:"id"`
	Name      string                `json:"name"`
	Entries   []ModelPortfolioEntry `json:"entries"`
	CreatedAt string                `json:"created_at"`
	UpdatedAt string                `json:"updated_at"`
}

// ModelPortfolioEntry maps a symbol to a target weight percentage.
type ModelPortfolioEntry struct {
	Symbol    string          `json:"symbol"`
	WeightPct decimal.Decimal `json:"weight_pct"`
}

// CreateRequest is the DTO for creating a model portfolio.
type CreateRequest struct {
	Name    string                `json:"name"`
	Entries []ModelPortfolioEntry `json:"entries"`
}

// UpdateRequest is the DTO for updating a model portfolio.
type UpdateRequest struct {
	Name    *string               `json:"name,omitempty"`
	Entries []ModelPortfolioEntry `json:"entries"`
}

// ModelPortfolioSummary holds lightweight data for dropdown selectors
// (id, name, entry count) without the full entries payload.
type ModelPortfolioSummary struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	EntryCount int    `json:"entry_count"`
}

// --- Errors ---

// ErrNotFound indicates the model portfolio was not found.
var ErrNotFound = &ModelPortfolioError{Code: "not_found", Message: "model portfolio not found"}

// ErrNameExists indicates a model portfolio with the same name already exists.
var ErrNameExists = &ModelPortfolioError{Code: "name_exists", Message: "a model portfolio with this name already exists"}

// ErrInvalidName indicates the name is empty or exceeds the maximum length.
var ErrInvalidName = &ModelPortfolioError{Code: "invalid_name", Message: "name must be between 1 and 100 characters"}

// ErrWeightSumNot100 indicates the sum of weights does not equal 100%.
var ErrWeightSumNot100 = &ModelPortfolioError{Code: "weight_sum_not_100", Message: "weights must sum to exactly 100%"}

// ErrInvalidWeight indicates a weight is zero or negative.
var ErrInvalidWeight = &ModelPortfolioError{Code: "invalid_weight", Message: "each weight must be greater than 0%"}

// ErrDuplicateSymbol indicates a symbol appears more than once in the entries.
var ErrDuplicateSymbol = &ModelPortfolioError{Code: "duplicate_symbol", Message: "entries contain duplicate symbols"}

// ErrEmptyEntries indicates the entries list is empty.
var ErrEmptyEntries = &ModelPortfolioError{Code: "empty_entries", Message: "at least one entry is required"}

// ModelPortfolioError is a typed error for model portfolio domain operations.
type ModelPortfolioError struct {
	Code    string
	Message string
}

func (e *ModelPortfolioError) Error() string {
	return e.Message
}

// Is implements errors.Is: two ModelPortfolioErrors match when their codes are equal.
// This allows errors.Is(err, ErrWeightSumNot100) to match dynamically-created errors
// that carry the same code but a different (more detailed) message.
func (e *ModelPortfolioError) Is(target error) bool {
	t, ok := target.(*ModelPortfolioError)
	return ok && e.Code == t.Code
}
