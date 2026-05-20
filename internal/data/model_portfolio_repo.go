package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// ModelPortfolioRepository provides data access for model portfolios,
// delegating to sqlc-generated queries. Entries are stored as JSON in the
// `entries` TEXT column and marshaled/unmarshaled at the repository boundary.
type ModelPortfolioRepository struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewModelPortfolioRepository creates a new model portfolio repository.
func NewModelPortfolioRepository(db queries.DBTX) *ModelPortfolioRepository {
	return &ModelPortfolioRepository{
		q:  queries.New(),
		db: db,
	}
}

// Create inserts a new model portfolio. Entries are marshaled to JSON.
func (r *ModelPortfolioRepository) Create(ctx context.Context, mp modelportfolio.ModelPortfolio) (modelportfolio.ModelPortfolio, error) {
	entriesJSON, err := json.Marshal(mp.Entries)
	if err != nil {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("marshal entries for model portfolio %q: %w", mp.Name, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := r.q.CreateModelPortfolio(ctx, r.db, queries.CreateModelPortfolioParams{
		Name:      mp.Name,
		Entries:   string(entriesJSON),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("create model portfolio %q: %w", mp.Name, err)
	}

	return toModelPortfolio(row)
}

// GetByID retrieves a model portfolio by its ID.
func (r *ModelPortfolioRepository) GetByID(ctx context.Context, id int64) (modelportfolio.ModelPortfolio, error) {
	row, err := r.q.GetModelPortfolioByID(ctx, r.db, id)
	if err != nil {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("get model portfolio %d: %w", id, err)
	}

	return toModelPortfolio(row)
}

// GetByName retrieves a model portfolio by its name (for uniqueness checks).
func (r *ModelPortfolioRepository) GetByName(ctx context.Context, name string) (modelportfolio.ModelPortfolio, error) {
	row, err := r.q.GetModelPortfolioByName(ctx, r.db, name)
	if err != nil {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("get model portfolio by name %q: %w", name, err)
	}

	return toModelPortfolio(row)
}

// List returns model portfolios with pagination.
func (r *ModelPortfolioRepository) List(ctx context.Context, limit, offset int) ([]modelportfolio.ModelPortfolio, error) {
	rows, err := r.q.ListModelPortfolios(ctx, r.db, queries.ListModelPortfoliosParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list model portfolios: %w", err)
	}

	result := make([]modelportfolio.ModelPortfolio, len(rows))
	for i, row := range rows {
		p, err := toModelPortfolio(row)
		if err != nil {
			return nil, err
		}
		result[i] = p
	}
	return result, nil
}

// Update updates an existing model portfolio.
func (r *ModelPortfolioRepository) Update(ctx context.Context, mp modelportfolio.ModelPortfolio) (modelportfolio.ModelPortfolio, error) {
	entriesJSON, err := json.Marshal(mp.Entries)
	if err != nil {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("marshal entries for model portfolio %d: %w", mp.ID, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := r.q.UpdateModelPortfolio(ctx, r.db, queries.UpdateModelPortfolioParams{
		Name:      mp.Name,
		Entries:   string(entriesJSON),
		UpdatedAt: now,
		ID:        mp.ID,
	})
	if err != nil {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("update model portfolio %d: %w", mp.ID, err)
	}

	return toModelPortfolio(row)
}

// Delete removes a model portfolio by its ID.
func (r *ModelPortfolioRepository) Delete(ctx context.Context, id int64) error {
	if err := r.q.DeleteModelPortfolio(ctx, r.db, id); err != nil {
		return fmt.Errorf("delete model portfolio %d: %w", id, err)
	}
	return nil
}

// toModelPortfolio converts an sqlc-generated ModelPortfolio row to the domain type,
// unmarshaling the entries JSON column.
func toModelPortfolio(row queries.ModelPortfolio) (modelportfolio.ModelPortfolio, error) {
	var entries []modelportfolio.ModelPortfolioEntry
	if err := json.Unmarshal([]byte(row.Entries), &entries); err != nil {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("unmarshal entries for model portfolio %d (%q): %w", row.ID, row.Name, err)
	}

	return modelportfolio.ModelPortfolio{
		ID:        row.ID,
		Name:      row.Name,
		Entries:   entries,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}
