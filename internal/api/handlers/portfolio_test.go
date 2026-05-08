package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
)

// testPortfolioRepo is a minimal in-memory mock repo for portfolio handler tests.
type testPortfolioRepo struct {
	portfolios map[int64]*portfolio.Portfolio
	names      map[string]int64 // name -> id
	nextID     int64
}

func newTestPortfolioRepo() *testPortfolioRepo {
	return &testPortfolioRepo{
		portfolios: make(map[int64]*portfolio.Portfolio),
		names:      make(map[string]int64),
		nextID:     1,
	}
}

func (r *testPortfolioRepo) Create(_ context.Context, p *portfolio.Portfolio) error {
	r.nextID++
	p.ID = r.nextID
	r.portfolios[p.ID] = p
	r.names[p.Name] = p.ID
	return nil
}

func (r *testPortfolioRepo) GetByID(_ context.Context, id int64) (*portfolio.Portfolio, error) {
	p, ok := r.portfolios[id]
	if !ok {
		return nil, portfolio.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *testPortfolioRepo) GetAll(_ context.Context, limit, offset int) ([]portfolio.Portfolio, error) {
	var result []portfolio.Portfolio
	for _, p := range r.portfolios {
		cp := *p
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

func (r *testPortfolioRepo) Update(_ context.Context, p *portfolio.Portfolio) error {
	r.portfolios[p.ID] = p
	return nil
}

func (r *testPortfolioRepo) Delete(_ context.Context, id int64) error {
	p, ok := r.portfolios[id]
	if !ok {
		return portfolio.ErrNotFound
	}
	delete(r.names, p.Name)
	delete(r.portfolios, id)
	return nil
}

func (r *testPortfolioRepo) GetByName(_ context.Context, name string) (*portfolio.Portfolio, error) {
	id, ok := r.names[name]
	if !ok {
		return nil, portfolio.ErrNotFound
	}
	p := r.portfolios[id]
	cp := *p
	return &cp, nil
}

func setupPortfolioHandler(t *testing.T) (*PortfolioHandler, *testPortfolioRepo) {
	t.Helper()
	repo := newTestPortfolioRepo()
	svc := portfolio.NewService(repo)
	return NewPortfolioHandler(svc), repo
}

func TestHandleCreate_Success(t *testing.T) {
	handler, _ := setupPortfolioHandler(t)

	body := `{"name": "Test Portfolio", "currency": "USD"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	if p.Name != "Test Portfolio" {
		t.Errorf("expected name 'Test Portfolio', got %q", p.Name)
	}
	if p.Currency != "USD" {
		t.Errorf("expected currency 'USD', got %q", p.Currency)
	}
}

func TestHandleCreate_InvalidBody(t *testing.T) {
	handler, _ := setupPortfolioHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreate_DuplicateName(t *testing.T) {
	handler, repo := setupPortfolioHandler(t)

	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Dup", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["Dup"] = 1

	body := `{"name": "Dup", "currency": "USD"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleCreate_InvalidCurrency(t *testing.T) {
	handler, _ := setupPortfolioHandler(t)

	body := `{"name": "Test", "currency": "XX"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList(t *testing.T) {
	handler, repo := setupPortfolioHandler(t)

	for i := 1; i <= 3; i++ {
		repo.portfolios[int64(i)] = &portfolio.Portfolio{ID: int64(i), Name: "P" + string(rune('A'+i-1)), Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var got []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&got)
	if len(got) != 3 {
		t.Errorf("expected 3 portfolios, got %d", len(got))
	}
}

func TestHandleList_Empty(t *testing.T) {
	handler, _ := setupPortfolioHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var portfolios []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&portfolios)
	if portfolios == nil {
		t.Error("expected empty array, got nil")
	}
	if len(portfolios) != 0 {
		t.Errorf("expected 0 portfolios, got %d", len(portfolios))
	}
}

func TestHandleList_DefaultPagination(t *testing.T) {
	handler, repo := setupPortfolioHandler(t)

	for i := 1; i <= 10; i++ {
		repo.portfolios[int64(i)] = &portfolio.Portfolio{ID: int64(i), Name: "P", Currency: "USD"}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var got []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&got)
	if len(got) != 10 {
		t.Errorf("expected 10 portfolios with default pagination, got %d", len(got))
	}
}

func TestHandleGet_Success(t *testing.T) {
	repo := newTestPortfolioRepo()
	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Test", Currency: "EUR", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	r := chi.NewRouter()
	portfolio.NewService(repo)
	NewPortfolioHandler(portfolio.NewService(repo)).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var got portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&got)
	if got.Name != "Test" {
		t.Errorf("expected 'Test', got %q", got.Name)
	}
	if got.Currency != "EUR" {
		t.Errorf("expected 'EUR', got %q", got.Currency)
	}
}

func TestHandleGet_NotFound(t *testing.T) {
	repo := newTestPortfolioRepo()
	r := chi.NewRouter()
	NewPortfolioHandler(portfolio.NewService(repo)).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGet_InvalidID(t *testing.T) {
	repo := newTestPortfolioRepo()
	r := chi.NewRouter()
	NewPortfolioHandler(portfolio.NewService(repo)).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/abc", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdate_Success(t *testing.T) {
	repo := newTestPortfolioRepo()
	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Old", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["Old"] = 1

	r := chi.NewRouter()
	NewPortfolioHandler(portfolio.NewService(repo)).RegisterRoutes(r)

	body := `{"name": "New Name"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/portfolios/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var got portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&got)
	if got.Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", got.Name)
	}
}

func TestHandleDelete(t *testing.T) {
	repo := newTestPortfolioRepo()
	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Test", Currency: "USD"}
	repo.names["Test"] = 1

	r := chi.NewRouter()
	NewPortfolioHandler(portfolio.NewService(repo)).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/portfolios/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	repo := newTestPortfolioRepo()
	r := chi.NewRouter()
	NewPortfolioHandler(portfolio.NewService(repo)).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestErrorResponseFormat(t *testing.T) {
	repo := newTestPortfolioRepo()
	r := chi.NewRouter()
	NewPortfolioHandler(portfolio.NewService(repo)).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "PORTFOLIO_NOT_FOUND" {
		t.Errorf("expected error code PORTFOLIO_NOT_FOUND, got %q", errResp.Code)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}
}
