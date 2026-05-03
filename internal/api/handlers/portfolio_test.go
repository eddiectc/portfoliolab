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
	"github.com/stretchr/testify/mock"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
)

func setupPortfolioHandler(t *testing.T) (*PortfolioHandler, *portfolio.MockRepository) {
	t.Helper()
	mockRepo := portfolio.NewMockRepository(t)
	svc := portfolio.NewService(mockRepo)
	return NewPortfolioHandler(svc), mockRepo
}

func TestHandleCreate_Success(t *testing.T) {
	handler, mockRepo := setupPortfolioHandler(t)

	mockRepo.On("GetByName", mock.Anything, "Test Portfolio").Return(nil, portfolio.ErrNotFound).Once()
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*portfolio.Portfolio")).Return(func(_ context.Context, p *portfolio.Portfolio) error {
		p.ID = 1
		return nil
	})

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
	handler, mockRepo := setupPortfolioHandler(t)

	existing := &portfolio.Portfolio{ID: 1, Name: "Dup", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	mockRepo.On("GetByName", mock.Anything, "Dup").Return(existing, nil).Once()

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
	handler, mockRepo := setupPortfolioHandler(t)

	portfolios := []portfolio.Portfolio{
		{ID: 1, Name: "PA", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: 2, Name: "PB", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: 3, Name: "PC", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	mockRepo.On("GetAll", mock.Anything, 50, 0).Return(portfolios, nil).Once()

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
	handler, mockRepo := setupPortfolioHandler(t)

	mockRepo.On("GetAll", mock.Anything, 50, 0).Return([]portfolio.Portfolio{}, nil).Once()

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
	handler, mockRepo := setupPortfolioHandler(t)

	// Create 10 portfolios
	portfolios := make([]portfolio.Portfolio, 10)
	for i := 0; i < 10; i++ {
		portfolios[i] = portfolio.Portfolio{ID: int64(i + 1), Name: "P", Currency: "USD"}
	}
	// Service defaults limit=0 to 50, so mock expects 50
	mockRepo.On("GetAll", mock.Anything, 50, 0).Return(portfolios, nil).Once()

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
	handler, mockRepo := setupPortfolioHandler(t)

	p := &portfolio.Portfolio{ID: 1, Name: "Test", Currency: "EUR", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	mockRepo.On("GetByID", mock.Anything, int64(1)).Return(p, nil).Once()

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

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
	handler, mockRepo := setupPortfolioHandler(t)

	mockRepo.On("GetByID", mock.Anything, int64(999)).Return(nil, portfolio.ErrNotFound).Once()

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGet_InvalidID(t *testing.T) {
	handler, _ := setupPortfolioHandler(t)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/abc", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdate_Success(t *testing.T) {
	handler, mockRepo := setupPortfolioHandler(t)

	old := &portfolio.Portfolio{ID: 1, Name: "Old", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	mockRepo.On("GetByID", mock.Anything, int64(1)).Return(old, nil).Once()
	mockRepo.On("GetByName", mock.Anything, "New Name").Return(nil, portfolio.ErrNotFound).Once()
	mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*portfolio.Portfolio")).Return(func(_ context.Context, p *portfolio.Portfolio) error {
		p.Name = "New Name"
		return nil
	})

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

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
	handler, mockRepo := setupPortfolioHandler(t)

	mockRepo.On("Delete", mock.Anything, int64(1)).Return(nil).Once()

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/portfolios/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	handler, mockRepo := setupPortfolioHandler(t)

	mockRepo.On("Delete", mock.Anything, int64(999)).Return(portfolio.ErrNotFound).Once()

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestErrorResponseFormat(t *testing.T) {
	handler, mockRepo := setupPortfolioHandler(t)

	mockRepo.On("GetByID", mock.Anything, int64(999)).Return(nil, portfolio.ErrNotFound).Once()

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

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
