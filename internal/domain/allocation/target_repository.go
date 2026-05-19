package allocation

import "context"

// TargetRepository provides data access for target allocations.
type TargetRepository interface {
	// GetByPortfolio retrieves all target allocations for a portfolio.
	GetByPortfolio(ctx context.Context, portfolioID int64) ([]TargetAllocation, error)

	// Upsert inserts or updates a target allocation for a portfolio+symbol.
	Upsert(ctx context.Context, ta TargetAllocation) error

	// DeleteBySymbol removes a target allocation for a portfolio+symbol.
	DeleteBySymbol(ctx context.Context, portfolioID int64, symbol string) error

	// DeleteByPortfolio removes all target allocations for a portfolio.
	DeleteByPortfolio(ctx context.Context, portfolioID int64) error
}
