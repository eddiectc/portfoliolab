package data

import (
	"context"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/symbolmapping"
)

// SymbolCheckerImpl checks symbol existence via the symbol mapping repository.
// It implements transaction.SymbolChecker.
type SymbolCheckerImpl struct {
	repo *SymbolMappingRepository
}

// NewSymbolChecker creates a new symbol checker backed by the given repository.
func NewSymbolChecker(repo *SymbolMappingRepository) *SymbolCheckerImpl {
	return &SymbolCheckerImpl{repo: repo}
}

// SymbolExists returns true if a symbol mapping with the given internal symbol exists.
func (c *SymbolCheckerImpl) SymbolExists(ctx context.Context, symbol string) bool {
	_, err := c.repo.GetByInternalSymbol(ctx, symbol)
	return err == nil
}

// SymbolCreatorImpl creates symbol mappings via the symbol mapping service.
// It implements transaction.SymbolCreator.
type SymbolCreatorImpl struct {
	service *symbolmapping.Service
}

// NewSymbolCreator creates a new symbol creator backed by the given service.
func NewSymbolCreator(service *symbolmapping.Service) *SymbolCreatorImpl {
	return &SymbolCreatorImpl{service: service}
}

// CreateSymbol creates a new symbol mapping with the given internal and market data symbol.
func (c *SymbolCreatorImpl) CreateSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error {
	_, err := c.service.Create(ctx, symbolmapping.CreateRequest{
		InternalSymbol:   internalSymbol,
		MarketDataSymbol: marketDataSymbol,
	})
	return err
}
