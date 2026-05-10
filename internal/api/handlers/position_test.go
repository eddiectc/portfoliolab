package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// --- Mocks (mirroring service_test.go patterns) ---

type mockPosRepo struct {
	mu              sync.RWMutex
	positions       []position.Position
	lots            []position.Lot
	consumptions    []position.LotConsumption
	getOpenErr      error
	getClosedErr    error
	getLotErr       error
	getConsumeErr   error
	recalculateErr  error
}

func newMockPosRepo() *mockPosRepo {
	return &mockPosRepo{
		positions:    []position.Position{},
		lots:         []position.Lot{},
		consumptions: []position.LotConsumption{},
	}
}

func (m *mockPosRepo) CreatePosition(_ context.Context, _ *position.Position) error    { return nil }
func (m *mockPosRepo) CreateLot(_ context.Context, _ *position.Lot) error             { return nil }
func (m *mockPosRepo) CreateConsumption(_ context.Context, _ *position.LotConsumption) error { return nil }
func (m *mockPosRepo) DeleteAllForAccount(_ context.Context, _ int64) error           { return nil }
func (m *mockPosRepo) Recalculate(_ context.Context, _ int64, _ *position.CalculateResult) error {
	return m.recalculateErr
}

func (m *mockPosRepo) GetOpenPositions(_ context.Context, accountID int64, _limit, _offset int) ([]position.Position, error) {
	if m.getOpenErr != nil {
		return nil, m.getOpenErr
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

func (m *mockPosRepo) GetClosedPositions(_ context.Context, accountID int64, _limit, _offset int) ([]position.Position, error) {
	if m.getClosedErr != nil {
		return nil, m.getClosedErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []position.Position
	for _, p := range m.positions {
		if p.AccountID == accountID && p.IsClosed {
			result = append(result, p)
		}
	}
	if result == nil {
		result = []position.Position{}
	}
	return result, nil
}

func (m *mockPosRepo) GetLotByLotID(_ context.Context, lotID string) (*position.Lot, error) {
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
	return nil, position.ErrLotNotFound
}

func (m *mockPosRepo) GetConsumptionsBySellLot(_ context.Context, sellLotID string) ([]position.LotConsumption, error) {
	if m.getConsumeErr != nil {
		return nil, m.getConsumeErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []position.LotConsumption
	for _, c := range m.consumptions {
		if c.SellLotID == sellLotID {
			result = append(result, c)
		}
	}
	if result == nil {
		result = []position.LotConsumption{}
	}
	return result, nil
}

type mockTxnRepo struct {
	byAccount map[int64][]transaction.Transaction
	listErr   error
}

func newMockTxnRepo() *mockTxnRepo {
	return &mockTxnRepo{byAccount: make(map[int64][]transaction.Transaction)}
}

func (m *mockTxnRepo) ListAllTransactionsByAccount(_ context.Context, accountID int64) ([]transaction.Transaction, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	txns, ok := m.byAccount[accountID]
	if !ok {
		return []transaction.Transaction{}, nil
	}
	result := make([]transaction.Transaction, len(txns))
	copy(result, txns)
	return result, nil
}

type mockPosAccountChecker struct {
	existing map[int64]bool
}

func newMockPosAccountChecker(ids ...int64) *mockPosAccountChecker {
	m := &mockPosAccountChecker{existing: make(map[int64]bool)}
	for _, id := range ids {
		m.existing[id] = true
	}
	return m
}

func (m *mockPosAccountChecker) AccountExists(_ context.Context, id int64) bool {
	return m.existing[id]
}

type mockPosPortfolioChecker struct {
	existing map[int64]bool
}

func newMockPosPortfolioChecker(ids ...int64) *mockPosPortfolioChecker {
	m := &mockPosPortfolioChecker{existing: make(map[int64]bool)}
	for _, id := range ids {
		m.existing[id] = true
	}
	return m
}

func (m *mockPosPortfolioChecker) PortfolioExists(_ context.Context, id int64) bool {
	return m.existing[id]
}

type mockPosAccountLister struct {
	allAccounts         []position.AccountRef
	accountsByPortfolio map[int64][]position.AccountRef
}

func newMockPosAccountLister() *mockPosAccountLister {
	return &mockPosAccountLister{
		allAccounts:         []position.AccountRef{},
		accountsByPortfolio: make(map[int64][]position.AccountRef),
	}
}

func (m *mockPosAccountLister) GetAllAccounts(_ context.Context) ([]position.AccountRef, error) {
	result := make([]position.AccountRef, len(m.allAccounts))
	copy(result, m.allAccounts)
	return result, nil
}

func (m *mockPosAccountLister) GetAccountsByPortfolio(_ context.Context, portfolioID int64) ([]position.AccountRef, error) {
	accounts, ok := m.accountsByPortfolio[portfolioID]
	if !ok {
		return []position.AccountRef{}, nil
	}
	result := make([]position.AccountRef, len(accounts))
	copy(result, accounts)
	return result, nil
}

// --- Setup ---

func setupPositionHandler(t *testing.T, accountIDs []int64, portfolioIDs []int64) (*PositionHandler, *mockPosRepo, *mockPosAccountLister) {
	t.Helper()
	posRepo := newMockPosRepo()
	txnRepo := newMockTxnRepo()
	accounts := newMockPosAccountChecker(accountIDs...)
	portfolios := newMockPosPortfolioChecker(portfolioIDs...)
	accountLister := newMockPosAccountLister()
	accountLister.allAccounts = make([]position.AccountRef, len(accountIDs))
	for i, id := range accountIDs {
		accountLister.allAccounts[i] = position.AccountRef{ID: id, Name: "Account " + string(rune('A'+i)), PortfolioID: 1}
	}
	svc := position.NewService(posRepo, txnRepo, accounts, portfolios, accountLister, nil)
	return NewPositionHandler(svc), posRepo, accountLister
}

func setupPositionRouter(t *testing.T, accountIDs []int64, portfolioIDs []int64) (*chi.Mux, *mockPosRepo) {
	t.Helper()
	r := chi.NewRouter()
	h, posRepo := setupPositionHandlerForRouter(t, accountIDs, portfolioIDs, r)
	_ = h
	return r, posRepo
}

func setupPositionHandlerForRouter(t *testing.T, accountIDs []int64, portfolioIDs []int64, r *chi.Mux) (*PositionHandler, *mockPosRepo) {
	t.Helper()
	posRepo := newMockPosRepo()
	txnRepo := newMockTxnRepo()
	accounts := newMockPosAccountChecker(accountIDs...)
	portfolios := newMockPosPortfolioChecker(portfolioIDs...)
	accountLister := newMockPosAccountLister()
	accountLister.allAccounts = make([]position.AccountRef, len(accountIDs))
	for i, id := range accountIDs {
		accountLister.allAccounts[i] = position.AccountRef{ID: id, Name: "Account " + string(rune('A'+i)), PortfolioID: 1}
	}
	svc := position.NewService(posRepo, txnRepo, accounts, portfolios, accountLister, nil)
	h := NewPositionHandler(svc)
	h.RegisterRoutes(r)
	return h, posRepo
}

// --- HandleListOpen Tests ---

func TestPosHandleListOpen_Success(t *testing.T) {
	handler, posRepo, _ := setupPositionHandler(t, []int64{1}, []int64{})
	posRepo.positions = []position.Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), IsClosed: false},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()

	handler.HandleListOpen(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Returns PositionWithMarket (enriched with market data fields).
	var items []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Errorf("expected 1 position, got %d", len(items))
	}
	if len(items) > 0 && items[0].Symbol != "AAPL" {
		t.Errorf("expected symbol 'AAPL', got %q", items[0].Symbol)
	}
	// Without market fetcher configured, market data should be unavailable.
	if len(items) > 0 && items[0].MarketDataAvailable {
		t.Error("expected MarketDataAvailable=false without fetcher configured")
	}
}

func TestPosHandleListOpen_Empty(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()

	handler.HandleListOpen(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var items []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&items)
	if items == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestPosHandleListOpen_WithAccountFilter(t *testing.T) {
	handler, posRepo, _ := setupPositionHandler(t, []int64{1, 2}, []int64{})
	posRepo.positions = []position.Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), IsClosed: false},
		{ID: 2, AccountID: 2, Symbol: "MSFT", Quantity: decimal.MustNew(500, 2), IsClosed: false},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/positions?account_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleListOpen(w, req)

	var items []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Errorf("expected 1 position for account_id=1, got %d", len(items))
	}
	if len(items) > 0 && items[0].Symbol != "AAPL" {
		t.Errorf("expected symbol 'AAPL', got %q", items[0].Symbol)
	}
}

func TestPosHandleListOpen_Pagination(t *testing.T) {
	handler, posRepo, _ := setupPositionHandler(t, []int64{1}, []int64{})
	for i := 0; i < 5; i++ {
		posRepo.positions = append(posRepo.positions, position.Position{
			ID: int64(i + 1), AccountID: 1, Symbol: "SYM",
			Quantity: decimal.MustNew(1000, 2), IsClosed: false,
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/positions?limit=2", nil)
	w := httptest.NewRecorder()

	handler.HandleListOpen(w, req)

	var items []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 items with limit=2, got %d", len(items))
	}
}

// --- HandleListClosed Tests ---

func TestPosHandleListClosed_Success(t *testing.T) {
	handler, posRepo, _ := setupPositionHandler(t, []int64{1}, []int64{})
	posRepo.positions = []position.Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), IsClosed: true},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/positions/closed", nil)
	w := httptest.NewRecorder()

	handler.HandleListClosed(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var items []position.Position
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Errorf("expected 1 position, got %d", len(items))
	}
}

func TestPosHandleListClosed_Empty(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodGet, "/api/positions/closed", nil)
	w := httptest.NewRecorder()

	handler.HandleListClosed(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var items []position.Position
	json.NewDecoder(w.Body).Decode(&items)
	if items == nil {
		t.Error("expected empty array, got nil")
	}
}

// --- HandleGetLot Tests ---

func TestPosHandleGetLot_Success(t *testing.T) {
	r, posRepo := setupPositionRouter(t, []int64{1}, []int64{})
	posRepo.lots = []position.Lot{
		{ID: 1, LotID: "LOT-TEST", AccountID: 1, Symbol: "AAPL", LotType: "buy", Quantity: decimal.MustNew(1000, 2)},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lots/LOT-TEST", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var details position.LotWithDetails
	json.NewDecoder(w.Body).Decode(&details)
	if details.LotID != "LOT-TEST" {
		t.Errorf("expected lot 'LOT-TEST', got %q", details.LotID)
	}
}

func TestPosHandleGetLot_NotFound(t *testing.T) {
	r, _ := setupPositionRouter(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodGet, "/api/lots/LOT-NONEXIST", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "LOT_NOT_FOUND" {
		t.Errorf("expected LOT_NOT_FOUND, got %q", errResp.Code)
	}
}

func TestPosHandleGetLot_EmptyID(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodGet, "/api/lots/", nil)
	w := httptest.NewRecorder()

	handler.HandleGetLot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- HandleRecalculate Tests ---

func TestPosHandleRecalculate_Account(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate?account_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleRecalculate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "recalculated" {
		t.Errorf("expected status 'recalculated', got %q", resp["status"])
	}
}

func TestPosHandleRecalculate_AccountNotFound(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate?account_id=999", nil)
	w := httptest.NewRecorder()

	handler.HandleRecalculate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "ACCOUNT_NOT_FOUND" {
		t.Errorf("expected ACCOUNT_NOT_FOUND, got %q", errResp.Code)
	}
}

func TestPosHandleRecalculate_Portfolio(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{1})

	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleRecalculate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["portfolio_id"] != "1" {
		t.Errorf("expected portfolio_id '1', got %q", resp["portfolio_id"])
	}
}

func TestPosHandleRecalculate_PortfolioNotFound(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate?portfolio_id=999", nil)
	w := httptest.NewRecorder()

	handler.HandleRecalculate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "PORTFOLIO_NOT_FOUND" {
		t.Errorf("expected PORTFOLIO_NOT_FOUND, got %q", errResp.Code)
	}
}

func TestPosHandleRecalculate_All(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate", nil)
	w := httptest.NewRecorder()

	handler.HandleRecalculate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["scope"] != "all" {
		t.Errorf("expected scope 'all', got %q", resp["scope"])
	}
}

func TestPosHandleRecalculate_InvalidAccountID(t *testing.T) {
	handler, _, _ := setupPositionHandler(t, []int64{1}, []int64{})

	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate?account_id=abc", nil)
	w := httptest.NewRecorder()

	handler.HandleRecalculate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Route Registration Tests ---

func TestPosRoutesRegistered(t *testing.T) {
	r, posRepo := setupPositionRouter(t, []int64{1}, []int64{})
	posRepo.positions = []position.Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), IsClosed: false},
	}

	tests := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/positions"},
		{http.MethodGet, "/api/positions/closed"},
		{http.MethodPost, "/api/positions/recalculate"},
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

func TestPosErrorResponseFormat(t *testing.T) {
	r, _ := setupPositionRouter(t, []int64{1}, []int64{})

	// Test LOT_NOT_FOUND
	req := httptest.NewRequest(http.MethodGet, "/api/lots/LOT-NONEXIST", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "LOT_NOT_FOUND" {
		t.Errorf("expected LOT_NOT_FOUND, got %q", errResp.Code)
	}

	// Test ACCOUNT_NOT_FOUND
	req = httptest.NewRequest(http.MethodPost, "/api/positions/recalculate?account_id=999", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "ACCOUNT_NOT_FOUND" {
		t.Errorf("expected ACCOUNT_NOT_FOUND, got %q", errResp.Code)
	}

	// Test PORTFOLIO_NOT_FOUND
	req = httptest.NewRequest(http.MethodPost, "/api/positions/recalculate?portfolio_id=999", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "PORTFOLIO_NOT_FOUND" {
		t.Errorf("expected PORTFOLIO_NOT_FOUND, got %q", errResp.Code)
	}
}

// --- Integration-style test with real router ---

func TestPosRouterIntegration(t *testing.T) {
	r, posRepo := setupPositionRouter(t, []int64{1}, []int64{})
	posRepo.positions = []position.Position{
		{ID: 1, AccountID: 1, Symbol: "AAPL", Quantity: decimal.MustNew(1000, 2), IsClosed: false},
		{ID: 2, AccountID: 1, Symbol: "MSFT", Quantity: decimal.MustNew(500, 2), IsClosed: false},
	}
	posRepo.lots = []position.Lot{
		{ID: 1, LotID: "LOT-TEST", AccountID: 1, Symbol: "AAPL", LotType: "buy", Quantity: decimal.MustNew(1000, 2)},
	}

	// GET /api/positions (returns PositionWithMarket)
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var items []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 positions, got %d", len(items))
	}

	// GET /api/lots/LOT-TEST
	req = httptest.NewRequest(http.MethodGet, "/api/lots/LOT-TEST", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for lot, got %d", w.Code)
	}

	// POST /api/positions/recalculate
	req = httptest.NewRequest(http.MethodPost, "/api/positions/recalculate", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for recalc, got %d", w.Code)
	}
}
