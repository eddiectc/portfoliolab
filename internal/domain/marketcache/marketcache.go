package marketcache

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/comparison"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// MarketCacheScheduler defines the scheduling interface for background
// market data fetches. Used by consumer services (position, transaction) to
// trigger cache updates without importing the concrete MarketCache type.
type MarketCacheScheduler interface {
	ScheduleSymbolFetch(symbol string, fromDate time.Time)
	ScheduleFxPairFetch(baseCurrency, quoteCurrency string, fromDate time.Time)
	RefreshAll(ctx context.Context)
}

// SymbolDiscoverer finds symbols and FX pairs that need market data.
type SymbolDiscoverer interface {
	// ActiveSymbols returns symbols with open positions.
	// Key: symbol, Value: earliest transaction date for that symbol.
	ActiveSymbols(ctx context.Context) (map[string]time.Time, error)
	// AllSymbols returns all symbols with any transactions (open + closed).
	AllSymbols(ctx context.Context) (map[string]time.Time, error)
	// ActiveFxPairs returns FX pairs needed for open positions.
	// Key: "BASE/QUOTE", Value: earliest transaction date.
	ActiveFxPairs(ctx context.Context) (map[string]time.Time, error)
}

// MarketDataRepository defines the subset of market data operations
// needed by the cache.
type MarketDataRepository interface {
	UpsertHistoricalPrices(ctx context.Context, symbol string, prices []market.HistoricalPrice, dataType string) error
	Upsert(ctx context.Context, m *market.MarketData) error
	GetLatestPriceDatePerSymbol(ctx context.Context, symbols []string) map[string]*time.Time
	GetLatestQuotesBatch(ctx context.Context, symbols []string) map[string]*market.MarketData
}

// MarketDataFetcher fetches market data from external providers.
type MarketDataFetcher interface {
	FetchQuotesBatch(ctx context.Context, symbols []string) map[string]*market.MarketData
	FetchHistoricalPricesBatch(ctx context.Context, symbols []string, start, end time.Time) (map[string][]market.HistoricalPrice, []string)
}

// CacheStatus holds the current state of the market data cache.
type CacheStatus struct {
	LastRefresh   time.Time `json:"last_refresh"`
	Refreshing    bool      `json:"refreshing"`
	FailedSymbols []string  `json:"failed_symbols"`
	TotalSymbols  int       `json:"total_symbols"`
}

// fetchRequest is a single fetch job sent to the background worker.
type fetchRequest struct {
	symbol   string
	fromDate time.Time
	isFx     bool
}

// MarketCache orchestrates background fetching of historical prices and FX
// rates. It runs a channel-based worker for on-demand fetches and a periodic
// ticker for current quotes and historical gap-fill.
type MarketCache struct {
	fetcher      MarketDataFetcher
	repo         MarketDataRepository
	discoverer   SymbolDiscoverer
	logger       *slog.Logger
	tickerInterval time.Duration

	mu                   sync.RWMutex
	inProgress           map[string]bool
	queued               map[string]bool
	lastRefresh          time.Time
	failedSymbols        map[string]string
	refreshAllInProgress bool
	totalSymbols         int

	fetchCh chan fetchRequest
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New creates a new MarketCache.
func New(fetcher MarketDataFetcher, repo MarketDataRepository, discoverer SymbolDiscoverer, logger *slog.Logger) *MarketCache {
	return &MarketCache{
		fetcher:        fetcher,
		repo:           repo,
		discoverer:     discoverer,
		logger:         logger,
		tickerInterval: 2 * time.Minute,
		inProgress:     make(map[string]bool),
		queued:         make(map[string]bool),
		failedSymbols:  make(map[string]string),
		fetchCh:        make(chan fetchRequest, 100),
	}
}

// Start launches the background worker and periodic ticker. It is non-blocking:
// goroutines are launched and return immediately. The periodic ticker does an
// immediate first pass on launch.
func (m *MarketCache) Start(ctx context.Context) {
	if m.ctx != nil {
		return // already started
	}
	m.ctx, m.cancel = context.WithCancel(ctx)

	m.wg.Add(2)
	go m.backgroundWorker()
	go m.periodicTicker()

	// Fetch benchmark data on startup (gap-fill only) so it's available
	// without requiring a manual "Refresh All" click.
	go func() {
		m.gapFillBenchmarks(ctx)
	}()
}

// Stop gracefully shuts down background goroutines.
func (m *MarketCache) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

// ScheduleSymbolFetch queues a historical price fetch for a stock symbol.
// If a fetch is already in progress or queued for this symbol, the request
// is silently ignored.
func (m *MarketCache) ScheduleSymbolFetch(symbol string, fromDate time.Time) {
	m.mu.Lock()
	if m.inProgress[symbol] || m.queued[symbol] {
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Debug("symbol fetch skipped (already queued/in-progress)", "symbol", symbol)
		}
		return
	}
	m.queued[symbol] = true
	m.mu.Unlock()

	select {
	case m.fetchCh <- fetchRequest{symbol: symbol, fromDate: fromDate}:
		if m.logger != nil {
			m.logger.Debug("symbol fetch queued", "symbol", symbol, "fromDate", fromDate.Format("2006-01-02"))
		}
	default:
		// Channel full — drop the request.
		m.mu.Lock()
		delete(m.queued, symbol)
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Warn("fetch channel full, dropping request", "symbol", symbol)
		}
	}
}

// ScheduleFxPairFetch queues a historical FX rate fetch for a currency pair.
// If a fetch is already in progress or queued for this pair, the request
// is silently ignored.
func (m *MarketCache) ScheduleFxPairFetch(baseCurrency, quoteCurrency string, fromDate time.Time) {
	pair := market.FormatFxPair(baseCurrency, quoteCurrency)

	m.mu.Lock()
	if m.inProgress[pair] || m.queued[pair] {
		m.mu.Unlock()
		return
	}
	m.queued[pair] = true
	m.mu.Unlock()

	select {
	case m.fetchCh <- fetchRequest{symbol: pair, fromDate: fromDate, isFx: true}:
	default:
		// Channel full — drop the request.
		m.mu.Lock()
		delete(m.queued, pair)
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Warn("fetch channel full, dropping FX request", "pair", pair)
		}
	}
}

// RefreshAll triggers a full refresh of all symbols and FX pairs. It runs in
// the background and returns immediately. Uses the cache's internal context
// (not the caller's) so it survives after the HTTP request ends.
func (m *MarketCache) RefreshAll(_ context.Context) {
	if m.logger != nil {
		m.logger.Info("refresh-all: triggered")
	}
	m.mu.Lock()
	m.refreshAllInProgress = true
	m.mu.Unlock()

	go func() {
		defer func() {
			m.mu.Lock()
			m.refreshAllInProgress = false
			m.lastRefresh = time.Now()
			m.mu.Unlock()
		}()

		m.doRefreshAll(m.ctx)
	}()
}

// GetStatus returns the current cache status.
func (m *MarketCache) GetStatus() CacheStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	failed := make([]string, 0, len(m.failedSymbols))
	for sym := range m.failedSymbols {
		failed = append(failed, sym)
	}

	return CacheStatus{
		LastRefresh:   m.lastRefresh,
		Refreshing:    m.refreshAllInProgress,
		FailedSymbols: failed,
		TotalSymbols:  m.totalSymbols,
	}
}

// --- Background worker ---

// backgroundWorker processes fetch requests from the channel.
func (m *MarketCache) backgroundWorker() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case req, ok := <-m.fetchCh:
			if !ok {
				return
			}
			m.processFetch(req)
		}
	}
}

// processFetch handles a single fetch request, managing in-progress state.
func (m *MarketCache) processFetch(req fetchRequest) {
	m.mu.Lock()
	m.inProgress[req.symbol] = true
	delete(m.queued, req.symbol)
	m.mu.Unlock()

	if m.logger != nil {
		kind := "symbol"
		if req.isFx {
			kind = "fx"
		}
		m.logger.Debug("starting background fetch", "symbol", req.symbol, "kind", kind, "fromDate", req.fromDate.Format("2006-01-02"))
	}

	defer func() {
		m.mu.Lock()
		delete(m.inProgress, req.symbol)
		m.mu.Unlock()
	}()

	if req.isFx {
		m.fetchFxPair(req.symbol, req.fromDate)
	} else {
		m.fetchHistorical(req.symbol, req.fromDate)
	}
}

// fetchHistorical fetches historical prices for a stock symbol via the
// background worker (uses internal context with timeout).
func (m *MarketCache) fetchHistorical(symbol string, fromDate time.Time) {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()
	m.doFetchHistorical(ctx, symbol, fromDate)
}

// fetchHistoricalDirect fetches historical prices for a stock symbol using
// the provided context (called from RefreshAll).
func (m *MarketCache) fetchHistoricalDirect(ctx context.Context, symbol string, fromDate time.Time) {
	m.doFetchHistorical(ctx, symbol, fromDate)
}

// doFetchHistorical does the actual fetch and upsert for a stock symbol.
func (m *MarketCache) doFetchHistorical(ctx context.Context, symbol string, fromDate time.Time) {
	now := time.Now().UTC()
	if m.logger != nil {
		m.logger.Debug("fetching historical prices", "symbol", symbol, "fromDate", fromDate.Format("2006-01-02"), "toDate", now.Format("2006-01-02"))
	}
	prices, failed := m.fetcher.FetchHistoricalPricesBatch(ctx, []string{symbol}, fromDate, now)

	if len(failed) > 0 {
		m.mu.Lock()
		m.failedSymbols[symbol] = "fetch failed"
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Warn("failed to fetch historical prices", "symbol", symbol)
		}
		return
	}

	if p, ok := prices[symbol]; ok && len(p) > 0 {
		if err := m.repo.UpsertHistoricalPrices(ctx, symbol, p, "stock"); err != nil {
			m.mu.Lock()
			m.failedSymbols[symbol] = err.Error()
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Warn("failed to upsert historical prices", "symbol", symbol, "error", err)
			}
		} else {
			m.mu.Lock()
			delete(m.failedSymbols, symbol)
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Info("cached historical prices", "symbol", symbol, "count", len(p), "dateRange", fmt.Sprintf("%s to %s", p[0].Date.Format("2006-01-02"), p[len(p)-1].Date.Format("2006-01-02")))
			}
		}
	} else if m.logger != nil {
		m.logger.Debug("fetch returned no prices", "symbol", symbol)
	}
}

// fetchFxPair fetches historical FX rates for a currency pair via the
// background worker (uses internal context with timeout).
func (m *MarketCache) fetchFxPair(pair string, fromDate time.Time) {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()
	m.doFetchFxPair(ctx, pair, fromDate)
}

// fetchFxPairDirect fetches historical FX rates using the provided context
// (called from RefreshAll).
func (m *MarketCache) fetchFxPairDirect(ctx context.Context, pair string, fromDate time.Time) {
	m.doFetchFxPair(ctx, pair, fromDate)
}

// doFetchFxPair does the actual fetch and upsert for an FX pair.
func (m *MarketCache) doFetchFxPair(ctx context.Context, pair string, fromDate time.Time) {
	base, quote := parseFxPair(pair)
	yahooSymbol := market.FxPairToYahooSymbol(base, quote)

	now := time.Now().UTC()
	if m.logger != nil {
		m.logger.Debug("fetching FX historical prices", "pair", pair, "yahooSymbol", yahooSymbol, "fromDate", fromDate.Format("2006-01-02"), "toDate", now.Format("2006-01-02"))
	}
	prices, failed := m.fetcher.FetchHistoricalPricesBatch(ctx, []string{yahooSymbol}, fromDate, now)

	if len(failed) > 0 {
		m.mu.Lock()
		m.failedSymbols[pair] = "fetch failed"
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Warn("failed to fetch FX historical prices", "pair", pair)
		}
		return
	}

	if p, ok := prices[yahooSymbol]; ok && len(p) > 0 {
		if err := m.repo.UpsertHistoricalPrices(ctx, pair, p, "fx"); err != nil {
			m.mu.Lock()
			m.failedSymbols[pair] = err.Error()
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Warn("failed to upsert FX prices", "pair", pair, "error", err)
			}
		} else {
			m.mu.Lock()
			delete(m.failedSymbols, pair)
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Info("cached FX historical prices", "pair", pair, "count", len(p), "dateRange", fmt.Sprintf("%s to %s", p[0].Date.Format("2006-01-02"), p[len(p)-1].Date.Format("2006-01-02")))
			}
		}
	} else if m.logger != nil {
		m.logger.Debug("FX fetch returned no prices", "pair", pair, "yahooSymbol", yahooSymbol)
	}
}

// --- Periodic ticker ---

// periodicTicker runs every tickerInterval to refresh current quotes and fill
// historical gaps. It does an immediate first pass on launch.
func (m *MarketCache) periodicTicker() {
	defer m.wg.Done()

	// Immediate first pass.
	m.doRefresh(m.ctx)

	ticker := time.NewTicker(m.tickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.doRefresh(m.ctx)
		}
	}
}

// doRefresh performs one refresh cycle: current quotes for active symbols and
// FX pairs, plus historical gap-fill (skipped if a manual refresh is in progress).
func (m *MarketCache) doRefresh(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	// Discover symbols and FX pairs.
	activeSymbols, _ := m.discoverer.ActiveSymbols(ctx)
	allSymbols, _ := m.discoverer.AllSymbols(ctx)
	activeFxPairs, _ := m.discoverer.ActiveFxPairs(ctx)

	if m.logger != nil {
		m.logger.Debug("refresh cycle", "activeSymbols", len(activeSymbols), "allSymbols", len(allSymbols), "activeFxPairs", len(activeFxPairs))
	}

	// Refresh current quotes for active symbols.
	if len(activeSymbols) > 0 {
		symbols := make([]string, 0, len(activeSymbols))
		for sym := range activeSymbols {
			symbols = append(symbols, sym)
		}
		quotes := m.fetcher.FetchQuotesBatch(ctx, symbols)
		if m.logger != nil {
			m.logger.Debug("current quotes fetched", "requested", len(symbols), "received", len(quotes))
		}
		for sym, quote := range quotes {
			if err := m.repo.Upsert(ctx, quote); err != nil {
				m.logWarn("failed to cache quote", "symbol", sym, "error", err)
			}
		}
	}

	// Refresh current quotes for active FX pairs.
	for pair := range activeFxPairs {
		m.refreshFxQuote(ctx, pair)
	}

	// Historical gap-fill (skip if manual refresh is in progress).
	m.mu.RLock()
	refreshAllInProgress := m.refreshAllInProgress
	m.mu.RUnlock()

	if !refreshAllInProgress {
		m.gapFillHistorical(ctx, allSymbols)
	} else {
		if m.logger != nil {
			m.logger.Debug("skipping historical gap-fill (manual refresh in progress)")
		}
	}

	// Update status.
	m.mu.Lock()
	m.lastRefresh = time.Now()
	m.totalSymbols = len(allSymbols) + len(activeFxPairs)
	m.mu.Unlock()
}

// refreshFxQuote fetches and caches the current quote for an FX pair.
func (m *MarketCache) refreshFxQuote(ctx context.Context, pair string) {
	base, quote := parseFxPair(pair)
	yahooSymbol := market.FxPairToYahooSymbol(base, quote)

	quotes := m.fetcher.FetchQuotesBatch(ctx, []string{yahooSymbol})
	if fxQuote, ok := quotes[yahooSymbol]; ok {
		fxQuote.Symbol = pair
		fxQuote.DataType = "fx"
		fxQuote.Currency = quote
		if err := m.repo.Upsert(ctx, fxQuote); err != nil {
			m.logWarn("failed to cache FX quote", "pair", pair, "error", err)
		}
	}
}

// gapFillHistorical checks each symbol's cached date range and schedules
// fetches for any gaps between the earliest transaction date and now.
func (m *MarketCache) gapFillHistorical(ctx context.Context, allSymbols map[string]time.Time) {
	if len(allSymbols) == 0 {
		if m.logger != nil {
			m.logger.Debug("gap-fill: no symbols to check")
		}
		return
	}

	symbols := make([]string, 0, len(allSymbols))
	for sym := range allSymbols {
		symbols = append(symbols, sym)
	}
	latestDates := m.repo.GetLatestPriceDatePerSymbol(ctx, symbols)

	if m.logger != nil {
		m.logger.Debug("gap-fill: latest cached dates", "symbolsChecked", len(symbols), "symbolsCached", len(latestDates))
	}

	now := time.Now().UTC()
	// Truncate to date-only for fair comparison with DB dates (YYYY-MM-DD midnight).
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	for sym, fromDate := range allSymbols {
		latestDate, hasCache := latestDates[sym]

		var fetchStart time.Time
		if !hasCache {
			// No cache at all — fetch from earliest transaction date.
			fetchStart = fromDate
			if m.logger != nil {
				m.logger.Debug("gap-fill: no cache, scheduling full fetch", "symbol", sym, "fromDate", fromDate.Format("2006-01-02"))
			}
		} else if latestDate.Before(tradingDayBeforeOrOn(nowDate)) {
			// Cache exists but not current — fetch from next trading day after
			// the latest cached date to now. Skips weekends so Yahoo actually
			// has data for the requested range.
			fetchStart = nextTradingDay(*latestDate)
			if m.logger != nil {
				m.logger.Debug("gap-fill: cache stale, scheduling gap fetch", "symbol", sym, "latestCached", latestDate.Format("2006-01-02"), "fetchStart", fetchStart.Format("2006-01-02"))
			}
		} else {
			// Fully covered — skip.
			if m.logger != nil {
				m.logger.Debug("gap-fill: cache current, skipping", "symbol", sym, "latestCached", latestDate.Format("2006-01-02"))
			}
			continue
		}

		// Skip if fetchStart is in the future (e.g. latest cached date was
		// Friday, next trading day is Monday, but today is Saturday).
		if fetchStart.After(nowDate) {
			if m.logger != nil {
				m.logger.Debug("gap-fill: fetchStart in future, skipping", "symbol", sym, "fetchStart", fetchStart.Format("2006-01-02"), "nowDate", nowDate.Format("2006-01-02"))
			}
			continue
		}

		m.ScheduleSymbolFetch(sym, fetchStart)
	}
}

// RefreshPredefinedBenchmarks fetches historical prices for all predefined
// benchmark tickers from 2000 to now. Used by RefreshAll (manual full refresh).
func (m *MarketCache) RefreshPredefinedBenchmarks(ctx context.Context) {
	predefined := comparison.GetPredefined()
	if m.logger != nil {
		m.logger.Info("refreshing predefined benchmarks (full)", "count", len(predefined))
	}

	fromDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()

	for ticker := range predefined {
		m.mu.Lock()
		m.inProgress[ticker] = true
		m.mu.Unlock()

		m.fetchBenchmarkDirect(ctx, ticker, fromDate, now)

		m.mu.Lock()
		delete(m.inProgress, ticker)
		m.mu.Unlock()
	}

	if m.logger != nil {
		m.logger.Info("benchmark refresh completed", "count", len(predefined))
	}
}

// gapFillBenchmarks fetches missing or stale benchmark data only.
// Used on startup to avoid fetching everything when cache is current.
func (m *MarketCache) gapFillBenchmarks(ctx context.Context) {
	predefined := comparison.GetPredefined()
	if m.logger != nil {
		m.logger.Info("gap-fill benchmarks", "count", len(predefined))
	}

	now := time.Now().UTC()
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	fromDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	// Check latest cached dates for all benchmarks.
	tickers := make([]string, 0, len(predefined))
	for ticker := range predefined {
		tickers = append(tickers, ticker)
	}
	latestDates := m.repo.GetLatestPriceDatePerSymbol(ctx, tickers)

	for ticker := range predefined {
		latestDate, hasCache := latestDates[ticker]

		var fetchStart time.Time
		if !hasCache {
			// No cache at all — fetch from 2000.
			fetchStart = fromDate
		} else if latestDate.Before(tradingDayBeforeOrOn(nowDate)) {
			// Cache exists but not current — fetch gap.
			fetchStart = nextTradingDay(*latestDate)
		} else {
			// Fully covered — skip.
			if m.logger != nil {
				m.logger.Debug("benchmark cache current, skipping", "ticker", ticker, "latestCached", latestDate.Format("2006-01-02"))
			}
			continue
		}

		// Skip if fetchStart is in the future.
		if fetchStart.After(nowDate) {
			continue
		}

		m.mu.Lock()
		m.inProgress[ticker] = true
		m.mu.Unlock()

		m.fetchBenchmarkDirect(ctx, ticker, fetchStart, now)

		m.mu.Lock()
		delete(m.inProgress, ticker)
		m.mu.Unlock()
	}

	if m.logger != nil {
		m.logger.Info("benchmark gap-fill completed", "count", len(predefined))
	}
}

// fetchBenchmarkDirect fetches historical prices for a benchmark ticker and
// upserts them. Uses the provided context (called from RefreshAll).
func (m *MarketCache) fetchBenchmarkDirect(ctx context.Context, ticker string, fromDate, toDate time.Time) {
	if m.logger != nil {
		m.logger.Debug("fetching benchmark prices", "ticker", ticker, "fromDate", fromDate.Format("2006-01-02"), "toDate", toDate.Format("2006-01-02"))
	}
	prices, failed := m.fetcher.FetchHistoricalPricesBatch(ctx, []string{ticker}, fromDate, toDate)

	if len(failed) > 0 {
		m.mu.Lock()
		m.failedSymbols[ticker] = "fetch failed"
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Warn("failed to fetch benchmark prices", "ticker", ticker)
		}
		return
	}

	if p, ok := prices[ticker]; ok && len(p) > 0 {
		if err := m.repo.UpsertHistoricalPrices(ctx, ticker, p, "stock"); err != nil {
			m.mu.Lock()
			m.failedSymbols[ticker] = err.Error()
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Warn("failed to upsert benchmark prices", "ticker", ticker, "error", err)
			}
		} else {
			m.mu.Lock()
			delete(m.failedSymbols, ticker)
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Info("cached benchmark prices", "ticker", ticker, "count", len(p), "dateRange", fmt.Sprintf("%s to %s", p[0].Date.Format("2006-01-02"), p[len(p)-1].Date.Format("2006-01-02")))
			}
		}
	} else if m.logger != nil {
		m.logger.Debug("benchmark fetch returned no prices", "ticker", ticker)
	}
}

// --- RefreshAll ---

// doRefreshAll performs a full refresh of all symbols, FX pairs, and benchmarks:
// current quotes plus historical from the earliest transaction date.
func (m *MarketCache) doRefreshAll(ctx context.Context) {
	allSymbols, _ := m.discoverer.AllSymbols(ctx)
	activeSymbols, _ := m.discoverer.ActiveSymbols(ctx)
	activeFxPairs, _ := m.discoverer.ActiveFxPairs(ctx)

	if m.logger != nil {
		m.logger.Info("refresh-all: discovered symbols", "allSymbols", len(allSymbols), "activeSymbols", len(activeSymbols), "activeFxPairs", len(activeFxPairs))
	}

	// Refresh current quotes for active symbols.
	if len(activeSymbols) > 0 {
		symbols := make([]string, 0, len(activeSymbols))
		for sym := range activeSymbols {
			symbols = append(symbols, sym)
		}
		quotes := m.fetcher.FetchQuotesBatch(ctx, symbols)
		for sym, quote := range quotes {
			if err := m.repo.Upsert(ctx, quote); err != nil {
				m.logWarn("failed to cache quote", "symbol", sym, "error", err)
			}
		}
	}

	// Refresh current quotes for active FX pairs.
	for pair := range activeFxPairs {
		m.refreshFxQuote(ctx, pair)
	}

	// Fetch historical for all symbols.
	for sym, fromDate := range allSymbols {
		m.mu.Lock()
		m.inProgress[sym] = true
		m.mu.Unlock()

		m.fetchHistoricalDirect(ctx, sym, fromDate)

		m.mu.Lock()
		delete(m.inProgress, sym)
		m.mu.Unlock()
	}

	// Fetch historical for FX pairs.
	for pair, fromDate := range activeFxPairs {
		m.mu.Lock()
		m.inProgress[pair] = true
		m.mu.Unlock()

		m.fetchFxPairDirect(ctx, pair, fromDate)

		m.mu.Lock()
		delete(m.inProgress, pair)
		m.mu.Unlock()
	}

	// Fetch historical for predefined benchmarks.
	m.RefreshPredefinedBenchmarks(ctx)

	if m.logger != nil {
		m.logger.Info("refresh-all: completed")
	}
}

// --- Helpers ---

func (m *MarketCache) logWarn(msg string, args ...any) {
	if m.logger != nil {
		m.logger.Warn(msg, args...)
	}
}

// parseFxPair parses "BASE/QUOTE" into its components.
func parseFxPair(pair string) (base, quote string) {
	for i, c := range pair {
		if c == '/' {
			return pair[:i], pair[i+1:]
		}
	}
	return pair, ""
}

// tradingDayBeforeOrOn returns the most recent trading day on or before the
// given date, skipping Saturday and Sunday. Used so weekend gaps don't
// trigger unnecessary gap-fetches.
func tradingDayBeforeOrOn(t time.Time) time.Time {
	d := t
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// nextTradingDay returns the next day after t that is not Saturday or Sunday.
func nextTradingDay(t time.Time) time.Time {
	d := t.AddDate(0, 0, 1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
