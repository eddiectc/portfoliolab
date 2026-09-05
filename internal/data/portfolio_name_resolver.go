package data

import (
	"context"

	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
)

// PortfolioNameResolverImpl returns the display name of a portfolio.
// It implements comparison.PortfolioNameSource.
type PortfolioNameResolverImpl struct {
	svc *portfolio.Service
}

// NewPortfolioNameResolver creates a new portfolio name resolver.
func NewPortfolioNameResolver(svc *portfolio.Service) *PortfolioNameResolverImpl {
	return &PortfolioNameResolverImpl{svc: svc}
}

// GetPortfolioName returns the name of the portfolio with the given ID.
// Returns an error if the portfolio does not exist.
func (r *PortfolioNameResolverImpl) GetPortfolioName(ctx context.Context, portfolioID int64) (string, error) {
	p, err := r.svc.Get(ctx, portfolioID)
	if err != nil {
		return "", err
	}
	return p.Name, nil
}
