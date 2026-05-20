package portfolio

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// Repository defines the data access interface for portfolios.
type Repository interface {
	Create(ctx context.Context, p *Portfolio) error
	GetByID(ctx context.Context, id int64) (*Portfolio, error)
	GetAll(ctx context.Context, limit, offset int) ([]Portfolio, error)
	ListAll(ctx context.Context) ([]Portfolio, error)
	Update(ctx context.Context, p *Portfolio) error
	Delete(ctx context.Context, id int64) error
	GetByName(ctx context.Context, name string) (*Portfolio, error)
}

// ErrNotFound indicates the requested portfolio does not exist.
var ErrNotFound = fmt.Errorf("portfolio not found")

// ErrNameExists indicates a portfolio with the same name already exists.
var ErrNameExists = fmt.Errorf("portfolio name already exists")

// ErrInvalidName indicates the portfolio name is invalid.
var ErrInvalidName = fmt.Errorf("invalid portfolio name")

// ErrInvalidCurrency indicates the currency code is invalid.
var ErrInvalidCurrency = fmt.Errorf("invalid currency code")

const defaultLimit = 50

// defaultCurrency is the fallback when no currency is specified.
const defaultCurrency = "USD"

// Service handles portfolio business logic.
type Service struct {
	repo Repository
}

// NewService creates a new portfolio service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// iso4217Regex matches 3-letter uppercase currency codes.
var iso4217Regex = regexp.MustCompile(`^[A-Z]{3}$`)

// Create creates a new portfolio.
// If currency is empty, it defaults to "USD".
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Portfolio, error) {
	if err := validateName(req.Name); err != nil {
		return nil, ErrInvalidName
	}
	currency := req.Currency
	if currency == "" {
		currency = defaultCurrency
	}
	if err := validateCurrency(currency); err != nil {
		return nil, ErrInvalidCurrency
	}

	// Check for duplicate name
	if _, err := s.repo.GetByName(ctx, req.Name); err == nil {
		return nil, ErrNameExists
	}

	now := time.Now()
	p := &Portfolio{
		Name:      req.Name,
		Currency:  currency,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create portfolio: %w", err)
	}

	return p, nil
}

// Get retrieves a portfolio by ID.
func (s *Service) Get(ctx context.Context, id int64) (*Portfolio, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get portfolio: %w", err)
	}
	return p, nil
}

// List retrieves all portfolios with pagination.
// A limit of 0 (or negative) defaults to defaultLimit (50).
func (s *Service) List(ctx context.Context, limit, offset int) ([]Portfolio, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	portfolios, err := s.repo.GetAll(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list portfolios: %w", err)
	}
	return portfolios, nil
}

// ListAll returns all portfolios without pagination.
func (s *Service) ListAll(ctx context.Context) ([]Portfolio, error) {
	portfolios, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all portfolios: %w", err)
	}
	return portfolios, nil
}

// Update updates an existing portfolio.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*Portfolio, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get portfolio for update: %w", err)
	}

	changed := false

	if req.Name != nil {
		if err := validateName(*req.Name); err != nil {
			return nil, ErrInvalidName
		}
		// Check for duplicate name (excluding current portfolio)
		existing, err := s.repo.GetByName(ctx, *req.Name)
		if err == nil && existing.ID != id {
			return nil, ErrNameExists
		}
		p.Name = *req.Name
		changed = true
	}

	if req.Currency != nil {
		if err := validateCurrency(*req.Currency); err != nil {
			return nil, ErrInvalidCurrency
		}
		p.Currency = *req.Currency
		changed = true
	}

	if changed {
		p.UpdatedAt = time.Now()
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("update portfolio: %w", err)
	}

	return p, nil
}

// Delete removes a portfolio by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete portfolio: %w", err)
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

func validateCurrency(currency string) error {
	if !iso4217Regex.MatchString(currency) {
		return fmt.Errorf("must be a valid ISO 4217 currency code (e.g. USD, EUR, GBP)")
	}
	return nil
}
