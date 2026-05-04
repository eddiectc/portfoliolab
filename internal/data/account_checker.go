package data

import (
	"context"
)

// AccountCheckerImpl checks account existence via the account repository.
// It implements transaction.AccountChecker.
type AccountCheckerImpl struct {
	repo *AccountRepository
}

// NewAccountChecker creates a new account checker backed by the given repository.
func NewAccountChecker(repo *AccountRepository) *AccountCheckerImpl {
	return &AccountCheckerImpl{repo: repo}
}

// AccountExists returns true if an account with the given ID exists.
func (c *AccountCheckerImpl) AccountExists(ctx context.Context, id int64) bool {
	_, err := c.repo.GetByID(ctx, id)
	return err == nil
}
