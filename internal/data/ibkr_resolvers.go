package data

import (
	"context"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/symbolmapping"
)

// SymbolResolverImpl resolves broker symbols to internal symbols and
// checks symbol existence. It implements ibkrimport.SymbolResolver.
type SymbolResolverImpl struct {
	repo *SymbolMappingRepository
}

// NewSymbolResolver creates a new symbol resolver backed by the given repository.
func NewSymbolResolver(repo *SymbolMappingRepository) *SymbolResolverImpl {
	return &SymbolResolverImpl{repo: repo}
}

// ResolveBrokerSymbol looks up a broker symbol for a given broker name
// and returns the associated internal symbol. Returns empty string if not found.
func (r *SymbolResolverImpl) ResolveBrokerSymbol(ctx context.Context, brokerName, brokerSymbol string) string {
	bs, err := r.repo.GetBrokerSymbolByBroker(ctx, brokerName, brokerSymbol)
	if err != nil {
		return ""
	}
	sm, err := r.repo.GetByID(ctx, bs.SymbolID)
	if err != nil {
		return ""
	}
	return sm.InternalSymbol
}

// SymbolExists checks if an internal symbol exists in the symbol map.
func (r *SymbolResolverImpl) SymbolExists(ctx context.Context, symbol string) bool {
	_, err := r.repo.GetByInternalSymbol(ctx, symbol)
	return err == nil
}

// BrokerSymbolAdderImpl adds broker symbol mappings to existing symbols.
// It implements ibkrimport.BrokerSymbolAdder.
type BrokerSymbolAdderImpl struct {
	repo    *SymbolMappingRepository
	service *symbolmapping.Service
}

// NewBrokerSymbolAdder creates a new broker symbol adder backed by the given repository and service.
func NewBrokerSymbolAdder(repo *SymbolMappingRepository, service *symbolmapping.Service) *BrokerSymbolAdderImpl {
	return &BrokerSymbolAdderImpl{repo: repo, service: service}
}

// AddBrokerSymbolMapping adds a broker symbol mapping for an existing internal symbol.
func (a *BrokerSymbolAdderImpl) AddBrokerSymbolMapping(ctx context.Context, brokerName, brokerSymbol, internalSymbol string) error {
	sm, err := a.repo.GetByInternalSymbol(ctx, internalSymbol)
	if err != nil {
		return symbolmapping.ErrNotFound
	}
	return a.service.AddBrokerSymbol(ctx, sm.ID, symbolmapping.BrokerSymbolRequest{
		BrokerName:   brokerName,
		BrokerSymbol: brokerSymbol,
	})
}
