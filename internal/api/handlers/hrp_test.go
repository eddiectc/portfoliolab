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
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// testHrpService is a minimal in-memory mock service for handler tests.
type testHrpService struct {
	computeErr       error
	computeResult    *hierarchicalriskparity.ServiceResult
	candidateSymbols []string
	candidatesErr    error
	portfolioSymbols map[int64][]string
	portfolioErr     error
	modelSymbols     map[int64][]string
	modelErr         error
}

func newTestHrpService() *testHrpService {
	return &testHrpService{
		portfolioSymbols: make(map[int64][]string),
		modelSymbols:     make(map[int64][]string),
	}
}

func (s *testHrpService) ComputeHrp(_ context.Context, _ hierarchicalriskparity.ComputeHrpRequest) (*hierarchicalriskparity.ServiceResult, error) {
	if s.computeErr != nil {
		return nil, s.computeErr
	}
	return s.computeResult, nil
}

func (s *testHrpService) GetCandidateSymbols(_ context.Context) ([]string, error) {
	if s.candidatesErr != nil {
		return nil, s.candidatesErr
	}
	return s.candidateSymbols, nil
}

func (s *testHrpService) GetSymbolsFromPortfolio(_ context.Context, id int64) ([]string, error) {
	if s.portfolioErr != nil {
		return nil, s.portfolioErr
	}
	syms, ok := s.portfolioSymbols[id]
	if !ok {
		return []string{}, nil
	}
	return syms, nil
}

func (s *testHrpService) GetSymbolsFromModelPortfolio(_ context.Context, id int64) ([]string, error) {
	if s.modelErr != nil {
		return nil, s.modelErr
	}
	syms, ok := s.modelSymbols[id]
	if !ok {
		return nil, hierarchicalriskparity.ErrInsufficientData
	}
	return syms, nil
}

func setupHrpHandler(t *testing.T) (*HrpHandler, *testHrpService) {
	t.Helper()
	svc := newTestHrpService()
	return NewHrpHandler(svc), svc
}

func makeTestHrpResult() *hierarchicalriskparity.ServiceResult {
	return &hierarchicalriskparity.ServiceResult{
		Result: &hierarchicalriskparity.HrpResult{
			Allocations: []hierarchicalriskparity.HrpAllocation{
				{
					Method:  "single",
					Weights: map[string]float64{"AAPL": 0.4, "MSFT": 0.6},
				},
				{
					Method:  "complete",
					Weights: map[string]float64{"AAPL": 0.5, "MSFT": 0.5},
				},
				{
					Method:  "average",
					Weights: map[string]float64{"AAPL": 0.45, "MSFT": 0.55},
				},
				{
					Method:  "ward",
					Weights: map[string]float64{"AAPL": 0.52, "MSFT": 0.48},
				},
			},
			Symbols:     []string{"AAPL", "MSFT"},
			TradingDays: 756,
			ComputedAt:  time.Now().UTC(),
		},
		Warnings: []string{"FX rate EUR/USD: converted"},
	}
}

// --- HandleComputeHrp tests ---

func TestHrpHandleComputeHrp_Success(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.computeResult = makeTestHrpResult()

	body := `{"symbols": ["AAPL", "MSFT"], "period": "3Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleComputeHrp(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp computeHrpResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(resp.Result.Allocations) != 4 {
		t.Errorf("expected 4 allocations, got %d", len(resp.Result.Allocations))
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(resp.Warnings))
	}
}

func TestHrpHandleComputeHrp_DefaultPeriod(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.computeResult = makeTestHrpResult()

	body := `{"symbols": ["AAPL", "MSFT"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleComputeHrp(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHrpHandleComputeHrp_WithBaseCurrency(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.computeResult = makeTestHrpResult()

	body := `{"symbols": ["AAPL", "MSFT"], "period": "3Y", "base_currency": "EUR"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleComputeHrp(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHrpHandleComputeHrp_Validation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid body",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "single symbol",
			body:       `{"symbols": ["AAPL"]}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INSUFFICIENT_SYMBOLS",
		},
		{
			name:       "too many symbols",
			body:       `{"symbols": ["A","B","C","D","E","F","G","H","I","J","K","L","M","N","O","P","Q","R","S","T","U"]}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "TOO_MANY_SYMBOLS",
		},
		{
			name:       "invalid period",
			body:       `{"symbols": ["AAPL", "MSFT"], "period": "10Y"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_PERIOD",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := setupHrpHandler(t)

			req := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()

			handler.HandleComputeHrp(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("expected %d, got %d", tc.wantStatus, w.Code)
			}

			var errResp APIError
			json.NewDecoder(w.Body).Decode(&errResp)
			if errResp.Code != tc.wantCode {
				t.Errorf("expected code %q, got %q", tc.wantCode, errResp.Code)
			}
		})
	}
}

func TestHrpHandleComputeHrp_ServiceError(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.computeErr = hierarchicalriskparity.ErrInsufficientData

	body := `{"symbols": ["AAPL", "MSFT"], "period": "3Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleComputeHrp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INSUFFICIENT_DATA" {
		t.Errorf("expected INSUFFICIENT_DATA, got %q", errResp.Code)
	}
}

func TestHrpHandleComputeHrp_NumericalFailure(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.computeErr = hierarchicalriskparity.ErrNumericalFailure

	body := `{"symbols": ["AAPL", "MSFT"], "period": "3Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleComputeHrp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "NUMERICAL_FAILURE" {
		t.Errorf("expected NUMERICAL_FAILURE, got %q", errResp.Code)
	}
}

func TestHrpHandleComputeHrp_EmptyState(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.computeResult = &hierarchicalriskparity.ServiceResult{
		Result: &hierarchicalriskparity.HrpResult{
			Symbols:    []string{"AAPL", "MSFT"},
			ComputedAt: time.Now().UTC(),
			Message:    "No price data available for any candidate symbol.",
		},
		Warnings:        []string{"AAPL: no price data", "MSFT: no price data"},
		ExcludedSymbols: []string{"AAPL", "MSFT"},
	}

	body := `{"symbols": ["AAPL", "MSFT"], "period": "3Y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleComputeHrp(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp computeHrpResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Result.Message == "" {
		t.Error("expected empty-state message")
	}
	if len(resp.ExcludedSymbols) != 2 {
		t.Errorf("expected 2 excluded symbols, got %d", len(resp.ExcludedSymbols))
	}
}

// --- HandleGetCandidateSymbols tests ---

func TestHrpHandleGetCandidateSymbols_Success(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.candidateSymbols = []string{"AAPL", "MSFT", "GOOG"}

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/symbols", nil)
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

func TestHrpHandleGetCandidateSymbols_Empty(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.candidateSymbols = []string{}

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/symbols", nil)
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

func TestHrpHandleGetCandidateSymbols_Error(t *testing.T) {
	handler, svc := setupHrpHandler(t)
	svc.candidatesErr = hierarchicalriskparity.ErrInsufficientData

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleGetCandidateSymbols(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- HandleGetPortfolioSymbols tests ---

func TestHrpHandleGetPortfolioSymbols_Success(t *testing.T) {
	svc := newTestHrpService()
	svc.portfolioSymbols[1] = []string{"AAPL", "MSFT", "GOOG"}

	r := chi.NewRouter()
	NewHrpHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/portfolio/1/symbols", nil)
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

func TestHrpHandleGetPortfolioSymbols_EmptyPortfolio(t *testing.T) {
	svc := newTestHrpService()
	r := chi.NewRouter()
	NewHrpHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/portfolio/999/symbols", nil)
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

func TestHrpHandleGetPortfolioSymbols_InvalidID(t *testing.T) {
	svc := newTestHrpService()
	r := chi.NewRouter()
	NewHrpHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/portfolio/abc/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- HandleGetModelPortfolioSymbols tests ---

func TestHrpHandleGetModelPortfolioSymbols_Success(t *testing.T) {
	svc := newTestHrpService()
	svc.modelSymbols[1] = []string{"AAPL", "BND"}

	r := chi.NewRouter()
	NewHrpHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/model-portfolio/1/symbols", nil)
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

func TestHrpHandleGetModelPortfolioSymbols_NotFound(t *testing.T) {
	svc := newTestHrpService()
	r := chi.NewRouter()
	NewHrpHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/model-portfolio/999/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// The mock returns ErrInsufficientData for unknown IDs.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHrpHandleGetModelPortfolioSymbols_InvalidID(t *testing.T) {
	svc := newTestHrpService()
	r := chi.NewRouter()
	NewHrpHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/model-portfolio/abc/symbols", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Route registration tests ---

func TestHrpRoutesRegistered(t *testing.T) {
	svc := newTestHrpService()
	r := chi.NewRouter()
	NewHrpHandler(svc).RegisterRoutes(r)

	routes := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/hrp/compute"},
		{http.MethodPost, "/api/hrp/save"},
		{http.MethodGet, "/api/hrp/symbols"},
		{http.MethodGet, "/api/hrp/portfolio/1/symbols"},
		{http.MethodGet, "/api/hrp/model-portfolio/1/symbols"},
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

// --- HandleSaveAsModelPortfolio tests ---

func TestHrpHandleSaveAsModelPortfolio_Success(t *testing.T) {
	handler, _ := setupHrpHandler(t)
	creator := &testModelPortfolioCreator{}
	handler.WithModelPortfolioCreator(creator)

	body := `{
		"name": "HRP Ward Portfolio",
		"entries": [
			{"symbol": "AAPL", "weight": 0.52},
			{"symbol": "MSFT", "weight": 0.48}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/save", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleSaveAsModelPortfolio(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body: %s", w.Code, w.Body.String())
	}

	var resp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Name != "HRP Ward Portfolio" {
		t.Errorf("expected name 'HRP Ward Portfolio', got %q", resp.Name)
	}
	if len(resp.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Entries))
	}
	// Verify fraction weights were converted to percentages.
	if !resp.Entries[0].WeightPct.Equal(decimal.MustParse("52.00")) {
		t.Errorf("expected weight 52.00, got %s", resp.Entries[0].WeightPct.String())
	}
	if !resp.Entries[1].WeightPct.Equal(decimal.MustParse("48.00")) {
		t.Errorf("expected weight 48.00, got %s", resp.Entries[1].WeightPct.String())
	}
}

func TestHrpHandleSaveAsModelPortfolio_NoCreator(t *testing.T) {
	handler, _ := setupHrpHandler(t)
	// Don't set creator.

	body := `{"name": "Test", "entries": [{"symbol": "AAPL", "weight": 1.0}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/save", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleSaveAsModelPortfolio(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "NOT_CONFIGURED" {
		t.Errorf("expected NOT_CONFIGURED, got %q", errResp.Code)
	}
}

func TestHrpHandleSaveAsModelPortfolio_Validation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid body",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "empty name",
			body:       `{"name": "", "entries": [{"symbol": "AAPL", "weight": 1.0}]}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_NAME",
		},
		{
			name:       "whitespace name",
			body:       `{"name": "   ", "entries": [{"symbol": "AAPL", "weight": 1.0}]}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_NAME",
		},
		{
			name:       "empty entries",
			body:       `{"name": "Test", "entries": []}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "EMPTY_ENTRIES",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := setupHrpHandler(t)
			creator := &testModelPortfolioCreator{}
			handler.WithModelPortfolioCreator(creator)

			req := httptest.NewRequest(http.MethodPost, "/api/hrp/save", bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()

			handler.HandleSaveAsModelPortfolio(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("expected %d, got %d", tc.wantStatus, w.Code)
			}

			var errResp APIError
			json.NewDecoder(w.Body).Decode(&errResp)
			if errResp.Code != tc.wantCode {
				t.Errorf("expected code %q, got %q", tc.wantCode, errResp.Code)
			}
		})
	}
}

func TestHrpHandleSaveAsModelPortfolio_ServiceError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"name exists", modelportfolio.ErrNameExists, http.StatusConflict, "NAME_EXISTS"},
		{"weight sum not 100", modelportfolio.ErrWeightSumNot100, http.StatusBadRequest, "WEIGHT_SUM_NOT_100"},
		{"invalid weight", modelportfolio.ErrInvalidWeight, http.StatusBadRequest, "INVALID_WEIGHT"},
		{"duplicate symbol", modelportfolio.ErrDuplicateSymbol, http.StatusBadRequest, "DUPLICATE_SYMBOL"},
		{"empty entries", modelportfolio.ErrEmptyEntries, http.StatusBadRequest, "EMPTY_ENTRIES"},
		{"unknown error", assertErr{msg: "db down"}, http.StatusInternalServerError, "INTERNAL_ERROR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := setupHrpHandler(t)
			creator := &testModelPortfolioCreator{err: tc.err}
			handler.WithModelPortfolioCreator(creator)

			body := `{"name": "Test", "entries": [{"symbol": "AAPL", "weight": 1.0}]}`
			req := httptest.NewRequest(http.MethodPost, "/api/hrp/save", bytes.NewBufferString(body))
			w := httptest.NewRecorder()

			handler.HandleSaveAsModelPortfolio(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("expected %d, got %d", tc.wantStatus, w.Code)
			}

			var errResp APIError
			json.NewDecoder(w.Body).Decode(&errResp)
			if errResp.Code != tc.wantCode {
				t.Errorf("expected code %q, got %q", tc.wantCode, errResp.Code)
			}
		})
	}
}
