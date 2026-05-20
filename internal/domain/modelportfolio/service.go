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
	Update(ctx context.Context, mp ModelPortfolio) (ModelPortfolio, error)
	Delete(ctx context.Context, id int64) error
}

// Service handles model portfolio business logic including validation
// and CRUD operations.
type Service struct {
	repo Repository
}

// NewService creates a new model portfolio service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates and persists a new model portfolio.
func (s *Service) Create(ctx context.Context, req CreateRequest) (ModelPortfolio, error) {
	if err := ValidateCreateRequest(req); err != nil {
		return ModelPortfolio{}, err
	}

	// Check name uniqueness.
	_, err := s.repo.GetByName(ctx, req.Name)
	if err == nil {
		return ModelPortfolio{}, ErrNameExists
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

// Update validates and updates an existing model portfolio.
// Name is only changed if provided in the request.
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
