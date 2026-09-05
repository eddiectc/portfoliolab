package data

import (
	"context"

	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
)

// PortfolioCurrencyCheckerImpl returns the base currency of a portfolio.
// It implements position.PortfolioCurrencyChecker.
type PortfolioCurrencyCheckerImpl struct {
	svc *portfolio.Service
}

// NewPortfolioCurrencyChecker creates a new portfolio currency checker.
func NewPortfolioCurrencyChecker(svc *portfolio.Service) *PortfolioCurrencyCheckerImpl {
	return &PortfolioCurrencyCheckerImpl{svc: svc}
}

// GetPortfolioCurrency returns the base currency of the portfolio with the
// given ID. Returns an error if the portfolio does not exist.
func (c *PortfolioCurrencyCheckerImpl) GetPortfolioCurrency(ctx context.Context, portfolioID int64) (string, error) {
	p, err := c.svc.Get(ctx, portfolioID)
	if err != nil {
		return "", err
	}
	return p.Currency, nil
}
