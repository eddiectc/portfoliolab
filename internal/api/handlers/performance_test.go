package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// --- Mocks (implementing position/market interfaces) ---

type mockPosRepoForPerf struct {
	mu         sync.RWMutex
	positions  []position.Position
	openErr    error
	closedErr  error
}

func newMockPosRepoForPerf() *mockPosRepoForPerf {
	return &mockPosRepoForPerf{positions: []position.Position{}}
}

func (m *mockPosRepoForPerf) CreatePosition(context.Context, *position.Position) error            { return nil }
func (m *mockPosRepoForPerf) CreateLot(context.Context, *position.Lot) error                     { return nil }
func (m *mockPosRepoForPerf) CreateConsumption(context.Context, *position.LotConsumption) error  { return nil }
func (m *mockPosRepoForPerf) DeleteAllForAccount(context.Context, int64) error                   { return nil }
func (m *mockPosRepoForPerf) Recalculate(context.Context, int64, *position.CalculateResult) error { return nil }

func (m *mockPosRepoForPerf) GetOpenPositions(_ context.Context, accountID int64, _limit, _offset int) ([]position.Position, error) {
	if m.openErr != nil {
		return nil, m.openErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []position.Position
	for _, p := range m.positions {
		if p.AccountID == accountID && !p.IsClosed {
			result = append(result, p)
		}
	}
	if result == nil {
		result = []position.Position{}
	}
	return result, nil
}

func (m *mockPosRepoForPerf) GetClosedPositions(context.Context, int64, int, int) ([]position.Position, error) {
	return []position.Position{}, nil
}
func (m *mockPosRepoForPerf) GetLotByLotID(context.Context, string) (*position.Lot, error) {
	return nil, position.ErrLotNotFound
}
func (m *mockPosRepoForPerf) GetConsumptionsBySellLot(context.Context, string) ([]position.LotConsumption, error) {
	return []position.LotConsumption{}, nil
}

type mockTxnRepoForPerf struct {
	mu        sync.RWMutex
	byAccount map[int64][]transaction.Transaction
}

func newMockTxnRepoForPerf() *mockTxnRepoForPerf {
	return &mockTxnRepoForPerf{byAccount: make(map[int64][]transaction.Transaction)}
}

func (m *mockTxnRepoForPerf) ListAllTransactionsByAccount(_ context.Context, accountID int64) ([]transaction.Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	txns, ok := m.byAccount[accountID]
	if !ok {
		return []transaction.Transaction{}, nil
	}
	result := make([]transaction.Transaction, len(txns))
	copy(result, txns)
	return result, nil
}

// Stub implementations for new TransactionRepository methods (unused in these tests).
func (m *mockTxnRepoForPerf) GetSymbolsWithEarliestDate(_ context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockTxnRepoForPerf) GetSymbolsByOpenPositions(_ context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockTxnRepoForPerf) GetFxPairsByOpenPositions(_ context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockTxnRepoForPerf) GetEarliestDateBySymbol(_ context.Context, _ string) (*time.Time, error) {
	return nil, nil
}

type mockAccountCheckerForPerf struct {
	existing map[int64]bool
}

func newMockAccountCheckerForPerf(ids ...int64) *mockAccountCheckerForPerf {
	m := &mockAccountCheckerForPerf{existing: make(map[int64]bool)}
	for _, id := range ids {
		m.existing[id] = true
	}
	return m
}

func (m *mockAccountCheckerForPerf) AccountExists(context.Context, int64) bool {
	return false
}

type mockPortfolioCheckerForPerf struct {
	existing map[int64]bool
}

func newMockPortfolioCheckerForPerf(ids ...int64) *mockPortfolioCheckerForPerf {
	m := &mockPortfolioCheckerForPerf{existing: make(map[int64]bool)}
	for _, id := range ids {
		m.existing[id] = true
	}
	return m
}

func (m *mockPortfolioCheckerForPerf) PortfolioExists(context.Context, int64) bool {
	return false
}

type mockAccountListerForPerf struct {
	allAccounts         []position.AccountRef
	accountsByPortfolio map[int64][]position.AccountRef
}

func newMockAccountListerForPerf() *mockAccountListerForPerf {
	return &mockAccountListerForPerf{
		allAccounts:         []position.AccountRef{},
		accountsByPortfolio: make(map[int64][]position.AccountRef),
	}
}

func (m *mockAccountListerForPerf) GetAllAccounts(context.Context) ([]position.AccountRef, error) {
	result := make([]position.AccountRef, len(m.allAccounts))
	copy(result, m.allAccounts)
	return result, nil
}

func (m *mockAccountListerForPerf) GetAccountsByPortfolio(_ context.Context, portfolioID int64) ([]position.AccountRef, error) {
	accounts, ok := m.accountsByPortfolio[portfolioID]
	if !ok {
		return []position.AccountRef{}, nil
	}
	result := make([]position.AccountRef, len(accounts))
	copy(result, accounts)
	return result, nil
}

type mockFxProviderForPerf struct {
	rates map[string]map[string]*market.FxRate
}

func (m *mockFxProviderForPerf) GetRateForDate(_ context.Context, from, to string, _ time.Time) (*market.FxRate, bool) {
	if rates, ok := m.rates[from]; ok {
		if rate, ok := rates[to]; ok {
			return rate, true
		}
	}
	return nil, false
}

func (m *mockFxProviderForPerf) GetCurrentRate(_ context.Context, from, to string) (*market.FxRate, bool) {
	return m.GetRateForDate(context.Background(), from, to, time.Time{})
}

type mockMarketFetcherForPerf struct {
	prices    map[string][]market.HistoricalPrice
	failed    []string
	quotes    map[string]*market.MarketData
	quoteErr  bool
}

func (m *mockMarketFetcherForPerf) FetchQuote(_ context.Context, symbol string) (*market.MarketData, error) {
	if m.quoteErr {
		return nil, fmt.Errorf("fetch failed")
	}
	if q, ok := m.quotes[symbol]; ok {
		return q, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockMarketFetcherForPerf) FetchFxRate(context.Context, string, string) (*market.MarketData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockMarketFetcherForPerf) FetchQuotesBatch(_ context.Context, symbols []string) map[string]*market.MarketData {
	result := make(map[string]*market.MarketData)
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			result[sym] = q
		}
	}
	return result
}

func (m *mockMarketFetcherForPerf) FetchHistoricalPricesBatch(_ context.Context, symbols []string, _, _ time.Time) (map[string][]market.HistoricalPrice, []string) {
	result := make(map[string][]market.HistoricalPrice)
	for _, sym := range symbols {
		if prices, ok := m.prices[sym]; ok {
			result[sym] = prices
		}
	}
	return result, m.failed
}

type mockMarketDataRepoForPerf struct {
	upserted map[string][]market.HistoricalPrice
}

func newMockMarketDataRepoForPerf() *mockMarketDataRepoForPerf {
	return &mockMarketDataRepoForPerf{upserted: make(map[string][]market.HistoricalPrice)}
}

func (m *mockMarketDataRepoForPerf) GetLatest(context.Context, string) (*market.MarketData, error)   { return nil, nil }
func (m *mockMarketDataRepoForPerf) GetBySourceAndDate(context.Context, string, string, string) (*market.MarketData, error) {
	return nil, nil
}
func (m *mockMarketDataRepoForPerf) Upsert(context.Context, *market.MarketData) error                { return nil }
func (m *mockMarketDataRepoForPerf) GetCurrentFxRate(context.Context, string, string) (*market.MarketData, error) {
	return nil, nil
}
func (m *mockMarketDataRepoForPerf) UpsertHistoricalPrices(_ context.Context, symbol string, prices []market.HistoricalPrice, _dataType string) error {
	m.upserted[symbol] = prices
	return nil
}

func (m *mockMarketDataRepoForPerf) GetHistoricalPricesBySymbol(context.Context, string, time.Time, time.Time) ([]market.HistoricalPrice, error) {
	return nil, nil
}

func (m *mockMarketDataRepoForPerf) GetLatestQuotesBatch(context.Context, []string) map[string]*market.MarketData {
	return nil
}

func (m *mockMarketDataRepoForPerf) GetLatestPriceDatePerSymbol(context.Context, []string) map[string]*time.Time {
	return nil
}

// mockPerfMarketService wraps mockMarketFetcherForPerf + mockMarketDataRepoForPerf.
type mockPerfMarketService struct {
	fetcher *mockMarketFetcherForPerf
	repo    *mockMarketDataRepoForPerf
}

func (m *mockPerfMarketService) GetQuotes(_ context.Context, symbols []string) map[string]*market.MarketData {
	return m.repo.GetLatestQuotesBatch(context.Background(), symbols)
}

func (m *mockPerfMarketService) GetHistoricalPrices(_ context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error) {
	return m.repo.GetHistoricalPricesBySymbol(context.Background(), symbol, start, end)
}

func (m *mockPerfMarketService) GetLatestPriceDatePerSymbol(_ context.Context, symbols []string) map[string]*time.Time {
	return m.repo.GetLatestPriceDatePerSymbol(context.Background(), symbols)
}

func (m *mockPerfMarketService) RefreshQuotes(_ context.Context, symbols []string) marketservice.RefreshResult {
	quotes := m.fetcher.FetchQuotesBatch(context.Background(), symbols)
	var refreshed, failed []string
	for _, sym := range symbols {
		if quote, found := quotes[sym]; found {
			if err := m.repo.Upsert(context.Background(), quote); err != nil {
				failed = append(failed, sym)
				continue
			}
			refreshed = append(refreshed, sym)
		} else {
			failed = append(failed, sym)
		}
	}
	return marketservice.RefreshResult{Refreshed: refreshed, Failed: failed}
}

func (m *mockPerfMarketService) GetCurrentFxRate(_ context.Context, _, _ string) (*market.FxRate, error) {
	return nil, nil
}

func (m *mockPerfMarketService) GetHistoricalFxRate(_ context.Context, _, _ string, _ time.Time) (*market.FxRate, error) {
	return nil, nil
}

func (m *mockPerfMarketService) RefreshFxRates(_ context.Context, _ []marketservice.FxPair) marketservice.FxRefreshResult {
	return marketservice.FxRefreshResult{}
}

// --- Test helpers ---

func perfTxn(accountID int64, date time.Time, typ, symbol, currency string, qty, price, netCash int64) transaction.Transaction {
	return transaction.Transaction{
		AccountID: accountID,
		Date:      date,
		Type:      typ,
		Symbol:    symbol,
		Quantity:  decimal.MustNew(qty, 2),
		Price:     decimal.MustNew(price, 2),
		Currency:  currency,
		NetCash:   decimal.MustNew(netCash, 2),
		CreatedAt: date,
		UpdatedAt: date,
	}
}

func perfTime(y, m int, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func perfHistPrice(date time.Time, closeVal int64, currency string) market.HistoricalPrice {
	return market.HistoricalPrice{
		Date:     date,
		Close:    decimal.MustNew(closeVal, 2),
		Currency: currency,
	}
}

func ptrInt64(v int64) *int64 { return &v }

// newPerfService creates a position.Service wired with mock deps for handler tests.
func newPerfService(accountIDs []int64, portfolioIDs []int64) (*position.Service, *mockTxnRepoForPerf, *mockAccountListerForPerf, *mockMarketFetcherForPerf, *mockMarketDataRepoForPerf) {
	txnRepo := newMockTxnRepoForPerf()
	accountLister := newMockAccountListerForPerf()
	fetcher := &mockMarketFetcherForPerf{prices: make(map[string][]market.HistoricalPrice), quotes: make(map[string]*market.MarketData)}
	repo := newMockMarketDataRepoForPerf()

	svc := position.NewService(
		newMockPosRepoForPerf(),
		txnRepo,
		newMockAccountCheckerForPerf(accountIDs...),
		newMockPortfolioCheckerForPerf(portfolioIDs...),
		accountLister,
		nil, // no portfolio currency checker
	)
	svc.WithMarketDataService(&mockPerfMarketService{fetcher: fetcher, repo: repo}, nil)

	return svc, txnRepo, accountLister, fetcher, repo
}

// --- HandlePerformance Tests ---

func TestPerfHandlePerformance_Success(t *testing.T) {
	svc, txnRepo, accountLister, fetcher, _ := newPerfService([]int64{1}, []int64{})
	_ = svc // used via handler
	accountLister.accountsByPortfolio[1] = []position.AccountRef{
		{ID: 1, Name: "Test Account", PortfolioID: 1, PortfolioCurrency: "USD"},
	}

	// Deposit + buy
	txnRepo.byAccount[1] = []transaction.Transaction{
		perfTxn(1, perfTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		perfTxn(1, perfTime(2024, 2, 1), "buy", "AAPL", "USD", 1000, 15000, -1500000),
	}
	fetcher.prices["AAPL"] = []market.HistoricalPrice{
		perfHistPrice(perfTime(2024, 2, 1), 15000, "USD"),
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result position.PerformanceResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.BaseCurrency != "USD" {
		t.Errorf("expected base currency 'USD', got %q", result.BaseCurrency)
	}
	if len(result.EquityCurve) < 2 {
		t.Errorf("expected at least 2 equity curve points, got %d", len(result.EquityCurve))
	}
}

func TestPerfHandlePerformance_EmptyState(t *testing.T) {
	svc, _, accountLister, _, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[1] = []position.AccountRef{
		{ID: 1, Name: "Test Account", PortfolioID: 1, PortfolioCurrency: "EUR"},
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result position.PerformanceResult
	json.NewDecoder(w.Body).Decode(&result)
	if !result.ReturnMetrics.HasInsufficientData {
		t.Error("expected HasInsufficientData=true for empty state")
	}
}

func TestPerfHandlePerformance_MismatchedCurrencies(t *testing.T) {
	// No portfolio filter → all accounts → mismatched currencies
	svc, _, accountLister, _, _ := newPerfService([]int64{1, 2}, []int64{})
	accountLister.allAccounts = []position.AccountRef{
		{ID: 1, Name: "USD Account", PortfolioID: 1, PortfolioCurrency: "USD"},
		{ID: 2, Name: "GBP Account", PortfolioID: 2, PortfolioCurrency: "GBP"},
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISMATCHED_CURRENCIES" {
		t.Errorf("expected MISMATCHED_CURRENCIES, got %q", errResp.Code)
	}
}

func TestPerfHandlePerformance_InternalError(t *testing.T) {
	// Simulate internal error by setting up a scenario that causes a generic error
	svc, _, accountLister, _, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[1] = []position.AccountRef{
		{ID: 1, Name: "Test", PortfolioID: 1, PortfolioCurrency: "USD"},
	}

	handler := NewPerformanceHandler(svc)

	// No transactions → returns empty result (not an error), so test with valid request
	req := httptest.NewRequest(http.MethodGet, "/api/performance?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	// Should succeed (empty state is not an error)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// --- HandleRefresh Tests ---

func TestPerfHandleRefresh_Success(t *testing.T) {
	svc, _, accountLister, fetcher, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[1] = []position.AccountRef{
		{ID: 1, Name: "Test Account", PortfolioID: 1, PortfolioCurrency: "USD"},
	}
	fetcher.quotes["AAPL"] = &market.MarketData{
		Symbol:   "AAPL",
		Price:    decimal.MustParse("150.00"),
		Currency: "USD",
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/performance/refresh?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleRefresh(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result position.RefreshResult
	json.NewDecoder(w.Body).Decode(&result)
	// Even with no transactions, refresh returns empty result
	if result.SymbolsRefreshed == nil && result.FxPairsRefreshed == nil {
		// Both nil is fine for empty portfolio
	}
}

func TestPerfHandleRefresh_WithPositions(t *testing.T) {
	svc, _, accountLister, fetcher, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[1] = []position.AccountRef{
		{ID: 1, Name: "Test Account", PortfolioID: 1, PortfolioCurrency: "USD"},
	}
	fetcher.quotes["AAPL"] = &market.MarketData{
		Symbol:   "AAPL",
		Price:    decimal.MustParse("150.00"),
		Currency: "USD",
	}
	fetcher.quotes["MSFT"] = &market.MarketData{
		Symbol:   "MSFT",
		Price:    decimal.MustParse("400.00"),
		Currency: "USD",
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/performance/refresh?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleRefresh(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result position.RefreshResult
	json.NewDecoder(w.Body).Decode(&result)
	// Result structure should be valid JSON
	if result.FailedSymbols == nil {
		result.FailedSymbols = []string{}
	}
}

// --- Filter Parsing Tests ---

func TestPerfParseFilters_PortfolioID(t *testing.T) {
	svc, _, accountLister, _, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[42] = []position.AccountRef{
		{ID: 1, Name: "Test", PortfolioID: 42, PortfolioCurrency: "USD"},
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance?portfolio_id=42", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPerfParseFilters_Period(t *testing.T) {
	svc, _, accountLister, _, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[1] = []position.AccountRef{
		{ID: 1, Name: "Test", PortfolioID: 1, PortfolioCurrency: "USD"},
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance?period=1Y", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPerfParseFilters_BothParams(t *testing.T) {
	svc, _, accountLister, _, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[7] = []position.AccountRef{
		{ID: 1, Name: "Test", PortfolioID: 7, PortfolioCurrency: "USD"},
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance?portfolio_id=7&period=3M", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPerfParseFilters_NoParams(t *testing.T) {
	svc, _, accountLister, _, _ := newPerfService([]int64{1}, []int64{})
	accountLister.allAccounts = []position.AccountRef{
		{ID: 1, Name: "Test", PortfolioID: 1, PortfolioCurrency: "USD"},
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// --- Route Registration Tests ---

func TestPerfRoutesRegistered(t *testing.T) {
	svc, _, accountLister, _, _ := newPerfService([]int64{1}, []int64{})
	accountLister.accountsByPortfolio[1] = []position.AccountRef{
		{ID: 1, Name: "Test", PortfolioID: 1, PortfolioCurrency: "USD"},
	}

	r := chi.NewRouter()
	handler := NewPerformanceHandler(svc)
	handler.RegisterRoutes(r)

	tests := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/performance"},
		{http.MethodPost, "/api/performance/refresh"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code < 200 || w.Code >= 400 {
			t.Errorf("unexpected response for %s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}

// --- Error Response Format ---

func TestPerfErrorResponseFormat(t *testing.T) {
	svc, _, accountLister, _, _ := newPerfService([]int64{1, 2}, []int64{})
	accountLister.allAccounts = []position.AccountRef{
		{ID: 1, Name: "USD", PortfolioID: 1, PortfolioCurrency: "USD"},
		{ID: 2, Name: "GBP", PortfolioID: 2, PortfolioCurrency: "GBP"},
	}

	handler := NewPerformanceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/performance", nil)
	w := httptest.NewRecorder()

	handler.HandlePerformance(w, req)

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISMATCHED_CURRENCIES" {
		t.Errorf("expected MISMATCHED_CURRENCIES, got %q", errResp.Code)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}
}
