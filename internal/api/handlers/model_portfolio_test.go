package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// testModelPortfolioService is a minimal in-memory mock service for model portfolio handler tests.
type testModelPortfolioService struct {
	portfolios map[int64]*modelportfolio.ModelPortfolio
	names      map[string]int64 // name -> id
	nextID     int64
}

func newTestModelPortfolioService() *testModelPortfolioService {
	return &testModelPortfolioService{
		portfolios: make(map[int64]*modelportfolio.ModelPortfolio),
		names:      make(map[string]int64),
		nextID:     1,
	}
}

func (s *testModelPortfolioService) Create(_ context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
	// Validate name.
	if req.Name == "" || len(req.Name) > 100 {
		return modelportfolio.ModelPortfolio{}, modelportfolio.ErrInvalidName
	}
	// Validate entries (mirrors real service validation).
	if err := modelportfolio.ValidateEntries(req.Entries); err != nil {
		return modelportfolio.ModelPortfolio{}, err
	}
	// Check name uniqueness.
	if _, exists := s.names[req.Name]; exists {
		return modelportfolio.ModelPortfolio{}, modelportfolio.ErrNameExists
	}

	s.nextID++
	mp := modelportfolio.ModelPortfolio{
		ID:      s.nextID,
		Name:    req.Name,
		Entries: req.Entries,
	}
	s.portfolios[mp.ID] = &mp
	s.names[mp.Name] = mp.ID
	return mp, nil
}

func (s *testModelPortfolioService) Get(_ context.Context, id int64) (modelportfolio.ModelPortfolio, error) {
	mp, ok := s.portfolios[id]
	if !ok {
		return modelportfolio.ModelPortfolio{}, modelportfolio.ErrNotFound
	}
	cp := *mp
	return cp, nil
}

func (s *testModelPortfolioService) List(_ context.Context, limit, offset int) ([]modelportfolio.ModelPortfolio, error) {
	var result []modelportfolio.ModelPortfolio
	for _, mp := range s.portfolios {
		cp := *mp
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

func (s *testModelPortfolioService) Update(_ context.Context, id int64, req modelportfolio.UpdateRequest) (modelportfolio.ModelPortfolio, error) {
	// Validate entries (mirrors real service validation).
	if err := modelportfolio.ValidateEntries(req.Entries); err != nil {
		return modelportfolio.ModelPortfolio{}, err
	}

	mp, ok := s.portfolios[id]
	if !ok {
		return modelportfolio.ModelPortfolio{}, modelportfolio.ErrNotFound
	}

	// Apply name change if provided.
	if req.Name != nil {
		// Check uniqueness of new name against other portfolios.
		if existingID, exists := s.names[*req.Name]; exists && existingID != id {
			return modelportfolio.ModelPortfolio{}, modelportfolio.ErrNameExists
		}
		delete(s.names, mp.Name)
		mp.Name = *req.Name
		s.names[mp.Name] = id
	}

	mp.Entries = req.Entries
	return *mp, nil
}

func (s *testModelPortfolioService) Delete(_ context.Context, id int64) error {
	mp, ok := s.portfolios[id]
	if !ok {
		return modelportfolio.ErrNotFound
	}
	delete(s.names, mp.Name)
	delete(s.portfolios, id)
	return nil
}

func setupModelPortfolioHandler(t *testing.T) (*ModelPortfolioHandler, *testModelPortfolioService) {
	t.Helper()
	svc := newTestModelPortfolioService()
	return NewModelPortfolioHandler(svc), svc
}

// --- Create tests ---

func TestModelPortfolioHandleCreate_Success(t *testing.T) {
	handler, _ := setupModelPortfolioHandler(t)

	body := `{"name": "60/40 Portfolio", "entries": [{"symbol": "AAPL", "weight_pct": "60.00"}, {"symbol": "BND", "weight_pct": "40.00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var mp modelportfolio.ModelPortfolio
	_ = json.NewDecoder(w.Body).Decode(&mp)
	if mp.Name != "60/40 Portfolio" {
		t.Errorf("expected name '60/40 Portfolio', got %q", mp.Name)
	}
	if len(mp.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(mp.Entries))
	}
}

func TestModelPortfolioHandleCreate_InvalidBody(t *testing.T) {
	handler, _ := setupModelPortfolioHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestModelPortfolioHandleCreate_DuplicateName(t *testing.T) {
	handler, svc := setupModelPortfolioHandler(t)

	svc.portfolios[1] = &modelportfolio.ModelPortfolio{ID: 1, Name: "Existing", Entries: []modelportfolio.ModelPortfolioEntry{}}
	svc.names["Existing"] = 1

	body := `{"name": "Existing", "entries": [{"symbol": "AAPL", "weight_pct": "100.00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestModelPortfolioHandleCreate_EmptyEntries(t *testing.T) {
	handler, _ := setupModelPortfolioHandler(t)

	body := `{"name": "Empty", "entries": []}`
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestModelPortfolioHandleCreate_InvalidWeightSum(t *testing.T) {
	handler, _ := setupModelPortfolioHandler(t)

	body := `{"name": "Bad Weights", "entries": [{"symbol": "AAPL", "weight_pct": "50.00"}, {"symbol": "BND", "weight_pct": "30.00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- List tests ---

func TestModelPortfolioHandleList(t *testing.T) {
	handler, svc := setupModelPortfolioHandler(t)

	for i := 1; i <= 3; i++ {
		svc.portfolios[int64(i)] = &modelportfolio.ModelPortfolio{
			ID:   int64(i),
			Name: "MP" + string(rune('A'+i-1)),
			Entries: []modelportfolio.ModelPortfolioEntry{
				{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)},
			},
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/model-portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var got []modelportfolio.ModelPortfolio
	_ = json.NewDecoder(w.Body).Decode(&got)
	if len(got) != 3 {
		t.Errorf("expected 3 model portfolios, got %d", len(got))
	}
}

func TestModelPortfolioHandleList_Empty(t *testing.T) {
	handler, _ := setupModelPortfolioHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/model-portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var portfolios []modelportfolio.ModelPortfolio
	_ = json.NewDecoder(w.Body).Decode(&portfolios)
	if portfolios == nil {
		t.Error("expected empty array, got nil")
	}
	if len(portfolios) != 0 {
		t.Errorf("expected 0 model portfolios, got %d", len(portfolios))
	}
}

// --- Get tests ---

func TestModelPortfolioHandleGet_Success(t *testing.T) {
	svc := newTestModelPortfolioService()
	svc.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:   1,
		Name: "Test Portfolio",
		Entries: []modelportfolio.ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustNew(6000, 2)},
			{Symbol: "BND", WeightPct: decimal.MustNew(4000, 2)},
		},
	}

	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/model-portfolios/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var got modelportfolio.ModelPortfolio
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.Name != "Test Portfolio" {
		t.Errorf("expected 'Test Portfolio', got %q", got.Name)
	}
	if len(got.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got.Entries))
	}
}

func TestModelPortfolioHandleGet_NotFound(t *testing.T) {
	svc := newTestModelPortfolioService()
	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/model-portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestModelPortfolioHandleGet_InvalidID(t *testing.T) {
	svc := newTestModelPortfolioService()
	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/model-portfolios/abc", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Update tests ---

func TestModelPortfolioHandleUpdate_Success(t *testing.T) {
	svc := newTestModelPortfolioService()
	svc.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:   1,
		Name: "Old Name",
		Entries: []modelportfolio.ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)},
		},
	}
	svc.names["Old Name"] = 1

	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	body := `{"name": "New Name", "entries": [{"symbol": "AAPL", "weight_pct": "100.00"}]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/model-portfolios/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var got modelportfolio.ModelPortfolio
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", got.Name)
	}
}

func TestModelPortfolioHandleUpdate_EntriesOnly(t *testing.T) {
	svc := newTestModelPortfolioService()
	svc.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:   1,
		Name: "Keep Name",
		Entries: []modelportfolio.ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)},
		},
	}
	svc.names["Keep Name"] = 1

	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	body := `{"entries": [{"symbol": "AAPL", "weight_pct": "60.00"}, {"symbol": "BND", "weight_pct": "40.00"}]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/model-portfolios/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var got modelportfolio.ModelPortfolio
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.Name != "Keep Name" {
		t.Errorf("expected name preserved 'Keep Name', got %q", got.Name)
	}
	if len(got.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got.Entries))
	}
}

// --- Delete tests ---

func TestModelPortfolioHandleDelete(t *testing.T) {
	svc := newTestModelPortfolioService()
	svc.portfolios[1] = &modelportfolio.ModelPortfolio{ID: 1, Name: "Test", Entries: []modelportfolio.ModelPortfolioEntry{}}
	svc.names["Test"] = 1

	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/model-portfolios/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestModelPortfolioHandleDelete_NotFound(t *testing.T) {
	svc := newTestModelPortfolioService()
	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/model-portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Error response format ---

func TestModelPortfolioErrorResponseFormat(t *testing.T) {
	svc := newTestModelPortfolioService()
	r := chi.NewRouter()
	NewModelPortfolioHandler(svc).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/model-portfolios/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MODEL_PORTFOLIO_NOT_FOUND" {
		t.Errorf("expected error code MODEL_PORTFOLIO_NOT_FOUND, got %q", errResp.Code)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestModelPortfolioDuplicateSymbolError(t *testing.T) {
	handler, _ := setupModelPortfolioHandler(t)

	body := `{"name": "Dup Symbols", "entries": [{"symbol": "AAPL", "weight_pct": "50.00"}, {"symbol": "AAPL", "weight_pct": "50.00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "DUPLICATE_SYMBOL" {
		t.Errorf("expected error code DUPLICATE_SYMBOL, got %q", errResp.Code)
	}
}
