package position

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- Mocks for refresh tests ---

type mockRefreshFetcher struct {
	mu         sync.Mutex
	quotes     map[string]*market.MarketData
	fetchErr   map[string]error
	batchCalls int
}

func newMockRefreshFetcher() *mockRefreshFetcher {
	return &mockRefreshFetcher{
		quotes:   make(map[string]*market.MarketData),
		fetchErr: make(map[string]error),
	}
}

func (m *mockRefreshFetcher) FetchQuote(_ context.Context, symbol string) (*market.MarketData, error) {
	if err, ok := m.fetchErr[symbol]; ok {
		return nil, err
	}
	if q, ok := m.quotes[symbol]; ok {
		return q, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockRefreshFetcher) FetchFxRate(_ context.Context, _, _ string) (*market.MarketData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRefreshFetcher) FetchQuotesBatch(_ context.Context, symbols []string) map[string]*market.MarketData {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchCalls++
	result := make(map[string]*market.MarketData)
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			result[sym] = q
		}
	}
	return result
}

func (m *mockRefreshFetcher) FetchHistoricalPricesBatch(_ context.Context, _ []string, _, _ time.Time) (map[string][]market.HistoricalPrice, []string) {
	return nil, nil
}

type mockRefreshRepo struct {
	mu         sync.Mutex
	upserted   []*market.MarketData
	upsertErr  error
}

func newMockRefreshRepo() *mockRefreshRepo {
	return &mockRefreshRepo{upserted: make([]*market.MarketData, 0)}
}

func (m *mockRefreshRepo) GetLatest(_ context.Context, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockRefreshRepo) GetBySourceAndDate(_ context.Context, _, _, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockRefreshRepo) Upsert(_ context.Context, md *market.MarketData) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upserted = append(m.upserted, md)
	return nil
}

func (m *mockRefreshRepo) GetCurrentFxRate(_ context.Context, _, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockRefreshRepo) UpsertHistoricalPrices(_ context.Context, _ string, _ []market.HistoricalPrice, _ string) error {
	return nil
}

func (m *mockRefreshRepo) GetHistoricalPricesBySymbol(context.Context, string, time.Time, time.Time) ([]market.HistoricalPrice, error) {
	return nil, nil
}

func (m *mockRefreshRepo) GetLatestQuotesBatch(context.Context, []string) map[string]*market.MarketData {
	return nil
}

func (m *mockRefreshRepo) GetLatestPriceDatePerSymbol(context.Context, []string) map[string]*time.Time {
	return nil
}

// mockRefreshMarketService wraps mockRefreshFetcher + mockRefreshRepo to implement MarketDataService.
type mockRefreshMarketService struct {
	fetcher *mockRefreshFetcher
	repo    *mockRefreshRepo
}

func (m *mockRefreshMarketService) GetQuotes(_ context.Context, _ []string) map[string]*market.MarketData {
	return nil
}

func (m *mockRefreshMarketService) GetHistoricalPrices(_ context.Context, _ string, _, _ time.Time) ([]market.HistoricalPrice, error) {
	return nil, nil
}

func (m *mockRefreshMarketService) GetLatestPriceDatePerSymbol(_ context.Context, _ []string) map[string]*time.Time {
	return nil
}

func (m *mockRefreshMarketService) RefreshQuotes(_ context.Context, symbols []string) marketservice.RefreshResult {
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

// mockRefreshAccountLister simulates account listing for refresh tests.
type mockRefreshAccountLister struct {
	accounts []AccountRef
}

func (m *mockRefreshAccountLister) GetAllAccounts(_ context.Context) ([]AccountRef, error) {
	return m.accounts, nil
}

func (m *mockRefreshAccountLister) GetAccountsByPortfolio(_ context.Context, portfolioID int64) ([]AccountRef, error) {
	var result []AccountRef
	for _, a := range m.accounts {
		if a.PortfolioID == portfolioID {
			result = append(result, a)
		}
	}
	if result == nil {
		result = []AccountRef{}
	}
	return result, nil
}

// mockRefreshTxnRepo simulates transaction listing for refresh tests.
type mockRefreshTxnRepo struct {
	mu        sync.RWMutex
	byAccount map[int64][]transaction.Transaction
}

func newMockRefreshTxnRepo() *mockRefreshTxnRepo {
	return &mockRefreshTxnRepo{byAccount: make(map[int64][]transaction.Transaction)}
}

func (m *mockRefreshTxnRepo) Add(accountID int64, txn transaction.Transaction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	txn.ID = int64(len(m.byAccount[accountID]) + 1)
	m.byAccount[accountID] = append(m.byAccount[accountID], txn)
}

func (m *mockRefreshTxnRepo) ListAllTransactionsByAccount(_ context.Context, accountID int64) ([]transaction.Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	txns := m.byAccount[accountID]
	if txns == nil {
		return []transaction.Transaction{}, nil
	}
	return txns, nil
}

// mockRefreshPositionRepo simulates open position listing for refresh tests.
type mockRefreshPositionRepo struct {
	positions []Position
}

func (m *mockRefreshPositionRepo) CreatePosition(_ context.Context, _ *Position) error {
	return nil
}

func (m *mockRefreshPositionRepo) CreateLot(_ context.Context, _ *Lot) error {
	return nil
}

func (m *mockRefreshPositionRepo) CreateConsumption(_ context.Context, _ *LotConsumption) error {
	return nil
}

func (m *mockRefreshPositionRepo) GetOpenPositions(_ context.Context, accountID int64, _, _ int) ([]Position, error) {
	var result []Position
	for _, p := range m.positions {
		if p.AccountID == accountID && !p.IsClosed {
			result = append(result, p)
		}
	}
	if result == nil {
		result = []Position{}
	}
	return result, nil
}

func (m *mockRefreshPositionRepo) GetClosedPositions(_ context.Context, _ int64, _, _ int) ([]Position, error) {
	return []Position{}, nil
}

func (m *mockRefreshPositionRepo) GetLotByLotID(_ context.Context, _ string) (*Lot, error) {
	return nil, nil
}

func (m *mockRefreshPositionRepo) GetConsumptionsBySellLot(_ context.Context, _ string) ([]LotConsumption, error) {
	return []LotConsumption{}, nil
}

func (m *mockRefreshPositionRepo) DeleteAllForAccount(_ context.Context, _ int64) error {
	return nil
}

func (m *mockRefreshPositionRepo) Recalculate(_ context.Context, _ int64, _ *CalculateResult) error {
	return nil
}

// mockRefreshFxProvider simulates FX rate provider for refresh tests.
type mockRefreshFxProvider struct {
	rates map[string]map[string]*market.FxRate
}

func (m *mockRefreshFxProvider) GetRateForDate(_ context.Context, from, to string, _ time.Time) (*market.FxRate, bool) {
	if rates, ok := m.rates[from]; ok {
		if rate, ok := rates[to]; ok {
			return rate, true
		}
	}
	return nil, false
}

func (m *mockRefreshFxProvider) GetCurrentRate(_ context.Context, from, to string) (*market.FxRate, bool) {
	return m.GetRateForDate(context.Background(), from, to, time.Time{})
}

// --- Helper ---

func makeQuote(symbol string, price int64, currency string) *market.MarketData {
	return &market.MarketData{
		Symbol:    symbol,
		Price:     decimal.MustNew(price, 2),
		Currency:  currency,
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "",
		FetchedAt: time.Now(),
	}
}

func makeFxRate(base, quote string, rate int64) *market.FxRate {
	return &market.FxRate{
		BaseCurrency:  base,
		QuoteCurrency: quote,
		Rate:          decimal.MustNew(rate, 4),
		FetchedAt:     time.Now(),
	}
}

// --- Tests ---

func TestRefreshMarketData_Success(t *testing.T) {
	fetcher := newMockRefreshFetcher()
	fetcher.quotes["AAPL"] = makeQuote("AAPL", 17500, "USD")
	fetcher.quotes["GOOGL"] = makeQuote("GOOGL", 14000, "USD")

	repo := newMockRefreshRepo()

	txnRepo := newMockRefreshTxnRepo()
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2025, 1, 15), Type: "buy",
		Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), Price: decimal.MustNew(17000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-1700000, 2),
	})
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2025, 2, 10), Type: "buy",
		Symbol: "GOOGL", Quantity: decimal.MustNew(500, 2), Price: decimal.MustNew(14000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-700000, 2),
	})

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"}},
	}

	posRepo := &mockRefreshPositionRepo{
		positions: []Position{
			{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), IsClosed: false},
			{ID: 2, AccountID: 1, Symbol: "GOOGL", Quantity: decimal.MustNew(500, 2), IsClosed: false},
		},
	}

	svc := &Service{
		positions:      posRepo,
		transactions:   txnRepo,
		accountLister:  accountLister,
		marketService: &mockRefreshMarketService{fetcher: fetcher, repo: repo},
	}

	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check symbols refreshed.
	if len(result.SymbolsRefreshed) != 2 {
		t.Fatalf("expected 2 symbols refreshed, got %d: %v", len(result.SymbolsRefreshed), result.SymbolsRefreshed)
	}
	sort.Strings(result.SymbolsRefreshed)
	if result.SymbolsRefreshed[0] != "AAPL" || result.SymbolsRefreshed[1] != "GOOGL" {
		t.Errorf("unexpected symbols: %v", result.SymbolsRefreshed)
	}

	// Check no failures.
	if len(result.FailedSymbols) != 0 {
		t.Errorf("expected no failed symbols, got %v", result.FailedSymbols)
	}

	// Check no FX pairs needed (all USD).
	if len(result.FxPairsRefreshed) != 0 {
		t.Errorf("expected no FX pairs, got %v", result.FxPairsRefreshed)
	}

	// Check upserted count.
	if len(repo.upserted) != 2 {
		t.Errorf("expected 2 upserts, got %d", len(repo.upserted))
	}

	// Check batch was called once.
	if fetcher.batchCalls != 1 {
		t.Errorf("expected 1 batch call, got %d", fetcher.batchCalls)
	}
}

func TestRefreshMarketData_PartialFailure(t *testing.T) {
	fetcher := newMockRefreshFetcher()
	fetcher.quotes["AAPL"] = makeQuote("AAPL", 17500, "USD")
	// GOOGL not in quotes map → simulates fetch failure

	repo := newMockRefreshRepo()

	txnRepo := newMockRefreshTxnRepo()
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2025, 1, 15), Type: "buy",
		Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), Price: decimal.MustNew(17000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-1700000, 2),
	})
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2025, 2, 10), Type: "buy",
		Symbol: "GOOGL", Quantity: decimal.MustNew(500, 2), Price: decimal.MustNew(14000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-700000, 2),
	})

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"}},
	}

	posRepo := &mockRefreshPositionRepo{}

	svc := &Service{
		positions:      posRepo,
		transactions:   txnRepo,
		accountLister:  accountLister,
		marketService: &mockRefreshMarketService{fetcher: fetcher, repo: repo},
	}

	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AAPL should succeed.
	if len(result.SymbolsRefreshed) != 1 || result.SymbolsRefreshed[0] != "AAPL" {
		t.Errorf("expected 1 symbol refreshed (AAPL), got %v", result.SymbolsRefreshed)
	}

	// GOOGL should fail.
	if len(result.FailedSymbols) != 1 || result.FailedSymbols[0] != "GOOGL" {
		t.Errorf("expected 1 failed symbol (GOOGL), got %v", result.FailedSymbols)
	}

	// Only AAPL should be upserted.
	if len(repo.upserted) != 1 {
		t.Errorf("expected 1 upsert, got %d", len(repo.upserted))
	}
}

func TestRefreshMarketData_EmptyPortfolio(t *testing.T) {
	fetcher := newMockRefreshFetcher()
	repo := newMockRefreshRepo()
	txnRepo := newMockRefreshTxnRepo()

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"}},
	}

	posRepo := &mockRefreshPositionRepo{}

	svc := &Service{
		positions:      posRepo,
		transactions:   txnRepo,
		accountLister:  accountLister,
		marketService: &mockRefreshMarketService{fetcher: fetcher, repo: repo},
	}

	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.SymbolsRefreshed) != 0 {
		t.Errorf("expected no symbols refreshed, got %v", result.SymbolsRefreshed)
	}
	if len(result.FailedSymbols) != 0 {
		t.Errorf("expected no failed symbols, got %v", result.FailedSymbols)
	}
	if len(result.FxPairsRefreshed) != 0 {
		t.Errorf("expected no FX pairs, got %v", result.FxPairsRefreshed)
	}

	// No batch call should be made.
	if fetcher.batchCalls != 0 {
		t.Errorf("expected 0 batch calls, got %d", fetcher.batchCalls)
	}
}

func TestRefreshMarketData_MultiCurrency(t *testing.T) {
	fetcher := newMockRefreshFetcher()
	fetcher.quotes["AAPL"] = makeQuote("AAPL", 17500, "USD")
	fetcher.quotes["SHEL.L"] = makeQuote("SHEL.L", 2500, "GBP")

	repo := newMockRefreshRepo()

	txnRepo := newMockRefreshTxnRepo()
	// USD transaction
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2025, 1, 15), Type: "buy",
		Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), Price: decimal.MustNew(17000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-1700000, 2),
	})
	// GBP transaction
	txnRepo.Add(2, transaction.Transaction{
		AccountID: 2, Date: testTime(2025, 2, 10), Type: "buy",
		Symbol: "SHEL.L", Quantity: decimal.MustNew(2000, 2), Price: decimal.MustNew(2500, 2),
		Currency: "GBP", NetCash: decimal.MustNew(-500000, 2),
	})
	// GBP deposit
	txnRepo.Add(2, transaction.Transaction{
		AccountID: 2, Date: testTime(2025, 1, 1), Type: "deposit",
		Symbol: "$CASH-GBP", Quantity: decimal.MustNew(100000, 2), Price: decimal.MustNew(100, 2),
		Currency: "GBP", NetCash: decimal.MustNew(100000, 2),
	})

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{
			{ID: 1, Name: "US Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "UK Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
		},
	}

	posRepo := &mockRefreshPositionRepo{}

	fxProvider := &mockRefreshFxProvider{
		rates: map[string]map[string]*market.FxRate{
			"GBP": {"USD": makeFxRate("GBP", "USD", 12500)},
		},
	}

	svc := &Service{
		positions:      posRepo,
		transactions:   txnRepo,
		accountLister:  accountLister,
		marketService: &mockRefreshMarketService{fetcher: fetcher, repo: repo},
		fxProvider:     fxProvider,
	}

	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both symbols should be refreshed.
	if len(result.SymbolsRefreshed) != 2 {
		t.Fatalf("expected 2 symbols refreshed, got %d: %v", len(result.SymbolsRefreshed), result.SymbolsRefreshed)
	}

	// GBP/USD FX pair should be refreshed.
	if len(result.FxPairsRefreshed) != 1 {
		t.Fatalf("expected 1 FX pair, got %d: %v", len(result.FxPairsRefreshed), result.FxPairsRefreshed)
	}
	if result.FxPairsRefreshed[0] != "GBP/USD" {
		t.Errorf("expected GBP/USD, got %s", result.FxPairsRefreshed[0])
	}
}

func TestRefreshMarketData_OpenPositionsOnly(t *testing.T) {
	fetcher := newMockRefreshFetcher()
	fetcher.quotes["MSFT"] = makeQuote("MSFT", 42000, "USD")

	repo := newMockRefreshRepo()
	txnRepo := newMockRefreshTxnRepo()

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"}},
	}

	// Open position exists but no transactions in the date range
	posRepo := &mockRefreshPositionRepo{
		positions: []Position{
			{ID: 1, AccountID: 1, Symbol: "MSFT", Quantity: decimal.MustNew(500, 2), IsClosed: false},
		},
	}

	svc := &Service{
		positions:      posRepo,
		transactions:   txnRepo,
		accountLister:  accountLister,
		marketService: &mockRefreshMarketService{fetcher: fetcher, repo: repo},
	}

	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// MSFT should be refreshed from open positions.
	if len(result.SymbolsRefreshed) != 1 || result.SymbolsRefreshed[0] != "MSFT" {
		t.Errorf("expected MSFT refreshed from open positions, got %v", result.SymbolsRefreshed)
	}
}

func TestRefreshMarketData_NoMarketFetcher(t *testing.T) {
	txnRepo := newMockRefreshTxnRepo()
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2025, 1, 15), Type: "buy",
		Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), Price: decimal.MustNew(17000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-1700000, 2),
	})

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"}},
	}

	posRepo := &mockRefreshPositionRepo{}

	svc := &Service{
		positions:     posRepo,
		transactions:  txnRepo,
		accountLister: accountLister,
		// No marketService set
	}

	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No symbols refreshed or failed (no fetcher).
	if len(result.SymbolsRefreshed) != 0 {
		t.Errorf("expected no symbols refreshed, got %v", result.SymbolsRefreshed)
	}
	if len(result.FailedSymbols) != 0 {
		t.Errorf("expected no failed symbols, got %v", result.FailedSymbols)
	}
}

func TestRefreshMarketData_PeriodFilter(t *testing.T) {
	fetcher := newMockRefreshFetcher()
	fetcher.quotes["AAPL"] = makeQuote("AAPL", 17500, "USD")
	fetcher.quotes["TSLA"] = makeQuote("TSLA", 20000, "USD")

	repo := newMockRefreshRepo()

	txnRepo := newMockRefreshTxnRepo()
	// Old transaction (outside 1M period)
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2024, 1, 15), Type: "buy",
		Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), Price: decimal.MustNew(17000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-1700000, 2),
	})
	// Recent transaction (inside 1M period)
	now := time.Now().UTC()
	recentDate := now.AddDate(0, 0, -15)
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: recentDate, Type: "buy",
		Symbol: "TSLA", Quantity: decimal.MustNew(500, 2), Price: decimal.MustNew(20000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-1000000, 2),
	})

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"}},
	}

	posRepo := &mockRefreshPositionRepo{}

	svc := &Service{
		positions:      posRepo,
		transactions:   txnRepo,
		accountLister:  accountLister,
		marketService: &mockRefreshMarketService{fetcher: fetcher, repo: repo},
	}

	// Use 1M period — should only pick up TSLA from transactions
	// (AAPL transaction is outside the date range)
	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{Period: "1M"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only TSLA should be in the transaction-derived symbols.
	// But AAPL is not in open positions, so it should not appear.
	if len(result.SymbolsRefreshed) != 1 || result.SymbolsRefreshed[0] != "TSLA" {
		t.Errorf("expected only TSLA refreshed, got %v", result.SymbolsRefreshed)
	}
}

func TestRefreshMarketData_PortfolioFilter(t *testing.T) {
	fetcher := newMockRefreshFetcher()
	fetcher.quotes["AAPL"] = makeQuote("AAPL", 17500, "USD")

	repo := newMockRefreshRepo()

	txnRepo := newMockRefreshTxnRepo()
	// Transaction in portfolio 1
	txnRepo.Add(1, transaction.Transaction{
		AccountID: 1, Date: testTime(2025, 1, 15), Type: "buy",
		Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), Price: decimal.MustNew(17000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-1700000, 2),
	})
	// Transaction in portfolio 2 (should not be included)
	txnRepo.Add(2, transaction.Transaction{
		AccountID: 2, Date: testTime(2025, 1, 15), Type: "buy",
		Symbol: "GOOGL", Quantity: decimal.MustNew(500, 2), Price: decimal.MustNew(14000, 2),
		Currency: "USD", NetCash: decimal.MustNew(-700000, 2),
	})

	accountLister := &mockRefreshAccountLister{
		accounts: []AccountRef{
			{ID: 1, Name: "US Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Other Broker", PortfolioID: 2, PortfolioCurrency: "USD"},
		},
	}

	posRepo := &mockRefreshPositionRepo{}

	portfolioID := int64(1)
	svc := &Service{
		positions:      posRepo,
		transactions:   txnRepo,
		accountLister:  accountLister,
		marketService: &mockRefreshMarketService{fetcher: fetcher, repo: repo},
	}

	result, err := svc.RefreshMarketData(ctx, PerformanceFilters{PortfolioID: &portfolioID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only AAPL (portfolio 1) should be refreshed, not GOOGL (portfolio 2).
	if len(result.SymbolsRefreshed) != 1 || result.SymbolsRefreshed[0] != "AAPL" {
		t.Errorf("expected only AAPL refreshed, got %v", result.SymbolsRefreshed)
	}
}
