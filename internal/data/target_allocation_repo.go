package data

import (
	"context"
	"fmt"
	"time"

	"github.com/eddiectc/portfoliolab/internal/data/queries"
	"github.com/eddiectc/portfoliolab/internal/domain/allocation"
	"github.com/govalues/decimal"
)

// TargetAllocationRepository provides data access for target allocations,
// delegating to sqlc-generated queries.
type TargetAllocationRepository struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewTargetAllocationRepository creates a new target allocation repository.
func NewTargetAllocationRepository(db queries.DBTX) *TargetAllocationRepository {
	return &TargetAllocationRepository{
		q:  queries.New(),
		db: db,
	}
}

// GetByPortfolio retrieves all target allocations for a portfolio.
func (r *TargetAllocationRepository) GetByPortfolio(ctx context.Context, portfolioID int64) ([]allocation.TargetAllocation, error) {
	rows, err := r.q.GetTargetAllocationsByPortfolio(ctx, r.db, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("get target allocations for portfolio %d: %w", portfolioID, err)
	}

	targets := make([]allocation.TargetAllocation, len(rows))
	for i, row := range rows {
		pct, err := decimal.Parse(row.TargetPct)
		if err != nil {
			return nil, fmt.Errorf("parse target_pct for symbol %q: %w", row.Symbol, err)
		}
		targets[i] = allocation.TargetAllocation{
			PortfolioID: row.PortfolioID,
			Symbol:      row.Symbol,
			TargetPct:   pct,
		}
	}
	return targets, nil
}

// Upsert inserts or updates a target allocation for a portfolio+symbol.
func (r *TargetAllocationRepository) Upsert(ctx context.Context, ta allocation.TargetAllocation) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.q.UpsertTargetAllocation(ctx, r.db, queries.UpsertTargetAllocationParams{
		PortfolioID: ta.PortfolioID,
		Symbol:      ta.Symbol,
		TargetPct:   ta.TargetPct.String(),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return fmt.Errorf("upsert target allocation %q: %w", ta.Symbol, err)
	}
	return nil
}

// DeleteBySymbol removes a target allocation for a portfolio+symbol.
func (r *TargetAllocationRepository) DeleteBySymbol(ctx context.Context, portfolioID int64, symbol string) error {
	_, err := r.q.DeleteTargetAllocationBySymbol(ctx, r.db, queries.DeleteTargetAllocationBySymbolParams{
		PortfolioID: portfolioID,
		Symbol:      symbol,
	})
	if err != nil {
		return fmt.Errorf("delete target allocation %q: %w", symbol, err)
	}
	return nil
}

// DeleteByPortfolio removes all target allocations for a portfolio.
func (r *TargetAllocationRepository) DeleteByPortfolio(ctx context.Context, portfolioID int64) error {
	_, err := r.q.DeleteTargetAllocationsByPortfolio(ctx, r.db, portfolioID)
	if err != nil {
		return fmt.Errorf("delete all target allocations for portfolio %d: %w", portfolioID, err)
	}
	return nil
}
