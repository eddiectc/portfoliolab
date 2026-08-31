package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
)

func TestAccount_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create a portfolio first
	body := json.RawMessage(`{"name": "Test Portfolio", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create an account
	accountBody := json.RawMessage(`{"name": "IBKR", "portfolio_id": 1}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(accountBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d", w.Code)
	}

	var a account.Account
	_ = json.NewDecoder(w.Body).Decode(&a)
	if a.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if a.Name != "IBKR" {
		t.Errorf("expected 'IBKR', got %q", a.Name)
	}
	if a.PortfolioID != 1 {
		t.Errorf("expected portfolio_id 1, got %d", a.PortfolioID)
	}

	// Get the account by ID
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/accounts/%d", a.ID), nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, getReq)

	if w2.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", w2.Code)
	}

	var got account.Account
	_ = json.NewDecoder(w2.Body).Decode(&got)
	if got.Name != "IBKR" {
		t.Errorf("expected 'IBKR', got %q", got.Name)
	}
}

func TestAccount_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var accounts []account.Account
	_ = json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}

func TestAccount_CreateListDelete(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create a portfolio
	body := json.RawMessage(`{"name": "Main", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create two accounts
	for _, name := range []string{"IBKR", "Fidelity"} {
		body := json.RawMessage(`{"name": "` + name + `", "portfolio_id": 1}`)
		req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", name, w.Code)
		}
	}

	// List
	req = httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var accounts []account.Account
	_ = json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(accounts))
	}

	// Delete first
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/accounts/1", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, deleteReq)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w2.Code)
	}

	// List again — should have 1
	req = httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var remaining []account.Account
	_ = json.NewDecoder(w.Body).Decode(&remaining)
	if len(remaining) != 1 {
		t.Errorf("expected 1 account after delete, got %d", len(remaining))
	}
}

func TestAccount_Update(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Main", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create account
	body = json.RawMessage(`{"name": "Original", "portfolio_id": 1}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d", w.Code)
	}

	// Update
	updateBody := json.RawMessage(`{"name": "Updated"}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/accounts/1", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	var a account.Account
	_ = json.NewDecoder(w.Body).Decode(&a)
	if a.Name != "Updated" {
		t.Errorf("expected 'Updated', got %q", a.Name)
	}
}

func TestAccount_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Main", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create first account
	body = json.RawMessage(`{"name": "Unique", "portfolio_id": 1}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create first: expected 201, got %d", w.Code)
	}

	// Try duplicate
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate name, got %d", w.Code)
	}
}

func TestAccount_CascadeDeletePortfolio(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Main", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create accounts under the portfolio
	for _, name := range []string{"IBKR", "Fidelity"} {
		body = json.RawMessage(`{"name": "` + name + `", "portfolio_id": 1}`)
		req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", name, w.Code)
		}
	}

	// Verify accounts exist
	req = httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var accounts []account.Account
	_ = json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts before cascade, got %d", len(accounts))
	}

	// Delete the portfolio
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/portfolios/1", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, deleteReq)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("delete portfolio: expected 204, got %d", w2.Code)
	}

	// Accounts should be cascade-deleted
	req = httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	_ = json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts after portfolio cascade delete, got %d", len(accounts))
	}
}

func TestAccount_Pagination(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Main", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create 5 accounts
	for i := 0; i < 5; i++ {
		body = json.RawMessage(`{"name": "A` + string(rune('0'+i)) + `", "portfolio_id": 1}`)
		req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create account %d: expected 201, got %d", i, w.Code)
		}
	}

	// Paginated list: limit=2
	req = httptest.NewRequest(http.MethodGet, "/api/accounts?limit=2", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var accounts []account.Account
	_ = json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts with limit=2, got %d", len(accounts))
	}
}

func TestAccount_NotFound(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent account, got %d", w.Code)
	}
}

func TestAccount_NonExistentPortfolio(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	body := json.RawMessage(`{"name": "Test", "portfolio_id": 999}`)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent portfolio, got %d", w.Code)
	}
}

func TestAccount_Pagination_ZeroLimitDefaultsTo50(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Main", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create 10 accounts
	for i := 0; i < 10; i++ {
		body = json.RawMessage(`{"name": "A` + string(rune('0'+i)) + `", "portfolio_id": 1}`)
		req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create account %d: expected 201, got %d", i, w.Code)
		}
	}

	// limit=0 should default to 50, returning all 10 accounts
	req = httptest.NewRequest(http.MethodGet, "/api/accounts?limit=0", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var accounts []account.Account
	_ = json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 10 {
		t.Errorf("expected 10 accounts with limit=0 (default 50), got %d", len(accounts))
	}
}

func TestAccount_Pagination_NegativeOffsetDefaultsTo0(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Main", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create 5 accounts
	for i := 0; i < 5; i++ {
		body = json.RawMessage(`{"name": "B` + string(rune('0'+i)) + `", "portfolio_id": 1}`)
		req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create account %d: expected 201, got %d", i, w.Code)
		}
	}

	// offset=-1 should default to 0, returning 3 accounts
	req = httptest.NewRequest(http.MethodGet, "/api/accounts?limit=3&offset=-1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var accounts []account.Account
	_ = json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 3 {
		t.Errorf("expected 3 accounts with offset=-1 (default 0), got %d", len(accounts))
	}
}
