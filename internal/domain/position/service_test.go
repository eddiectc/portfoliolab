package position

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/govalues/decimal"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
	"github.com/arch-portfolio-lab/portfoliolab/internal/market"
)

// ctx is a test context.
var ctx = context.Background()

// --- Mocks ---

type mockPositionRepository struct {
	mu              sync.RWMutex
	positions       []Position
	lots            []Lot
	consumptions    []LotConsumption
	deleteErr       error
	createPosErr    error
	createLotErr    error
	createConsumeErr error
	getOpenErr      error
	getClosedErr    error
	getLotErr       error
	getConsumeErr   error
	recalculateErr  error
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		nil, // no FX provider
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
		CostBasis: decimal.MustNew(0, 2),
		SellPrice: ptrDecimal(decimal.MustNew(16000, 2)),
		RealizedPnL: decimal.MustNew(10000, 2),
		OpenDate: now(), CreatedAt: now(), UpdatedAt: now(),
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
		nil, // no FX provider
	)

	details, err := svc.GetLotDetails(ctx, "LOT-TEST")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if details.Lot.LotID != "LOT-TEST" {
		t.Errorf("expected lot LOT-TEST, got %s", details.Lot.LotID)
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
		nil, // no FX provider
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

type mockMarketDataFetcher struct {
	quotes map[string]*market.MarketData
	err    error
}

func (m *mockMarketDataFetcher) FetchQuote(_ context.Context, symbol string) (*market.MarketData, error) {
	if m.err != nil {
		return nil, m.err
	}
	if q, ok := m.quotes[symbol]; ok {
		return q, nil
	}
	return nil, fmt.Errorf("no quote for %s", symbol)
}

func (m *mockMarketDataFetcher) FetchFxRate(_ context.Context, _ string) (*market.MarketData, error) {
	return nil, fmt.Errorf("not implemented")
}

type mockMarketDataRepo struct {
	upserted []*market.MarketData
	upsertErr error
}

func (m *mockMarketDataRepo) GetLatest(_ context.Context, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockMarketDataRepo) GetBySourceAndDate(_ context.Context, _, _, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockMarketDataRepo) Upsert(_ context.Context, md *market.MarketData) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserted = append(m.upserted, md)
	return nil
}

func (m *mockMarketDataRepo) GetCurrentFxRate(_ context.Context, _ string) (*market.MarketData, error) {
	return nil, nil
}

// --- EnrichWithMarketData tests ---

func TestEnrichWithMarketData_NoFetcher(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, nil,
	)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions)

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
		nil, nil,
	)

	fetcher := &mockMarketDataFetcher{quotes: map[string]*market.MarketData{}}
	repo := &mockMarketDataRepo{}
	svc.WithMarketDataFetcher(fetcher, repo, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "$CASH-USD", Quantity: decimal.MustNew(50000, 2), CostBasis: decimal.MustNew(0, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions)

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
	// No market price fetched for cash.
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
		nil, nil,
	)

	// AAPL quote at 170.00
	price := decimal.MustNew(17000, 2)
	fetcher := &mockMarketDataFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: price, Currency: "USD", DataType: "stock", Source: "yahoo"},
		},
	}
	repo := &mockMarketDataRepo{}
	svc.WithMarketDataFetcher(fetcher, repo, nil)

	// Quantity 10.00, CostBasis -15000.00 (total cost of 15000.00)
	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions)

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
	// Verify the quote was cached.
	if len(repo.upserted) != 1 {
		t.Errorf("expected 1 cached quote, got %d", len(repo.upserted))
	}
}

func TestEnrichWithMarketData_FetchError(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, nil,
	)

	fetcher := &mockMarketDataFetcher{err: fmt.Errorf("network error")}
	repo := &mockMarketDataRepo{}
	svc.WithMarketDataFetcher(fetcher, repo, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].MarketDataAvailable {
		t.Error("expected MarketDataAvailable=false after fetch error")
	}
}

func TestEnrichWithMarketData_MultiplePositions(t *testing.T) {
	svc := NewService(
		newMockPositionRepository(),
		newMockTransactionRepository(),
		newMockAccountChecker(),
		newMockPortfolioChecker(),
		newMockAccountLister(),
		nil, nil,
	)

	fetcher := &mockMarketDataFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(17000, 2), Currency: "USD", DataType: "stock"},
			// MSFT will fail (not in map)
		},
	}
	repo := &mockMarketDataRepo{}
	svc.WithMarketDataFetcher(fetcher, repo, nil)

	positions := []Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), CostBasis: decimal.MustNew(-1500000, 2)},
		{ID: 2, AccountID: 1, Symbol: "MSFT", Quantity: decimal.MustNew(500, 2), CostBasis: decimal.MustNew(-3000000, 2)},
		{ID: 3, AccountID: 1, Symbol: "$CASH-USD", Quantity: decimal.MustNew(50000, 2), CostBasis: decimal.MustNew(0, 2)},
	}

	result := svc.EnrichWithMarketData(ctx, positions)

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// AAPL: available
	if !result[0].MarketDataAvailable {
		t.Error("expected AAPL market data available")
	}
	// MSFT: unavailable (fetch error)
	if result[1].MarketDataAvailable {
		t.Error("expected MSFT market data unavailable")
	}
	// Cash: available
	if !result[2].MarketDataAvailable {
		t.Error("expected cash market data available")
	}
}
