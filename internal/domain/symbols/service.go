package symbols

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// ErrNotFound indicates no cached symbol details exist for the requested symbol.
var ErrNotFound = errors.New("symbol details not found")

// StaleThreshold is the duration after which cached symbol details are
// considered stale and eligible for background refresh.
const StaleThreshold = 7 * 24 * time.Hour

// SymbolDetailsRepository persists and retrieves cached symbol details.
type SymbolDetailsRepository interface {
	Upsert(ctx context.Context, details *symbol.SymbolDetails) error
	GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbol.SymbolDetails, error)
	ListStale(ctx context.Context, olderThan time.Time) ([]symbol.StaleSymbol, error)
}

// Service orchestrates fetching, caching, and retrieval of symbol details.
type Service struct {
	repo    SymbolDetailsRepository
	fetcher market.SymbolDetailsFetcher
}

// NewService creates a new symbol details service.
func NewService(repo SymbolDetailsRepository, fetcher market.SymbolDetailsFetcher) *Service {
	return &Service{repo: repo, fetcher: fetcher}
}

// FetchAndStore fetches symbol details from the market data provider and
// stores them in the cache. Returns an error if the fetch or storage fails.
func (s *Service) FetchAndStore(ctx context.Context, internalSymbol, marketDataSymbol string) error {
	details, err := s.fetcher.FetchSymbolDetails(ctx, marketDataSymbol)
	if err != nil {
		return fmt.Errorf("fetch symbol details for %s: %w", internalSymbol, err)
	}

	details.InternalSymbol = internalSymbol

	if err := s.repo.Upsert(ctx, details); err != nil {
		return fmt.Errorf("store symbol details for %s: %w", internalSymbol, err)
	}

	return nil
}

// GetByInternalSymbol retrieves cached symbol details for a symbol.
// Returns an error (wrapping ErrNotFound) if no details are cached.
func (s *Service) GetByInternalSymbol(ctx context.Context, internalSymbol string) (*symbol.SymbolDetails, error) {
	details, err := s.repo.GetByInternalSymbol(ctx, internalSymbol)
	if err != nil {
		return nil, fmt.Errorf("get symbol details for %s: %w", internalSymbol, err)
	}
	return details, nil
}

// GetStaleSymbols returns symbols whose cached details are older than the
// stale threshold (7 days). Returns internal_symbol + market_data_symbol pairs
// suitable for refresh. Cash symbols ($CASH*) are excluded since they have
// no market data to fetch.
func (s *Service) GetStaleSymbols(ctx context.Context) ([]symbol.StaleSymbol, error) {
	olderThan := time.Now().Add(-StaleThreshold)
	stale, err := s.repo.ListStale(ctx, olderThan)
	if err != nil {
		return nil, fmt.Errorf("list stale symbol details: %w", err)
	}
	var filtered []symbol.StaleSymbol
	for _, s := range stale {
		if strings.HasPrefix(s.InternalSymbol, "$CASH") {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered, nil
}

// RefreshSymbol re-fetches and updates the cached details for a single symbol.
// Returns an error if the fetch or storage fails.
func (s *Service) RefreshSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error {
	return s.FetchAndStore(ctx, internalSymbol, marketDataSymbol)
}
