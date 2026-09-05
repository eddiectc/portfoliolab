package position

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/marketservice"
	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/eddiectc/portfoliolab/internal/market"
)

// ctx is a test context.
var ctx = context.Background()

// --- Mocks ---

type mockPositionRepository struct {
	mu               sync.RWMutex
	positions        []Position
	lots             []Lot
	consumptions     []LotConsumption
	deleteErr        error
	createPosErr     error
	createLotErr     error
	createConsumeErr error
	getOpenErr       error
	getClosedErr     error
	getLotErr        error
	getConsumeErr    error
	recalculateErr   error
}

func newMockPositionRepository() *mockPositionRepository {
	return &mockPositionRepository{
		positions:    []Position{},
		lots:         []Lot{},
		consumptions: []LotConsumption{},
	}
}

func (m *mockPositionRepository) CreatePosition(_ context.Context, p *Position) error {
	if m.createPosErr != nil {
		return m.createPosErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p.ID = int64(len(m.positions) + 1)
	m.positions = append(m.positions, *p)
	return nil
}

func (m *mockPositionRepository) CreateLot(_ context.Context, l *Lot) error {
	if m.createLotErr != nil {
		return m.createLotErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	l.ID = int64(len(m.lots) + 1)
	m.lots = append(m.lots, *l)
	return nil
}

func (m *mockPositionRepository) CreateConsumption(_ context.Context, c *LotConsumption) error {
	if m.createConsumeErr != nil {
		return m.createConsumeErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c.ID = int64(len(m.consumptions) + 1)
	m.consumptions = append(m.consumptions, *c)
	return nil
}

func (m *mockPositionRepository) GetOpenPositions(_ context.Context, accountID int64, _limit, _offset int) ([]Position, error) {
	if m.getOpenErr != nil {
		return nil, m.getOpenErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
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

func (m *mockPositionRepository) GetClosedPositions(_ context.Context, accountID int64, _limit, _offset int) ([]Position, error) {
	if m.getClosedErr != nil {
		return nil, m.getClosedErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Position
	for _, p := range m.positions {
		if p.AccountID == accountID && p.IsClosed {
			result = append(result, p)
		}
	}
	if result == nil {
		result = []Position{}
	}
	return result, nil
}

func (m *mockPositionRepository) GetLotByLotID(_ context.Context, lotID string) (*Lot, error) {
	if m.getLotErr != nil {
		return nil, m.getLotErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, l := range m.lots {
		if l.LotID == lotID {
			return &l, nil
		}
	}
	return nil, ErrLotNotFound
}

func (m *mockPositionRepository) GetConsumptionsBySellLot(_ context.Context, sellLotID string) ([]LotConsumption, error) {
	if m.getConsumeErr != nil {
		return nil, m.getConsumeErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []LotConsumption
	for _, c := range m.consumptions {
		if c.SellLotID == sellLotID {
			result = append(result, c)
		}
	}
	if result == nil {
		result = []LotConsumption{}
	}
	return result, nil
}

func (m *mockPositionRepository) DeleteAllForAccount(_ context.Context, accountID int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.positions = nil
	m.lots = nil
	m.consumptions = nil
	return nil
}

func (m *mockPositionRepository) Recalculate(_ context.Context, accountID int64, result *CalculateResult) error {
	if m.recalculateErr != nil {
		return m.recalculateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Simulate real behavior: delete old, insert new.
	m.positions = nil
	m.lots = nil
	m.consumptions = nil
	now := time.Now()
	for i := range result.Lots {
		result.Lots[i].CreatedAt = now
		result.Lots[i].UpdatedAt = now
		result.Lots[i].ID = int64(len(m.lots) + 1)
		m.lots = append(m.lots, result.Lots[i])
	}
	for i := range result.Consumptions {
		result.Consumptions[i].CreatedAt = now
		result.Consumptions[i].ID = int64(len(m.consumptions) + 1)
		m.consumptions = append(m.consumptions, result.Consumptions[i])
	}
	allPositions := append(append(result.OpenPositions, result.ClosedPositions...), result.CashPositions...)
	for i := range allPositions {
		allPositions[i].CreatedAt = now
		allPositions[i].UpdatedAt = now
		allPositions[i].ID = int64(len(m.positions) + 1)
		m.positions = append(m.positions, allPositions[i])
	}
	return nil
}

type mockTransactionRepository struct {
	mu        sync.RWMutex
	byAccount map[int64][]transaction.Transaction
	listErr   error
}

func newMockTransactionRepository() *mockTransactionRepository {
	return &mockTransactionRepository{
		byAccount: make(map[int64][]transaction.Transaction),
	}
}

func (m *mockTransactionRepository) SetTransactions(accountID int64, txns []transaction.Transaction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byAccount[accountID] = txns
}

func (m *mockTransactionRepository) ListAllTransactionsByAccount(_ context.Context, accountID int64) ([]transaction.Transaction, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
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
func (m *mockTransactionRepository) GetSymbolsWithEarliestDate(_ context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockTransactionRepository) GetSymbolsByOpenPositions(_ context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockTransactionRepository) GetFxPairsByOpenPositions(_ context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockTransactionRepository) GetEarliestDateBySymbol(_ context.Context, _ string) (*time.Time, error) {
	return nil, nil
}

type mockAccountChecker struct {
	mu       sync.RWMutex
	existing map[int64]bool
}

func newMockAccountChecker(ids ...int64) *mockAccountChecker {
	m := &mockAccountChecker{
		existing: make(map[int64]bool),
	}
	for _, id := range ids {
		m.existing[id] = true
	}
	return m
}

func (m *mockAccountChecker) AccountExists(_ context.Context, id int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.existing[id]
}

type mockPortfolioChecker struct {
	mu       sync.RWMutex
	existing map[int64]bool
}

func newMockPortfolioChecker(ids ...int64) *mockPortfolioChecker {
	m := &mockPortfolioChecker{
		existing: make(map[int64]bool),
	}
	for _, id := range ids {
		m.existing[id] = true
	}
	return m
}

func (m *mockPortfolioChecker) PortfolioExists(_ context.Context, id int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.existing[id]
}

type mockAccountLister struct {
	allAccounts         []AccountRef
	accountsByPortfolio map[int64][]AccountRef
	err                 error
}

func newMockAccountLister() *mockAccountLister {
	return &mockAccountLister{
		allAccounts:         []AccountRef{},
		accountsByPortfolio: make(map[int64][]AccountRef),
	}
}

func (m *mockAccountLister) SetAllAccounts(accounts []AccountRef) {
	m.allAccounts = accounts
}

func (m *mockAccountLister) SetAccountsByPortfolio(portfolioID int64, accounts []AccountRef) {
	m.accountsByPortfolio[portfolioID] = accounts
}

// mockPortfolioCurrencyChecker returns portfolio base currencies.
type mockPortfolioCurrencyChecker struct {
	currencies map[int64]string
}

func newMockPortfolioCurrencyChecker(currencies map[int64]string) *mockPortfolioCurrencyChecker {
	if currencies == nil {
		currencies = make(map[int64]string)
	}
	return &mockPortfolioCurrencyChecker{currencies: currencies}
}

func (m *mockPortfolioCurrencyChecker) GetPortfolioCurrency(_ context.Context, portfolioID int64) (string, error) {
	if c, ok := m.currencies[portfolioID]; ok {
		return c, nil
	}
	return "", nil
}

func (m *mockAccountLister) GetAllAccounts(_ context.Context) ([]AccountRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]AccountRef, len(m.allAccounts))
	copy(result, m.allAccounts)
	return result, nil
}

func (m *mockAccountLister) GetAccountsByPortfolio(_ context.Context, portfolioID int64) ([]AccountRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	accounts, ok := m.accountsByPortfolio[portfolioID]
	if !ok {
		return []AccountRef{}, nil
	}
	result := make([]AccountRef, len(accounts))
	copy(result, accounts)
	return result, nil
}

// --- Test helpers ---

func now() time.Time {
	return time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
}

var lotCounter int64

func nextLotID() string {
	lotCounter++
	return fmt.Sprintf("LOT-%06d", lotCounter)
}

func makeBuyTxn(accountID int64, symbol string, date time.Time, qty, price int64) transaction.Transaction {
	return transaction.Transaction{
		AccountID: accountID,
		Date:      date,
		Type:      "buy",
		Symbol:    symbol,
		Quantity:  decimal.MustNew(qty, 2),
		Price:     decimal.MustNew(price, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-qty*price, 2),
		LotID:     ptrStr(nextLotID()),
		CreatedAt: date,
		UpdatedAt: date,
	}
}

func makeSellTxn(accountID int64, symbol string, date time.Time, qty, price int64) transaction.Transaction {
	return transaction.Transaction{
		AccountID: accountID,
		Date:      date,
		Type:      "sell",
		Symbol:    symbol,
		Quantity:  decimal.MustNew(qty, 2),
		Price:     decimal.MustNew(price, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(qty*price, 2),
		LotID:     ptrStr(nextLotID()),
		CreatedAt: date,
		UpdatedAt: date,
	}
}

// --- RecalculateAccount tests ---

func TestRecalculateAccount_AccountNotFound(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(), // no accounts
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	err := svc.RecalculateAccount(ctx, 999)
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestRecalculateAccount_EmptyTransactions(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()
	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	// No transactions for account 1.
	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	posRepo.mu.RLock()
	defer posRepo.mu.RUnlock()
	if len(posRepo.positions) != 0 {
		t.Errorf("expected no positions, got %d", len(posRepo.positions))
	}
}

func TestRecalculateAccount_SimpleBuy(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()

	// Single buy transaction.
	buy := makeBuyTxn(1, "AAPL", now(), 1000, 15000)
	txnRepo.SetTransactions(1, []transaction.Transaction{buy})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	posRepo.mu.RLock()
	defer posRepo.mu.RUnlock()

	// Should have 2 positions: 1 open (AAPL) + 1 cash.
	if len(posRepo.positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(posRepo.positions))
	}

	// Find the AAPL open position.
	var foundAAPL bool
	for _, p := range posRepo.positions {
		if p.Symbol == "AAPL" && !p.IsClosed {
			foundAAPL = true
			if !p.Quantity.Equal(decimal.MustNew(1000, 2)) {
				t.Errorf("expected quantity 10.00, got %s", p.Quantity.String())
			}
		}
	}
	if !foundAAPL {
		t.Error("expected AAPL open position not found")
	}

	// Should have 1 lot.
	if len(posRepo.lots) != 1 {
		t.Fatalf("expected 1 lot, got %d", len(posRepo.lots))
	}
	if posRepo.lots[0].LotType != "buy" {
		t.Errorf("expected buy lot, got %s", posRepo.lots[0].LotType)
	}
}

func TestRecalculateAccount_BuyThenSell(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()

	buy := makeBuyTxn(1, "AAPL", now(), 1000, 15000)
	sell := makeSellTxn(1, "AAPL", now().AddDate(0, 0, 1), 1000, 16000)
	txnRepo.SetTransactions(1, []transaction.Transaction{buy, sell})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	posRepo.mu.RLock()
	defer posRepo.mu.RUnlock()

	// Should have positions (open AAPL + cash).
	if len(posRepo.positions) < 1 {
		t.Errorf("expected at least 1 position, got %d", len(posRepo.positions))
	}

	// Should have 2 lots (1 buy, 1 sell).
	if len(posRepo.lots) != 2 {
		t.Errorf("expected 2 lots, got %d", len(posRepo.lots))
	}

	// Should have 1 consumption (sell consumes buy via FIFO).
	if len(posRepo.consumptions) != 1 {
		t.Errorf("expected 1 consumption, got %d", len(posRepo.consumptions))
	}
	if len(posRepo.consumptions) > 0 {
		c := posRepo.consumptions[0]
		if !c.QuantityConsumed.Equal(decimal.MustNew(1000, 2)) {
			t.Errorf("expected consumed qty 10.00, got %s", c.QuantityConsumed.String())
		}
	}
}

// --- GetOpenPositions tests ---

func TestGetOpenPositions_NoAccounts(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	result, err := svc.GetOpenPositions(ctx, []int64{}, 10, 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d positions", len(result))
	}
}

func TestGetOpenPositions_SingleAccount(t *testing.T) {
	posRepo := newMockPositionRepository()

	// Seed an open position.
	posRepo.positions = []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), IsClosed: false, OpenDate: now()},
		{ID: 2, AccountID: 1, Symbol: "MSFT", Quantity: decimal.MustNew(500, 2), IsClosed: false, OpenDate: now().AddDate(0, 0, 1)},
	}

	svc := NewService(
		posRepo,
		newMockTransactionRepository(),
		newMockAccountChecker(1),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	result, err := svc.GetOpenPositions(ctx, []int64{1}, 10, 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(result))
	}

	// Should be sorted by symbol.
	if result[0].Symbol != "AAPL" {
		t.Errorf("expected first position AAPL, got %s", result[0].Symbol)
	}
	if result[1].Symbol != "MSFT" {
		t.Errorf("expected second position MSFT, got %s", result[1].Symbol)
	}
}

func TestGetOpenPositions_Pagination(t *testing.T) {
	posRepo := newMockPositionRepository()

	// Seed 5 open positions.
	for i := 0; i < 5; i++ {
		posRepo.positions = append(posRepo.positions, Position{
			ID: int64(i + 1), AccountID: 1, Symbol: fmt.Sprintf("SYM%02d", i),
			Quantity: decimal.MustNew(1000, 2), IsClosed: false, OpenDate: now(),
		})
	}

	svc := NewService(
		posRepo,
		newMockTransactionRepository(),
		newMockAccountChecker(1),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	// Page 1: limit 2, offset 0.
	result, err := svc.GetOpenPositions(ctx, []int64{1}, 2, 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 positions, got %d", len(result))
	}

	// Page 2: limit 2, offset 2.
	result, err = svc.GetOpenPositions(ctx, []int64{1}, 2, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 positions, got %d", len(result))
	}

	// Page 3: limit 2, offset 4.
	result, err = svc.GetOpenPositions(ctx, []int64{1}, 2, 4)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 position, got %d", len(result))
	}

	// Page 4: limit 2, offset 6 (beyond data).
	result, err = svc.GetOpenPositions(ctx, []int64{1}, 2, 6)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 positions, got %d", len(result))
	}
}

// --- RecalculatePortfolio tests ---

func TestRecalculatePortfolio_PortfolioNotFound(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(), // no portfolios
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	err := svc.RecalculatePortfolio(ctx, 999)
	if !errors.Is(err, ErrPortfolioNotFound) {
		t.Fatalf("expected ErrPortfolioNotFound, got %v", err)
	}
}

// --- RecalculateAll tests ---

func TestRecalculateAll_NoAccounts(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	err := svc.RecalculateAll(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// --- GetLotDetails tests ---

func TestGetLotDetails_NotFound(t *testing.T) {
	posRepo := newMockPositionRepository()
	svc := NewService(
		posRepo,
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	_, err := svc.GetLotDetails(ctx, "nonexistent")
	if !errors.Is(err, ErrLotNotFound) {
		t.Fatalf("expected ErrLotNotFound, got %v", err)
	}
}

func TestGetLotDetails_WithConsumptions(t *testing.T) {
	posRepo := newMockPositionRepository()

	// Seed a sell lot and consumptions.
	lot := Lot{
		ID: 1, LotID: "LOT-TEST", AccountID: 1, Symbol: "AAPL",
		LotType: "sell", Quantity: decimal.MustNew(1000, 2),
		CostBasis:   decimal.MustNew(0, 2),
		SellPrice:   ptrDecimal(decimal.MustNew(16000, 2)),
		RealizedPnL: decimal.MustNew(10000, 2),
		OpenDate:    now(), CreatedAt: now(), UpdatedAt: now(),
	}
	posRepo.lots = []Lot{lot}
	posRepo.consumptions = []LotConsumption{
		{ID: 1, SellLotID: "LOT-TEST", BuyLotID: "LOT-BUY1", QuantityConsumed: decimal.MustNew(1000, 2)},
	}

	svc := NewService(
		posRepo,
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	details, err := svc.GetLotDetails(ctx, "LOT-TEST")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if details.LotID != "LOT-TEST" {
		t.Errorf("expected lot LOT-TEST, got %s", details.LotID)
	}
	if len(details.Consumptions) != 1 {
		t.Errorf("expected 1 consumption, got %d", len(details.Consumptions))
	}
}

// --- GetLotInfo tests ---

func TestGetLotInfo(t *testing.T) {
	posRepo := newMockPositionRepository()
	posRepo.lots = []Lot{
		{ID: 1, LotID: "LOT-BUY1", AccountID: 1, Symbol: "AAPL", LotType: "buy"},
	}

	svc := NewService(
		posRepo,
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, // no portfolio currency checker
	)

	info, err := svc.GetLotInfo(ctx, "LOT-BUY1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.AccountID != 1 {
		t.Errorf("expected account 1, got %d", info.AccountID)
	}
	if info.Symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got %s", info.Symbol)
	}
	if info.LotType != "buy" {
		t.Errorf("expected lot type buy, got %s", info.LotType)
	}
}

// --- Mocks for market data ---

// mockMarketDataService implements MarketDataService for tests.
type mockMarketDataService struct {
	quotes        map[string]*market.MarketData
	historical    map[string][]market.HistoricalPrice
	latestDates   map[string]*time.Time
	currentFx     map[string]*market.FxRate // key: "BASE/QUOTE"
	historicalFx  map[string]*market.FxRate // key: "BASE/QUOTE"
	refreshed     []string
	refreshFailed []string
}

func (m *mockMarketDataService) GetQuotes(_ context.Context, symbols []string) map[string]*market.MarketData {
	result := make(map[string]*market.MarketData)
	if m.quotes == nil {
		return result
	}
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			result[sym] = q
		}
	}
	return result
}

func (m *mockMarketDataService) GetHistoricalPrices(_ context.Context, symbol string, _, _ time.Time) ([]market.HistoricalPrice, error) {
	if m.historical == nil {
		return nil, nil
	}
	prices, ok := m.historical[symbol]
	if !ok {
		return nil, nil
	}
	return prices, nil
}

func (m *mockMarketDataService) GetLatestPriceDatePerSymbol(_ context.Context, symbols []string) map[string]*time.Time {
	result := make(map[string]*time.Time)
	if m.latestDates == nil {
		return result
	}
	for _, sym := range symbols {
		if t, ok := m.latestDates[sym]; ok {
			result[sym] = t
		}
	}
	return result
}

func (m *mockMarketDataService) RefreshQuotes(_ context.Context, symbols []string) marketservice.RefreshResult {
	// Simulate: all symbols succeed if quotes are available, fail otherwise.
	var refreshed, failed []string
	for _, sym := range symbols {
		if m.quotes != nil && m.quotes[sym] != nil {
			refreshed = append(refreshed, sym)
		} else {
			failed = append(failed, sym)
		}
	}
	m.refreshed = refreshed
	m.refreshFailed = failed
	return marketservice.RefreshResult{Refreshed: refreshed, Failed: failed}
}

func (m *mockMarketDataService) GetCurrentFxRate(_ context.Context, base, quote string) (*market.FxRate, error) {
	if m.currentFx == nil {
		return nil, nil
	}
	pair := market.FormatFxPair(base, quote)
	if rate, ok := m.currentFx[pair]; ok {
		return rate, nil
	}
	return nil, nil
}

func (m *mockMarketDataService) GetHistoricalFxRate(_ context.Context, base, quote string, _ time.Time) (*market.FxRate, error) {
	if m.historicalFx == nil {
		return nil, nil
	}
	pair := market.FormatFxPair(base, quote)
	if rate, ok := m.historicalFx[pair]; ok {
		return rate, nil
	}
	return nil, nil
}

func (m *mockMarketDataService) RefreshFxRates(_ context.Context, _ []marketservice.FxPair) marketservice.FxRefreshResult {
	return marketservice.FxRefreshResult{}
}

// mockCacheScheduler records scheduled fetches for verification in tests.
type mockCacheScheduler struct {
	mu            sync.Mutex
	symbolFetches []struct {
		symbol   string
		fromDate time.Time
	}
	fxPairFetches []struct {
		base, quote string
		fromDate    time.Time
	}
	refreshAllCalls int
}

func (m *mockCacheScheduler) ScheduleSymbolFetch(symbol string, fromDate time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.symbolFetches = append(m.symbolFetches, struct {
		symbol   string
		fromDate time.Time
	}{symbol, fromDate})
}

func (m *mockCacheScheduler) ScheduleFxPairFetch(base, quote string, fromDate time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fxPairFetches = append(m.fxPairFetches, struct {
		base, quote string
		fromDate    time.Time
	}{base, quote, fromDate})
}

func (m *mockCacheScheduler) RefreshAll(context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshAllCalls++
}

func (m *mockCacheScheduler) GetSymbolFetches() []struct {
	symbol   string
	fromDate time.Time
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]struct {
		symbol   string
		fromDate time.Time
	}, len(m.symbolFetches))
	copy(result, m.symbolFetches)
	return result
}

func (m *mockCacheScheduler) GetFxPairFetches() []struct {
	base, quote string
	fromDate    time.Time
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]struct {
		base, quote string
		fromDate    time.Time
	}, len(m.fxPairFetches))
	copy(result, m.fxPairFetches)
	return result
}

// --- EnrichWithMarketData tests ---

func TestEnrichWithMarketData_NoFetcher(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].MarketDataAvailable {
		t.Error("expected MarketDataAvailable=false without fetcher")
	}
	if result[0].MarketPrice != nil {
		t.Error("expected nil MarketPrice without fetcher")
	}
}

func TestEnrichWithMarketData_CashPosition(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	svc.WithMarketDataService(&mockMarketDataService{quotes: nil}, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "$CASH-USD", Quantity: decimal.MustNew(50000, 2), CostBasis: decimal.MustNew(0, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].MarketDataAvailable {
		t.Error("expected MarketDataAvailable=true for cash position")
	}
	// MarketValue should equal the balance (quantity).
	if !result[0].MarketValue.Equal(decimal.MustNew(50000, 2)) {
		t.Errorf("expected MarketValue 500.00, got %s", result[0].MarketValue.String())
	}
	// No market price for cash.
	if result[0].MarketPrice != nil {
		t.Error("expected nil MarketPrice for cash position")
	}
	// No unrealized P&L for cash.
	if !result[0].UnrealizedPnL.IsZero() {
		t.Errorf("expected zero UnrealizedPnL for cash, got %s", result[0].UnrealizedPnL.String())
	}
}

func TestEnrichWithMarketData_Success(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	// AAPL quote at 170.00 — pre-populated in cache.
	price := decimal.MustNew(17000, 2)
	svc.WithMarketDataService(&mockMarketDataService{quotes: map[string]*market.MarketData{
		"AAPL": {Symbol: "AAPL", Price: price, Currency: "USD", DataType: "stock", Source: "yahoo"},
	}}, nil)

	// Quantity 10.00, CostBasis -15000.00 (total cost of 15000.00)
	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	r := result[0]
	if !r.MarketDataAvailable {
		t.Error("expected MarketDataAvailable=true")
	}
	if r.MarketPrice == nil || !r.MarketPrice.Equal(price) {
		t.Errorf("expected MarketPrice 170.00, got %v", r.MarketPrice)
	}
	// MarketValue = 10.00 × 170.00 = 1700.00 (scale 4)
	wantMV := decimal.MustNew(17000000, 4)
	if !r.MarketValue.Equal(wantMV) {
		t.Errorf("expected MarketValue %s, got %s", wantMV.String(), r.MarketValue.String())
	}
	// UnrealizedPnL = 1700.00 + (-15000.00) = -13300.00 (scale 4)
	wantUP := decimal.MustNew(-133000000, 4)
	if !r.UnrealizedPnL.Equal(wantUP) {
		t.Errorf("expected UnrealizedPnL %s, got %s", wantUP.String(), r.UnrealizedPnL.String())
	}
	// P&L% = -13300 / 15000 * 100 = -88.67%
	if r.UnrealizedPnlPct == nil {
		t.Error("expected non-nil UnrealizedPnlPct")
	}
}

func TestEnrichWithMarketData_MissingCachedQuote(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	// Cache has no data for AAPL.
	svc.WithMarketDataService(&mockMarketDataService{quotes: nil}, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].MarketDataAvailable {
		t.Error("expected MarketDataAvailable=false when no cached quote")
	}
}

func TestEnrichWithMarketData_MultiplePositions(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	// Cache has AAPL but not MSFT.
	svc.WithMarketDataService(&mockMarketDataService{quotes: map[string]*market.MarketData{
		"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(17000, 2), Currency: "USD", DataType: "stock"},
	}}, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
		{ID: 2, AccountID: 1, Symbol: "MSFT", Quantity: decimal.MustNew(500, 2), CostBasis: decimal.MustNew(-3000000, 2)},
		{ID: 3, AccountID: 1, Symbol: "$CASH-USD", Quantity: decimal.MustNew(50000, 2), CostBasis: decimal.MustNew(0, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions, "")

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// AAPL: available
	if !result[0].MarketDataAvailable {
		t.Error("expected AAPL market data available")
	}
	// MSFT: unavailable (no cached quote)
	if result[1].MarketDataAvailable {
		t.Error("expected MSFT market data unavailable")
	}
	// Cash: available
	if !result[2].MarketDataAvailable {
		t.Error("expected cash market data available")
	}
}

func TestEnrichWithMarketData_GbpConversion(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	// UK stock priced in pence — the fetcher layer converts GBp→GBP,
	// so by the time the position service sees it, price is already in GBP.
	// ARCI.L at 3.50 GBP.
	gbpPrice := decimal.MustNew(350, 2) // 3.50 GBP (already converted from 350 GBp)
	svc.WithMarketDataService(&mockMarketDataService{quotes: map[string]*market.MarketData{
		"ARCI.L": {Symbol: "ARCI.L", Price: gbpPrice, Currency: "GBP", DataType: "stock", Source: "yahoo"},
	}}, nil)

	// Quantity 100, CostBasis -350.00 GBP (bought at 3.50 GBP/share).
	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "ARCI.L", Currency: "GBP",
			Quantity: decimal.MustNew(10000, 2), CostBasis: decimal.MustNew(-35000, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	r := result[0]
	if !r.MarketDataAvailable {
		t.Error("expected MarketDataAvailable=true")
	}
	// Price is already in GBP (fetcher converted GBp→GBP).
	wantPrice := decimal.MustNew(350, 2)
	if r.MarketPrice == nil || !r.MarketPrice.Equal(wantPrice) {
		t.Errorf("expected MarketPrice 3.50 (GBP), got %v", r.MarketPrice)
	}
	// MarketValue = 100 × 3.50 = 350.00.
	wantMV := decimal.MustNew(3500000, 4)
	if !r.MarketValue.Equal(wantMV) {
		t.Errorf("expected MarketValue %s, got %s", wantMV.String(), r.MarketValue.String())
	}
	// UnrealizedPnL = 350.00 + (-350.00) = 0.00.
	if !r.UnrealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected UnrealizedPnL 0.00, got %s", r.UnrealizedPnL.String())
	}
}

func TestGetClosedPositionsSummary_SumsAllPositions(t *testing.T) {
	repo := newMockPositionRepository()
	list := &mockAccountLister{
		allAccounts: []AccountRef{
			{ID: 1, Name: "Acc1", PortfolioID: 1, PortfolioCurrency: "USD"},
		},
	}
	svc := NewService(repo, newMockTransactionRepository(),
		newMockAccountChecker(), newMockPortfolioChecker(),
		list, nil)

	// Create 3 closed positions with known RealizedPnlBase.
	rpnl1 := decimal.MustNew(20000, 2) // 200.00
	rpnl2 := decimal.MustNew(-5000, 2) // -50.00
	rpnl3 := decimal.MustNew(30000, 2) // 300.00
	_ = repo.CreatePosition(ctx, &Position{
		AccountID: 1, Symbol: "AAPL", Currency: "USD", IsClosed: true,
		RealizedPnlBase: &rpnl1,
	})
	_ = repo.CreatePosition(ctx, &Position{
		AccountID: 1, Symbol: "MSFT", Currency: "USD", IsClosed: true,
		RealizedPnlBase: &rpnl2,
	})
	_ = repo.CreatePosition(ctx, &Position{
		AccountID: 1, Symbol: "GOOG", Currency: "USD", IsClosed: true,
		RealizedPnlBase: &rpnl3,
	})

	summary, err := svc.GetClosedPositionsSummary(ctx, ListFilters{}, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 200.00 + (-50.00) + 300.00 = 450.00
	want := decimal.MustNew(45000, 2)
	if !summary.TotalRealizedPnLB.Equal(want) {
		t.Errorf("expected TotalRealizedPnLB %s, got %s", want.String(), summary.TotalRealizedPnLB.String())
	}
}

func TestGetClosedPositionsSummary_PaginationDoesNotAffectSummary(t *testing.T) {
	repo := newMockPositionRepository()
	list := &mockAccountLister{
		allAccounts: []AccountRef{
			{ID: 1, Name: "Acc1", PortfolioID: 1, PortfolioCurrency: "USD"},
		},
	}
	svc := NewService(repo, newMockTransactionRepository(),
		newMockAccountChecker(), newMockPortfolioChecker(),
		list, nil)

	// Create 50 closed positions, each with RealizedPnlBase = 100.00
	for i := 0; i < 50; i++ {
		rpnl := decimal.MustNew(10000, 2)
		_ = repo.CreatePosition(ctx, &Position{
			AccountID: 1, Symbol: fmt.Sprintf("SYM%02d", i), Currency: "USD", IsClosed: true,
			RealizedPnlBase: &rpnl,
		})
	}

	// Fetch only 10 positions (page 1) — should NOT affect the summary.
	_, err := svc.GetClosedPositionsFiltered(ctx, ListFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Summary should sum ALL 50 positions, not just the 10 on the page.
	summary, err := svc.GetClosedPositionsSummary(ctx, ListFilters{}, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := decimal.MustNew(500000, 2) // 50 × 100.00 = 5000.00
	if !summary.TotalRealizedPnLB.Equal(want) {
		t.Errorf("expected TotalRealizedPnLB %s (all 50), got %s", want.String(), summary.TotalRealizedPnLB.String())
	}
}

func TestGetClosedPositionsSummary_FilterByAccount(t *testing.T) {
	repo := newMockPositionRepository()
	list := &mockAccountLister{
		allAccounts: []AccountRef{
			{ID: 1, Name: "Acc1", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Acc2", PortfolioID: 1, PortfolioCurrency: "USD"},
		},
	}
	svc := NewService(repo, newMockTransactionRepository(),
		newMockAccountChecker(), newMockPortfolioChecker(),
		list, nil)

	// Account 1: 200.00
	rpnl1 := decimal.MustNew(20000, 2)
	_ = repo.CreatePosition(ctx, &Position{
		AccountID: 1, Symbol: "AAPL", Currency: "USD", IsClosed: true,
		RealizedPnlBase: &rpnl1,
	})
	// Account 2: 300.00
	rpnl2 := decimal.MustNew(30000, 2)
	_ = repo.CreatePosition(ctx, &Position{
		AccountID: 2, Symbol: "MSFT", Currency: "USD", IsClosed: true,
		RealizedPnlBase: &rpnl2,
	})

	// Filter by account 1 only.
	accountID := int64(1)
	summary, err := svc.GetClosedPositionsSummary(ctx, ListFilters{AccountID: &accountID}, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := decimal.MustNew(20000, 2)
	if !summary.TotalRealizedPnLB.Equal(want) {
		t.Errorf("expected TotalRealizedPnLB %s (account 1 only), got %s", want.String(), summary.TotalRealizedPnLB.String())
	}
}

func TestGetOpenPositionsSummary_SumsAllPositions(t *testing.T) {
	repo := newMockPositionRepository()
	list := &mockAccountLister{
		allAccounts: []AccountRef{
			{ID: 1, Name: "Acc1", PortfolioID: 1, PortfolioCurrency: "USD"},
		},
	}
	svc := NewService(repo, newMockTransactionRepository(),
		newMockAccountChecker(), newMockPortfolioChecker(),
		list, nil)

	// Create 3 open positions with known values.
	// CostBasis is negative (cash outflow), so total cost = Abs(CostBasis).
	cb1 := decimal.MustNew(-1000000, 2) // -10000.00
	cb2 := decimal.MustNew(-2000000, 2) // -20000.00
	_ = repo.CreatePosition(ctx, &Position{
		AccountID: 1, Symbol: "AAPL", Currency: "USD", IsClosed: false,
		Quantity: decimal.MustNew(10000, 2), CostBasis: cb1,
	})
	_ = repo.CreatePosition(ctx, &Position{
		AccountID: 1, Symbol: "MSFT", Currency: "USD", IsClosed: false,
		Quantity: decimal.MustNew(10000, 2), CostBasis: cb2,
	})

	// Wire cached market data so enrichment works.
	svc.WithMarketDataService(&mockMarketDataService{quotes: map[string]*market.MarketData{
		"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(11000, 2), Currency: "USD"},
		"MSFT": {Symbol: "MSFT", Price: decimal.MustNew(22000, 2), Currency: "USD"},
	}}, nil)

	summary, err := svc.GetOpenPositionsSummary(ctx, ListFilters{}, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Cost basis: 10000.00 + 20000.00 = 30000.00
	wantCB := decimal.MustNew(3000000, 2)
	if !summary.TotalCostBasisBase.Equal(wantCB) {
		t.Errorf("expected TotalCostBasisBase %s, got %s", wantCB.String(), summary.TotalCostBasisBase.String())
	}
	// Market value: 100×110 + 100×220 = 11000 + 22000 = 33000.00
	wantMV := decimal.MustNew(3300000, 2)
	if !summary.TotalMktValueBase.Equal(wantMV) {
		t.Errorf("expected TotalMktValueBase %s, got %s", wantMV.String(), summary.TotalMktValueBase.String())
	}
	// Unrealized P&L: 33000 - 30000 = 3000.00
	wantPnL := decimal.MustNew(300000, 2)
	if !summary.TotalUnrealizedPnLB.Equal(wantPnL) {
		t.Errorf("expected TotalUnrealizedPnLB %s, got %s", wantPnL.String(), summary.TotalUnrealizedPnLB.String())
	}
}

func TestGetOpenPositionsSummary_PaginationDoesNotAffectSummary(t *testing.T) {
	repo := newMockPositionRepository()
	list := &mockAccountLister{
		allAccounts: []AccountRef{
			{ID: 1, Name: "Acc1", PortfolioID: 1, PortfolioCurrency: "USD"},
		},
	}
	svc := NewService(repo, newMockTransactionRepository(),
		newMockAccountChecker(), newMockPortfolioChecker(),
		list, nil)

	// Create 25 open positions, each with cost basis -1000.00
	for i := 0; i < 25; i++ {
		cb := decimal.MustNew(-100000, 2)
		_ = repo.CreatePosition(ctx, &Position{
			AccountID: 1, Symbol: fmt.Sprintf("SYM%02d", i), Currency: "USD", IsClosed: false,
			Quantity: decimal.MustNew(10000, 2), CostBasis: cb,
		})
	}

	// Wire cached market data with same price as cost basis (P&L = 0).
	cachedQuotes := make(map[string]*market.MarketData)
	for i := 0; i < 25; i++ {
		sym := fmt.Sprintf("SYM%02d", i)
		cachedQuotes[sym] = &market.MarketData{Symbol: sym, Price: decimal.MustNew(10000, 2), Currency: "USD"}
	}
	svc.WithMarketDataService(&mockMarketDataService{quotes: cachedQuotes}, nil)

	// Fetch only 10 positions (page 1) — should NOT affect the summary.
	_, err := svc.GetOpenPositionsFiltered(ctx, ListFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Summary should sum ALL 25 positions.
	summary, err := svc.GetOpenPositionsSummary(ctx, ListFilters{}, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantCB := decimal.MustNew(2500000, 2) // 25 × 1000.00
	if !summary.TotalCostBasisBase.Equal(wantCB) {
		t.Errorf("expected TotalCostBasisBase %s (all 25), got %s", wantCB.String(), summary.TotalCostBasisBase.String())
	}
}

// --- D8: UnrealizedPnlPct computation tests ---

func TestUnrealizedPnlPct_PositivePnl(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	// Bought 10 shares at $100, now at $120 → 20% gain.
	// CostBasis = -1000.00 (10 × $100, negative = cash outflow)
	// MarketValue = 10 × $120 = 1200.00
	// UnrealizedPnL = 1200.00 + (-1000.00) = 200.00
	// PnlPct = 200 / 1000 × 100 = 20.00%
	price := decimal.MustNew(12000, 2)
	svc.WithMarketDataService(&mockMarketDataService{quotes: map[string]*market.MarketData{
		"AAPL": {Symbol: "AAPL", Price: price, Currency: "USD"},
	}}, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-100000, 2)},
	}
	result := svc.EnrichWithMarketData(ctx, positions, "")

	if result[0].UnrealizedPnlPct == nil {
		t.Fatal("expected non-nil UnrealizedPnlPct")
	}
	want := decimal.MustNew(200000, 4) // 20.00% (scale 4 from computation)
	if !result[0].UnrealizedPnlPct.Equal(want) {
		t.Errorf("expected PnlPct %s, got %s", want.String(), result[0].UnrealizedPnlPct.String())
	}
}

func TestUnrealizedPnlPct_NegativePnl(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	// Bought 10 shares at $100, now at $80 → -20% loss.
	// CostBasis = -1000.00
	// MarketValue = 10 × $80 = 800.00
	// UnrealizedPnL = 800.00 + (-1000.00) = -200.00
	// PnlPct = -200 / 1000 × 100 = -20.00%
	price := decimal.MustNew(8000, 2)
	svc.WithMarketDataService(&mockMarketDataService{quotes: map[string]*market.MarketData{
		"AAPL": {Symbol: "AAPL", Price: price, Currency: "USD"},
	}}, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-100000, 2)},
	}
	result := svc.EnrichWithMarketData(ctx, positions, "")

	if result[0].UnrealizedPnlPct == nil {
		t.Fatal("expected non-nil UnrealizedPnlPct")
	}
	want := decimal.MustNew(-200000, 4) // -20.00%
	if !result[0].UnrealizedPnlPct.Equal(want) {
		t.Errorf("expected PnlPct %s, got %s", want.String(), result[0].UnrealizedPnlPct.String())
	}
}

func TestUnrealizedPnlPct_ZeroPnl(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	// Bought 10 shares at $100, now at $100 → 0% P&L.
	// CostBasis = -1000.00
	// MarketValue = 10 × $100 = 1000.00
	// UnrealizedPnL = 1000.00 + (-1000.00) = 0.00
	// PnlPct = 0 / 1000 × 100 = 0.00%
	price := decimal.MustNew(10000, 2)
	svc.WithMarketDataService(&mockMarketDataService{quotes: map[string]*market.MarketData{
		"AAPL": {Symbol: "AAPL", Price: price, Currency: "USD"},
	}}, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-100000, 2)},
	}
	result := svc.EnrichWithMarketData(ctx, positions, "")

	if result[0].UnrealizedPnlPct == nil {
		t.Fatal("expected non-nil UnrealizedPnlPct")
	}
	if !result[0].UnrealizedPnlPct.IsZero() {
		t.Errorf("expected PnlPct 0, got %s", result[0].UnrealizedPnlPct.String())
	}
}

// --- T2: Recalculate error propagation test ---

func TestRecalculateAccount_RepositoryError(t *testing.T) {
	posRepo := newMockPositionRepository()
	posRepo.recalculateErr = fmt.Errorf("disk full")
	txnRepo := newMockTransactionRepository()

	// Add a transaction so recalculate is triggered.
	txnRepo.SetTransactions(1, []transaction.Transaction{
		makeBuyTxn(1, "AAPL", now(), 1000, 15000),
	})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	err := svc.RecalculateAccount(ctx, 1)
	if err == nil {
		t.Fatal("expected error from repository, got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("expected error to contain 'disk full', got: %v", err)
	}
}

func TestRecalculateAccount_TransactionListError(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()
	txnRepo.listErr = fmt.Errorf("connection reset")

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil,
	)

	err := svc.RecalculateAccount(ctx, 1)
	if err == nil {
		t.Fatal("expected error from transaction list, got nil")
	}
	if !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("expected error to contain 'connection reset', got: %v", err)
	}
}

// --- RecalculateAccount cache scheduling tests ---

func TestRecalculateAccount_SchedulesSymbolFetch(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()

	buy := makeBuyTxn(1, "AAPL", now(), 1000, 15000)
	txnRepo.SetTransactions(1, []transaction.Transaction{buy})

	scheduler := &mockCacheScheduler{}

	accountLister := newMockAccountLister()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
	})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(1),
		accountLister,
		newMockPortfolioCurrencyChecker(map[int64]string{1: "USD"}),
	)
	svc.WithMarketCache(scheduler)

	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have scheduled a fetch for AAPL.
	fetches := scheduler.GetSymbolFetches()
	if len(fetches) != 1 {
		t.Fatalf("expected 1 symbol fetch, got %d", len(fetches))
	}
	if fetches[0].symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got %s", fetches[0].symbol)
	}
	if !fetches[0].fromDate.Equal(now()) {
		t.Errorf("expected fromDate %v, got %v", now(), fetches[0].fromDate)
	}

	// No FX pair fetches (USD position in USD portfolio).
	if len(scheduler.GetFxPairFetches()) != 0 {
		t.Errorf("expected 0 FX pair fetches, got %d", len(scheduler.GetFxPairFetches()))
	}
}

func TestRecalculateAccount_SchedulesFxPairFetch(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()

	// GBP buy transaction in a USD portfolio.
	gbpBuy := makeBuyTxn(1, "SHEL.L", now(), 2000, 2500)
	gbpBuy.Currency = "GBP"
	gbpBuy.NetCash = decimal.MustNew(-500000, 2)
	txnRepo.SetTransactions(1, []transaction.Transaction{gbpBuy})

	scheduler := &mockCacheScheduler{}

	accountLister := newMockAccountLister()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
	})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(1),
		accountLister,
		newMockPortfolioCurrencyChecker(map[int64]string{1: "USD"}),
	)
	svc.WithMarketCache(scheduler)

	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have scheduled a symbol fetch for SHEL.L.
	symbolFetches := scheduler.GetSymbolFetches()
	if len(symbolFetches) != 1 || symbolFetches[0].symbol != "SHEL.L" {
		t.Errorf("expected 1 symbol fetch for SHEL.L, got %d: %v", len(symbolFetches), symbolFetches)
	}

	// Should have scheduled an FX pair fetch for GBP/USD.
	fxFetches := scheduler.GetFxPairFetches()
	if len(fxFetches) != 1 {
		t.Fatalf("expected 1 FX pair fetch, got %d", len(fxFetches))
	}
	if fxFetches[0].base != "GBP" || fxFetches[0].quote != "USD" {
		t.Errorf("expected GBP/USD, got %s/%s", fxFetches[0].base, fxFetches[0].quote)
	}
}

func TestRecalculateAccount_SkipsCashSymbols(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()

	// Cash deposit only — no stock symbols.
	cashTxn := transaction.Transaction{
		AccountID: 1,
		Date:      now(),
		Type:      "deposit",
		Symbol:    "$CASH-USD",
		Quantity:  decimal.MustNew(100000, 2),
		Price:     decimal.MustNew(100, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(100000, 2),
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	txnRepo.SetTransactions(1, []transaction.Transaction{cashTxn})

	scheduler := &mockCacheScheduler{}

	accountLister := newMockAccountLister()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
	})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(1),
		accountLister,
		newMockPortfolioCurrencyChecker(map[int64]string{1: "USD"}),
	)
	svc.WithMarketCache(scheduler)

	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// No symbol fetches (only cash position).
	if len(scheduler.GetSymbolFetches()) != 0 {
		t.Errorf("expected 0 symbol fetches, got %d", len(scheduler.GetSymbolFetches()))
	}
	if len(scheduler.GetFxPairFetches()) != 0 {
		t.Errorf("expected 0 FX pair fetches, got %d", len(scheduler.GetFxPairFetches()))
	}
}

func TestRecalculateAccount_NoScheduler(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()

	buy := makeBuyTxn(1, "AAPL", now(), 1000, 15000)
	txnRepo.SetTransactions(1, []transaction.Transaction{buy})

	accountLister := newMockAccountLister()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
	})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(1),
		accountLister,
		newMockPortfolioCurrencyChecker(map[int64]string{1: "USD"}),
	)
	// No WithMarketCache call — scheduler is nil.

	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Recalculation should succeed even without a scheduler.
	posRepo.mu.RLock()
	defer posRepo.mu.RUnlock()
	if len(posRepo.positions) == 0 {
		t.Error("expected positions to be calculated")
	}
}

func TestRecalculateAccount_MultipleSymbols(t *testing.T) {
	posRepo := newMockPositionRepository()
	txnRepo := newMockTransactionRepository()

	// Two different symbols.
	aaplBuy := makeBuyTxn(1, "AAPL", now(), 1000, 15000)
	googlBuy := makeBuyTxn(1, "GOOGL", now().AddDate(0, 0, 1), 500, 14000)
	txnRepo.SetTransactions(1, []transaction.Transaction{aaplBuy, googlBuy})

	scheduler := &mockCacheScheduler{}

	accountLister := newMockAccountLister()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
	})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(1),
		accountLister,
		newMockPortfolioCurrencyChecker(map[int64]string{1: "USD"}),
	)
	svc.WithMarketCache(scheduler)

	err := svc.RecalculateAccount(ctx, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have scheduled fetches for both symbols.
	fetches := scheduler.GetSymbolFetches()
	if len(fetches) != 2 {
		t.Fatalf("expected 2 symbol fetches, got %d", len(fetches))
	}

	// Check both symbols are present.
	symbols := make(map[string]bool)
	for _, f := range fetches {
		symbols[f.symbol] = true
	}
	if !symbols["AAPL"] || !symbols["GOOGL"] {
		t.Errorf("expected AAPL and GOOGL, got %v", symbols)
	}
}

func TestRecalculateAccount_SchedulesAfterRecalcError_NoSchedule(t *testing.T) {
	posRepo := newMockPositionRepository()
	posRepo.recalculateErr = fmt.Errorf("db error")
	txnRepo := newMockTransactionRepository()

	buy := makeBuyTxn(1, "AAPL", now(), 1000, 15000)
	txnRepo.SetTransactions(1, []transaction.Transaction{buy})

	scheduler := &mockCacheScheduler{}

	accountLister := newMockAccountLister()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, Name: "Broker", PortfolioID: 1, PortfolioCurrency: "USD"},
	})

	svc := NewService(
		posRepo,
		txnRepo,
		newMockAccountChecker(1),
		newMockPortfolioChecker(1),
		accountLister,
		newMockPortfolioCurrencyChecker(map[int64]string{1: "USD"}),
	)
	svc.WithMarketCache(scheduler)

	err := svc.RecalculateAccount(ctx, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should NOT have scheduled any fetches because recalc failed.
	if len(scheduler.GetSymbolFetches()) != 0 {
		t.Errorf("expected 0 symbol fetches after recalc error, got %d", len(scheduler.GetSymbolFetches()))
	}
}

// --- sortPositions tests ---

func TestSortPositions_CloseDateTieBreak(t *testing.T) {
	// Same symbol, same open date — two sell lots closed on different
	// dates. The earlier close date must sort first.
	closed := []Position{
		{ID: 1, Symbol: "VWRP.L", OpenDate: mustTime("2024-07-01"), CloseDate: ptrTime(mustTime("2025-07-15")), IsClosed: true},
		{ID: 2, Symbol: "VWRP.L", OpenDate: mustTime("2024-07-01"), CloseDate: ptrTime(mustTime("2025-02-01")), IsClosed: true},
		{ID: 3, Symbol: "VWRP.L", OpenDate: mustTime("2024-07-01"), CloseDate: ptrTime(mustTime("2025-05-10")), IsClosed: true},
	}

	sortPositions(closed)

	wantIDs := []int64{2, 3, 1} // close date ASC: 2025-02-01, 2025-05-10, 2025-07-15
	for i, p := range closed {
		if p.ID != wantIDs[i] {
			t.Errorf("position %d: expected ID %d, got ID %d", i, wantIDs[i], p.ID)
		}
	}
}

func TestSortPositions_SymbolAndOpenDateStillPrimary(t *testing.T) {
	// Close date must not override symbol or open-date ordering.
	closed := []Position{
		{ID: 1, Symbol: "BAA", OpenDate: mustTime("2024-01-01"), CloseDate: ptrTime(mustTime("2024-06-01"))},
		{ID: 2, Symbol: "AAA", OpenDate: mustTime("2024-03-01"), CloseDate: ptrTime(mustTime("2024-01-01"))},
		{ID: 3, Symbol: "AAA", OpenDate: mustTime("2024-02-01"), CloseDate: ptrTime(mustTime("2024-12-01"))},
	}

	sortPositions(closed)

	// Expected order: AAA 2024-02-01 (ID 3), AAA 2024-03-01 (ID 2), BAA (ID 1).
	wantIDs := []int64{3, 2, 1}
	for i, p := range closed {
		if p.ID != wantIDs[i] {
			t.Errorf("position %d: expected ID %d, got ID %d", i, wantIDs[i], p.ID)
		}
	}
}

func TestSortPositions_OpenPositionsUnaffectedByNilCloseDate(t *testing.T) {
	open := []Position{
		{ID: 1, Symbol: "BBB", OpenDate: mustTime("2024-01-01")},
		{ID: 2, Symbol: "AAA", OpenDate: mustTime("2024-05-01")},
	}

	sortPositions(open)

	wantIDs := []int64{2, 1}
	for i, p := range open {
		if p.ID != wantIDs[i] {
			t.Errorf("position %d: expected ID %d, got ID %d", i, wantIDs[i], p.ID)
		}
	}
}
