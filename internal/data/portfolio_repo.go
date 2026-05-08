package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/data/queries"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
)

// ErrNotFound indicates the requested row does not exist.
var ErrNotFound = fmt.Errorf("not found")

// PortfolioRepository provides data access for portfolios,
// delegating to sqlc-generated queries.
type PortfolioRepository struct {
	q    *queries.Queries
	db   queries.DBTX
}

// NewPortfolioRepository creates a new portfolio repository.
func NewPortfolioRepository(db *sql.DB) *PortfolioRepository {
	return &PortfolioRepository{
		q:  queries.New(),
		db: db,
	}
}

// toDomain converts a sqlc Portfolio to a domain Portfolio.
// SQLite stores timestamps as text, so sqlc generates string fields;
// this function parses them back to time.Time.
func toDomain(p queries.Portfolio) (*portfolio.Portfolio, error) {
	createdAt, err := parseTime(p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &portfolio.Portfolio{
		ID:        p.ID,
		Name:      p.Name,
		Currency:  p.Currency,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// parseTime parses a SQLite datetime string into time.Time.
// SQLite datetime format: "YYYY-MM-DD HH:MM:SS"
func parseTime(s string) (time.Time, error) {
	// Try RFC3339 first (used when we write timestamps)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Fall back to SQLite default format
	return time.Parse("2006-01-02 15:04:05", s)
}

// Create inserts a new portfolio and returns it with the generated ID.
func (r *PortfolioRepository) Create(ctx context.Context, p *portfolio.Portfolio) error {
	result, err := r.q.CreatePortfolio(ctx, r.db, queries.CreatePortfolioParams{
		Name:      p.Name,
		Currency:  p.Currency,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("insert portfolio: %w", err)
	}
	p.ID = result.ID
	return nil
}

// GetByID retrieves a portfolio by its ID.
func (r *PortfolioRepository) GetByID(ctx context.Context, id int64) (*portfolio.Portfolio, error) {
	p, err := r.q.GetPortfolio(ctx, r.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get portfolio by id %d: %w", id, err)
	}
	return toDomain(p)
}

// GetAll retrieves all portfolios with pagination.
// If limit is 0, defaults are applied by the caller.
func (r *PortfolioRepository) GetAll(ctx context.Context, limit, offset int) ([]portfolio.Portfolio, error) {
	ps, err := r.q.ListPortfolios(ctx, r.db, queries.ListPortfoliosParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list portfolios: %w", err)
	}

	portfolios := make([]portfolio.Portfolio, len(ps))
	for i, p := range ps {
		d, err := toDomain(p)
		if err != nil {
			return nil, fmt.Errorf("parse portfolio %d: %w", p.ID, err)
		}
		portfolios[i] = *d
	}
	return portfolios, nil
}

// Update modifies an existing portfolio.
func (r *PortfolioRepository) Update(ctx context.Context, p *portfolio.Portfolio) error {
	_, err := r.q.UpdatePortfolio(ctx, r.db, queries.UpdatePortfolioParams{
		Name:      p.Name,
		Currency:  p.Currency,
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
		ID:        p.ID,
	})
	if err != nil {
		return fmt.Errorf("update portfolio %d: %w", p.ID, err)
	}
	return nil
}

// Delete removes a portfolio by ID.
func (r *PortfolioRepository) Delete(ctx context.Context, id int64) error {
	rows, err := r.q.DeletePortfolio(ctx, r.db, id)
	if err != nil {
		return fmt.Errorf("delete portfolio %d: %w", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByName retrieves a portfolio by its name.
func (r *PortfolioRepository) GetByName(ctx context.Context, name string) (*portfolio.Portfolio, error) {
	p, err := r.q.GetPortfolioByName(ctx, r.db, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get portfolio by name %q: %w", name, err)
	}
	return toDomain(p)
}
