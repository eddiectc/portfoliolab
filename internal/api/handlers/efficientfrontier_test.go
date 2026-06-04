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

	"codeberg.org/eddiectc/portfoliolab/internal/domain/efficientfrontier"
)

// testEfficientFrontierService is a minimal in-memory mock service for handler tests.
type testEfficientFrontierService struct {
	computeErr       error
	computeResult    *efficientfrontier.ServiceResult
	candidateSymbols []string
	candidatesErr    error
	portfolioSymbols map[int64][]string
	portfolioErr     error
	modelSymbols     map[int64][]string
	modelErr         error
}

func newTestEfficientFrontierService() *testEfficientFrontierService {
	return &testEfficientFrontierService{
		portfolioSymbols: make(map[int64][]string),
		modelSymbols:     make(map[int64][]string),
	}
}

func (s *testEfficientFrontierService) ComputeFrontier(_ context.Context, _ efficientfrontier.ComputeFrontierRequest) (*efficientfrontier.ServiceResult, error) {
	if s.computeErr != nil {
		return nil, s.computeErr
	}
	return s.computeResult, nil
}

func (s *testEfficientFrontierService) GetCandidateSymbols(_ context.Context) ([]string, error) {
	if s.candidatesErr != nil {
		return nil, s.candidatesErr
	}
	return s.candidateSymbols, nil
}

func (s *testEfficientFrontierService) GetSymbolsFromPortfolio(_ context.Context, id int64) ([]string, error) {
	if s.portfolioErr != nil {
		return nil, s.portfolioErr
	}
	syms, ok := s.portfolioSymbols[id]
	if !ok {
		return []string{}, nil
	}
	return syms, nil
}

func (s *testEfficientFrontierService) GetSymbolsFromModelPortfolio(_ context.Context, id int64) ([]string, error) {
	if s.modelErr != nil {
		return nil, s.modelErr
	}
	syms, ok := s.modelSymbols[id]
	if !ok {
		return nil, efficientfrontier.ErrInsufficientData
	}
	return syms, nil
}

func setupEfficientFrontierHandler(t *testing.T) (*EfficientFrontierHandler, *testEfficientFrontierService) {
	t.Helper()
	svc := newTestEfficientFrontierService()
	return NewEfficientFrontierHandler(svc), svc
}

func makeTestFrontierResult() *efficientfrontier.ServiceResult {
	return &efficientfrontier.ServiceResult{
		Result: &efficientfrontier.FrontierResult{
			FrontierPoints: []efficientfrontier.FrontierPoint{
				{ReturnPct: 8.0, VolatilityPct: 10.0, SharpeRatio: 0.36, Weights: []float64{0.5, 0.5}},
				{ReturnPct: 12.0, VolatilityPct: 15.0, SharpeRatio: 0.40, Weights: []float64{0.7, 0.3}},
			},
			MaxSharpe: &efficientfrontier.OptimizedPortfolio{
				Name: "Max Sharpe", ReturnPct: 12.0, VolatilityPct: 15.0,
				SharpeRatio: 0.40, Weights: []float64{0.7, 0.3},
			},
			MinVariance: &efficientfrontier.OptimizedPortfolio{
				Name: "Min Variance", ReturnPct: 8.0, VolatilityPct: 10.0,
				SharpeRatio: 0.36, Weights: []float64{0.5, 0.5},
			},
			Symbols:     []string{"AAPL", "MSFT"},
			TradingDays: 252,
			ComputedAt:  time.Now().UTC(),
		},
		Warnings: []string{"FX rate EUR/USD: converted"},
	}
}

// --- HandleComputeFrontier tests ---

func TestEfficientFrontierHandleComputeFrontier_Success(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.computeResult = makeTestFrontierResult()

	body := `{"symbols": ["AAPL", "MSFT"], "period": "1Y", "risk_free_rate": 0.045}`
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleComputeFrontier(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp computeFrontierResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(resp.Result.FrontierPoints) != 2 {
		t.Errorf("expected 2 frontier points, got %d", len(resp.Result.FrontierPoints))
	}
	if resp.Result.MaxSharpe == nil {
		t.Error("expected max Sharpe portfolio")
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(resp.Warnings))
	}
}

func TestEfficientFrontierHandleComputeFrontier_DefaultPeriod(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.computeResult = makeTestFrontierResult()

	body := `{"symbols": ["AAPL", "MSFT"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleComputeFrontier(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestEfficientFrontierHandleComputeFrontier_Validation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "invalid body",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "too few symbols",
			body:       `{"symbols": ["AAPL"]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "too many symbols",
			body:       `{"symbols": ["A","B","C","D","E","F","G","H","I","J","K"]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid period",
			body:       `{"symbols": ["AAPL", "MSFT"], "period": "10Y"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := setupEfficientFrontierHandler(t)

			req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()

			handler.HandleComputeFrontier(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("expected %d, got %d", tc.wantStatus, w.Code)
			}
		})
	}
}

func TestEfficientFrontierHandleComputeFrontier_ServiceError(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.computeErr = efficientfrontier.ErrInsufficientData

	body := `{"symbols": ["AAPL", "MSFT"], "period": "1Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleComputeFrontier(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INSUFFICIENT_DATA" {
		t.Errorf("expected INSUFFICIENT_DATA, got %q", errResp.Code)
	}
}

func TestEfficientFrontierHandleComputeFrontier_SingularMatrix(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.computeErr = efficientfrontier.ErrSingularMatrix

	body := `{"symbols": ["AAPL", "MSFT"], "period": "1Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleComputeFrontier(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "SINGULAR_MATRIX" {
		t.Errorf("expected SINGULAR_MATRIX, got %q", errResp.Code)
	}
}

func TestEfficientFrontierHandleComputeFrontier_NumericalFailure(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.computeErr = efficientfrontier.ErrNumericalFailure

	body := `{"symbols": ["AAPL", "MSFT"], "period": "1Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleComputeFrontier(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "NUMERICAL_FAILURE" {
		t.Errorf("expected NUMERICAL_FAILURE, got %q", errResp.Code)
	}
}

func TestEfficientFrontierHandleComputeFrontier_EmptyCandidateSet(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.computeResult = &efficientfrontier.ServiceResult{
		Result: &efficientfrontier.FrontierResult{
			Symbols:    []string{"AAPL", "MSFT"},
			ComputedAt: time.Now().UTC(),
			Message:    "No price data available for any candidate symbol.",
		},
		Warnings:        []string{"AAPL: no price data", "MSFT: no price data"},
		ExcludedSymbols: []string{"AAPL", "MSFT"},
	}

	body := `{"symbols": ["AAPL", "MSFT"], "period": "1Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleComputeFrontier(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp computeFrontierResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Result.Message == "" {
		t.Error("expected empty-state message")
	}
	if len(resp.ExcludedSymbols) != 2 {
		t.Errorf("expected 2 excluded symbols, got %d", len(resp.ExcludedSymbols))
	}
}

// --- HandleGetCandidateSymbols tests ---

func TestEfficientFrontierHandleGetCandidateSymbols_Success(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.candidateSymbols = []string{"AAPL", "MSFT", "GOOG"}

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleGetCandidateSymbols(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp symbolsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Symbols) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(resp.Symbols))
	}
}

func TestEfficientFrontierHandleGetCandidateSymbols_Empty(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.candidateSymbols = []string{}

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleGetCandidateSymbols(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp symbolsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Symbols == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestEfficientFrontierHandleGetCandidateSymbols_Error(t *testing.T) {
	handler, svc := setupEfficientFrontierHandler(t)
	svc.candidatesErr = efficientfrontier.ErrInsufficientData

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleGetCandidateSymbols(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- HandleGetPortfolioSymbols tests ---

func TestEfficientFrontierHandleGetPortfolioSymbols_Success(t *testing.T) {
	svc := newTestEfficientFrontierService()
	svc.portfolioSymbols[1] = []string{"AAPL", "MSFT", "GOOG"}

	r := chi.NewRouter()
	NewEfficientFrontierHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/portfolio/1/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp symbolsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Symbols) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(resp.Symbols))
	}
}

func TestEfficientFrontierHandleGetPortfolioSymbols_EmptyPortfolio(t *testing.T) {
	svc := newTestEfficientFrontierService()
	r := chi.NewRouter()
	NewEfficientFrontierHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/portfolio/999/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp symbolsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Symbols == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestEfficientFrontierHandleGetPortfolioSymbols_InvalidID(t *testing.T) {
	svc := newTestEfficientFrontierService()
	r := chi.NewRouter()
	NewEfficientFrontierHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/portfolio/abc/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- HandleGetModelPortfolioSymbols tests ---

func TestEfficientFrontierHandleGetModelPortfolioSymbols_Success(t *testing.T) {
	svc := newTestEfficientFrontierService()
	svc.modelSymbols[1] = []string{"AAPL", "BND"}

	r := chi.NewRouter()
	NewEfficientFrontierHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/model-portfolio/1/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp symbolsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(resp.Symbols))
	}
}

func TestEfficientFrontierHandleGetModelPortfolioSymbols_NotFound(t *testing.T) {
	svc := newTestEfficientFrontierService()
	r := chi.NewRouter()
	NewEfficientFrontierHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/model-portfolio/999/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// The mock returns ErrInsufficientData for unknown IDs.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestEfficientFrontierHandleGetModelPortfolioSymbols_InvalidID(t *testing.T) {
	svc := newTestEfficientFrontierService()
	r := chi.NewRouter()
	NewEfficientFrontierHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/model-portfolio/abc/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Route registration tests ---

func TestEfficientFrontierRoutesRegistered(t *testing.T) {
	svc := newTestEfficientFrontierService()
	r := chi.NewRouter()
	NewEfficientFrontierHandler(svc).RegisterRoutes(r)

	routes := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/efficient-frontier/compute"},
		{http.MethodGet, "/api/efficient-frontier/symbols"},
		{http.MethodGet, "/api/efficient-frontier/portfolio/1/symbols"},
		{http.MethodGet, "/api/efficient-frontier/model-portfolio/1/symbols"},
	}

	for _, tc := range routes {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		// Routes should not return 404.
		if w.Code == http.StatusNotFound {
			t.Errorf("expected route %s %s to be registered, got 404", tc.method, tc.path)
		}
	}
}
