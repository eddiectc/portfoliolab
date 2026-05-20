package account

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Repository defines the data access interface for accounts.
type Repository interface {
	Create(ctx context.Context, a *Account) error
	GetByID(ctx context.Context, id int64) (*Account, error)
	GetAll(ctx context.Context, limit, offset int) ([]Account, error)
	ListAll(ctx context.Context) ([]Account, error)
	GetByPortfolio(ctx context.Context, portfolioID int64, limit, offset int) ([]Account, error)
	GetByName(ctx context.Context, name string) (*Account, error)
	Update(ctx context.Context, a *Account) error
	Delete(ctx context.Context, id int64) error
}

// PortfolioChecker defines the interface for checking portfolio existence.
type PortfolioChecker interface {
	PortfolioExists(ctx context.Context, id int64) bool
}

// ErrNotFound indicates the requested account does not exist.
var ErrNotFound = fmt.Errorf("account not found")

// ErrNameExists indicates an account with the same name already exists.
var ErrNameExists = fmt.Errorf("account name already exists")

// ErrInvalidName indicates the account name is invalid.
var ErrInvalidName = fmt.Errorf("invalid account name")

// ErrPortfolioNotFound indicates the referenced portfolio does not exist.
var ErrPortfolioNotFound = fmt.Errorf("portfolio not found")

const defaultLimit = 50

// Service handles account business logic.
type Service struct {
	repo       Repository
	portfolios PortfolioChecker
}

// NewService creates a new account service.
func NewService(repo Repository, portfolios PortfolioChecker) *Service {
	return &Service{repo: repo, portfolios: portfolios}
}

// Create creates a new account.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Account, error) {
	name := strings.TrimSpace(req.Name)

	if err := validateName(name); err != nil {
		return nil, ErrInvalidName
	}

	if !s.portfolios.PortfolioExists(ctx, req.PortfolioID) {
		return nil, ErrPortfolioNotFound
	}

	// Check for duplicate name (globally unique)
	if _, err := s.repo.GetByName(ctx, name); err == nil {
		return nil, ErrNameExists
	}

	now := time.Now()
	a := &Account{
		Name:        name,
		PortfolioID: req.PortfolioID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	return a, nil
}

// Get retrieves an account by ID.
func (s *Service) Get(ctx context.Context, id int64) (*Account, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return a, nil
}

// List retrieves all accounts with pagination.
// A limit of 0 (or negative) defaults to defaultLimit (50).
func (s *Service) List(ctx context.Context, limit, offset int) ([]Account, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	accounts, err := s.repo.GetAll(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return accounts, nil
}

// ListAll returns all accounts without pagination.
func (s *Service) ListAll(ctx context.Context) ([]Account, error) {
	accounts, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all accounts: %w", err)
	}
	return accounts, nil
}

// ListByPortfolio retrieves accounts for a specific portfolio with pagination.
// A limit of 0 (or negative) defaults to defaultLimit (50).
func (s *Service) ListByPortfolio(ctx context.Context, portfolioID int64, limit, offset int) ([]Account, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	accounts, err := s.repo.GetByPortfolio(ctx, portfolioID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list accounts by portfolio: %w", err)
	}
	return accounts, nil
}

// Update updates an existing account.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*Account, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get account for update: %w", err)
	}

	changed := false

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if err := validateName(name); err != nil {
			return nil, ErrInvalidName
		}
		// Check for duplicate name (excluding current account)
		existing, err := s.repo.GetByName(ctx, name)
		if err == nil && existing.ID != id {
			return nil, ErrNameExists
		}
		a.Name = name
		changed = true
	}

	if req.PortfolioID != nil {
		if !s.portfolios.PortfolioExists(ctx, *req.PortfolioID) {
			return nil, ErrPortfolioNotFound
		}
		a.PortfolioID = *req.PortfolioID
		changed = true
	}

	if changed {
		a.UpdatedAt = time.Now()
	}

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, fmt.Errorf("update account: %w", err)
	}

	return a, nil
}

// Delete removes an account by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 100 {
		return fmt.Errorf("name must be at most 100 characters")
	}
	return nil
}
