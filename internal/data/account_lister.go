package data

import (
	"context"
	"fmt"

	"github.com/arch-portfolio-lab/portfoliolab/internal/data/queries"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/position"
)

// AccountListerImpl lists accounts for position queries.
// It implements position.AccountLister.
type AccountListerImpl struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewAccountLister creates a new account lister backed by the given repository.
func NewAccountLister(repo *AccountRepository) *AccountListerImpl {
	return &AccountListerImpl{
		q:  repo.q,
		db: repo.db,
	}
}

// GetAllAccounts retrieves all accounts without pagination, including
// portfolio currency for FX conversion.
func (l *AccountListerImpl) GetAllAccounts(ctx context.Context) ([]position.AccountRef, error) {
	accounts, err := l.q.ListAllAccountsWithPortfolioCurrency(ctx, l.db)
	if err != nil {
		return nil, fmt.Errorf("list all accounts: %w", err)
	}
	result := make([]position.AccountRef, len(accounts))
	for i, a := range accounts {
		result[i] = position.AccountRef{
			ID:                a.ID,
			Name:              a.Name,
			PortfolioID:       a.PortfolioID,
			PortfolioCurrency: a.PortfolioCurrency,
		}
	}
	return result, nil
}

// GetAccountsByPortfolio retrieves all accounts for a portfolio without pagination,
// including portfolio currency for FX conversion.
func (l *AccountListerImpl) GetAccountsByPortfolio(ctx context.Context, portfolioID int64) ([]position.AccountRef, error) {
	accounts, err := l.q.GetAccountsByPortfolioWithCurrency(ctx, l.db, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("list accounts for portfolio %d: %w", portfolioID, err)
	}
	result := make([]position.AccountRef, len(accounts))
	for i, a := range accounts {
		result[i] = position.AccountRef{
			ID:                a.ID,
			Name:              a.Name,
			PortfolioID:       a.PortfolioID,
			PortfolioCurrency: a.PortfolioCurrency,
		}
	}
	return result, nil
}
