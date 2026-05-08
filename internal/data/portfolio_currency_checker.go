package data

import (
	"context"
	"fmt"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
)

// PortfolioCurrencyCheckerImpl returns the base currency of a portfolio.
// It implements position.PortfolioCurrencyChecker.
type PortfolioCurrencyCheckerImpl struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewPortfolioCurrencyChecker creates a new portfolio currency checker.
func NewPortfolioCurrencyChecker(repo *PortfolioRepository) *PortfolioCurrencyCheckerImpl {
	return &PortfolioCurrencyCheckerImpl{
		q:  repo.q,
		db: repo.db,
	}
}

// GetPortfolioCurrency returns the base currency of the portfolio with the
// given ID. Returns an error if the portfolio does not exist.
func (c *PortfolioCurrencyCheckerImpl) GetPortfolioCurrency(ctx context.Context, portfolioID int64) (string, error) {
	p, err := c.q.GetPortfolio(ctx, c.db, portfolioID)
	if err != nil {
		return "", fmt.Errorf("get portfolio %d: %w", portfolioID, err)
	}
	return p.Currency, nil
}
