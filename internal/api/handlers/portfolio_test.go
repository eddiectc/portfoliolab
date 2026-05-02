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

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
)

// testRepo is a minimal in-memory mock repo for handler tests.
type testRepo struct {
	portfolios map[int64]*portfolio.Portfolio
	names      map[string]int64
	nextID     int64
}

func newTestRepo() *testRepo {
	return &testRepo{
		portfolios: make(map[int64]*portfolio.Portfolio),
		names:      make(map[string]int64),
		nextID:     1,
	}
}

func (r *testRepo) Create(_ context.Context, p *portfolio.Portfolio) error {
	r.nextID++
	p.ID = r.nextID
	r.portfolios[p.ID] = p
	r.names[p.Name] = p.ID
	return nil
}

func (r *testRepo) GetByID(_ context.Context, id int64) (*portfolio.Portfolio, error) {
	p, ok := r.portfolios[id]
	if !ok {
		return nil, portfolio.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *testRepo) GetAll(_ context.Context, limit, offset int) ([]portfolio.Portfolio, error) {
	var result []portfolio.Portfolio
	for _, p := range r.portfolios {
		cp := *p
		result = append(result, cp)
	}
	return result, nil
}

func (r *testRepo) Update(_ context.Context, p *portfolio.Portfolio) error {
	r.portfolios[p.ID] = p
	return nil
}

func (r *testRepo) Delete(_ context.Context, id int64) error {
	p, ok := r.portfolios[id]
	if !ok {
		return portfolio.ErrNotFound
	}
	delete(r.portfolios, id)
	delete(r.names, p.Name)
	return nil
}

func (r *testRepo) GetByName(_ context.Context, name string) (*portfolio.Portfolio, error) {
	id, ok := r.names[name]
	if !ok {
		return nil, portfolio.ErrNotFound
	}
	p := r.portfolios[id]
	cp := *p
	return &cp, nil
}

func TestHandleCreate_Success(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)
	handler := NewPortfolioHandler(svc)

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
	repo := newTestRepo()
	svc := portfolio.NewService(repo)
	handler := NewPortfolioHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreate_DuplicateName(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)
	handler := NewPortfolioHandler(svc)

	// Pre-create a portfolio with the name
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
	repo := newTestRepo()
	svc := portfolio.NewService(repo)
	handler := NewPortfolioHandler(svc)

	body := `{"name": "Test", "currency": "XX"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleList(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)
	handler := NewPortfolioHandler(svc)

	// Seed some portfolios
	for i := 1; i <= 3; i++ {
		repo.portfolios[int64(i)] = &portfolio.Portfolio{ID: int64(i), Name: "P" + string(rune('A'+i-1)), Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var portfolios []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&portfolios)
	if len(portfolios) != 3 {
		t.Errorf("expected 3 portfolios, got %d", len(portfolios))
	}
}

func TestHandleList_Empty(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)
	handler := NewPortfolioHandler(svc)

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

func TestHandleGet_Success(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)

	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Test", Currency: "EUR", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	r := chi.NewRouter()
	NewPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	if p.Name != "Test" {
		t.Errorf("expected 'Test', got %q", p.Name)
	}
	if p.Currency != "EUR" {
		t.Errorf("expected 'EUR', got %q", p.Currency)
	}
}

func TestHandleGet_NotFound(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)

	r := chi.NewRouter()
	NewPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGet_InvalidID(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)

	r := chi.NewRouter()
	NewPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/abc", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdate_Success(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)

	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Old", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["Old"] = 1

	r := chi.NewRouter()
	NewPortfolioHandler(svc).RegisterRoutes(r)

	body := `{"name": "New Name"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/portfolios/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	if p.Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", p.Name)
	}
}

func TestHandleDelete(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)

	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Test", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["Test"] = 1

	r := chi.NewRouter()
	NewPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/portfolios/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)

	r := chi.NewRouter()
	NewPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestErrorResponseFormat(t *testing.T) {
	repo := newTestRepo()
	svc := portfolio.NewService(repo)

	r := chi.NewRouter()
	NewPortfolioHandler(svc).RegisterRoutes(r)

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
