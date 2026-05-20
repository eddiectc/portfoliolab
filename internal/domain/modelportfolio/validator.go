package modelportfolio

import (
	"fmt"

	"github.com/govalues/decimal"
)

// maxNameLength is the maximum allowed length for a model portfolio name.
const maxNameLength = 100

// ValidateCreateRequest validates all fields of a create request.
// Checks: name length, non-empty entries, each weight > 0, no duplicate symbols,
// weights sum to 100%.
func ValidateCreateRequest(req CreateRequest) error {
	if err := validateName(req.Name); err != nil {
		return err
	}
	return ValidateEntries(req.Entries)
}

// ValidateUpdateRequest validates only the non-nil/non-empty fields of an update request.
// Name is validated only if provided; entries are always required.
func ValidateUpdateRequest(req UpdateRequest) error {
	if req.Name != nil {
		if err := validateName(*req.Name); err != nil {
			return err
		}
	}
	return ValidateEntries(req.Entries)
}

// ValidateEntries validates the entries slice: non-empty, each weight > 0,
// no duplicate symbols, weights sum to 100% (with 0.01% tolerance).
func ValidateEntries(entries []ModelPortfolioEntry) error {
	if len(entries) == 0 {
		return ErrEmptyEntries
	}

	// Check each weight is > 0.
	for _, e := range entries {
		if !e.WeightPct.IsPos() {
			return ErrInvalidWeight
		}
	}

	// Check for duplicate symbols.
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if _, exists := seen[e.Symbol]; exists {
			return ErrDuplicateSymbol
		}
		seen[e.Symbol] = struct{}{}
	}

	// Validate sum == 100 (with 0.01% tolerance).
	var sum decimal.Decimal
	for _, e := range entries {
		sum, _ = sum.Add(e.WeightPct)
	}
	hundred := decimal.MustNew(10000, 2)
	diff, _ := sum.Sub(hundred)
	absDiff := diff.Abs()
	tolerance := decimal.MustNew(1, 2) // 0.01%
	exceeds, _ := absDiff.Sub(tolerance)
	if exceeds.IsPos() {
		delta, _ := sum.Sub(hundred)
		return &ModelPortfolioError{
			Code:    "weight_sum_not_100",
			Message: fmt.Sprintf("weights must sum to exactly 100%% (current total: %s%%, delta: %s%%)", sum.String(), delta.String()),
		}
	}

	return nil
}

// validateName checks that the name is non-empty and within the max length.
func validateName(name string) error {
	if name == "" || len(name) > maxNameLength {
		return ErrInvalidName
	}
	return nil
}
