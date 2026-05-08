package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// --- Test helpers ---

// testTxRepo is a minimal in-memory mock repo for transaction handler tests.
type testTxRepo struct {
	items  map[int64]*transaction.Transaction
	nextID int64
}

func newTestTxRepo() *testTxRepo {
	return &testTxRepo{
		items:  make(map[int64]*transaction.Transaction),
		nextID: 1,
	}
}

func (r *testTxRepo) Create(_ context.Context, t *transaction.Transaction) error {
	t.ID = r.nextID
	r.nextID++
	r.items[t.ID] = t
	return nil
}

func (r *testTxRepo) GetByID(_ context.Context, id int64) (*transaction.Transaction, error) {
	t, ok := r.items[id]
	if !ok {
		return nil, transaction.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *testTxRepo) List(_ context.Context, filters transaction.ListFilters, limit, offset int) ([]transaction.Transaction, error) {
	var result []transaction.Transaction
	for _, t := range r.items {
		cp := *t
		result = append(result, cp)
	}
	// Simple filter
	if filters.AccountID != nil {
		var filtered []transaction.Transaction
		for _, t := range result {
			if t.AccountID == *filters.AccountID {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}
	if offset > 0 && offset < len(result) {
		result = result[offset:]
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *testTxRepo) ListWithAccount(_ context.Context, filters transaction.ListFilters, limit, offset int) ([]transaction.TransactionWithAccount, error) {
	items, _ := r.List(nil, filters, limit, offset)
	result := make([]transaction.TransactionWithAccount, len(items))
	for i, t := range items {
		result[i] = transaction.TransactionWithAccount{
			Transaction: t,
			AccountName: "",
		}
	}
	return result, nil
}

func (r *testTxRepo) Update(_ context.Context, t *transaction.Transaction) error {
	r.items[t.ID] = t
	return nil
}

func (r *testTxRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.items[id]; !ok {
		return transaction.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

// testTxAccountChecker is a mock account checker.
type testTxAccountChecker struct {
	ids map[int64]bool
}

func newTestTxAccountChecker(ids ...int64) *testTxAccountChecker {
	m := &testTxAccountChecker{ids: make(map[int64]bool)}
	for _, id := range ids {
		m.ids[id] = true
	}
	return m
}

func (m *testTxAccountChecker) AccountExists(_ context.Context, id int64) bool {
	return m.ids[id]
}

// testTxSymbolChecker is a mock symbol checker.
type testTxSymbolChecker struct {
	symbols map[string]bool
}

func newTestTxSymbolChecker(symbols ...string) *testTxSymbolChecker {
	m := &testTxSymbolChecker{symbols: make(map[string]bool)}
	for _, s := range symbols {
		m.symbols[s] = true
	}
	return m
}

func (m *testTxSymbolChecker) SymbolExists(_ context.Context, symbol string) bool {
	return m.symbols[symbol]
}

// testTxSymbolCreator is a mock symbol creator.
type testTxSymbolCreator struct {
	checker *testTxSymbolChecker
}

func newTestTxSymbolCreator(checker *testTxSymbolChecker) *testTxSymbolCreator {
	return &testTxSymbolCreator{checker: checker}
}

func (m *testTxSymbolCreator) CreateSymbol(_ context.Context, internalSymbol, _ string) error {
	m.checker.symbols[internalSymbol] = true
	return nil
}

func setupTransactionHandler(t *testing.T, accountIDs []int64, symbols []string) (*TransactionHandler, *testTxRepo, *testTxSymbolChecker) {
	t.Helper()
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker(accountIDs...)
	symCheck := newTestTxSymbolChecker(symbols...)
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)
	return NewTransactionHandler(svc), repo, symCheck
}

// txBody creates a JSON body for a create request.
func txBody(accountID int64, date, typ, symbol, currency string, qty int64, price int64, netCash int64) string {
	return fmt.Sprintf(
		`{"account_id":%d,"date":"%s","type":"%s","symbol":"%s","quantity":%d,"price":%d,"currency":"%s","net_cash":%d}`,
		accountID, date, typ, symbol, qty, price, currency, netCash,
	)
}

// --- Create Tests ---

func TestTxHandleCreate_Success(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	body := txBody(3, "2025-01-15", "buy", "AAPL", "USD", 10, 15000, -150000)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var tx transaction.Transaction
	json.NewDecoder(w.Body).Decode(&tx)
	if tx.Symbol != "AAPL" {
		t.Errorf("expected symbol 'AAPL', got %q", tx.Symbol)
	}
	if tx.Type != "buy" {
		t.Errorf("expected type 'buy', got %q", tx.Type)
	}
}

func TestTxHandleCreate_CashAutoCreate(t *testing.T) {
	handler, _, symCheck := setupTransactionHandler(t, []int64{3}, []string{})

	body := txBody(3, "2025-01-01", "deposit", "$CASH-USD", "USD", 1000000, 1, 1000000)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	if !symCheck.SymbolExists(context.Background(), "$CASH-USD") {
		t.Error("expected $CASH-USD to be auto-created")
	}
}

func TestTxHandleCreate_InvalidBody(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTxHandleCreate_NonExistentAccount(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	body := txBody(999, "2025-01-15", "buy", "AAPL", "USD", 10, 15000, -150000)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTxHandleCreate_NonExistentSymbol(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	body := txBody(3, "2025-01-15", "buy", "XYZZY", "USD", 10, 15000, -150000)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTxHandleCreate_InvalidType(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	body := txBody(3, "2025-01-15", "exchange", "AAPL", "USD", 10, 15000, -150000)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTxHandleCreate_InvalidPrice(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	body := txBody(3, "2025-01-15", "buy", "AAPL", "USD", 10, 0, 0)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTxHandleCreate_InvalidCurrency(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	body := txBody(3, "2025-01-15", "buy", "AAPL", "US", 10, 15000, -150000)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- List Tests ---

func TestTxHandleList(t *testing.T) {
	handler, repo, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	now := time.Now()
	for i := int64(1); i <= 3; i++ {
		repo.items[i] = &transaction.Transaction{
			ID: i, AccountID: 3, Date: now, Type: "buy", Symbol: "AAPL",
			Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
			Currency: "USD", CreatedAt: now, UpdatedAt: now,
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var items []transaction.Transaction
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestTxHandleList_Empty(t *testing.T) {
	handler, _, _ := setupTransactionHandler(t, []int64{}, []string{})

	req := httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var items []transaction.Transaction
	json.NewDecoder(w.Body).Decode(&items)
	if items == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestTxHandleList_WithFilters(t *testing.T) {
	handler, repo, _ := setupTransactionHandler(t, []int64{3, 5}, []string{"AAPL", "MSFT"})

	now := time.Now()
	id := int64(1)
	for _, sym := range []string{"AAPL", "MSFT"} {
		repo.items[id] = &transaction.Transaction{
			ID: id, AccountID: 3, Date: now, Type: "buy", Symbol: sym,
			Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
			Currency: "USD", CreatedAt: now, UpdatedAt: now,
		}
		id++
	}
	repo.items[3] = &transaction.Transaction{
		ID: 3, AccountID: 5, Date: now, Type: "buy", Symbol: "AAPL",
		Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
		Currency: "USD", CreatedAt: now, UpdatedAt: now,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/transactions?account_id=3", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	var items []transaction.Transaction
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 items for account_id=3, got %d", len(items))
	}
}

func TestTxHandleList_Pagination(t *testing.T) {
	handler, repo, _ := setupTransactionHandler(t, []int64{3}, []string{"AAPL"})

	now := time.Now()
	for i := int64(1); i <= 5; i++ {
		repo.items[i] = &transaction.Transaction{
			ID: i, AccountID: 3, Date: now, Type: "buy", Symbol: "AAPL",
			Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
			Currency: "USD", CreatedAt: now, UpdatedAt: now,
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/transactions?limit=2", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	var items []transaction.Transaction
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 items with limit=2, got %d", len(items))
	}
}

// --- Get Tests ---

func TestTxHandleGet_Success(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker(3)
	symCheck := newTestTxSymbolChecker("AAPL")
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	repo.items[1] = &transaction.Transaction{
		ID: 1, AccountID: 3, Date: time.Now(), Type: "buy", Symbol: "AAPL",
		Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
		Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/transactions/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var tx transaction.Transaction
	json.NewDecoder(w.Body).Decode(&tx)
	if tx.Symbol != "AAPL" {
		t.Errorf("expected 'AAPL', got %q", tx.Symbol)
	}
}

func TestTxHandleGet_NotFound(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker()
	symCheck := newTestTxSymbolChecker()
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/transactions/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTxHandleGet_InvalidID(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker()
	symCheck := newTestTxSymbolChecker()
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/transactions/abc", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Update Tests ---

func TestTxHandleUpdate_Success(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker(3)
	symCheck := newTestTxSymbolChecker("AAPL")
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	repo.items[1] = &transaction.Transaction{
		ID: 1, AccountID: 3, Date: time.Now(), Type: "buy", Symbol: "AAPL",
		Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
		Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	body := `{"type": "sell"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/transactions/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var tx transaction.Transaction
	json.NewDecoder(w.Body).Decode(&tx)
	if tx.Type != "sell" {
		t.Errorf("expected type 'sell', got %q", tx.Type)
	}
}

func TestTxHandleUpdate_NotFound(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker()
	symCheck := newTestTxSymbolChecker()
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	body := `{"type": "sell"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/transactions/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTxHandleUpdate_InvalidType(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker(3)
	symCheck := newTestTxSymbolChecker("AAPL")
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	repo.items[1] = &transaction.Transaction{
		ID: 1, AccountID: 3, Date: time.Now(), Type: "buy", Symbol: "AAPL",
		Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
		Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	body := `{"type": "exchange"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/transactions/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTxHandleUpdate_NoChanges(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker(3)
	symCheck := newTestTxSymbolChecker("AAPL")
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	original := time.Now()
	repo.items[1] = &transaction.Transaction{
		ID: 1, AccountID: 3, Date: original, Type: "buy", Symbol: "AAPL",
		Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
		Currency: "USD", CreatedAt: original, UpdatedAt: original,
	}

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	body := `{}`
	req := httptest.NewRequest(http.MethodPatch, "/api/transactions/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Delete Tests ---

func TestTxHandleDelete_Success(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker(3)
	symCheck := newTestTxSymbolChecker("AAPL")
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	repo.items[1] = &transaction.Transaction{
		ID: 1, AccountID: 3, Date: time.Now(), Type: "buy", Symbol: "AAPL",
		Quantity: decimal.MustNew(10, 0), Price: decimal.MustNew(150, 0),
		Currency: "USD",
	}

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/transactions/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestTxHandleDelete_NotFound(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker()
	symCheck := newTestTxSymbolChecker()
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/transactions/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Error Response Format ---

func TestTxErrorResponseFormat(t *testing.T) {
	repo := newTestTxRepo()
	accounts := newTestTxAccountChecker(3)
	symCheck := newTestTxSymbolChecker("AAPL")
	symCreate := newTestTxSymbolCreator(symCheck)
	svc := transaction.NewService(repo, accounts, symCheck, symCreate, nil, nil)

	r := chi.NewRouter()
	NewTransactionHandler(svc).RegisterRoutes(r)

	// Test TRANSACTION_NOT_FOUND
	req := httptest.NewRequest(http.MethodGet, "/api/transactions/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "TRANSACTION_NOT_FOUND" {
		t.Errorf("expected TRANSACTION_NOT_FOUND, got %q", errResp.Code)
	}

	// Test ACCOUNT_NOT_FOUND
	body := txBody(999, "2025-01-15", "buy", "AAPL", "USD", 10, 15000, -150000)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "ACCOUNT_NOT_FOUND" {
		t.Errorf("expected ACCOUNT_NOT_FOUND, got %q", errResp.Code)
	}

	// Test SYMBOL_NOT_FOUND
	body = txBody(3, "2025-01-15", "buy", "XYZZY", "USD", 10, 15000, -150000)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "SYMBOL_NOT_FOUND" {
		t.Errorf("expected SYMBOL_NOT_FOUND, got %q", errResp.Code)
	}

	// Test INVALID_TYPE
	body = txBody(3, "2025-01-15", "exchange", "AAPL", "USD", 10, 15000, -150000)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_TYPE" {
		t.Errorf("expected INVALID_TYPE, got %q", errResp.Code)
	}

	// Test INVALID_PRICE
	body = txBody(3, "2025-01-15", "buy", "AAPL", "USD", 10, 0, -150000)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PRICE" {
		t.Errorf("expected INVALID_PRICE, got %q", errResp.Code)
	}

	// Test INVALID_NET_CASH
	body = txBody(3, "2025-01-15", "buy", "AAPL", "USD", 10, 15000, 0)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_NET_CASH" {
		t.Errorf("expected INVALID_NET_CASH, got %q", errResp.Code)
	}

	// Test INVALID_CURRENCY
	body = txBody(3, "2025-01-15", "buy", "AAPL", "US", 10, 15000, -150000)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_CURRENCY" {
		t.Errorf("expected INVALID_CURRENCY, got %q", errResp.Code)
	}

	// Test INVALID_QUANTITY
	body = txBody(3, "2025-01-15", "buy", "AAPL", "USD", 0, 15000, 0)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_QUANTITY" {
		t.Errorf("expected INVALID_QUANTITY, got %q", errResp.Code)
	}

	// Test INVALID_SYMBOL
	body = `{"account_id":3,"date":"2025-01-15","type":"buy","symbol":"","quantity":10,"price":15000,"currency":"USD"}`
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_SYMBOL" {
		t.Errorf("expected INVALID_SYMBOL, got %q", errResp.Code)
	}

	// Test INVALID_DATE
	body = `{"account_id":3,"date":"not-a-date","type":"buy","symbol":"AAPL","quantity":10,"price":15000,"currency":"USD"}`
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_DATE" {
		t.Errorf("expected INVALID_DATE, got %q", errResp.Code)
	}
}
