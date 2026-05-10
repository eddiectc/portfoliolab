package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
)

// AccountRepository provides data access for accounts,
// delegating to sqlc-generated queries.
type AccountRepository struct {
	q    *queries.Queries
	db   queries.DBTX
}

// NewAccountRepository creates a new account repository.
func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{
		q:  queries.New(),
		db: db,
	}
}

// toAccount converts a sqlc Account to a domain Account.
// SQLite stores timestamps as text, so sqlc generates string fields;
// this function parses them back to time.Time.
func toAccount(a queries.Account) (*account.Account, error) {
	createdAt, err := parseTime(a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &account.Account{
		ID:          a.ID,
		Name:        a.Name,
		PortfolioID: a.PortfolioID,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

// Create inserts a new account and returns it with the generated ID.
func (r *AccountRepository) Create(ctx context.Context, a *account.Account) error {
	result, err := r.q.CreateAccount(ctx, r.db, queries.CreateAccountParams{
		Name:        a.Name,
		PortfolioID: a.PortfolioID,
		CreatedAt:   a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   a.UpdatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert account: %w", err)
	}
	a.ID = result.ID
	return nil
}

// GetByID retrieves an account by its ID.
func (r *AccountRepository) GetByID(ctx context.Context, id int64) (*account.Account, error) {
	a, err := r.q.GetAccount(ctx, r.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account by id %d: %w", id, err)
	}
	return toAccount(a)
}

// GetAll retrieves all accounts with pagination.
func (r *AccountRepository) GetAll(ctx context.Context, limit, offset int) ([]account.Account, error) {
	accounts, err := r.q.ListAccounts(ctx, r.db, queries.ListAccountsParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	result := make([]account.Account, len(accounts))
	for i, a := range accounts {
		d, err := toAccount(a)
		if err != nil {
			return nil, fmt.Errorf("parse account %d: %w", a.ID, err)
		}
		result[i] = *d
	}
	return result, nil
}

// GetByPortfolio retrieves accounts for a specific portfolio with pagination.
func (r *AccountRepository) GetByPortfolio(ctx context.Context, portfolioID int64, limit, offset int) ([]account.Account, error) {
	accounts, err := r.q.GetAccountsByPortfolio(ctx, r.db, queries.GetAccountsByPortfolioParams{
		PortfolioID: portfolioID,
		Limit:       int64(limit),
		Offset:      int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list accounts by portfolio %d: %w", portfolioID, err)
	}

	result := make([]account.Account, len(accounts))
	for i, a := range accounts {
		d, err := toAccount(a)
		if err != nil {
			return nil, fmt.Errorf("parse account %d: %w", a.ID, err)
		}
		result[i] = *d
	}
	return result, nil
}

// GetByName retrieves an account by its name.
func (r *AccountRepository) GetByName(ctx context.Context, name string) (*account.Account, error) {
	a, err := r.q.GetAccountByName(ctx, r.db, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account by name %q: %w", name, err)
	}
	return toAccount(a)
}

// Update modifies an existing account.
func (r *AccountRepository) Update(ctx context.Context, a *account.Account) error {
	_, err := r.q.UpdateAccount(ctx, r.db, queries.UpdateAccountParams{
		Name:        a.Name,
		PortfolioID: a.PortfolioID,
		UpdatedAt:   a.UpdatedAt.Format(time.RFC3339),
		ID:          a.ID,
	})
	if err != nil {
		return fmt.Errorf("update account %d: %w", a.ID, err)
	}
	return nil
}

// Delete removes an account by ID.
func (r *AccountRepository) Delete(ctx context.Context, id int64) error {
	rows, err := r.q.DeleteAccount(ctx, r.db, id)
	if err != nil {
		return fmt.Errorf("delete account %d: %w", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPortfolioID returns the portfolio ID for an account.
func (r *AccountRepository) GetPortfolioID(ctx context.Context, accountID int64) (int64, error) {
	account, err := r.q.GetAccount(ctx, r.db, accountID)
	if err != nil {
		return 0, fmt.Errorf("get account %d: %w", accountID, err)
	}
	return account.PortfolioID, nil
}
