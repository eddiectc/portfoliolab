package data

import (
	"context"
)

// PortfolioCheckerImpl checks portfolio existence via the portfolio repository.
// It implements account.PortfolioChecker.
type PortfolioCheckerImpl struct {
	repo *PortfolioRepository
}

// NewPortfolioChecker creates a new portfolio checker backed by the given repository.
func NewPortfolioChecker(repo *PortfolioRepository) *PortfolioCheckerImpl {
	return &PortfolioCheckerImpl{repo: repo}
}

// PortfolioExists returns true if a portfolio with the given ID exists.
func (c *PortfolioCheckerImpl) PortfolioExists(ctx context.Context, id int64) bool {
	_, err := c.repo.GetByID(ctx, id)
	return err == nil
}
