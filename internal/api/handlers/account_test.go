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

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
)

// testAccountRepo is a minimal in-memory mock repo for account handler tests.
type testAccountRepo struct {
	accounts    map[int64]*account.Account
	names       map[string]int64
	byPortfolio map[int64][]int64
	nextID      int64
}

func newTestAccountRepo() *testAccountRepo {
	return &testAccountRepo{
		accounts:    make(map[int64]*account.Account),
		names:       make(map[string]int64),
		byPortfolio: make(map[int64][]int64),
		nextID:      1,
	}
}

func (r *testAccountRepo) Create(_ context.Context, a *account.Account) error {
	r.nextID++
	a.ID = r.nextID
	r.accounts[a.ID] = a
	r.names[a.Name] = a.ID
	r.byPortfolio[a.PortfolioID] = append(r.byPortfolio[a.PortfolioID], a.ID)
	return nil
}

func (r *testAccountRepo) GetByID(_ context.Context, id int64) (*account.Account, error) {
	a, ok := r.accounts[id]
	if !ok {
		return nil, account.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *testAccountRepo) GetAll(_ context.Context, limit, offset int) ([]account.Account, error) {
	var result []account.Account
	for _, a := range r.accounts {
		cp := *a
		result = append(result, cp)
	}
	if offset > 0 && offset < len(result) {
		result = result[offset:]
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *testAccountRepo) GetByPortfolio(_ context.Context, portfolioID int64, limit, offset int) ([]account.Account, error) {
	ids := r.byPortfolio[portfolioID]
	var result []account.Account
	for _, id := range ids {
		a := r.accounts[id]
		cp := *a
		result = append(result, cp)
	}
	return result, nil
}

func (r *testAccountRepo) GetByName(_ context.Context, name string) (*account.Account, error) {
	id, ok := r.names[name]
	if !ok {
		return nil, account.ErrNotFound
	}
	a := r.accounts[id]
	cp := *a
	return &cp, nil
}

func (r *testAccountRepo) Update(_ context.Context, a *account.Account) error {
	r.accounts[a.ID] = a
	return nil
}

func (r *testAccountRepo) Delete(_ context.Context, id int64) error {
	a, ok := r.accounts[id]
	if !ok {
		return account.ErrNotFound
	}
	delete(r.accounts, id)
	delete(r.names, a.Name)
	return nil
}

// testPortfolioChecker is a mock that always says portfolios exist.
type testPortfolioChecker struct {
	ids map[int64]bool
}

func newTestPortfolioChecker(ids ...int64) *testPortfolioChecker {
	m := &testPortfolioChecker{ids: make(map[int64]bool)}
	for _, id := range ids {
		m.ids[id] = true
	}
	return m
}

func (m *testPortfolioChecker) PortfolioExists(_ context.Context, id int64) bool {
	return m.ids[id]
}

func setupAccountHandler(t *testing.T, portfolioIDs ...int64) (*AccountHandler, *testAccountRepo) {
	t.Helper()
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(portfolioIDs...)
	svc := account.NewService(repo, checker)
	return NewAccountHandler(svc), repo
}

func TestAccountHandleCreate_Success(t *testing.T) {
	handler, repo := setupAccountHandler(t, 1)

	body := `{"name": "IBKR", "portfolio_id": 1}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var a account.Account
	json.NewDecoder(w.Body).Decode(&a)
	if a.Name != "IBKR" {
		t.Errorf("expected name 'IBKR', got %q", a.Name)
	}
	if a.PortfolioID != 1 {
		t.Errorf("expected portfolio_id 1, got %d", a.PortfolioID)
	}
	_ = repo
}

func TestAccountHandleCreate_InvalidBody(t *testing.T) {
	handler, _ := setupAccountHandler(t, 1)

	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccountHandleCreate_EmptyName(t *testing.T) {
	handler, _ := setupAccountHandler(t, 1)

	body := `{"name": "", "portfolio_id": 1}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccountHandleCreate_NameTooLong(t *testing.T) {
	handler, _ := setupAccountHandler(t, 1)

	// 101 characters exceeds the 100-char limit
	longName := string(make([]byte, 101))
	body := fmt.Sprintf(`{"name": "%s", "portfolio_id": 1}`, longName)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccountHandleCreate_DuplicateName(t *testing.T) {
	handler, repo := setupAccountHandler(t, 1)

	repo.accounts[1] = &account.Account{ID: 1, Name: "IBKR", PortfolioID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["IBKR"] = 1

	body := `{"name": "IBKR", "portfolio_id": 1}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestAccountHandleCreate_NonExistentPortfolio(t *testing.T) {
	handler, _ := setupAccountHandler(t, 1) // only portfolio 1 exists

	body := `{"name": "New", "portfolio_id": 999}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAccountHandleList(t *testing.T) {
	handler, repo := setupAccountHandler(t, 1)

	for i := 1; i <= 3; i++ {
		repo.accounts[int64(i)] = &account.Account{ID: int64(i), Name: "A" + string(rune('A'+i-1)), PortfolioID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var accounts []account.Account
	json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(accounts))
	}
}

func TestAccountHandleList_Empty(t *testing.T) {
	handler, _ := setupAccountHandler(t, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var accounts []account.Account
	json.NewDecoder(w.Body).Decode(&accounts)
	if accounts == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestAccountHandleList_Pagination(t *testing.T) {
	handler, repo := setupAccountHandler(t, 1)

	for i := 1; i <= 5; i++ {
		repo.accounts[int64(i)] = &account.Account{ID: int64(i), Name: "A" + string(rune('A'+i-1)), PortfolioID: 1}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/accounts?limit=2", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	var accounts []account.Account
	json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts with limit=2, got %d", len(accounts))
	}
}

func TestAccountHandleList_DefaultPagination(t *testing.T) {
	handler, repo := setupAccountHandler(t, 1)

	// Create 10 accounts
	for i := 1; i <= 10; i++ {
		repo.accounts[int64(i)] = &account.Account{ID: int64(i), Name: fmt.Sprintf("Acc%d", i), PortfolioID: 1}
	}

	// No pagination params — should return all 10
	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var accounts []account.Account
	json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 10 {
		t.Errorf("expected 10 accounts with default pagination, got %d", len(accounts))
	}
}

func TestAccountHandleList_ByPortfolio(t *testing.T) {
	handler, repo := setupAccountHandler(t, 1, 2)

	// Create accounts across two portfolios
	repo.accounts[1] = &account.Account{ID: 1, Name: "Acc1", PortfolioID: 1}
	repo.accounts[2] = &account.Account{ID: 2, Name: "Acc2", PortfolioID: 2}
	repo.accounts[3] = &account.Account{ID: 3, Name: "Acc3", PortfolioID: 1}
	repo.byPortfolio[1] = []int64{1, 3}
	repo.byPortfolio[2] = []int64{2}

	// Filter by portfolio_id=1 — should return 2 accounts
	req := httptest.NewRequest(http.MethodGet, "/api/accounts?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var accounts []account.Account
	json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts for portfolio 1, got %d", len(accounts))
	}
	for _, a := range accounts {
		if a.PortfolioID != 1 {
			t.Errorf("expected portfolio_id 1, got %d", a.PortfolioID)
		}
	}
}

func TestAccountHandleList_ByPortfolio_InvalidID(t *testing.T) {
	handler, _ := setupAccountHandler(t, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts?portfolio_id=not-a-number", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccountHandleList_ByPortfolio_Empty(t *testing.T) {
	handler, repo := setupAccountHandler(t, 1)

	// No accounts for portfolio 999
	req := httptest.NewRequest(http.MethodGet, "/api/accounts?portfolio_id=999", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var accounts []account.Account
	json.NewDecoder(w.Body).Decode(&accounts)
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts for non-existent portfolio, got %d", len(accounts))
	}
	_ = repo
}

func TestAccountHandleGet_Success(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	repo.accounts[1] = &account.Account{ID: 1, Name: "IBKR", PortfolioID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var a account.Account
	json.NewDecoder(w.Body).Decode(&a)
	if a.Name != "IBKR" {
		t.Errorf("expected 'IBKR', got %q", a.Name)
	}
}

func TestAccountHandleGet_NotFound(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAccountHandleGet_InvalidID(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/abc", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccountHandleUpdate_Success(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	repo.accounts[1] = &account.Account{ID: 1, Name: "Old", PortfolioID: 1}
	repo.names["Old"] = 1

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	body := `{"name": "New Name"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var a account.Account
	json.NewDecoder(w.Body).Decode(&a)
	if a.Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", a.Name)
	}
}

func TestAccountHandleUpdate_NotFound(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	body := `{"name": "New"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAccountHandleUpdate_DuplicateName(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	repo.accounts[1] = &account.Account{ID: 1, Name: "Alpha", PortfolioID: 1}
	repo.accounts[2] = &account.Account{ID: 2, Name: "Beta", PortfolioID: 1}
	repo.names["Alpha"] = 1
	repo.names["Beta"] = 2

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	body := `{"name": "Alpha"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/2", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestAccountHandleUpdate_InvalidName(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	repo.accounts[1] = &account.Account{ID: 1, Name: "Test", PortfolioID: 1}
	repo.names["Test"] = 1

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	body := `{"name": ""}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccountHandleDelete_Success(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	repo.accounts[1] = &account.Account{ID: 1, Name: "Test", PortfolioID: 1}
	repo.names["Test"] = 1

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/accounts/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestAccountHandleDelete_NotFound(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/accounts/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAccountErrorResponseFormat(t *testing.T) {
	repo := newTestAccountRepo()
	checker := newTestPortfolioChecker(1)
	svc := account.NewService(repo, checker)

	r := chi.NewRouter()
	NewAccountHandler(svc).RegisterRoutes(r)

	// Test ACCOUNT_NOT_FOUND
	req := httptest.NewRequest(http.MethodGet, "/api/accounts/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "ACCOUNT_NOT_FOUND" {
		t.Errorf("expected error code ACCOUNT_NOT_FOUND, got %q", errResp.Code)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}

	// Test INVALID_NAME
	body := `{"name": "", "portfolio_id": 1}`
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_NAME" {
		t.Errorf("expected error code INVALID_NAME, got %q", errResp.Code)
	}

	// Test ACCOUNT_NAME_EXISTS
	repo.accounts[1] = &account.Account{ID: 1, Name: "Dup", PortfolioID: 1}
	repo.names["Dup"] = 1
	body = `{"name": "Dup", "portfolio_id": 1}`
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "ACCOUNT_NAME_EXISTS" {
		t.Errorf("expected error code ACCOUNT_NAME_EXISTS, got %q", errResp.Code)
	}

	// Test PORTFOLIO_NOT_FOUND
	body = `{"name": "Test", "portfolio_id": 999}`
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "PORTFOLIO_NOT_FOUND" {
		t.Errorf("expected error code PORTFOLIO_NOT_FOUND, got %q", errResp.Code)
	}
}
