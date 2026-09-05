package data

import (
	"context"
	"fmt"

	"github.com/eddiectc/portfoliolab/internal/data/queries"
)

// MarketDataSymbolResolverImpl maps an internal symbol to its market data provider
// symbol (e.g., Yahoo Finance ticker) by looking up the symbol mapping.
type MarketDataSymbolResolverImpl struct {
	q  *queries.Queries
	db queries.DBTX
}

// NewMarketDataSymbolResolver creates a new market data symbol resolver.
func NewMarketDataSymbolResolver(repo *SymbolMappingRepository) *MarketDataSymbolResolverImpl {
	return &MarketDataSymbolResolverImpl{
		q:  repo.q,
		db: repo.db,
	}
}

// GetMarketDataSymbol returns the market data provider symbol for the given
// internal symbol. Returns an error if the symbol mapping is not found.
func (r *MarketDataSymbolResolverImpl) GetMarketDataSymbol(ctx context.Context, internalSymbol string) (string, error) {
	sm, err := r.q.GetSymbolMappingByInternalSymbol(ctx, r.db, internalSymbol)
	if err != nil {
		return "", fmt.Errorf("get market data symbol for %q: %w", internalSymbol, err)
	}
	return sm.MarketDataSymbol, nil
}
