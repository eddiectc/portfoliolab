package marketservice

import (
	"context"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// Service provides market data to consumers.
// It reads from the cache (populated by the MarketCache background service)
// and can trigger live refreshes.
// Consumers don't know or care whether data comes from cache or a live fetch.
type Service struct {
	fetcher MarketDataFetcher
	repo    MarketDataRepository
}

// MarketDataFetcher fetches market data from external providers.
type MarketDataFetcher interface {
	FetchQuotesBatch(ctx context.Context, symbols []string) map[string]*market.MarketData
}

// MarketDataRepository reads and writes cached market data.
type MarketDataRepository interface {
	GetLatestQuotesBatch(ctx context.Context, symbols []string) map[string]*market.MarketData
	GetHistoricalPricesBySymbol(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
	GetLatestPriceDatePerSymbol(ctx context.Context, symbols []string) map[string]*time.Time
	Upsert(ctx context.Context, m *market.MarketData) error
}

// New creates a new market data service.
func New(fetcher MarketDataFetcher, repo MarketDataRepository) *Service {
	return &Service{
		fetcher: fetcher,
		repo:    repo,
	}
}

// GetQuotes returns current quotes for the given symbols from the cache.
// Symbols with no cached quote are omitted from the result.
func (s *Service) GetQuotes(ctx context.Context, symbols []string) map[string]*market.MarketData {
	if s.repo == nil {
		return make(map[string]*market.MarketData)
	}
	return s.repo.GetLatestQuotesBatch(ctx, symbols)
}

// GetHistoricalPrices returns cached historical prices for a symbol within
// [start, end], sorted by date ASC. Returns empty slice if no data found.
func (s *Service) GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.GetHistoricalPricesBySymbol(ctx, symbol, start, end)
}

// GetLatestPriceDatePerSymbol returns the latest cached date per symbol,
// excluding current (date='') entries. Symbols with no cached history are
// omitted from the result.
func (s *Service) GetLatestPriceDatePerSymbol(ctx context.Context, symbols []string) map[string]*time.Time {
	if s.repo == nil {
		return make(map[string]*time.Time)
	}
	return s.repo.GetLatestPriceDatePerSymbol(ctx, symbols)
}

// RefreshResult holds the result of a quote refresh operation.
type RefreshResult struct {
	Refreshed []string // symbols successfully refreshed
	Failed    []string // symbols that failed to refresh
}

// RefreshQuotes fetches current quotes for the given symbols from the live
// provider and upserts them into the cache. Returns which symbols succeeded
// and which failed.
func (s *Service) RefreshQuotes(ctx context.Context, symbols []string) RefreshResult {
	if s.fetcher == nil || len(symbols) == 0 {
		return RefreshResult{
			Refreshed: nil,
			Failed:    symbols,
		}
	}

	quotes := s.fetcher.FetchQuotesBatch(ctx, symbols)

	var refreshed, failed []string
	for _, sym := range symbols {
		quote, found := quotes[sym]
		if !found {
			failed = append(failed, sym)
			continue
		}
		if s.repo != nil {
			if err := s.repo.Upsert(ctx, quote); err != nil {
				failed = append(failed, sym)
				continue
			}
		}
		refreshed = append(refreshed, sym)
	}

	return RefreshResult{
		Refreshed: refreshed,
		Failed:    failed,
	}
}
