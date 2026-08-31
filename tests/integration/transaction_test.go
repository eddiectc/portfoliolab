package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// setupTx creates a portfolio, account, and symbol mapping for transaction tests.
func setupTx(t *testing.T) (db *sql.DB, router http.Handler, portfolioID int64, accountID int64) {
	t.Helper()
	db = setupTestDB(t)
	router, _ = api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create portfolio
	body := json.RawMessage(`{"name": "Test Portfolio", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}
	var p portfolio.Portfolio
	_ = json.NewDecoder(w.Body).Decode(&p)
	portfolioID = p.ID

	// Create account
	body = json.RawMessage(`{"name": "Test Account", "portfolio_id": ` + fmt.Sprintf("%d", portfolioID) + `}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d", w.Code)
	}
	var a account.Account
	_ = json.NewDecoder(w.Body).Decode(&a)
	accountID = a.ID

	// Create symbol mapping for AAPL
	body = json.RawMessage(`{"internal_symbol": "AAPL", "market_data_symbol": "AAPL"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol mapping: expected 201, got %d", w.Code)
	}

	return db, router, portfolioID, accountID
}

func TestTransaction_CreateAndGet(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var tx transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&tx)
	if tx.Symbol != "AAPL" {
		t.Errorf("expected symbol 'AAPL', got %q", tx.Symbol)
	}
	if tx.Type != "buy" {
		t.Errorf("expected type 'buy', got %q", tx.Type)
	}
	if tx.Currency != "USD" {
		t.Errorf("expected currency 'USD', got %q", tx.Currency)
	}

	// Get by ID
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/transactions/%d", tx.ID), nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, getReq)

	if w2.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", w2.Code)
	}

	var got transaction.Transaction
	_ = json.NewDecoder(w2.Body).Decode(&got)
	if got.Symbol != "AAPL" {
		t.Errorf("expected 'AAPL', got %q", got.Symbol)
	}
	if got.Quantity.String() != "10" {
		t.Errorf("expected quantity '10', got %q", got.Quantity.String())
	}
}

func TestTransaction_CreateSell(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create a buy first
	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-10","type":"buy","symbol":"AAPL","quantity":20,"price":140,"currency":"USD","net_cash":-2800}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create buy: expected 201, got %d", w.Code)
	}

	// Create a sell
	body = fmt.Sprintf(`{"account_id":%d,"date":"2025-02-15","type":"sell","symbol":"AAPL","quantity":-5,"price":160,"currency":"USD","net_cash":800}`, accountID)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var tx transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&tx)
	if tx.Type != "sell" {
		t.Errorf("expected type 'sell', got %q", tx.Type)
	}
	if tx.Quantity.String() != "-5" {
		t.Errorf("expected quantity '-5', got %q", tx.Quantity.String())
	}
}

func TestTransaction_CreateCashDeposit(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-01","type":"deposit","symbol":"$CASH-USD","quantity":10000,"price":1,"currency":"USD","net_cash":10000}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var tx transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&tx)
	if tx.Symbol != "$CASH-USD" {
		t.Errorf("expected symbol '$CASH-USD', got %q", tx.Symbol)
	}
	if tx.Type != "deposit" {
		t.Errorf("expected type 'deposit', got %q", tx.Type)
	}
}

func TestTransaction_ListEmpty(t *testing.T) {
	_, router, _, _ := setupTx(t)

	req := httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(items))
	}
}

func TestTransaction_CreateListDelete(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create two transactions
	for i, typ := range []string{"buy", "sell"} {
		body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-%02d","type":"%s","symbol":"AAPL","quantity":%d,"price":150,"currency":"USD","net_cash":-1500}`,
			accountID, 10+i, typ, 10)
		req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", typ, w.Code)
		}
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(items))
	}

	// Delete first
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/transactions/1", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, deleteReq)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w2.Code)
	}

	// List again — should have 1
	req = httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var remaining []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&remaining)
	if len(remaining) != 1 {
		t.Errorf("expected 1 transaction after delete, got %d", len(remaining))
	}
}

func TestTransaction_Update(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create
	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	var original transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&original)
	originalUpdatedAt := original.UpdatedAt

	// Update type
	updateBody := `{"type": "sell"}`
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/transactions/%d", original.ID), bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	var updated transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&updated)
	if updated.Type != "sell" {
		t.Errorf("expected type 'sell', got %q", updated.Type)
	}
	if updated.UpdatedAt.Equal(originalUpdatedAt) {
		t.Error("expected updated_at to change")
	}
}

func TestTransaction_UpdateNoChanges(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create
	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	var original transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&original)

	// Fetch current state (to get the DB-stored updated_at)
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/transactions/%d", original.ID), nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, getReq)

	var current transaction.Transaction
	_ = json.NewDecoder(w2.Body).Decode(&current)
	dbUpdatedAt := current.UpdatedAt

	// No-op update
	updateBody := `{}`
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/transactions/%d", original.ID), bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	var updated transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&updated)
	if !updated.UpdatedAt.Equal(dbUpdatedAt) {
		t.Errorf("expected updated_at to remain unchanged for no-op update: got %v, want %v", updated.UpdatedAt, dbUpdatedAt)
	}
}

func TestTransaction_FilterByAccount(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create transactions on two different accounts
	// First, create a second account
	acctBody := json.RawMessage(`{"name": "Account 2", "portfolio_id": 1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(acctBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account 2: expected 201, got %d", w.Code)
	}
	var a2 account.Account
	_ = json.NewDecoder(w.Body).Decode(&a2)

	// Create on account 1
	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tx 1: expected 201, got %d", w.Code)
	}

	// Create on account 2
	body = fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, a2.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tx 2: expected 201, got %d", w.Code)
	}

	// Filter by account 1
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/transactions?account_id=%d", accountID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Errorf("expected 1 transaction for account %d, got %d", accountID, len(items))
	}
}

func TestTransaction_FilterBySymbol(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create another symbol mapping for MSFT
	smBody := json.RawMessage(`{"internal_symbol": "MSFT", "market_data_symbol": "MSFT"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(smBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create MSFT mapping: expected 201, got %d", w.Code)
	}

	// Create AAPL transaction
	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create AAPL tx: expected 201, got %d", w.Code)
	}

	// Create MSFT transaction
	body = fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"MSFT","quantity":5,"price":300,"currency":"USD","net_cash":-1500}`, accountID)
	req = httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create MSFT tx: expected 201, got %d", w.Code)
	}

	// Filter by symbol
	req = httptest.NewRequest(http.MethodGet, "/api/transactions?symbol=AAPL", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Errorf("expected 1 AAPL transaction, got %d", len(items))
	}
	if items[0].Symbol != "AAPL" {
		t.Errorf("expected symbol 'AAPL', got %q", items[0].Symbol)
	}
}

func TestTransaction_FilterByDateRange(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create transactions on different dates
	for i, date := range []string{"2025-01-01", "2025-06-15", "2025-12-31"} {
		body := fmt.Sprintf(`{"account_id":%d,"date":"%s","type":"buy","symbol":"AAPL","quantity":%d,"price":150,"currency":"USD","net_cash":-1500}`,
			accountID, date, 10+i)
		req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create tx %s: expected 201, got %d", date, w.Code)
		}
	}

	// Filter by date range (should match 2025-06-15 only)
	req := httptest.NewRequest(http.MethodGet, "/api/transactions?date_from=2025-03-01&date_to=2025-09-01", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Errorf("expected 1 transaction in date range, got %d", len(items))
	}
}

func TestTransaction_Pagination(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create 5 transactions
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-%02d","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`,
			accountID, 1+i)
		req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create tx %d: expected 201, got %d", i, w.Code)
		}
	}

	// Paginated list: limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/transactions?limit=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 transactions with limit=2, got %d", len(items))
	}
}

func TestTransaction_CascadeDeleteAccount(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	// Create a transaction
	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tx: expected 201, got %d", w.Code)
	}

	// Delete the account
	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/accounts/%d", accountID), nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, deleteReq)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("delete account: expected 204, got %d", w2.Code)
	}

	// Verify transaction is gone
	req = httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("expected 0 transactions after account delete, got %d", len(items))
	}
}

func TestTransaction_CascadeDeletePortfolio(t *testing.T) {
	_, router, portfolioID, accountID := setupTx(t)

	// Create a transaction
	body := fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tx: expected 201, got %d", w.Code)
	}

	// Delete the portfolio (cascades through accounts to transactions)
	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/portfolios/%d", portfolioID), nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, deleteReq)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("delete portfolio: expected 204, got %d", w2.Code)
	}

	// Verify transaction is gone
	req = httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var items []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("expected 0 transactions after portfolio delete, got %d", len(items))
	}
}

func TestTransaction_ValidationErrors(t *testing.T) {
	_, router, _, accountID := setupTx(t)

	tests := []struct {
		name     string
		body     string
		want     int
		wantCode string
	}{
		{
			name:     "invalid type",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"exchange","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "INVALID_TYPE",
		},
		{
			name:     "invalid price (zero)",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":0,"currency":"USD","net_cash":-1500}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "INVALID_PRICE",
		},
		{
			name:     "invalid quantity (zero)",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":0,"price":150,"currency":"USD","net_cash":-1500}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "INVALID_QUANTITY",
		},
		{
			name:     "invalid currency",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"US","net_cash":-1500}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "INVALID_CURRENCY",
		},
		{
			name:     "invalid date",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"not-a-date","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "INVALID_DATE",
		},
		{
			name:     "empty symbol",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "INVALID_SYMBOL",
		},
		{
			name:     "non-existent account",
			body:     `{"account_id":999,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`,
			want:     http.StatusNotFound,
			wantCode: "ACCOUNT_NOT_FOUND",
		},
		{
			name:     "non-existent symbol",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"XYZZY","quantity":10,"price":150,"currency":"USD","net_cash":-1500}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "SYMBOL_NOT_FOUND",
		},
		{
			name:     "missing net_cash (zero)",
			body:     fmt.Sprintf(`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":150,"currency":"USD","net_cash":0}`, accountID),
			want:     http.StatusBadRequest,
			wantCode: "INVALID_NET_CASH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.want {
				t.Errorf("expected %d, got %d", tt.want, w.Code)
			}

			var resp map[string]string
			_ = json.NewDecoder(w.Body).Decode(&resp)
			if resp["code"] != tt.wantCode {
				t.Errorf("expected error code %q, got %q", tt.wantCode, resp["code"])
			}
		})
	}
}
