package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
)

var ErrNotFound = fmt.Errorf("not found")

// PortfolioRepository provides data access for portfolios.
type PortfolioRepository struct {
	db *sql.DB
}

// NewPortfolioRepository creates a new portfolio repository.
func NewPortfolioRepository(db *sql.DB) *PortfolioRepository {
	return &PortfolioRepository{db: db}
}

// scanPortfolio scans a portfolio row from a sql.Row or sql.Rows.
// SQLite stores datetime as text, so we scan into string and parse.
func scanPortfolio(scanDest ...interface{}) (*portfolio.Portfolio, error) {
	if len(scanDest) != 5 {
		return nil, fmt.Errorf("expected 5 scan destinations, got %d", len(scanDest))
	}

	idPtr := scanDest[0].(*int64)
	namePtr := scanDest[1].(*string)
	currencyPtr := scanDest[2].(*string)
	createdAtPtr := scanDest[3].(*string)
	updatedAtPtr := scanDest[4].(*string)

	createdAt, err := parseTime(*createdAtPtr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(*updatedAtPtr)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &portfolio.Portfolio{
		ID:        *idPtr,
		Name:      *namePtr,
		Currency:  *currencyPtr,
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
	query := `INSERT INTO portfolios (name, currency, created_at, updated_at) VALUES (?, ?, ?, ?)`
	result, err := r.db.ExecContext(ctx, query, p.Name, p.Currency, p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert portfolio: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	p.ID = id
	return nil
}

// GetByID retrieves a portfolio by its ID.
func (r *PortfolioRepository) GetByID(ctx context.Context, id int64) (*portfolio.Portfolio, error) {
	query := `SELECT id, name, currency, created_at, updated_at FROM portfolios WHERE id = ?`
	var idVal int64
	var name, currency, createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, query, id).Scan(&idVal, &name, &currency, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get portfolio by id %d: %w", id, err)
	}
	p, err := scanPortfolio(&idVal, &name, &currency, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan portfolio: %w", err)
	}
	return p, nil
}

// GetAll retrieves all portfolios with optional pagination.
// If limit is 0, no LIMIT is applied (return all).
func (r *PortfolioRepository) GetAll(ctx context.Context, limit, offset int) ([]portfolio.Portfolio, error) {
	query := `SELECT id, name, currency, created_at, updated_at FROM portfolios ORDER BY created_at DESC`
	args := []interface{}{}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list portfolios: %w", err)
	}
	defer rows.Close()

	var portfolios []portfolio.Portfolio
	for rows.Next() {
		var idVal int64
		var name, currency, createdAt, updatedAt string
		if err := rows.Scan(&idVal, &name, &currency, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan portfolio row: %w", err)
		}
		p, err := scanPortfolio(&idVal, &name, &currency, &createdAt, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse portfolio: %w", err)
		}
		portfolios = append(portfolios, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portfolios: %w", err)
	}

	return portfolios, nil
}

// Update modifies an existing portfolio.
func (r *PortfolioRepository) Update(ctx context.Context, p *portfolio.Portfolio) error {
	query := `UPDATE portfolios SET name = ?, currency = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, p.Name, p.Currency, p.UpdatedAt.Format(time.RFC3339), p.ID)
	if err != nil {
		return fmt.Errorf("update portfolio %d: %w", p.ID, err)
	}
	return nil
}

// Delete removes a portfolio by ID.
func (r *PortfolioRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM portfolios WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete portfolio %d: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete rows: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByName retrieves a portfolio by its name.
func (r *PortfolioRepository) GetByName(ctx context.Context, name string) (*portfolio.Portfolio, error) {
	query := `SELECT id, name, currency, created_at, updated_at FROM portfolios WHERE name = ?`
	var idVal int64
	var nameVal, currency, createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, query, name).Scan(&idVal, &nameVal, &currency, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get portfolio by name %q: %w", name, err)
	}
	p, err := scanPortfolio(&idVal, &nameVal, &currency, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan portfolio: %w", err)
	}
	return p, nil
}
