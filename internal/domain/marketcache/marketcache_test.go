package marketcache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// ctx is a test context.
var ctx = context.Background()

// --- Mocks ---

type mockFetcher struct {
	mu                   sync.RWMutex
	quotes               map[string]*market.MarketData
	historical           map[string][]market.HistoricalPrice
	historicalFail       map[string]bool
	fetchHistoricalCalls int
	lastStart            time.Time // last start date passed to FetchHistoricalPricesBatch
	lastEnd              time.Time // last end date passed to FetchHistoricalPricesBatch
}

func (m *mockFetcher) FetchQuotesBatch(_ context.Context, symbols []string) map[string]*market.MarketData {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*market.MarketData)
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			result[sym] = q
		}
	}
	return result
}

// benchPricesToday returns a slice of HistoricalPrice for the given benchmark
// symbols with today's date, so gap-fill sees current cache and skips.
func benchPricesToday(symbols []string) map[string][]market.HistoricalPrice {
	nowDate := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), time.Now().UTC().Day(), 0, 0, 0, 0, time.UTC)
	prices := []market.HistoricalPrice{
		{Date: nowDate, Close: decimal.MustNew(500000, 2), Currency: "USD"},
	}
	result := make(map[string][]market.HistoricalPrice, len(symbols))
	for _, sym := range symbols {
		result[sym] = prices
	}
	return result
}

func (m *mockFetcher) FetchHistoricalPricesBatch(_ context.Context, symbols []string, start, end time.Time) (map[string][]market.HistoricalPrice, []string) {
	m.mu.Lock()
	m.fetchHistoricalCalls++
	m.lastStart = start
	m.lastEnd = end
	m.mu.Unlock()

	result := make(map[string][]market.HistoricalPrice)
	var failed []string

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, sym := range symbols {
		if m.historicalFail[sym] {
			failed = append(failed, sym)
			continue
		}
		if prices, ok := m.historical[sym]; ok {
			var filtered []market.HistoricalPrice
			for _, p := range prices {
				if (!start.IsZero() && p.Date.Before(start)) || (!end.IsZero() && p.Date.After(end)) {
					continue
				}
				filtered = append(filtered, p)
			}
			if len(filtered) > 0 {
				result[sym] = filtered
			}
		}
	}

	return result, failed
}

func (m *mockFetcher) HistoricalCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fetchHistoricalCalls
}

func (m *mockFetcher) LastFetchRange() (time.Time, time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastStart, m.lastEnd
}

type mockRepo struct {
	mu               sync.RWMutex
	historicalPrices map[string]map[string]decimal.Decimal // symbol -> date string -> price
	upserted         []*market.MarketData
	upsertErr        error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		historicalPrices: make(map[string]map[string]decimal.Decimal),
		upserted:         []*market.MarketData{},
	}
}

func (m *mockRepo) UpsertHistoricalPrices(_ context.Context, symbol string, prices []market.HistoricalPrice, _dataType string) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.historicalPrices[symbol]; !ok {
		m.historicalPrices[symbol] = make(map[string]decimal.Decimal)
	}
	for _, p := range prices {
		m.historicalPrices[symbol][p.Date.Format("2006-01-02")] = p.Close
	}
	return nil
}

func (m *mockRepo) Upsert(_ context.Context, md *market.MarketData) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upserted = append(m.upserted, md)
	return nil
}

func (m *mockRepo) GetLatestPriceDatePerSymbol(_ context.Context, symbols []string) map[string]*time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*time.Time)
	for _, sym := range symbols {
		dates, ok := m.historicalPrices[sym]
		if !ok {
			continue
		}
		var latest time.Time
		for dateStr := range dates {
			t, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue
			}
			if t.After(latest) {
				latest = t
			}
		}
		if !latest.IsZero() {
			result[sym] = &latest
		}
	}
	return result
}

func (m *mockRepo) GetLatestQuotesBatch(_ context.Context, _ []string) map[string]*market.MarketData {
	return nil
}

func (m *mockRepo) HasHistorical(symbol string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.historicalPrices[symbol]
	return ok
}

func (m *mockRepo) UpsertedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.upserted)
}

func (m *mockRepo) UpsertedSymbols() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	symbols := make([]string, 0, len(m.upserted))
	for _, md := range m.upserted {
		symbols = append(symbols, md.Symbol)
	}
	return symbols
}

type mockDiscoverer struct {
	mu            sync.RWMutex
	allSymbols    []string
	activeFxPairs map[string]time.Time
	err           error
}

func (m *mockDiscoverer) SetAllSymbols(s []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allSymbols = s
}

func (m *mockDiscoverer) SetActiveFxPairs(p map[string]time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeFxPairs = p
}

func (m *mockDiscoverer) AllSymbols(_ context.Context) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.allSymbols))
	copy(result, m.allSymbols)
	return result, nil
}

func (m *mockDiscoverer) ActiveFxPairs(_ context.Context) (map[string]time.Time, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]time.Time, len(m.activeFxPairs))
	for k, v := range m.activeFxPairs {
		result[k] = v
	}
	return result, nil
}

// --- Test helpers ---

func waitBackground(t *testing.T, duration time.Duration) {
	t.Helper()
	time.Sleep(duration)
}

func assertErr(msg string) error {
	return fmt.Errorf("%s", msg)
}

// --- Tests ---

func TestScheduleSymbolFetch_TriggersFetch(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now.AddDate(0, 0, -1), Close: decimal.MustNew(17000, 2), Currency: "USD"},
		{Date: now, Close: decimal.MustNew(17500, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"AAPL": prices},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleSymbolFetch("AAPL", now.AddDate(0, 0, -1))
	waitBackground(t, 100*time.Millisecond)

	if !repo.HasHistorical("AAPL") {
		t.Error("expected AAPL prices to be cached")
	}
}

func TestScheduleSymbolFetch_RespectsFromDate(t *testing.T) {
	now := time.Now().UTC()
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	fromDate := nowDate.AddDate(0, 0, -5) // 5 days ago

	prices := []market.HistoricalPrice{
		{Date: nowDate.AddDate(0, 0, -10), Close: decimal.MustNew(16000, 2), Currency: "USD"},
		{Date: fromDate, Close: decimal.MustNew(17000, 2), Currency: "USD"},
		{Date: nowDate, Close: decimal.MustNew(17500, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"AAPL": prices},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleSymbolFetch("AAPL", fromDate)
	waitBackground(t, 100*time.Millisecond)

	// The fetch worker should use the requested fromDate, not historicalFrom (2000-01-01).
	start, end := fetcher.LastFetchRange()
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	wantDay := time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, time.UTC)
	if startDay.Before(wantDay) {
		t.Errorf("expected fetch start >= %s, got %s (processFetch ignores req.fromDate)", wantDay.Format("2006-01-02"), startDay.Format("2006-01-02"))
	}
	if !end.IsZero() && end.Before(nowDate) {
		t.Errorf("expected fetch end >= %s, got %s", nowDate.Format("2006-01-02"), end.Format("2006-01-02"))
	}
}

func TestScheduleSymbolFetch_ConcurrentProtection(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now, Close: decimal.MustNew(17000, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"AAPL": prices},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	// Schedule two fetches for the same symbol rapidly.
	cache.ScheduleSymbolFetch("AAPL", now)
	cache.ScheduleSymbolFetch("AAPL", now)

	waitBackground(t, 100*time.Millisecond)

	// Only one scheduled fetch should have been processed (concurrent protection).
	calls := fetcher.HistoricalCalls()
	if calls != 1 {
		t.Errorf("expected 1 historical fetch call (concurrent protection), got %d", calls)
	}
}

func TestScheduleSymbolFetch_FetchFailure(t *testing.T) {
	now := time.Now().UTC()

	fetcher := &mockFetcher{
		quotes:         map[string]*market.MarketData{},
		historical:     map[string][]market.HistoricalPrice{},
		historicalFail: map[string]bool{"AAPL": true},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleSymbolFetch("AAPL", now)
	waitBackground(t, 100*time.Millisecond)

	status := cache.GetStatus()
	if len(status.FailedSymbols) != 1 {
		t.Errorf("expected 1 failed symbol, got %d: %v", len(status.FailedSymbols), status.FailedSymbols)
	}
	if status.FailedSymbols[0] != "AAPL" {
		t.Errorf("expected failed symbol AAPL, got %s", status.FailedSymbols[0])
	}
}

func TestScheduleSymbolFetch_UpsertError(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now, Close: decimal.MustNew(17000, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"AAPL": prices},
	}
	repo := newMockRepo()
	repo.upsertErr = assertErr("disk full")
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleSymbolFetch("AAPL", now)
	waitBackground(t, 100*time.Millisecond)

	status := cache.GetStatus()
	if len(status.FailedSymbols) != 1 {
		t.Errorf("expected 1 failed symbol after upsert error, got %d", len(status.FailedSymbols))
	}
}

func TestScheduleFxPairFetch(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now.AddDate(0, 0, -1), Close: decimal.MustNew(12734, 4), Currency: "USD"},
		{Date: now, Close: decimal.MustNew(12750, 4), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"GBPUSD=X": prices},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleFxPairFetch("GBP", "USD", now.AddDate(0, 0, -1))
	waitBackground(t, 100*time.Millisecond)

	// Verify FX data was cached with "GBP/USD" symbol.
	if !repo.HasHistorical("GBP/USD") {
		t.Error("expected GBP/USD FX prices to be cached")
	}
}

func TestScheduleFxPairFetch_ConcurrentProtection(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now, Close: decimal.MustNew(12734, 4), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"GBPUSD=X": prices},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	// No benchmark lister → no benchmark fetches.

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleFxPairFetch("GBP", "USD", now)
	cache.ScheduleFxPairFetch("GBP", "USD", now)

	waitBackground(t, 100*time.Millisecond)

	calls := fetcher.HistoricalCalls()
	if calls != 1 {
		t.Errorf("expected 1 historical fetch call (1 FX), got %d", calls)
	}
}

func TestStatus_Tracking(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now, Close: decimal.MustNew(17000, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:         map[string]*market.MarketData{},
		historical:     map[string][]market.HistoricalPrice{"AAPL": prices},
		historicalFail: map[string]bool{"MSFT": true},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	// Initial status.
	status := cache.GetStatus()
	if status.Refreshing {
		t.Error("expected Refreshing=false initially")
	}
	if len(status.FailedSymbols) != 0 {
		t.Errorf("expected no failed symbols initially, got %v", status.FailedSymbols)
	}

	// Schedule one success and one failure.
	cache.ScheduleSymbolFetch("AAPL", now)
	cache.ScheduleSymbolFetch("MSFT", now)
	waitBackground(t, 100*time.Millisecond)

	status = cache.GetStatus()
	if len(status.FailedSymbols) != 1 {
		t.Errorf("expected 1 failed symbol, got %d", len(status.FailedSymbols))
	}
}

func TestRefreshAll(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now.AddDate(0, 0, -1), Close: decimal.MustNew(17000, 2), Currency: "USD"},
		{Date: now, Close: decimal.MustNew(17500, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(17500, 2), Currency: "USD"},
		},
		historical: map[string][]market.HistoricalPrice{"AAPL": prices},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	
	discoverer.SetAllSymbols([]string{"AAPL"})

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.RefreshAll(ctx)
	waitBackground(t, 100*time.Millisecond)

	// Verify historical prices were cached.
	if !repo.HasHistorical("AAPL") {
		t.Error("expected AAPL historical prices to be cached")
	}

	// Verify current quote was cached.
	upserted := repo.UpsertedSymbols()
	foundQuote := false
	for _, sym := range upserted {
		if sym == "AAPL" {
			foundQuote = true
			break
		}
	}
	if !foundQuote {
		t.Error("expected AAPL current quote to be cached")
	}

	// Verify status shows refresh completed.
	status := cache.GetStatus()
	if status.Refreshing {
		t.Error("expected Refreshing=false after RefreshAll completes")
	}
	if status.TotalSymbols != 1 {
		t.Errorf("expected TotalSymbols=1, got %d", status.TotalSymbols)
	}
}

func TestPeriodicTicker_RefreshesQuotes(t *testing.T) {
	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(17500, 2), Currency: "USD"},
		},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	discoverer.SetAllSymbols([]string{"AAPL"})
	

	cache := New(fetcher, repo, discoverer, nil)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Wait for the first periodic tick (immediate first pass + one more tick).
	waitBackground(t, 150*time.Millisecond)

	// Verify quote was cached by the periodic ticker.
	upserted := repo.UpsertedSymbols()
	foundQuote := false
	for _, sym := range upserted {
		if sym == "AAPL" {
			foundQuote = true
			break
		}
	}
	if !foundQuote {
		t.Error("expected AAPL quote to be cached by periodic ticker")
	}

	// Verify status was updated.
	status := cache.GetStatus()
	if status.TotalSymbols != 1 {
		t.Errorf("expected TotalSymbols=1, got %d", status.TotalSymbols)
	}
}

func TestPeriodicTicker_GapFill(t *testing.T) {
	now := time.Now().UTC()
	allPrices := []market.HistoricalPrice{
		{Date: now.AddDate(0, 0, -10), Close: decimal.MustNew(16000, 2), Currency: "USD"},
		{Date: now.AddDate(0, 0, -5), Close: decimal.MustNew(16500, 2), Currency: "USD"},
		{Date: now, Close: decimal.MustNew(17500, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"AAPL": allPrices},
	}
	repo := newMockRepo()

	// Pre-seed repo with old data (only the first price).
	oldPrice := []market.HistoricalPrice{
		{Date: now.AddDate(0, 0, -10), Close: decimal.MustNew(16000, 2), Currency: "USD"},
	}
	repo.UpsertHistoricalPrices(ctx, "AAPL", oldPrice, "stock")

	discoverer := &mockDiscoverer{}
	discoverer.SetAllSymbols([]string{"AAPL"})
	

	cache := New(fetcher, repo, discoverer, nil)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Wait for periodic tick to trigger gap-fill.
	waitBackground(t, 200*time.Millisecond)

	// Verify gap was filled (more prices cached).
	repo.mu.RLock()
	count := len(repo.historicalPrices["AAPL"])
	repo.mu.RUnlock()
	if count <= 1 {
		t.Errorf("expected more than 1 cached price after gap-fill, got %d", count)
	}
}

func TestPeriodicTicker_SkipsGapFillDuringManualRefresh(t *testing.T) {
	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	discoverer.SetAllSymbols([]string{"AAPL"})
	

	cache := New(fetcher, repo, discoverer, nil)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Trigger a manual refresh (sets refreshAllInProgress = true).
	cache.RefreshAll(ctx)

	// Wait briefly — the periodic ticker should fire but skip gap-fill.
	waitBackground(t, 150*time.Millisecond)

	// The manual refresh should have completed.
	status := cache.GetStatus()
	if status.Refreshing {
		t.Error("expected Refreshing=false after RefreshAll completes")
	}
}

func TestPeriodicTicker_EmptyPortfolio(t *testing.T) {
	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	// Empty — no symbols.
	// No benchmark lister → no benchmark fetches.

	cache := New(fetcher, repo, discoverer, nil)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Wait for a periodic tick.
	waitBackground(t, 150*time.Millisecond)

	// No fetches at all (no symbols, no benchmarks).
	calls := fetcher.HistoricalCalls()
	if calls != 0 {
		t.Errorf("expected 0 historical fetch calls, got %d", calls)
	}

	status := cache.GetStatus()
	if status.TotalSymbols != 0 {
		t.Errorf("expected TotalSymbols=0, got %d", status.TotalSymbols)
	}
}

func TestStartStop_Lifecycle(t *testing.T) {
	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)

	// Start should be non-blocking.
	cache.Start(ctx)

	// Stop should wait for goroutines to finish.
	cache.Stop()

	// Multiple stops should not panic.
	cache.Stop()
}

func TestStart_Duplicate(t *testing.T) {
	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)

	cache.Start(ctx)
	cache.Start(ctx) // Should be a no-op.

	cache.Stop()
}

func TestGetStatus_InitialState(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)

	status := cache.GetStatus()
	if status.Refreshing {
		t.Error("expected Refreshing=false in initial state")
	}
	if len(status.FailedSymbols) != 0 {
		t.Errorf("expected no failed symbols, got %v", status.FailedSymbols)
	}
	if status.TotalSymbols != 0 {
		t.Errorf("expected TotalSymbols=0, got %d", status.TotalSymbols)
	}
}

func TestRefreshAll_FxPairs(t *testing.T) {
	now := time.Now().UTC()
	fxPrices := []market.HistoricalPrice{
		{Date: now.AddDate(0, 0, -1), Close: decimal.MustNew(12734, 4), Currency: "USD"},
		{Date: now, Close: decimal.MustNew(12750, 4), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"GBPUSD=X": {Symbol: "GBPUSD=X", Price: decimal.MustNew(12750, 4), Currency: "USD"},
		},
		historical: map[string][]market.HistoricalPrice{"GBPUSD=X": fxPrices},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	discoverer.SetActiveFxPairs(map[string]time.Time{"GBP/USD": now.AddDate(0, 0, -10)})

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.RefreshAll(ctx)
	waitBackground(t, 100*time.Millisecond)

	// Verify FX data was upserted with "GBP/USD" symbol.
	upserted := repo.UpsertedSymbols()
	foundFx := false
	for _, sym := range upserted {
		if sym == "GBP/USD" {
			foundFx = true
			break
		}
	}
	if !foundFx {
		t.Errorf("expected GBP/USD in upserted symbols, got %v", upserted)
	}
}

func TestPartialFailure_SomeSucceedSomeFail(t *testing.T) {
	now := time.Now().UTC()
	prices := []market.HistoricalPrice{
		{Date: now, Close: decimal.MustNew(17000, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:         map[string]*market.MarketData{},
		historical:     map[string][]market.HistoricalPrice{"AAPL": prices},
		historicalFail: map[string]bool{"MSFT": true},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleSymbolFetch("AAPL", now)
	cache.ScheduleSymbolFetch("MSFT", now)
	waitBackground(t, 200*time.Millisecond)

	// AAPL should be cached.
	if !repo.HasHistorical("AAPL") {
		t.Error("expected AAPL to be cached")
	}

	// MSFT should be in failed symbols.
	status := cache.GetStatus()
	if len(status.FailedSymbols) != 1 {
		t.Errorf("expected 1 failed symbol, got %d", len(status.FailedSymbols))
	}
	if status.FailedSymbols[0] != "MSFT" {
		t.Errorf("expected failed symbol MSFT, got %s", status.FailedSymbols[0])
	}
}

func TestFullFailure_AllSymbolsFail(t *testing.T) {
	now := time.Now().UTC()

	fetcher := &mockFetcher{
		quotes:         map[string]*market.MarketData{},
		historical:     map[string][]market.HistoricalPrice{},
		historicalFail: map[string]bool{"AAPL": true, "MSFT": true},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	cache.Start(ctx)
	defer cache.Stop()

	cache.ScheduleSymbolFetch("AAPL", now)
	cache.ScheduleSymbolFetch("MSFT", now)
	waitBackground(t, 200*time.Millisecond)

	status := cache.GetStatus()
	if len(status.FailedSymbols) != 2 {
		t.Errorf("expected 2 failed symbols, got %d", len(status.FailedSymbols))
	}

	// Existing cache should be preserved (nothing was added, so nothing to preserve).
	// This is verified by the fact that no data was upserted.
	if repo.UpsertedCount() != 0 {
		t.Errorf("expected 0 upserted entries, got %d", repo.UpsertedCount())
	}
}

func TestGapFill_NoCache_FetchesFromEarliest(t *testing.T) {
	now := time.Now().UTC()
	earliest := now.AddDate(0, 0, -30)
	allPrices := []market.HistoricalPrice{
		{Date: earliest, Close: decimal.MustNew(15000, 2), Currency: "USD"},
		{Date: now, Close: decimal.MustNew(17500, 2), Currency: "USD"},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{"AAPL": allPrices},
	}
	repo := newMockRepo()
	// No pre-seeded data for AAPL.

	discoverer := &mockDiscoverer{}
	discoverer.SetAllSymbols([]string{"AAPL"})

	cache := New(fetcher, repo, discoverer, nil)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Wait for periodic tick to trigger gap-fill.
	waitBackground(t, 200*time.Millisecond)

	// Should have fetched from earliest date (30 days ago).
	if !repo.HasHistorical("AAPL") {
		t.Error("expected AAPL to be cached after gap-fill")
	}
}

func TestGapFill_FullCache_SkipsFetch(t *testing.T) {
	now := time.Now().UTC()

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()

	// Pre-seed with today's price (fully covered).
	repo.UpsertHistoricalPrices(ctx, "AAPL", []market.HistoricalPrice{
		{Date: now, Close: decimal.MustNew(17000, 2), Currency: "USD"},
	}, "stock")

	discoverer := &mockDiscoverer{}
	discoverer.SetAllSymbols([]string{"AAPL"})

	// No benchmark lister → no benchmark fetches.
	cache := New(fetcher, repo, discoverer, nil)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Wait for periodic tick.
	waitBackground(t, 150*time.Millisecond)

	// No fetches at all (cache current, no benchmarks).
	calls := fetcher.HistoricalCalls()
	if calls != 0 {
		t.Errorf("expected 0 historical fetch calls, got %d", calls)
	}
}

func TestParseFxPair(t *testing.T) {
	tests := []struct {
		input    string
		wantBase string
		wantQuote string
	}{
		{"GBP/USD", "GBP", "USD"},
		{"EUR/GBP", "EUR", "GBP"},
		{"USD/JPY", "USD", "JPY"},
		{"NONSLASH", "NONSLASH", ""},
	}

	for _, tt := range tests {
		base, quote := parseFxPair(tt.input)
		if base != tt.wantBase {
			t.Errorf("parseFxPair(%q): base = %q, want %q", tt.input, base, tt.wantBase)
		}
		if quote != tt.wantQuote {
			t.Errorf("parseFxPair(%q): quote = %q, want %q", tt.input, quote, tt.wantQuote)
		}
	}
}

func TestPeriodicTicker_FxQuotes(t *testing.T) {
	now := time.Now().UTC()

	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"GBPUSD=X": {Symbol: "GBPUSD=X", Price: decimal.MustNew(12750, 4), Currency: "USD"},
		},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	discoverer.SetActiveFxPairs(map[string]time.Time{"GBP/USD": now.AddDate(0, 0, -10)})

	cache := New(fetcher, repo, discoverer, nil)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Wait for periodic tick.
	waitBackground(t, 150*time.Millisecond)

	// Verify FX quote was cached with "GBP/USD" symbol.
	upserted := repo.UpsertedSymbols()
	foundFx := false
	for _, sym := range upserted {
		if sym == "GBP/USD" {
			foundFx = true
			break
		}
	}
	if !foundFx {
		t.Errorf("expected GBP/USD in upserted symbols from periodic ticker, got %v", upserted)
	}
}

type mockSymbolDetailsRefresh struct {
	mu            sync.RWMutex
	staleSymbols  []symbol.StaleSymbol
	refreshErrors map[string]bool // keyed by internalSymbol
	authError     error           // if set, first RefreshSymbol returns this
	refreshCalls  int
}

func (m *mockSymbolDetailsRefresh) SetStaleSymbols(symbols []symbol.StaleSymbol) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.staleSymbols = symbols
}

func (m *mockSymbolDetailsRefresh) SetRefreshError(internalSymbol string, fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.refreshErrors == nil {
		m.refreshErrors = make(map[string]bool)
	}
	m.refreshErrors[internalSymbol] = fail
}

func (m *mockSymbolDetailsRefresh) SetAuthError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authError = err
}

func (m *mockSymbolDetailsRefresh) GetStaleSymbols(_ context.Context) ([]symbol.StaleSymbol, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]symbol.StaleSymbol, len(m.staleSymbols))
	copy(result, m.staleSymbols)
	return result, nil
}

func (m *mockSymbolDetailsRefresh) RefreshSymbol(_ context.Context, internalSymbol, _ string) error {
	m.mu.Lock()
	m.refreshCalls++
	fail := m.refreshErrors[internalSymbol]
	authErr := m.authError
	m.mu.Unlock()
	if authErr != nil {
		return authErr
	}
	if fail {
		return fmt.Errorf("fetch failed for %s", internalSymbol)
	}
	return nil
}

func (m *mockSymbolDetailsRefresh) RefreshCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.refreshCalls
}

func TestRefreshStaleSymbolDetails_NoSource_Skips(t *testing.T) {
	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}

	cache := New(fetcher, repo, discoverer, nil)
	// No symbol details refresh source set.
	cache.Start(ctx)
	defer cache.Stop()

	// Should not panic — nil source is a no-op.
	cache.refreshStaleSymbolDetails(ctx)
}

func TestRefreshStaleSymbolDetails_NoStaleSymbols_Skips(t *testing.T) {
	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(nil)

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)
	cache.Start(ctx)
	defer cache.Stop()

	cache.refreshStaleSymbolDetails(ctx)

	calls := source.RefreshCalls()
	if calls != 0 {
		t.Errorf("expected 0 refresh calls, got %d", calls)
	}
}

func TestRefreshStaleSymbolDetails_RefreshesStale(t *testing.T) {
	now := time.Now().UTC()
	staleSymbols := []symbol.StaleSymbol{
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: now.AddDate(0, 0, -10)},
		{InternalSymbol: "MSFT", MarketDataSymbol: "MSFT", FetchedAt: now.AddDate(0, 0, -8)},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(staleSymbols)

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)
	// Don't Start() — call refreshStaleSymbolDetails directly to avoid
	// the periodic ticker's immediate first pass interfering with the count.
	cache.refreshStaleSymbolDetails(ctx)

	calls := source.RefreshCalls()
	if calls != 2 {
		t.Errorf("expected 2 refresh calls, got %d", calls)
	}
}

func TestRefreshStaleSymbolDetails_PartialFailure(t *testing.T) {
	now := time.Now().UTC()
	staleSymbols := []symbol.StaleSymbol{
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: now.AddDate(0, 0, -10)},
		{InternalSymbol: "MSFT", MarketDataSymbol: "MSFT", FetchedAt: now.AddDate(0, 0, -8)},
		{InternalSymbol: "GOOG", MarketDataSymbol: "GOOG", FetchedAt: now.AddDate(0, 0, -14)},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(staleSymbols)
	source.SetRefreshError("MSFT", true) // MSFT fails

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)
	// Don't Start() — call directly to isolate this test.
	cache.refreshStaleSymbolDetails(ctx)

	// All 3 should be attempted (failure doesn't stop the batch).
	calls := source.RefreshCalls()
	if calls != 3 {
		t.Errorf("expected 3 refresh calls (all attempted despite partial failure), got %d", calls)
	}
}

func TestRefreshStaleSymbolDetails_RateLimiting(t *testing.T) {
	now := time.Now().UTC()
	staleSymbols := []symbol.StaleSymbol{
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: now.AddDate(0, 0, -10)},
		{InternalSymbol: "MSFT", MarketDataSymbol: "MSFT", FetchedAt: now.AddDate(0, 0, -8)},
		{InternalSymbol: "GOOG", MarketDataSymbol: "GOOG", FetchedAt: now.AddDate(0, 0, -14)},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(staleSymbols)

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)

	// With 3 symbols and 500ms delay between them, should take ~1s total.
	start := time.Now()
	cache.refreshStaleSymbolDetails(ctx)
	duration := time.Since(start)

	// Should be at least 1s (2 delays of 500ms) and less than 2s.
	if duration < 900*time.Millisecond {
		t.Errorf("expected rate limiting (>= 1s), got %v", duration)
	}
	if duration > 2*time.Second {
		t.Errorf("refresh took too long (%v), expected ~1s", duration)
	}
}

func TestRefreshStaleSymbolDetails_PeriodicTickerIntegration(t *testing.T) {
	now := time.Now().UTC()
	staleSymbols := []symbol.StaleSymbol{
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: now.AddDate(0, 0, -10)},
	}

	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(17500, 2), Currency: "USD"},
		},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	discoverer.SetAllSymbols([]string{"AAPL"})
	
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(staleSymbols)

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)
	cache.tickerInterval = 50 * time.Millisecond
	cache.Start(ctx)
	defer cache.Stop()

	// Wait for the periodic ticker's immediate first pass + 500ms rate limit delay.
	waitBackground(t, 1200*time.Millisecond)

	// Symbol details refresh should have been called at least once by the periodic ticker.
	calls := source.RefreshCalls()
	if calls < 1 {
		t.Errorf("expected at least 1 refresh call from periodic ticker, got %d", calls)
	}

	// Clear stale symbols so subsequent ticks don't refresh again.
	source.SetStaleSymbols(nil)

	// Wait for another tick cycle.
	waitBackground(t, 200*time.Millisecond)

	// No additional calls after clearing stale symbols.
	callsAfterClear := source.RefreshCalls()
	if callsAfterClear != calls {
		t.Errorf("expected no additional refresh calls after clearing stale, got %d (was %d, now %d)", callsAfterClear, calls, callsAfterClear)
	}
}

func TestRefreshStaleSymbolDetails_EveryCycle(t *testing.T) {
	staleSymbols := []symbol.StaleSymbol{
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: time.Now().AddDate(0, 0, -10)},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(staleSymbols)

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)

	// Symbol details are refreshed on every cycle (including the first).
	cache.doRefresh(ctx)
	calls := source.RefreshCalls()
	if calls != 1 {
		t.Errorf("expected 1 refresh call, got %d", calls)
	}

	cache.doRefresh(ctx)
	calls = source.RefreshCalls()
	if calls != 2 {
		t.Errorf("expected 2 refresh calls, got %d", calls)
	}
}

func TestRefreshStaleSymbolDetails_AbortOnAuthError(t *testing.T) {
	staleSymbols := []symbol.StaleSymbol{
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: time.Now().AddDate(0, 0, -10)},
		{InternalSymbol: "MSFT", MarketDataSymbol: "MSFT", FetchedAt: time.Now().AddDate(0, 0, -10)},
		{InternalSymbol: "GOOGL", MarketDataSymbol: "GOOGL", FetchedAt: time.Now().AddDate(0, 0, -10)},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(staleSymbols)
	// Simulate Yahoo 429 rate limit
	source.SetAuthError(fmt.Errorf("failed to get Yahoo crumb: get crumb: status 429"))

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)

	cache.doRefresh(ctx)

	// Only 1 refresh call (first symbol), batch aborted on auth error.
	calls := source.RefreshCalls()
	if calls != 1 {
		t.Errorf("expected 1 refresh call (batch aborted on auth error), got %d", calls)
	}
}

func TestRefreshStaleSymbolDetails_NonAuthErrorContinues(t *testing.T) {
	staleSymbols := []symbol.StaleSymbol{
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: time.Now().AddDate(0, 0, -10)},
		{InternalSymbol: "MSFT", MarketDataSymbol: "MSFT", FetchedAt: time.Now().AddDate(0, 0, -10)},
	}

	fetcher := &mockFetcher{
		quotes:     map[string]*market.MarketData{},
		historical: map[string][]market.HistoricalPrice{},
	}
	repo := newMockRepo()
	discoverer := &mockDiscoverer{}
	source := &mockSymbolDetailsRefresh{}
	source.SetStaleSymbols(staleSymbols)
	// First symbol fails with a non-auth error (e.g., symbol not found)
	source.SetRefreshError("AAPL", true)

	cache := New(fetcher, repo, discoverer, nil)
	cache.WithSymbolDetailsRefresh(source)

	cache.doRefresh(ctx)

	// Both symbols should be attempted (non-auth error doesn't abort batch).
	calls := source.RefreshCalls()
	if calls != 2 {
		t.Errorf("expected 2 refresh calls (non-auth error continues), got %d", calls)
	}
}

