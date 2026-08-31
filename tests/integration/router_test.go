package integration

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// stubMarketFetcher implements api.MarketDataFetcher without any network
// access. Integration tests must never reach real market data providers —
// data is seeded directly into the in-memory DB instead.
type stubMarketFetcher struct{}

var errNoNetwork = errors.New("no network access in tests")

func (stubMarketFetcher) FetchQuote(_ context.Context, _ string) (*market.MarketData, error) {
	return nil, errNoNetwork
}

func (stubMarketFetcher) FetchFxRate(_ context.Context, _, _ string) (*market.MarketData, error) {
	return nil, errNoNetwork
}

func (stubMarketFetcher) FetchQuotesBatch(_ context.Context, _ []string) map[string]*market.MarketData {
	return nil
}

func (stubMarketFetcher) FetchHistoricalPricesBatch(_ context.Context, _ []string, _, _ time.Time) (map[string][]market.HistoricalPrice, []string) {
	return nil, nil
}

func (stubMarketFetcher) FetchSymbolDetails(_ context.Context, _ string) (*symbol.SymbolDetails, error) {
	return nil, errNoNetwork
}

// newTestRouter builds a router wired like production but with a stubbed
// market data fetcher, so no test makes a real network call.
func newTestRouter(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	router, _ := api.Router(db, testLogger(),
		api.WithTemplatesDir("../../templates"),
		api.WithMarketDataFetcher(stubMarketFetcher{}))
	return router
}
