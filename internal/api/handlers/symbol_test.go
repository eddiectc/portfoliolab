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

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// --- Test helpers ---

// testSMRepo is a minimal in-memory mock repo for symbol mapping handler tests.
type testSMRepo struct {
	mappings      map[int64]*symbolmapping.SymbolMapping
	byInternal    map[string]int64
	brokerSymbols map[int64][]*symbolmapping.BrokerSymbol
	byBroker      map[string]*symbolmapping.BrokerSymbol
	nextID        int64
	inUseIDs      map[int64]bool
}

func newTestSMRepo() *testSMRepo {
	return &testSMRepo{
		mappings:      make(map[int64]*symbolmapping.SymbolMapping),
		byInternal:    make(map[string]int64),
		brokerSymbols: make(map[int64][]*symbolmapping.BrokerSymbol),
		byBroker:      make(map[string]*symbolmapping.BrokerSymbol),
		nextID:        1,
		inUseIDs:      make(map[int64]bool),
	}
}

func (r *testSMRepo) Create(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	r.nextID++
	sm.ID = r.nextID
	cp := *sm
	cp.BrokerSymbols = nil
	r.mappings[sm.ID] = &cp
	r.byInternal[sm.InternalSymbol] = sm.ID
	return nil
}

func (r *testSMRepo) GetByID(_ context.Context, id int64) (*symbolmapping.SymbolMapping, error) {
	sm, ok := r.mappings[id]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	cp := *sm
	if len(sm.BrokerSymbols) > 0 {
		cp.BrokerSymbols = make([]symbolmapping.BrokerSymbol, len(sm.BrokerSymbols))
		copy(cp.BrokerSymbols, sm.BrokerSymbols)
	}
	return &cp, nil
}

func (r *testSMRepo) GetByInternalSymbol(_ context.Context, internalSymbol string) (*symbolmapping.SymbolMapping, error) {
	id, ok := r.byInternal[internalSymbol]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	return r.GetByID(context.Background(), id)
}

func (r *testSMRepo) GetAll(_ context.Context, limit, offset int) ([]symbolmapping.SymbolMapping, error) {
	var result []symbolmapping.SymbolMapping
	for _, sm := range r.mappings {
		cp := *sm
		if len(sm.BrokerSymbols) > 0 {
			cp.BrokerSymbols = make([]symbolmapping.BrokerSymbol, len(sm.BrokerSymbols))
			copy(cp.BrokerSymbols, sm.BrokerSymbols)
		}
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

func (r *testSMRepo) Update(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	old, ok := r.mappings[sm.ID]
	if !ok {
		return symbolmapping.ErrNotFound
	}
	if old.InternalSymbol != sm.InternalSymbol {
		delete(r.byInternal, old.InternalSymbol)
		r.byInternal[sm.InternalSymbol] = sm.ID
	}
	smCopy := *sm
	smCopy.BrokerSymbols = old.BrokerSymbols
	r.mappings[sm.ID] = &smCopy
	return nil
}

func (r *testSMRepo) Delete(_ context.Context, id int64) error {
	sm, ok := r.mappings[id]
	if !ok {
		return symbolmapping.ErrNotFound
	}
	delete(r.byInternal, sm.InternalSymbol)
	delete(r.mappings, id)
	return nil
}

func (r *testSMRepo) AddBrokerSymbol(_ context.Context, symbolMappingID int64, brokerName, brokerSymbol string) error {
	bs := &symbolmapping.BrokerSymbol{
		ID:           r.nextID + 1000,
		SymbolID:     symbolMappingID,
		BrokerName:   brokerName,
		BrokerSymbol: brokerSymbol,
		CreatedAt:    time.Now(),
	}
	r.nextID++
	r.brokerSymbols[symbolMappingID] = append(r.brokerSymbols[symbolMappingID], bs)
	key := fmt.Sprintf("%s|%s", brokerName, brokerSymbol)
	r.byBroker[key] = bs
	if sm, ok := r.mappings[symbolMappingID]; ok {
		sm.BrokerSymbols = append(sm.BrokerSymbols, *bs)
	}
	return nil
}

func (r *testSMRepo) GetBrokerSymbolByBroker(_ context.Context, brokerName, brokerSymbol string) (*symbolmapping.BrokerSymbol, error) {
	key := fmt.Sprintf("%s|%s", brokerName, brokerSymbol)
	bs, ok := r.byBroker[key]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	cp := *bs
	return &cp, nil
}

func (r *testSMRepo) HasReferencingTransactions(_ context.Context, id int64) (bool, error) {
	return r.inUseIDs[id], nil
}

func setupSymbolHandler(t *testing.T) (*SymbolHandler, *testSMRepo) {
	t.Helper()
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)
	return NewSymbolHandler(svc, nil), repo
}

// --- Create Tests ---

func TestSymbolHandleCreate_Success(t *testing.T) {
	handler, _ := setupSymbolHandler(t)

	body := `{"internal_symbol": "AAPL", "market_data_symbol": "AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", sm.InternalSymbol)
	}
	if sm.MarketDataSymbol != "AAPL" {
		t.Errorf("expected market data symbol 'AAPL', got %q", sm.MarketDataSymbol)
	}
}

func TestSymbolHandleCreate_WithBrokerSymbols(t *testing.T) {
	handler, _ := setupSymbolHandler(t)

	body := `{"internal_symbol": "AAPL", "market_data_symbol": "AAPL", "broker_symbols": [{"broker_name": "IBKR", "broker_symbol": "AAPL.US"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if len(sm.BrokerSymbols) != 1 {
		t.Errorf("expected 1 broker symbol, got %d", len(sm.BrokerSymbols))
	}
}

func TestSymbolHandleCreate_InvalidBody(t *testing.T) {
	handler, _ := setupSymbolHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSymbolHandleCreate_EmptyInternalSymbol(t *testing.T) {
	handler, _ := setupSymbolHandler(t)

	body := `{"internal_symbol": "", "market_data_symbol": "AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSymbolHandleCreate_WithBenchmark_ReturnsTrue(t *testing.T) {
	handler, _ := setupSymbolHandler(t)

	body := `{"internal_symbol": "^GSPC", "market_data_symbol": "^GSPC", "is_benchmark": true}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if !sm.IsBenchmark {
		t.Error("expected is_benchmark to be true")
	}
}

func TestSymbolHandleCreate_WithoutBenchmark_ReturnsFalse(t *testing.T) {
	handler, _ := setupSymbolHandler(t)

	body := `{"internal_symbol": "AAPL", "market_data_symbol": "AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if sm.IsBenchmark {
		t.Error("expected is_benchmark to be false by default")
	}
}

func TestSymbolHandleCreate_DuplicateInternalSymbol(t *testing.T) {
	handler, repo := setupSymbolHandler(t)

	// Seed existing mapping
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	body := `{"internal_symbol": "AAPL", "market_data_symbol": "AAPL.LON"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreate(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// --- List Tests ---

func TestSymbolHandleList(t *testing.T) {
	handler, repo := setupSymbolHandler(t)

	for i := 1; i <= 3; i++ {
		id := int64(i)
		internal := fmt.Sprintf("SYM%d", i)
		repo.mappings[id] = &symbolmapping.SymbolMapping{ID: id, InternalSymbol: internal, MarketDataSymbol: internal}
		repo.byInternal[internal] = id
	}

	req := httptest.NewRequest(http.MethodGet, "/api/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var mappings []symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&mappings)
	if len(mappings) != 3 {
		t.Errorf("expected 3 mappings, got %d", len(mappings))
	}
}

func TestSymbolHandleList_Empty(t *testing.T) {
	handler, _ := setupSymbolHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var mappings []symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&mappings)
	if mappings == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestSymbolHandleList_Pagination(t *testing.T) {
	handler, repo := setupSymbolHandler(t)

	for i := 1; i <= 5; i++ {
		id := int64(i)
		internal := fmt.Sprintf("SYM%d", i)
		repo.mappings[id] = &symbolmapping.SymbolMapping{ID: id, InternalSymbol: internal, MarketDataSymbol: internal}
		repo.byInternal[internal] = id
	}

	req := httptest.NewRequest(http.MethodGet, "/api/symbols?limit=2", nil)
	w := httptest.NewRecorder()

	handler.HandleList(w, req)

	var mappings []symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&mappings)
	if len(mappings) != 2 {
		t.Errorf("expected 2 mappings with limit=2, got %d", len(mappings))
	}
}

// --- Get Tests ---

func TestSymbolHandleGet_Success(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected 'AAPL', got %q", sm.InternalSymbol)
	}
}

func TestSymbolHandleGet_NotFound(t *testing.T) {
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSymbolHandleGet_InvalidID(t *testing.T) {
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/abc", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Update Tests ---

func TestSymbolHandleUpdate_Success(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"market_data_symbol": "AAPL.LON"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/symbols/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if sm.MarketDataSymbol != "AAPL.LON" {
		t.Errorf("expected market data symbol 'AAPL.LON', got %q", sm.MarketDataSymbol)
	}
}

func TestSymbolHandleUpdate_InternalSymbol(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "APPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["APPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"internal_symbol": "AAPL"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/symbols/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", sm.InternalSymbol)
	}
}

func TestSymbolHandleUpdate_DuplicateInternalSymbol(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "APPL", MarketDataSymbol: "AAPL"}
	repo.mappings[2] = &symbolmapping.SymbolMapping{ID: 2, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["APPL"] = 1
	repo.byInternal["AAPL"] = 2
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"internal_symbol": "AAPL"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/symbols/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestSymbolHandleUpdate_EnableBenchmark(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "^GSPC", MarketDataSymbol: "^GSPC", IsBenchmark: false}
	repo.byInternal["^GSPC"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"is_benchmark": true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/symbols/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if !sm.IsBenchmark {
		t.Error("expected is_benchmark to be true after enabling")
	}
}

func TestSymbolHandleUpdate_DisableBenchmark(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "^GSPC", MarketDataSymbol: "^GSPC", IsBenchmark: true}
	repo.byInternal["^GSPC"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"is_benchmark": false}`
	req := httptest.NewRequest(http.MethodPatch, "/api/symbols/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sm symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&sm)
	if sm.IsBenchmark {
		t.Error("expected is_benchmark to be false after disabling")
	}
}

func TestSymbolHandleUpdate_InvalidSymbol(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"internal_symbol": ""}`
	req := httptest.NewRequest(http.MethodPatch, "/api/symbols/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSymbolHandleUpdate_NotFound(t *testing.T) {
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"market_data_symbol": "NEW"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/symbols/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Delete Tests ---

func TestSymbolHandleDelete_Success(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/symbols/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestSymbolHandleDelete_NotFound(t *testing.T) {
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/symbols/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSymbolHandleDelete_InUse(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	repo.inUseIDs[1] = true
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/api/symbols/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// --- AddBrokerSymbol Tests ---

func TestSymbolHandleAddBrokerSymbol_Success(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"broker_name": "IBKR", "broker_symbol": "AAPL.US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols/1/broker-symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestSymbolHandleAddBrokerSymbol_NotFound(t *testing.T) {
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"broker_name": "IBKR", "broker_symbol": "AAPL.US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols/999/broker-symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSymbolHandleAddBrokerSymbol_DuplicateBrokerSymbol(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.mappings[2] = &symbolmapping.SymbolMapping{ID: 2, InternalSymbol: "AAPL-ALT", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	repo.byInternal["AAPL-ALT"] = 2
	// Broker symbol already mapped to ID 1
	repo.byBroker["IBKR|AAPL.US"] = &symbolmapping.BrokerSymbol{ID: 1001, SymbolID: 1, BrokerName: "IBKR", BrokerSymbol: "AAPL.US"}
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	// Try to add same broker symbol to mapping 2
	body := `{"broker_name": "IBKR", "broker_symbol": "AAPL.US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols/2/broker-symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestSymbolHandleAddBrokerSymbol_InvalidBody(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/api/symbols/1/broker-symbols", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSymbolHandleAddBrokerSymbol_InvalidID(t *testing.T) {
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	body := `{"broker_name": "IBKR", "broker_symbol": "AAPL.US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/symbols/abc/broker-symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Error Response Format ---

func TestSymbolErrorResponseFormat(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	r := chi.NewRouter()
	NewSymbolHandler(svc, nil).RegisterRoutes(r)

	// Test SYMBOL_NOT_FOUND
	req := httptest.NewRequest(http.MethodGet, "/api/symbols/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "SYMBOL_NOT_FOUND" {
		t.Errorf("expected error code SYMBOL_NOT_FOUND, got %q", errResp.Code)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}

	// Test INVALID_SYMBOL
	body := `{"internal_symbol": "", "market_data_symbol": "X"}`
	req = httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_SYMBOL" {
		t.Errorf("expected error code INVALID_SYMBOL, got %q", errResp.Code)
	}

	// Test INTERNAL_SYMBOL_EXISTS
	body = `{"internal_symbol": "AAPL", "market_data_symbol": "AAPL"}`
	req = httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INTERNAL_SYMBOL_EXISTS" {
		t.Errorf("expected error code INTERNAL_SYMBOL_EXISTS, got %q", errResp.Code)
	}

	// Test SYMBOL_IN_USE
	repo.inUseIDs[1] = true
	req = httptest.NewRequest(http.MethodDelete, "/api/symbols/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "SYMBOL_IN_USE" {
		t.Errorf("expected error code SYMBOL_IN_USE, got %q", errResp.Code)
	}
}

// --- Preview Tests ---

func TestHandlePreview_MissingSymbol(t *testing.T) {
	repo := newTestSMRepo()
	svc := symbolmapping.NewService(repo)
	handler := NewSymbolHandler(svc, nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["code"] != "MISSING_SYMBOL" {
		t.Errorf("expected error code MISSING_SYMBOL, got %q", errResp["code"])
	}
}

func TestHandlePreview_NoFetcher(t *testing.T) {
	repo := newTestSMRepo()
	// Service without quote fetcher
	svc := symbolmapping.NewService(repo)
	handler := NewSymbolHandler(svc, nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/preview?symbol=AAPL", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["code"] != "PREVIEW_FAILED" {
		t.Errorf("expected error code PREVIEW_FAILED, got %q", errResp["code"])
	}
}

// testQuoteFetcher is a mock MarketDataFetcher for handler preview tests.
// It maps requested symbols to market data, allowing simulation of auto-correction.
type testQuoteFetcher struct {
	data map[string]*market.MarketData
	err  error
}

func (f *testQuoteFetcher) FetchQuote(_ context.Context, symbol string) (*market.MarketData, error) {
	if f.err != nil {
		return nil, f.err
	}
	d, ok := f.data[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	cp := *d
	return &cp, nil
}

func (f *testQuoteFetcher) FetchFxRate(_ context.Context, baseCurrency, quoteCurrency string) (*market.MarketData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *testQuoteFetcher) FetchQuotesBatch(_ context.Context, symbols []string) map[string]*market.MarketData {
	result := make(map[string]*market.MarketData)
	if f.err != nil {
		return result
	}
	for _, sym := range symbols {
		if d, ok := f.data[sym]; ok {
			cp := *d
			result[sym] = &cp
		}
	}
	return result
}

func (f *testQuoteFetcher) FetchHistoricalPricesBatch(_ context.Context, _ []string, _, _ time.Time) (map[string][]market.HistoricalPrice, []string) {
	return nil, nil
}

func TestHandlePreview_SymbolsMatch(t *testing.T) {
	repo := newTestSMRepo()
	fetcher := &testQuoteFetcher{
		data: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(17850, 2), Currency: "USD", DataType: "stock", Source: "yahoo", Date: ""},
		},
	}
	svc := symbolmapping.NewService(repo, symbolmapping.WithMarketDataFetcher(fetcher))
	handler := NewSymbolHandler(svc, nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/preview?symbol=AAPL", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp PreviewResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got %q", resp.Symbol)
	}
	if resp.CorrectedSymbol != "" {
		t.Errorf("expected empty corrected_symbol when symbols match, got %q", resp.CorrectedSymbol)
	}
}

func TestHandlePreview_AutoCorrectedSymbol(t *testing.T) {
	repo := newTestSMRepo()
	// Simulate go-yfinance auto-correcting "AAP" → "AAPL"
	fetcher := &testQuoteFetcher{
		data: map[string]*market.MarketData{
			"AAP": {Symbol: "AAPL", Price: decimal.MustNew(17850, 2), Currency: "USD", DataType: "stock", Source: "yahoo", Date: ""},
		},
	}
	svc := symbolmapping.NewService(repo, symbolmapping.WithMarketDataFetcher(fetcher))
	handler := NewSymbolHandler(svc, nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/preview?symbol=AAP", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp PreviewResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got %q", resp.Symbol)
	}
	if resp.CorrectedSymbol != "AAPL" {
		t.Errorf("expected corrected_symbol AAPL, got %q", resp.CorrectedSymbol)
	}
}

func TestHandlePreview_CaseInsensitiveMatch(t *testing.T) {
	repo := newTestSMRepo()
	// Fetcher returns data with lowercase symbol for uppercase request —
	// this should NOT be flagged as auto-correction (same symbol, different case)
	fetcher := &testQuoteFetcher{
		data: map[string]*market.MarketData{
			"AAPL": {Symbol: "aapl", Price: decimal.MustNew(17850, 2), Currency: "USD", DataType: "stock", Source: "yahoo", Date: ""},
		},
	}
	svc := symbolmapping.NewService(repo, symbolmapping.WithMarketDataFetcher(fetcher))
	handler := NewSymbolHandler(svc, nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Request uppercase, get lowercase back — should NOT be flagged as correction
	req := httptest.NewRequest(http.MethodGet, "/api/symbols/preview?symbol=AAPL", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp PreviewResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.CorrectedSymbol != "" {
		t.Errorf("expected empty corrected_symbol for case-insensitive match, got %q", resp.CorrectedSymbol)
	}
}

func TestHandlePreview_FetchError(t *testing.T) {
	repo := newTestSMRepo()
	fetcher := &testQuoteFetcher{
		err: fmt.Errorf("network timeout"),
	}
	svc := symbolmapping.NewService(repo, symbolmapping.WithMarketDataFetcher(fetcher))
	handler := NewSymbolHandler(svc, nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/preview?symbol=INVALID", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["code"] != "PREVIEW_FAILED" {
		t.Errorf("expected error code PREVIEW_FAILED, got %q", errResp["code"])
	}
}

// --- Enriched Get Tests (with symbol details) ---

// testDetailsRepo is a minimal in-memory mock repo for symbol details handler tests.
type testDetailsRepo struct {
	byInternal map[string]*symbol.SymbolDetails
}

func newTestDetailsRepo() *testDetailsRepo {
	return &testDetailsRepo{byInternal: make(map[string]*symbol.SymbolDetails)}
}

func (r *testDetailsRepo) Upsert(_ context.Context, details *symbol.SymbolDetails) error {
	r.byInternal[details.InternalSymbol] = details
	return nil
}

func (r *testDetailsRepo) GetByInternalSymbol(_ context.Context, internalSymbol string) (*symbol.SymbolDetails, error) {
	d, ok := r.byInternal[internalSymbol]
	if !ok {
		return nil, symbols.ErrNotFound
	}
	cp := *d
	return &cp, nil
}

func (r *testDetailsRepo) ListStale(_ context.Context, _ time.Time) ([]symbol.StaleSymbol, error) {
	return nil, nil
}

func TestHandleGet_WithDetails(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	detailsRepo := newTestDetailsRepo()
	detailsRepo.byInternal["AAPL"] = &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		LongName:       "Apple Inc.",
		Exchange:       "NMS",
		Currency:       "USD",
		QuoteType:      "EQUITY",
		FetchedAt:      time.Now(),
	}
	detailsSvc := symbols.NewService(detailsRepo, nil)

	handler := NewSymbolHandler(svc, detailsSvc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SymbolGetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", resp.InternalSymbol)
	}
	if resp.SymbolDetails == nil {
		t.Fatal("expected non-nil symbol_details")
	}
	if resp.SymbolDetails.ShortName != "Apple Inc." {
		t.Errorf("expected short name 'Apple Inc.', got %q", resp.SymbolDetails.ShortName)
	}
	if resp.SymbolDetails.Exchange != "NMS" {
		t.Errorf("expected exchange 'NMS', got %q", resp.SymbolDetails.Exchange)
	}
}

func TestHandleGet_NoCachedDetails(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	detailsRepo := newTestDetailsRepo()
	// No details cached for AAPL
	detailsSvc := symbols.NewService(detailsRepo, nil)

	handler := NewSymbolHandler(svc, detailsSvc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SymbolGetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", resp.InternalSymbol)
	}
	if resp.SymbolDetails != nil {
		t.Error("expected nil symbol_details when no cache")
	}
}

func TestHandleGet_NoDetailsService(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	// No details service wired
	handler := NewSymbolHandler(svc, nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SymbolGetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", resp.InternalSymbol)
	}
	if resp.SymbolDetails != nil {
		t.Error("expected nil symbol_details when no details service")
	}
}

func TestHandleGet_WithETFDetalis(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "VWRL", MarketDataSymbol: "VWRL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["VWRL"] = 1
	svc := symbolmapping.NewService(repo)

	detailsRepo := newTestDetailsRepo()
	detailsRepo.byInternal["VWRL"] = &symbol.SymbolDetails{
		InternalSymbol: "VWRL",
		ShortName:      "Vanguard FTSE All-World",
		LongName:       "Vanguard FTSE All-World UCITS ETF",
		Exchange:       "ILS",
		Currency:       "GBP",
		QuoteType:      "ETF",
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 0.035},
			{Symbol: "MSFT", Name: "Microsoft Corp", Percent: 0.028},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 0.25},
			{Sector: "financial_services", Percent: 0.15},
		},
		AggregatePositions: &symbol.AggregatePositions{
			Stock: 0.99,
			Cash:  0.01,
		},
		FundProfile: &symbol.FundProfile{
			Family:             "Vanguard",
			LegalType:          "Exchange Traded Fund",
			TotalNetAssets:     5000000000,
			AnnualExpenseRatio: 0.0022,
		},
		FetchedAt: time.Now(),
	}
	detailsSvc := symbols.NewService(detailsRepo, nil)

	handler := NewSymbolHandler(svc, detailsSvc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SymbolGetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.SymbolDetails == nil {
		t.Fatal("expected non-nil symbol_details")
	}
	if resp.SymbolDetails.QuoteType != "ETF" {
		t.Errorf("expected quote type 'ETF', got %q", resp.SymbolDetails.QuoteType)
	}
	if len(resp.SymbolDetails.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(resp.SymbolDetails.TopHoldings))
	}
	if resp.SymbolDetails.TopHoldings[0].Symbol != "AAPL" {
		t.Errorf("expected first holding 'AAPL', got %q", resp.SymbolDetails.TopHoldings[0].Symbol)
	}
	if len(resp.SymbolDetails.SectorWeightings) != 2 {
		t.Fatalf("expected 2 sector weightings, got %d", len(resp.SymbolDetails.SectorWeightings))
	}
	if resp.SymbolDetails.AggregatePositions == nil {
		t.Fatal("expected non-nil aggregate_positions")
	}
	if resp.SymbolDetails.AggregatePositions.Stock != 0.99 {
		t.Errorf("expected stock position 0.99, got %f", resp.SymbolDetails.AggregatePositions.Stock)
	}
	if resp.SymbolDetails.FundProfile == nil {
		t.Fatal("expected non-nil fund_profile")
	}
	if resp.SymbolDetails.FundProfile.Family != "Vanguard" {
		t.Errorf("expected family 'Vanguard', got %q", resp.SymbolDetails.FundProfile.Family)
	}
}

// --- Geographic Allocation Tests ---

func TestHandleGet_GeographicAllocations_SortedDescending(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "VWRL", MarketDataSymbol: "VWRL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["VWRL"] = 1
	svc := symbolmapping.NewService(repo)

	detailsRepo := newTestDetailsRepo()
	// Insert in reverse order to verify sorting
	detailsRepo.byInternal["VWRL"] = &symbol.SymbolDetails{
		InternalSymbol: "VWRL",
		ShortName:      "Vanguard FTSE All-World",
		QuoteType:      "ETF",
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "Japan", Percent: 0.15},
			{Country: "United Kingdom", Percent: 0.05},
			{Country: "United States", Percent: 0.60},
		},
		FetchedAt: time.Now(),
	}
	detailsSvc := symbols.NewService(detailsRepo, nil)

	handler := NewSymbolHandler(svc, detailsSvc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SymbolGetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.SymbolDetails == nil {
		t.Fatal("expected non-nil symbol_details")
	}

	allocs := resp.SymbolDetails.GeographicAllocations
	if len(allocs) != 3 {
		t.Fatalf("expected 3 geographic allocations, got %d", len(allocs))
	}
	// Verify sorted by percent descending
	if allocs[0].Country != "United States" || allocs[0].Percent != 0.60 {
		t.Errorf("expected first allocation {United States, 0.60}, got {%s, %f}", allocs[0].Country, allocs[0].Percent)
	}
	if allocs[1].Country != "Japan" || allocs[1].Percent != 0.15 {
		t.Errorf("expected second allocation {Japan, 0.15}, got {%s, %f}", allocs[1].Country, allocs[1].Percent)
	}
	if allocs[2].Country != "United Kingdom" || allocs[2].Percent != 0.05 {
		t.Errorf("expected third allocation {United Kingdom, 0.05}, got {%s, %f}", allocs[2].Country, allocs[2].Percent)
	}
}

func TestHandleGet_GeographicAllocations_SingleElementStock(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	detailsRepo := newTestDetailsRepo()
	detailsRepo.byInternal["AAPL"] = &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		QuoteType:      "EQUITY",
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 1.0},
		},
		FetchedAt: time.Now(),
	}
	detailsSvc := symbols.NewService(detailsRepo, nil)

	handler := NewSymbolHandler(svc, detailsSvc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SymbolGetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.SymbolDetails == nil {
		t.Fatal("expected non-nil symbol_details")
	}

	allocs := resp.SymbolDetails.GeographicAllocations
	if len(allocs) != 1 {
		t.Fatalf("expected 1 geographic allocation, got %d", len(allocs))
	}
	if allocs[0].Country != "United States" {
		t.Errorf("expected country 'United States', got %q", allocs[0].Country)
	}
	if allocs[0].Percent != 1.0 {
		t.Errorf("expected percent 1.0, got %f", allocs[0].Percent)
	}
}

func TestHandleGet_GeographicAllocations_Missing(t *testing.T) {
	repo := newTestSMRepo()
	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.byInternal["AAPL"] = 1
	svc := symbolmapping.NewService(repo)

	detailsRepo := newTestDetailsRepo()
	detailsRepo.byInternal["AAPL"] = &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		QuoteType:      "EQUITY",
		// GeographicAllocations is nil
		FetchedAt: time.Now(),
	}
	detailsSvc := symbols.NewService(detailsRepo, nil)

	handler := NewSymbolHandler(svc, detailsSvc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SymbolGetResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.SymbolDetails == nil {
		t.Fatal("expected non-nil symbol_details")
	}
	// With omitempty and nil/empty slice, geographic_allocations should be absent from JSON
	// (omitempty on nil slice = omitted; on empty slice = omitted)
	if resp.SymbolDetails.GeographicAllocations != nil {
		t.Errorf("expected nil geographic_allocations, got %v", resp.SymbolDetails.GeographicAllocations)
	}
}
