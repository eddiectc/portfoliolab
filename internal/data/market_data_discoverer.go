package data

import (
	"context"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/marketcache"
)

// marketDataDiscoverer implements marketcache.SymbolDiscoverer.
// Symbols come from symbol_mappings; FX pairs from transactions.
type marketDataDiscoverer struct {
	symbolRepo *SymbolMappingRepository
	txnRepo    *TransactionRepository
}

// NewMarketDataDiscoverer creates a SymbolDiscoverer for the market cache.
func NewMarketDataDiscoverer(symbolRepo *SymbolMappingRepository, txnRepo *TransactionRepository) marketcache.SymbolDiscoverer {
	return &marketDataDiscoverer{
		symbolRepo: symbolRepo,
		txnRepo:    txnRepo,
	}
}

func (d *marketDataDiscoverer) AllSymbols(ctx context.Context) ([]string, error) {
	return d.symbolRepo.AllMarketDataSymbols(ctx)
}

func (d *marketDataDiscoverer) ActiveFxPairs(ctx context.Context) (map[string]time.Time, error) {
	return d.txnRepo.GetFxPairsByOpenPositions(ctx)
}
