package modelportfolio

import (
	"context"
	"fmt"
)

// Repository provides data access for model portfolios.
type Repository interface {
	Create(ctx context.Context, mp ModelPortfolio) (ModelPortfolio, error)
	GetByID(ctx context.Context, id int64) (ModelPortfolio, error)
	GetByName(ctx context.Context, name string) (ModelPortfolio, error)
	List(ctx context.Context, limit, offset int) ([]ModelPortfolio, error)
	ListAll(ctx context.Context) ([]ModelPortfolio, error)
	Update(ctx context.Context, mp ModelPortfolio) (ModelPortfolio, error)
	Delete(ctx context.Context, id int64) error
}

// SymbolChecker defines the interface for checking symbol existence.
type SymbolChecker interface {
	SymbolExists(ctx context.Context, symbol string) bool
}

// SymbolCreator defines the interface for creating symbols (used for inline
// symbol creation when building model portfolios).
type SymbolCreator interface {
	CreateSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error
}

// Service handles model portfolio business logic including validation
// and CRUD operations.
type Service struct {
	repo         Repository
	symbolCheck  SymbolChecker
	symbolCreate SymbolCreator
}

// NewService creates a new model portfolio service.
// symbolCheck and symbolCreate may be nil if inline symbol creation is not needed.
func NewService(repo Repository, symbolCheck SymbolChecker, symbolCreate SymbolCreator) *Service {
	return &Service{
		repo:         repo,
		symbolCheck:  symbolCheck,
		symbolCreate: symbolCreate,
	}
}

// Create validates and persists a new model portfolio.
// If symbol checking/creation is configured, missing symbols are auto-created.
func (s *Service) Create(ctx context.Context, req CreateRequest) (ModelPortfolio, error) {
	if err := ValidateCreateRequest(req); err != nil {
		return ModelPortfolio{}, err
	}

	// Check name uniqueness.
	_, err := s.repo.GetByName(ctx, req.Name)
	if err == nil {
		return ModelPortfolio{}, ErrNameExists
	}

	// Ensure all entry symbols exist; auto-create missing ones.
	if err := s.ensureSymbols(ctx, req.Entries); err != nil {
		return ModelPortfolio{}, fmt.Errorf("ensure symbols: %w", err)
	}

	mp := ModelPortfolio{
		Name:    req.Name,
		Entries: req.Entries,
	}

	result, err := s.repo.Create(ctx, mp)
	if err != nil {
		return ModelPortfolio{}, fmt.Errorf("create model portfolio: %w", err)
	}

	return result, nil
}

// Get retrieves a model portfolio by its ID.
func (s *Service) Get(ctx context.Context, id int64) (ModelPortfolio, error) {
	mp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ModelPortfolio{}, fmt.Errorf("get model portfolio: %w", err)
	}
	return mp, nil
}

// List returns model portfolios with pagination.
// Returns an empty slice (not nil) when no portfolios exist.
func (s *Service) List(ctx context.Context, limit, offset int) ([]ModelPortfolio, error) {
	ports, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list model portfolios: %w", err)
	}
	if ports == nil {
		ports = []ModelPortfolio{}
	}
	return ports, nil
}

// ListAll returns all model portfolios without pagination.
// Returns an empty slice (not nil) when no portfolios exist.
func (s *Service) ListAll(ctx context.Context) ([]ModelPortfolio, error) {
	ports, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all model portfolios: %w", err)
	}
	if ports == nil {
		ports = []ModelPortfolio{}
	}
	return ports, nil
}

// Update validates and updates an existing model portfolio.
// Name is only changed if provided in the request.
// If symbol checking/creation is configured, missing symbols are auto-created.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (ModelPortfolio, error) {
	if err := ValidateUpdateRequest(req); err != nil {
		return ModelPortfolio{}, err
	}

	// Fetch existing portfolio.
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ModelPortfolio{}, fmt.Errorf("get model portfolio for update: %w", err)
	}

	// Apply name change if provided.
	if req.Name != nil {
		// Check uniqueness of new name against other portfolios.
		existingByName, err := s.repo.GetByName(ctx, *req.Name)
		if err == nil && existingByName.ID != id {
			return ModelPortfolio{}, ErrNameExists
		}
		existing.Name = *req.Name
	}

	// Ensure all entry symbols exist; auto-create missing ones.
	if err := s.ensureSymbols(ctx, req.Entries); err != nil {
		return ModelPortfolio{}, fmt.Errorf("ensure symbols: %w", err)
	}

	existing.Entries = req.Entries

	result, err := s.repo.Update(ctx, existing)
	if err != nil {
		return ModelPortfolio{}, fmt.Errorf("update model portfolio: %w", err)
	}

	return result, nil
}

// Delete removes a model portfolio by its ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	// Verify it exists first (for explicit error).
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("model portfolio not found")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete model portfolio: %w", err)
	}
	return nil
}

// GetAllForSelector returns lightweight summaries for dropdown selectors
// (id, name, entry count) without the full entries payload.
func (s *Service) GetAllForSelector(ctx context.Context) ([]ModelPortfolioSummary, error) {
	ports, err := s.repo.List(ctx, 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("list model portfolios for selector: %w", err)
	}

	summaries := make([]ModelPortfolioSummary, len(ports))
	for i, p := range ports {
		summaries[i] = ModelPortfolioSummary{
			ID:         p.ID,
			Name:       p.Name,
			EntryCount: len(p.Entries),
		}
	}
	return summaries, nil
}

// ensureSymbols checks that every symbol in the entries exists in the system.
// If symbol checking/creation is configured and a symbol is missing, it is
// auto-created (internalSymbol == ticker, marketDataSymbol == ticker).
// If symbol checking is not configured, the check is skipped (backward compat).
func (s *Service) ensureSymbols(ctx context.Context, entries []ModelPortfolioEntry) error {
	if s.symbolCheck == nil || s.symbolCreate == nil {
		return nil
	}
	for _, e := range entries {
		if !s.symbolCheck.SymbolExists(ctx, e.Symbol) {
			if err := s.symbolCreate.CreateSymbol(ctx, e.Symbol, e.Symbol); err != nil {
				return fmt.Errorf("create symbol %q: %w", e.Symbol, err)
			}
		}
	}
	return nil
}
